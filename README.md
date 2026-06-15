# Yukari

Birthday dan anniversary email reader untuk Kyou.id.

Yukari membaca Hanayo/Kyou MySQL menggunakan SQL files dari `data/sql`, membangun job payload lengkap, membuat voucher alias per-user, dan mendorong job ke Redis untuk Makoto kirimkan. Penerima harus memiliki email non-kosong dan `email_verified_at IS NOT NULL`.

## Flow

```text
Hanayo DB
  -> Yukari reader (birthday / anniversary)
  -> Voucher alias creation (vouchers + voucher_pricing_aliases + voucher_rules)
  -> Redis queue
  -> Makoto sender
```

## Pipeline

Yukari mendukung dua pipeline yang berjalan secara independent:

| Pipeline | Trigger | Queue |
|---|---|---|
| Birthday | `DATE(birthdate) = hari ini` | `birthday_email_jobs` |
| Anniversary | `DATE(created_at) = hari ini` + min 1 tahun + total orders > 300rb + order dalam 1 tahun terakhir | `anniversary_email_jobs` |

Dikontrol via `YUKARI_MODE`:
- `birthday` — hanya birthday
- `anniversary` — hanya anniversary (butuh `YUKARI_ANNIVERSARY_ENABLED=true`)
- `all` — keduanya (default)

## Commands

```sh
make test
make build
make run
```

## Binaries

| Binary | Fungsi |
|---|---|
| `yukari` | Main job harian — query users dan enqueue ke Redis |
| `forcejob` | Force kirim birthday ke 1 user (`YUKARI_FORCE_USER=<id>`) |
| `forcejob-anniversary` | Force kirim anniversary ke 1 user (`YUKARI_FORCE_USER=<id>`) |
| `migrateemailaudit` | Migrasi tabel `email_delivery_logs` |

## Force Job

Untuk test kirim anniversary ke user spesifik tanpa menunggu jadwal:

```sh
YUKARI_FORCE_USER=12345 forcejob-anniversary
```

- Bypass eligibility check (total orders, active check)
- Buat voucher ke DB (idempoten — reuse kalau sudah ada di tahun yang sama)
- **Tidak** tulis audit log
- Enqueue job ke Redis → Makoto proses dan kirim email

## Environment

```env
YUKARI_TIMEZONE=Asia/Jakarta
YUKARI_SQL_DIR=data/sql
YUKARI_MODE=all

OLD_DATABASE_HOST=mariadb
OLD_DATABASE_PORT=3306
OLD_DATABASE_NAME=kyouid_kyou
OLD_DATABASE_USERNAME=user
OLD_DATABASE_PASSWORD=secret

VOUCHER_CODE_SECRET=change-me

REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0

# Birthday
YUKARI_QUEUE_NAME=birthday_email_jobs

# Anniversary
YUKARI_ANNIVERSARY_ENABLED=true
YUKARI_ANNIVERSARY_QUEUE_NAME=anniversary_email_jobs
YUKARI_ANNIVERSARY_VOUCHER_CONFIG=data/vouchers/anniversary.json
```

## Voucher

Yukari membuat voucher alias per-user sebelum enqueue. Kode voucher bersifat deterministik (HMAC dari `user_id + tahun + secret`), sehingga retry dalam tahun yang sama menghasilkan kode yang sama.

- **Birthday**: prefix default, durasi dari `data/vouchers/birthday.json`
- **Anniversary**: prefix `ANV`, durasi hardcode 14 hari, config dari `data/vouchers/anniversary.json`

Kalau voucher tahun itu sudah ada → Yukari skip user (tidak enqueue duplikat).

DB user yang dipakai (`OLD_DATABASE_*`) harus punya akses write ke `vouchers`, `voucher_pricing_aliases`, dan `voucher_rules`.

## Audit Log

Yukari menulis ke `email_delivery_logs` saat job di-enqueue (`status=queued`) dan saat user di-skip (`status=skipped`). Cek duplikat per tahun berdasarkan kolom `feature`:

- `birthday_voucher` — untuk birthday
- `anniversary_voucher` — untuk anniversary

## Coolify Scheduled Tasks

Jalankan `yukari` sekali sehari. Contoh setup dua task terpisah:

| Field | Birthday | Anniversary |
|---|---|---|
| Command | `env YUKARI_MODE=birthday yukari` | `env YUKARI_MODE=anniversary YUKARI_ANNIVERSARY_ENABLED=true yukari` |
| Frequency | `0 17 * * *` | `0 17 * * *` |
| Timeout | 300 | 300 |

Atau satu task kalau `.env` sudah `YUKARI_MODE=all` dan `YUKARI_ANNIVERSARY_ENABLED=true`:

```
Command: yukari
Frequency: 0 17 * * *
```

`0 17 * * *` = jam 17:00 UTC = jam 00:00 WIB.
