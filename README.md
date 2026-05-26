# Yukari

Birthday email reader for Kyou.id.

Yukari reads Hanayo/Kyou MySQL using SQL files from `data/sql`, builds complete birthday email job payloads, and pushes them to Redis for Makoto to send. Birthday recipients must have a non-empty email and `email_verified_at IS NOT NULL`.

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

OLD_DATABASE_HOST=mariadb
OLD_DATABASE_PORT=3306
OLD_DATABASE_NAME=kyouid_kyou
OLD_DATABASE_USERNAME=readonly_user
OLD_DATABASE_PASSWORD=secret

REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0
YUKARI_QUEUE_NAME=birthday_email_jobs
```

Yukari builds the MySQL DSN from `OLD_DATABASE_*` and enables `parseTime=true` automatically because it scans `users.birthdate` into Go `time.Time`.
