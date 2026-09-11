// fanoutd owns the timeline_entries table for Tailgate. Rails owns members, the
// follow graph and activities; this service turns "member X posted activity A"
// into rows in each follower's timeline, and serves timelines back with keyset
// pagination.
//
// Guarantees, and where each one lives:
//   - exactly-once per (follower, activity): UNIQUE (member_id, activity_id) +
//     INSERT ... ON CONFLICT DO NOTHING. /fanout may be called any number of times.
//   - unfollow removes the author from the follower's timeline atomically with
//     the edge already being gone in Rails; it is a plain DELETE keyed by both ids.
//   - follow backfills the followee's last N activities, also idempotently.
//   - hybrid fan-out: authors at or above CELEBRITY_THRESHOLD followers are NOT
//     materialised (mode=pull); /timeline merges their activities at read time
//     from the follow graph. A post by a million-follower author costs one row.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	db        *pgxpool.Pool
	threshold int
	backfill  int
	batch     int
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func main() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, env("DATABASE_URL", "postgres://localhost/tailgate_development"))
	if err != nil {
		log.Fatal(err)
	}
	if err := assertSchema(ctx, pool); err != nil {
		log.Fatal(err)
	}
	thr, _ := strconv.Atoi(env("CELEBRITY_THRESHOLD", "40"))
	s := &Server{db: pool, threshold: thr, backfill: 50, batch: 1000}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("POST /fanout", s.fanout)
	mux.HandleFunc("POST /follow", s.follow)
	mux.HandleFunc("POST /unfollow", s.unfollow)
	mux.HandleFunc("GET /timeline/{member}", s.timeline)
	addr := ":" + env("PORT", "8080")
	log.Printf("fanoutd listening on %s (celebrity threshold %d)", addr, thr)
	log.Fatal(http.ListenAndServe(addr, logging(mux)))
}

func logging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := time.Now()
		h.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(t).Round(time.Millisecond))
	})
}

// assertSchema refuses to start against a database that lacks the table or the
// unique index the whole exactly-once story depends on.
func assertSchema(ctx context.Context, db *pgxpool.Pool) error {
	var n int
	err := db.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE tablename='timeline_entries' AND indexdef ILIKE 'CREATE UNIQUE INDEX%(member_id, activity_id)'`).Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("timeline_entries is missing UNIQUE (member_id, activity_id); refusing to start")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Ping(r.Context()); err != nil {
		writeJSON(w, 503, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

type fanoutReq struct {
	ActivityID int64 `json:"activity_id"`
}

// Fanout materialises one activity into every follower's timeline, or marks it
// pull-mode when the author is a celebrity. Idempotent.
func (s *Server) Fanout(ctx context.Context, activityID int64) (mode string, delivered int64, err error) {
	var authorID int64
	var createdAt time.Time
	var followers int
	err = s.db.QueryRow(ctx, `SELECT a.member_id, a.created_at, m.followers_count FROM activities a JOIN members m ON m.id=a.member_id WHERE a.id=$1`, activityID).Scan(&authorID, &createdAt, &followers)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, fmt.Errorf("activity %d not found", activityID)
	}
	if err != nil {
		return "", 0, err
	}
	if followers >= s.threshold {
		return "pull", 0, nil
	}
	// Batched INSERT ... SELECT with ON CONFLICT DO NOTHING. The unique index is
	// the guarantee; a second call inserts zero rows and reports delivered=0.
	var lastID int64
	for {
		tag, err := s.db.Exec(ctx, `
			INSERT INTO timeline_entries (member_id, activity_id, author_id, activity_created_at)
			SELECT f.follower_id, $1, $2, $3
			  FROM follows f
			 WHERE f.followee_id = $2 AND f.follower_id > $4
			 ORDER BY f.follower_id
			 LIMIT $5
			ON CONFLICT (member_id, activity_id) DO NOTHING`,
			activityID, authorID, createdAt, lastID, s.batch)
		if err != nil {
			return "", delivered, err
		}
		delivered += tag.RowsAffected()
		// advance the cursor to the last follower id considered in this batch
		var next *int64
		if err := s.db.QueryRow(ctx, `SELECT max(follower_id) FROM (SELECT follower_id FROM follows WHERE followee_id=$1 AND follower_id > $2 ORDER BY follower_id LIMIT $3) t`, authorID, lastID, s.batch).Scan(&next); err != nil {
			return "", delivered, err
		}
		if next == nil {
			break
		}
		lastID = *next
	}
	return "push", delivered, nil
}

func (s *Server) fanout(w http.ResponseWriter, r *http.Request) {
	var req fanoutReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ActivityID == 0 {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "activity_id required"})
		return
	}
	mode, n, err := s.Fanout(r.Context(), req.ActivityID)
	if err != nil {
		code := 500
		if strings.Contains(err.Error(), "not found") {
			code = 404
		}
		writeJSON(w, code, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "mode": mode, "delivered": n})
}

type edgeReq struct {
	FollowerID int64 `json:"follower_id"`
	FolloweeID int64 `json:"followee_id"`
}

// Follow backfills the followee's recent push-mode activities into the follower's
// timeline. Celebrity authors are not backfilled: their rows come from /timeline's
// pull branch, so backfilling them would double-show.
func (s *Server) Follow(ctx context.Context, followerID, followeeID int64) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		INSERT INTO timeline_entries (member_id, activity_id, author_id, activity_created_at)
		SELECT $1, a.id, a.member_id, a.created_at
		  FROM activities a JOIN members m ON m.id = a.member_id
		 WHERE a.member_id = $2 AND m.followers_count < $3
		 ORDER BY a.created_at DESC LIMIT $4
		ON CONFLICT (member_id, activity_id) DO NOTHING`, followerID, followeeID, s.threshold, s.backfill)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Server) Unfollow(ctx context.Context, followerID, followeeID int64) (int64, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM timeline_entries WHERE member_id=$1 AND author_id=$2`, followerID, followeeID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Server) follow(w http.ResponseWriter, r *http.Request) {
	var e edgeReq
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil || e.FollowerID == 0 || e.FolloweeID == 0 {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "follower_id and followee_id required"})
		return
	}
	n, err := s.Follow(r.Context(), e.FollowerID, e.FolloweeID)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "backfilled": n})
}

func (s *Server) unfollow(w http.ResponseWriter, r *http.Request) {
	var e edgeReq
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil || e.FollowerID == 0 || e.FolloweeID == 0 {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "follower_id and followee_id required"})
		return
	}
	n, err := s.Unfollow(r.Context(), e.FollowerID, e.FolloweeID)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "removed": n})
}

type entry struct {
	ActivityID int64     `json:"activity_id"`
	AuthorID   int64     `json:"author_id"`
	CreatedAt  time.Time `json:"created_at"`
	Source     string    `json:"source"`
	Cursor     string    `json:"cursor"`
}

// Timeline merges the materialised (push) rows with celebrity (pull) activities
// at read time, using one keyset cursor "<rfc3339nano>|<activity_id>" over both.
func (s *Server) Timeline(ctx context.Context, memberID int64, before string, limit int) ([]entry, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	beforeTS := time.Now().Add(time.Hour)
	beforeID := int64(1<<62)
	if before != "" {
		parts := strings.SplitN(before, "|", 2)
		if len(parts) == 2 {
			if t, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
				beforeTS = t
			}
			if id, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				beforeID = id
			}
		}
	}
	rows, err := s.db.Query(ctx, `
		(SELECT t.activity_id, t.author_id, t.activity_created_at, 'push' AS source
		   FROM timeline_entries t
		  WHERE t.member_id = $1 AND (t.activity_created_at, t.activity_id) < ($2, $3)
		  ORDER BY t.activity_created_at DESC, t.activity_id DESC LIMIT $4)
		UNION ALL
		(SELECT a.id, a.member_id, a.created_at, 'pull'
		   FROM activities a
		   JOIN follows f ON f.followee_id = a.member_id AND f.follower_id = $1
		   JOIN members m ON m.id = a.member_id
		  WHERE m.followers_count >= $5 AND (a.created_at, a.id) < ($2, $3)
		  ORDER BY a.created_at DESC, a.id DESC LIMIT $4)
		ORDER BY 3 DESC, 1 DESC LIMIT $4`, memberID, beforeTS, beforeID, limit, s.threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entry{}
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.ActivityID, &e.AuthorID, &e.CreatedAt, &e.Source); err != nil {
			return nil, err
		}
		e.Cursor = e.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(e.ActivityID, 10)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("member"), 10, 64)
	if err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "bad member id"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	out, err := s.Timeline(r.Context(), id, r.URL.Query().Get("before"), limit)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, out)
}
