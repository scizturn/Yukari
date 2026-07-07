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

# Wishlist back-in (reader hanya jalan hari Jumat; window carry-over ~21 hari
# dihitung otomatis dari tanggal run — tidak ada START_AT)
YUKARI_WISHLIST_BACK_IN_QUEUE_NAME=wishlist_back_in_email_jobs
YUKARI_WISHLIST_BACK_IN_VOUCHER_CONFIG=data/vouchers/wishlist_back_in.json
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

Code blocker discounted wishlist sudah ditangani:

- audit dibuat sebelum enqueue agar Makoto selalu mempunyai row untuk update; jika Redis enqueue gagal, status langsung dikompensasi menjadi `failed` sehingga tidak ikut suppression 7 hari;
- query user, wishlist item, dan fill sama-sama mewajibkan item `ready`, stok/availability valid, non-adult, dan `0 < discount_price < original_price`;
- unit test reader mencakup payload sukses, empty-item skip, dan kompensasi audit ketika enqueue gagal.

Release mass-send tetap menunggu checklist operasional Makoto: preference center nyata, seed send, inbox rendering, dan konfigurasi provider unsubscribe/suppression.

# Wishlist back-in

Kampanye **user-centric**: satu email per user berisi item wishlist **milik user itu sendiri** yang baru **balik stok** minggu ini. Beda dari discounted-wishlist yang mulai dari diskon, ini mulai dari **event restock** di `stock_logs`.

**Kapan jalan.** Reader self-gating: `go run ./cmd/yukari wishlist-back-in` hanya bekerja hari **Jumat** (hari lain langsung return 0). Window scan restock **~21 hari** (carry-over) dihitung otomatis dari tanggal run di `YUKARI_TIMEZONE`. Tidak ada `START_AT`.

**Carry-over.** Tiap user dibatasi maks 5 item/email. Kalau item yang restock lebih dari 5, sisanya **tidak hilang**: window 21 hari + dedup per-(user,item) bikin item yang belum ke-fire tetap jadi kandidat dan difire di Jumat berikutnya, sampai antriannya habis (atau item umur restock-nya lewat 21 hari). Contoh: 6 item balik → 5 difire Jumat ini, 1 sisanya Jumat depan.

**"Balik stok" = event `stock_logs`** dengan `is_restock=1`, `type='increase'`, `description='Increased via Insert Stock (Adjusment)'`, dan JSON `before_all_stock=0 → after_all_stock>0`. Berlaku untuk item **`ready`** maupun **`PO`** yang slotnya reopen; PO di-guard `po_deadline IS NULL OR po_deadline >= CURRENT_DATE` supaya PO yang sudah tutup tidak ikut.

## Wishlist Back-In Conditions

User + item dipilih hanya jika:

- user punya **verified email**;
- item ada di **wishlist user**, dan punya event restock 0→>0 (di atas) **dalam window ~21 hari** (carry-over);
- item sekarang `stock>0`, `is_available=1`, status `ready`/`PO` (PO deadline masih buka), non-adult;
- user **belum** dikirimi item tersebut dalam **90 hari terakhir** (dedup per `(user, item)` via `email_delivery_logs.metadata.item_ids`, cooldown 90 hari — restock lagi setelah >90 hari boleh re-notify).

Tiap user diberi **maksimum 5 item** per email, diurut **restock terbaru** dulu.

**Section "Gas, Nemenin Yang Udah Kamu Beli" (cross-sell).** Anchor = satu item yang **sudah dibeli** user, se-series/kategori dengan hero item (`wishlist_back_in_companion.sql`). Dari situ ditarik **6 item Most Popular Kyou** (`kyou_search_score` live 14 hari — cermin fill winback) di series/kategori yang sama (`wishlist_back_in_reco.sql`), **exclude** item yang sudah dibeli/di-wishlist, ready/in-stock/non-adult/non-admin/non-Wakeari. Section hanya tampil kalau ada anchor **dan** genap 6 rekomendasi; kalau kurang → hilang (N/A).

**Harga & badge** mengikuti hanamaru persis: badge status (`ready`/`PO`/`LPO`/`BO`/Revive), harga diskon (harga diskon + coret asli, hanya kalau `discount_qty > 0` aktif) atau DP (`DP IDR <dp> / <full>` untuk PO), else harga polos. Dirender di Makoto.

**PERF.** Query (`wishlist_back_in_user_items.sql`) memakai `STRAIGHT_JOIN` dengan `stock_logs` sebagai driving table supaya window memangkas lebih dulu — tanpa ini planner full-scan `items` (~215k row, 70s+). ~1.5s untuk window 21 hari, tanpa disk temp table (30d ≈ 7s, 90d ≈ 17s kalau window diperlebar).

## Wishlist Back-In Voucher

Config: `data/vouchers/wishlist_back_in.json` (`pricing_voucher_id` = head; kode per-user di-generate `WBI`+HMAC per ISO week). Tiap voucher **scoped**: rule `user` (hardcoded — cuma user itu) + rule `item_id = {{item_ids}}` (cuma item yang restock di email itu) + `item_age_min`/`gp_ratio_min` dari config. `item_types: []` → item **PO** juga bisa pakai. Default 10% off max 150k, 14 hari, `requires_claim`, 1×/user.

**Anti-spam (1 voucher hidup/user).** Sebelum bikin voucher baru, reader cek apakah user masih punya voucher WBI **aktif & belum dipakai** (`voucher_claims.used_at` NULL). Kalau ya → **reuse**, item baru cuma **ditambah** ke `item_id` rule-nya (nggak bikin voucher baru). Kalau sudah dipakai (atau expired) → voucher baru di-generate walau belum 14 hari (voucher one-shot yang sudah dipakai = selesai). Logika di `internal/repository/voucher.go` (`reusableWishlistBackInVoucher` + `extendItemIDRule`).

### Development checklist

```sh
# 1. Unit test
go test ./...

# 2. Lihat payload dari data production-like tanpa enqueue
go run ./cmd/previewjob-wishlist-back-in

# 3. Enqueue satu user untuk seed/test send (bypass window & dedup; bikin voucher
#    asli via pricing head — user_id / email / name)
YUKARI_FORCE_USER=12345 go run ./cmd/forcejob wishlist-back-in

# 4. Jalankan batch (hanya efektif hari Jumat; hari lain no-op)
go run ./cmd/yukari wishlist-back-in
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
- `discounted_wishlist` — cooldown 7 hari untuk discounted wishlist; hanya `queued`, `sending`, dan `sent` yang menekan pengiriman berikutnya

## Coolify Scheduled Tasks

Jalankan satu scheduled task per campaign. Untuk discounted wishlist:

```text
Command: yukari discounted-wishlist
Frequency: 0 13 * * *
Timeout: 300
```

`0 13 * * *` = jam 13:00 UTC = jam 20:00 WIB. Discounted wishlist nembak diskon
yang **mulai di hari run** (`discount_start_date = DATE(now)`), jadi jalanin
SORE/MALAM setelah diskon go-live — bukan tengah malam. Penting: jam 20:00 WIB =
13:00 UTC, tanggal UTC & WIB masih sama; kalau ditaruh tengah malam (00:00 WIB =
17:00 UTC hari sebelumnya) tanggalnya geser dan nggak nangkep diskon yang bener.

Wishlist back-in **hanya jalan hari Jumat** (reader self-gating). Set cron di hari Jumat:

```text
Command: yukari wishlist-back-in
Frequency: 0 13 * * 5     # Jumat 13:00 UTC = Jumat 20:00 WIB
Timeout: 300
```

⚠️ **Jebakan UTC↔WIB.** Reader ngecek "hari Jumat" pakai `YUKARI_TIMEZONE` (Asia/Jakarta), sedangkan cron Coolify jalan di UTC. Pastikan waktunya tetap **Jumat di WIB** — mis. `0 13 * * 5` (Jumat 20:00 WIB) aman, tapi `0 20 * * 5` = Sabtu 03:00 WIB → reader lihat Sabtu → **no-op, kampanye minggu itu ke-skip** (bukan cuma telat: window-nya geser). Alternatif aman: cron harian `0 13 * * *` — reader no-op 6 hari, cuma Jumat yang kerja.

**Seed/test send via Coolify (forcejob).** Buat kirim uji ke satu user tanpa nunggu Jumat, bikin task terpisah:

```text
Command: forcejob wishlist-back-in
Environment: YUKARI_FORCE_USER=<user_id | email | nama>
Frequency: (manual / on-demand — jalanin sekali)
Timeout: 300
```

Ini bypass window & dedup (ambil item wishlist available user itu), **bikin voucher asli** (via pricing head), dan enqueue. Makoto yang aktif akan render + kirim ke user tsb — jadi pakai user test / email sendiri.

Contoh task lama untuk birthday dan anniversary:

| Field | Birthday | Anniversary |
|---|---|---|
| Command | `yukari birthday` | `yukari anniversary` |
| Frequency | `0 17 * * *` | `0 17 * * *` |
| Timeout | 300 | 300 |
