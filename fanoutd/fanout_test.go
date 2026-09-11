package main

// These tests run against a real Postgres (TEST_DATABASE_URL, default
// tailgate_test) because the properties under test live in the database:
// the unique index, ON CONFLICT DO NOTHING, and the keyset merge.
import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testServer(t *testing.T) (*Server, context.Context) {
	t.Helper()
	ctx := context.Background()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/tailgate_test"
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Skip("no postgres:", err)
	}
	if err := assertSchema(ctx, pool); err != nil {
		t.Skip("schema not migrated:", err)
	}
	_, err = pool.Exec(ctx, `TRUNCATE timeline_entries, outbox_events, activities, follows, members RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{db: pool, threshold: 5, backfill: 50, batch: 3}, ctx
}

func mkMember(t *testing.T, s *Server, ctx context.Context, handle string) int64 {
	var id int64
	if err := s.db.QueryRow(ctx, `INSERT INTO members (handle, display_name, created_at, updated_at) VALUES ($1,$1,now(),now()) RETURNING id`, handle).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func mkFollow(t *testing.T, s *Server, ctx context.Context, follower, followee int64) {
	if _, err := s.db.Exec(ctx, `INSERT INTO follows (follower_id, followee_id, created_at, updated_at) VALUES ($1,$2,now(),now())`, follower, followee); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `UPDATE members SET followers_count = followers_count + 1 WHERE id=$1`, followee); err != nil {
		t.Fatal(err)
	}
}

func mkActivity(t *testing.T, s *Server, ctx context.Context, author int64, at time.Time) int64 {
	var id int64
	if err := s.db.QueryRow(ctx, `INSERT INTO activities (member_id, kind, payload, created_at, updated_at) VALUES ($1,'streak','{}',$2,$2) RETURNING id`, author, at).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func count(t *testing.T, s *Server, ctx context.Context, q string, args ...any) int64 {
	var n int64
	if err := s.db.QueryRow(ctx, q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Property 1: exactly-once per (follower, activity), no matter how many times
// /fanout is called, concurrently or not, and no matter how batches fall.
func TestFanoutExactlyOnceUnderDuplicateCalls(t *testing.T) {
	s, ctx := testServer(t)
	author := mkMember(t, s, ctx, "author")
	var followers []int64
	for i := 0; i < 7; i++ { // 7 followers with batch=3 forces three batches
		f := mkMember(t, s, ctx, fmt.Sprintf("f%d", i))
		followers = append(followers, f)
		mkFollow(t, s, ctx, f, author)
	}
	// threshold is 5; bump it for this test so the author is push-mode
	s.threshold = 100
	act := mkActivity(t, s, ctx, author, time.Now())

	var wg sync.WaitGroup
	var mu sync.Mutex
	var total int64
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mode, n, err := s.Fanout(ctx, act)
			if err != nil {
				t.Error(err)
				return
			}
			if mode != "push" {
				t.Errorf("mode=%s", mode)
			}
			mu.Lock()
			total += n
			mu.Unlock()
		}()
	}
	wg.Wait()
	if total != 7 {
		t.Fatalf("total deliveries across 8 concurrent calls = %d, want exactly 7", total)
	}
	if n := count(t, s, ctx, `SELECT count(*) FROM timeline_entries WHERE activity_id=$1`, act); n != 7 {
		t.Fatalf("timeline rows = %d, want 7", n)
	}
	for _, f := range followers {
		if n := count(t, s, ctx, `SELECT count(*) FROM timeline_entries WHERE member_id=$1 AND activity_id=$2`, f, act); n != 1 {
			t.Fatalf("follower %d has %d copies", f, n)
		}
	}
}

// Property 2: a celebrity author is never materialised, and their activity still
// appears exactly once in a follower's merged timeline, in the right order.
func TestCelebrityPullMergesAtReadTime(t *testing.T) {
	s, ctx := testServer(t)
	celeb := mkMember(t, s, ctx, "celeb")
	normal := mkMember(t, s, ctx, "normal")
	me := mkMember(t, s, ctx, "me")
	for i := 0; i < 6; i++ { // >= threshold 5
		mkFollow(t, s, ctx, mkMember(t, s, ctx, fmt.Sprintf("x%d", i)), celeb)
	}
	mkFollow(t, s, ctx, me, celeb)
	mkFollow(t, s, ctx, me, normal)
	base := time.Now().Add(-time.Hour)
	a1 := mkActivity(t, s, ctx, normal, base.Add(1*time.Minute))
	a2 := mkActivity(t, s, ctx, celeb, base.Add(2*time.Minute))
	a3 := mkActivity(t, s, ctx, normal, base.Add(3*time.Minute))
	for _, a := range []int64{a1, a2, a3} {
		if _, _, err := s.Fanout(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, s, ctx, `SELECT count(*) FROM timeline_entries WHERE author_id=$1`, celeb); n != 0 {
		t.Fatalf("celebrity was materialised into %d rows", n)
	}
	tl, err := s.Timeline(ctx, me, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	got := []int64{}
	for _, e := range tl {
		got = append(got, e.ActivityID)
	}
	want := []int64{a3, a2, a1}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("timeline order = %v, want %v", got, want)
	}
	if tl[1].Source != "pull" || tl[0].Source != "push" {
		t.Fatalf("sources = %s,%s", tl[0].Source, tl[1].Source)
	}
	// keyset pagination continues across the merged stream without gaps or repeats
	page1, _ := s.Timeline(ctx, me, "", 2)
	page2, _ := s.Timeline(ctx, me, page1[len(page1)-1].Cursor, 2)
	if len(page1) != 2 || len(page2) != 1 || page2[0].ActivityID != a1 {
		t.Fatalf("pagination broke: %v / %v", page1, page2)
	}
}

// Property 3: unfollow removes exactly that author's rows and nothing else;
// follow backfills idempotently.
func TestFollowBackfillAndUnfollowPrune(t *testing.T) {
	s, ctx := testServer(t)
	s.threshold = 100
	a := mkMember(t, s, ctx, "a")
	b := mkMember(t, s, ctx, "b")
	me := mkMember(t, s, ctx, "me")
	mkFollow(t, s, ctx, me, b)
	acts := []int64{mkActivity(t, s, ctx, a, time.Now().Add(-3*time.Minute)), mkActivity(t, s, ctx, a, time.Now().Add(-2*time.Minute)), mkActivity(t, s, ctx, b, time.Now().Add(-time.Minute))}
	for _, x := range acts {
		s.Fanout(ctx, x)
	}
	// me follows a AFTER a posted: backfill must bring a's two activities in, once.
	mkFollow(t, s, ctx, me, a)
	n1, _ := s.Follow(ctx, me, a)
	n2, _ := s.Follow(ctx, me, a)
	if n1 != 2 || n2 != 0 {
		t.Fatalf("backfill = %d then %d, want 2 then 0", n1, n2)
	}
	if n := count(t, s, ctx, `SELECT count(*) FROM timeline_entries WHERE member_id=$1`, me); n != 3 {
		t.Fatalf("timeline has %d rows, want 3", n)
	}
	removed, _ := s.Unfollow(ctx, me, a)
	if removed != 2 {
		t.Fatalf("unfollow removed %d, want 2", removed)
	}
	if n := count(t, s, ctx, `SELECT count(*) FROM timeline_entries WHERE member_id=$1 AND author_id=$2`, me, b); n != 1 {
		t.Fatalf("unfollow of a damaged b's rows: %d", n)
	}
}
