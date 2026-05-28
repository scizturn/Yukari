# Yukari

Birthday email reader for Kyou.id.

Yukari reads Hanayo/Kyou MySQL using SQL files from `data/sql`, builds complete birthday email job payloads, and pushes them to Redis for Makoto to send. Birthday recipients must have a non-empty email and `email_verified_at IS NOT NULL`. Wishlist and FYP payloads include public CDN image URLs from the `images` table when available.

## Flow

```text
Hanayo DB
  -> Yukari reader
  -> optional Voucher 2.5 alias creation
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

VOUCHER_CODE_SECRET=change-me

REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0
YUKARI_QUEUE_NAME=birthday_email_jobs
```

Yukari builds the MySQL DSN from `OLD_DATABASE_*` and enables `parseTime=true` automatically because it scans `users.birthdate` into Go `time.Time`.

When `data/vouchers/birthday.json` exists and contains `pricing_voucher_id` or `pricing_voucher_code`, Yukari creates a per-user alias voucher before enqueueing the Redis job. The generated job includes `voucher_code`, and Makoto sends that code instead of requesting a voucher from Kyou.id. Voucher codes are deterministic HMAC values from user/year plus `VOUCHER_CODE_SECRET`, so retries and birthday-date changes in the same year reuse the same random-looking code. If that yearly voucher already exists, Yukari skips enqueueing another birthday email. Voucher writes use the same `OLD_DATABASE_*` DSN, so that database user must be allowed to write `vouchers`, `voucher_pricing_aliases`, and `voucher_rules`. Yukari does not write `voucher_claims`; users claim from the email link.
