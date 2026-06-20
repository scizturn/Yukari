# Yukari

Email campaign reader untuk Kyou.id, termasuk discounted wishlist.

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

Setiap campaign dijalankan sebagai subcommand terpisah:

| Pipeline | Trigger | Queue |
|---|---|---|
| Birthday | `DATE(birthdate) = hari ini` | `birthday_email_jobs` |
| Anniversary | `DATE(created_at) = hari ini` + min 1 tahun + total orders > 300rb + order dalam 1 tahun terakhir | `anniversary_email_jobs` |
| Discounted wishlist | diskon wishlist mulai kemarin, masih aktif, stok tersedia | `discounted_wishlist_email_jobs` |

Contoh: `yukari birthday`, `yukari anniversary`, atau `yukari discounted-wishlist`. `YUKARI_MODE` dan mode `all` tidak digunakan oleh binary saat ini.

## Commands

```sh
make test
make build
go run ./cmd/yukari discounted-wishlist
```

## Binaries

| Binary | Fungsi |
|---|---|
| `yukari <campaign>` | Query user eligible dan enqueue campaign ke Redis |
| `forcejob <campaign>` | Force enqueue ke satu user (`YUKARI_FORCE_USER=<id>`) |
| `migrateemailaudit` | Migrasi tabel `email_delivery_logs` |

## Force Job

Untuk test discounted wishlist ke user spesifik tanpa menunggu jadwal:

```sh
YUKARI_FORCE_USER=12345 forcejob discounted-wishlist
```

- Bypass eligibility user harian dan memaksa `is_active=true`
- Tetap membaca item wishlist diskon dan rekomendasi dari DB
- Bisa enqueue job tanpa item wishlist diskon; gunakan hanya untuk development
- **Tidak** tulis audit log
- Enqueue job ke Redis → Makoto proses dan kirim email

Preview payload tanpa enqueue:

```sh
go run ./cmd/previewjob-discounted-wishlist
```

## Environment

```env
YUKARI_TIMEZONE=Asia/Jakarta
YUKARI_SQL_DIR=data/sql

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

# Discounted wishlist
YUKARI_DISCOUNTED_WISHLIST_QUEUE_NAME=discounted_wishlist_email_jobs
```

## Discounted Wishlist Conditions

User production dipilih hanya jika:

- mempunyai verified email;
- mempunyai item wishlist dengan `discount_start_date = kemarin` di timezone `YUKARI_TIMEZONE`;
- diskon belum berakhir, nama diskon terisi, stok lebih dari 0, item tersedia, dan bukan item adult;
- tidak memiliki audit `queued`, `sending`, atau `sent` untuk feature `discounted_wishlist` dalam 7 hari terakhir.

Item wishlist menjadi konten utama. Item diskon lain dari series yang sama dipakai sebagai fill, bukan sebagai wishlist, dengan maksimum 12 item per query. Jika query konten utama kosong, user dicatat `skipped` dan tidak di-enqueue.

### Development checklist

```sh
# 1. Jalankan semua unit test
go test ./...

# 2. Lihat payload dari data production-like tanpa enqueue
go run ./cmd/previewjob-discounted-wishlist

# 3. Enqueue satu penerima yang memang aman untuk test
YUKARI_FORCE_USER=12345 go run ./cmd/forcejob discounted-wishlist

# 4. Jalankan batch eligible (akan enqueue semua hasil query)
go run ./cmd/yukari discounted-wishlist
```

Pastikan migrasi `email_delivery_logs` sudah terpasang dan queue name sama persis dengan Makoto sebelum langkah 3 atau 4.

### Production readiness

Belum siap untuk enable mass-send. Blocker yang masih harus dibereskan:

- audit `queued` saat ini ditulis **sebelum** Redis enqueue; kegagalan Redis dapat membuat user tersuppress selama 7 hari walaupun job tidak pernah masuk queue;
- query item wishlist belum mewajibkan `status='ready'` dan belum memvalidasi `discount_price > 0 AND discount_price < original_price`, berbeda dengan query fill;
- belum ada unit test reader discounted wishlist untuk eligibility, empty-item skip, urutan audit/enqueue, dan error handling.

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

Jalankan satu scheduled task per campaign. Untuk discounted wishlist:

```text
Command: yukari discounted-wishlist
Frequency: 0 17 * * *
Timeout: 300
```

`0 17 * * *` = jam 17:00 UTC = jam 00:00 WIB. Jadwal tengah malam penting karena query memilih diskon yang mulai pada tanggal kemarin.

Contoh task lama untuk birthday dan anniversary:

| Field | Birthday | Anniversary |
|---|---|---|
| Command | `yukari birthday` | `yukari anniversary` |
| Frequency | `0 17 * * *` | `0 17 * * *` |
| Timeout | 300 | 300 |
