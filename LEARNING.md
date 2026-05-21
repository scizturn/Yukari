# Yukari Learning Notes

Yukari is the **reader** half of the birthday email system. It looks at the Hanayo MySQL database every day, finds users whose birthday is today, and pushes a job for each of them onto Redis. Makoto picks those jobs up and sends the email.

## System Overview

```text
Hanayo MySQL                                  Redis                                  Internet
┌──────────────┐    SELECT     ┌────────────┐  RPUSH   ┌────────────────┐  BLPOP   ┌────────────┐
│   users      │ ───────────►  │  Yukari    │ ───────► │ birthday_email │ ───────► │  Makoto    │
│   wishlists  │               │ (reader)   │          │   _jobs        │          │  (sender)  │
│   items      │               └────────────┘          └────────────────┘          └────────────┘
│   user_item  │
│    _actions  │
│   orders     │
└──────────────┘
```

## What Yukari Does

For each daily run, Yukari:

1. Loads SQL files from `data/sql/`.
2. Asks Hanayo MySQL for users whose `birthdate` matches today's `MM-DD`.
3. For each user, fetches:
   - Wishlist items.
   - FYP items based on user actions.
   - Popular items as a fallback when FYP is empty.
   - A flag for whether the user already converted recently.
4. Skips users that have already converted.
5. Builds a single JSON payload per user.
6. Pushes the payload to Redis `birthday_email_jobs`.

When `DATABASE_DSN` is empty, Yukari runs against an in-memory stub repository so you can verify the queue flow without a tunnel.

## File Map

```text
cmd/yukari/main.go                   CLI entry; wires config, MySQL, Redis, reader.
internal/config/config.go            Reads env vars (DATABASE_DSN, REDIS_ADDR, queue name, timezone).
internal/domain/types.go             Shared structs reused by Makoto: User, WishlistItem, FYPItem, BirthdayJob.
internal/queue/redis.go              Redis-backed producer (RPUSH).
internal/queue/codec.go              JSON encode/decode shared with Makoto.
internal/sqlfiles/sqlfiles.go        File-based SQL loader.
internal/repository/repository.go    Store interface + StubStore for local mode.
internal/repository/mysql.go         MySQL implementation that maps rows into domain structs.
internal/reader/reader.go            Orchestrates daily run: lookup users, hydrate, enqueue.
internal/reader/reader_test.go       Tests for the reader using fake store and fake queue.
data/sql/*.sql                       SQL queries used at runtime.
```

## Concepts Worth Studying

- **Repository pattern**: `repository.Store` describes everything Yukari needs from MySQL. `StubStore` and `OpenMySQLStore` are two implementations of that contract.
- **External SQL files**: queries live as plain `.sql` files in `data/sql/`. The Go code reads them by name. This keeps SQL reviewable by anyone, even non-Go developers.
- **Composition over inheritance**: `Reader{ Store, Queue }` has no logic-leaking magic; tests pass fakes directly.
- **JSON contract sharing**: domain structs are tagged with `json:"…"` so the same struct serializes/deserializes identically in Yukari and Makoto. The shared queue codec assumes that.
- **Time handling**: `MAKOTO_TIMEZONE` is parsed via `time.LoadLocation` so MM-DD matching uses the correct local day, not UTC.
- **Database driver**: `github.com/go-sql-driver/mysql` is registered via blank import in `internal/repository/mysql.go`. Look at how `database/sql.Open` returns a `*sql.DB` connection pool, not a single connection.
- **Result mapping**: `rows.Scan(&user.ID, &user.Name, …)` shows how to translate SQL columns to Go fields. The MySQL driver requires `parseTime=true` for `time.Time` columns.

## Environment

```env
YUKARI_TIMEZONE=Asia/Jakarta
YUKARI_SQL_DIR=data/sql
DATABASE_DSN=

REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=
REDIS_DB=0
YUKARI_QUEUE_NAME=birthday_email_jobs
```

DSN format expected by `database/sql`:

```text
user:password@tcp(host:3306)/dbname?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci
```

For the AWS port forward used by `tomori-nao`:

```env
DATABASE_DSN=USER:PASSWORD@tcp(127.0.0.1:10110)/DBNAME?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci
```

Use a read-only DB user with `SELECT` on:

```text
users, wishlists, items, user_item_actions, orders
```

## Running Locally

1. Start Redis:
   ```sh
   docker run --rm -p 6379:6379 --name makoto-redis redis:7-alpine
   ```
2. Run Yukari with stubs (no DB):
   ```sh
   cd /Users/sleepyreinze/Dev/Yukari
   REDIS_ADDR=127.0.0.1:6379 go run ./cmd/yukari
   ```
   Expected log:
   ```text
   DATABASE_DSN is empty; using stub repository
   yukari enqueued 1 birthday email job(s)
   ```
3. Run Yukari against Hanayo (after starting the tunnel):
   ```sh
   cd /Users/sleepyreinze/Dev/Yukari
   DATABASE_DSN='USER:PASSWORD@tcp(127.0.0.1:10110)/DBNAME?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci' \
   REDIS_ADDR=127.0.0.1:6379 \
   go run ./cmd/yukari
   ```
4. Inspect Redis queue length:
   ```sh
   docker exec -it makoto-redis redis-cli LLEN birthday_email_jobs
   ```

## Tests

```sh
go test ./...
```

Reader tests use a fake store and a fake queue so the suite runs without MySQL or Redis. SQL loader tests load the real files in `data/sql/` so file changes are caught.

## SQL Files Cheat Sheet

```text
data/sql/birthday_users.sql   Users with birthday today.
data/sql/wishlist_items.sql   Wishlist items per user.
data/sql/fyp_items.sql        Personalized items based on user actions.
data/sql/popular_items.sql    Fallback items when FYP is empty.
data/sql/user_converted.sql   Whether a user converted in the campaign window.
```

When the schema changes, edit the SQL files directly. The Go code only references their filenames, so query iteration does not require redeploys of business logic.

## Pairing With Makoto

The job payload Yukari emits matches what Makoto consumes. Keep these fields stable across both repos:

```json
{
  "job_id": "birthday-2026-05-21-user-123",
  "user_id": "123",
  "birthday_date": "2026-05-21T00:00:00+07:00",
  "user": {
    "id": "123",
    "name": "Garvin",
    "email": "garvin@example.com",
    "is_active": true
  },
  "wishlist_items": [],
  "fyp_items": [],
  "popular_items": [],
  "attempt": 1
}
```

Makoto handles HTML rendering and template variables. Yukari should not pre-render; it stays focused on truthful data.
