# Yukari

Birthday email reader for Kyou.id.

Yukari reads Hanayo/Kyou MySQL using SQL files from `data/sql`, builds complete birthday email job payloads, and pushes them to Redis for Makoto to send.

## Flow

```text
Hanayo DB
  -> Yukari reader
  -> Redis birthday_email_jobs
  -> Makoto sender
```

## Commands

```sh
make test
make build
make run
```

## Environment

```env
YUKARI_TIMEZONE=Asia/Jakarta
YUKARI_SQL_DIR=data/sql
DATABASE_DSN=root:root@tcp(mariadb:3306)/kyouid_kyou?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci

REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0
YUKARI_QUEUE_NAME=birthday_email_jobs
```

Use `parseTime=true` in `DATABASE_DSN` because Yukari scans `users.birthdate` into Go `time.Time`.
