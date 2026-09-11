# Tailgate

A social activity layer for a picks app: profiles, follows, and a feed of what the people
you follow are doing (slips placed, entries won, streaks). Two services with one boundary:

- **Rails 8** owns members, the follow graph, activities and a transactional **outbox**.
- **`fanoutd` (Go)** owns `timeline_entries` and turns `activity.created` events into
  per-follower timeline rows, then serves timelines with keyset pagination.

The point of the project is the boundary and the guarantees on either side of it, not the UI.

## Guarantees, and where each one lives

| Property | Enforced by |
|---|---|
| An activity reaches each follower's timeline **exactly once**, no matter how many times fan-out runs | `UNIQUE (member_id, activity_id)` + `INSERT … ON CONFLICT DO NOTHING` in `fanoutd` |
| A crash between "activity written" and "job enqueued" cannot strand a post | `outbox_events` row created in the **same transaction** as the activity; `FanoutJob` publishes it and marks `published_at` |
| A retried job cannot publish twice | job is a no-op once `published_at` is set; the server side is idempotent anyway |
| A follow edge is unique and never self-referential | unique index on `(follower_id, followee_id)` + a `CHECK (follower_id <> followee_id)` |
| A post by an author with a million followers costs one row | **hybrid fan-out**: authors at or above `CELEBRITY_THRESHOLD` followers are not materialised (`mode=pull`); `/timeline` merges their activities at read time from the follow graph |
| Pagination is gap-free and repeat-free across the merged push+pull stream | one keyset cursor `(activity_created_at, activity_id)` applied to both branches of the `UNION` |
| Unfollow removes exactly that author's rows | `DELETE … WHERE member_id = $follower AND author_id = $followee` on an index built for it |
| Follow backfills recent history, once | `INSERT … SELECT … ON CONFLICT DO NOTHING` over the followee's last 50 push-mode activities |
| `fanoutd` cannot start against a schema without the unique index | `assertSchema` at boot refuses |

## Tests

`fanoutd/fanout_test.go` runs against a real Postgres because every property above lives in the
database, not in Go:

1. **Exactly-once under duplicate calls** — 7 followers, batch size 3 (so three batches), eight
   concurrent `/fanout` calls for the same activity. Total deliveries reported across all eight
   calls must be exactly 7 and every follower must hold exactly one row.
2. **Celebrity pull merges at read time** — a celebrity author is never materialised, their
   activity still appears once in a follower's feed in the correct position, and keyset
   pagination across the merged stream has no gaps or repeats.
3. **Follow backfill and unfollow prune** — backfill inserts 2 then 0 on repeat; unfollow removes
   exactly the author's rows and leaves other authors' rows alone.

Rails tests (`bin/rails test`) cover the outbox: an activity always gets its event in the same
transaction, a job that runs twice publishes once, and a failed publish leaves the event pending
with the error recorded and the retry scheduled.

```
cd fanoutd && TEST_DATABASE_URL=postgres://localhost/tailgate_test go test ./...
bin/rails test
```

## Run it

```
bin/rails db:create db:migrate
cd fanoutd && go run . &                         # DATABASE_URL, PORT (default 8080), CELEBRITY_THRESHOLD (40)
FANOUTD_URL=http://localhost:8080 bin/rails db:seed  # 61 members, ~590 follows, 400 activities
FANOUTD_URL=http://localhost:8080 bin/rails s
```

Browse as any member (top-right), post an activity from your own profile, follow and unfollow
people and watch the feed change. `/inspector` shows pending outbox events, celebrity authors and
per-activity delivery counts.

## Design notes

- **Why an outbox and not `after_commit` → HTTP?** Because the HTTP call can fail after the
  commit and there is then no record that anything was owed. The outbox row *is* the record.
- **Why a separate Go service at all?** Fan-out is the part of a social feed whose cost is a
  function of follower count, not request count. It wants a small, boring, horizontally scalable
  process with one table and two indexes, not a slot in the monolith's job queue.
- **Why does Rails read `timeline_entries` through fanoutd rather than directly?** So the table
  has one writer and one reader contract. The Rails `TimelineEntry` model is `readonly?` and
  exists only for the inspector's counts.
- **What is deliberately missing:** ranking, blocks/mutes, private accounts, and a real
  denormalised profile-stats rollup. Each is a follow-on with the same shape: a table with a
  unique constraint that makes the invalid state unrepresentable.

## Stack

Rails 8.1, PostgreSQL 17, Solid Queue (in-process via Puma), importmap, plain CSS. Go 1.24 with
pgx v5, `net/http` with Go 1.22 route patterns, a scratch container. Schema is `db/structure.sql`.
