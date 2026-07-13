-- Eligible users for the discounted-wishlist campaign: user punya item di
-- wishlist yang diskonnya MULAI di hari run ("pas mulai"), masih aktif, beneran
-- lebih murah, in-stock/ready/non-adult, email verified, dan belum dikirimin
-- discounted_wishlist dalam 7 hari terakhir (cooldown).
--
-- `?` = tanggal run (now). `discount_start_date = DATE(?)` → nembak diskon yang
-- start-nya hari itu. Jalanin scheduled task-nya SETELAH diskon go-live di hari
-- yang sama (mis. 20:00 WIB = 13:00 UTC, tanggal UTC & WIB masih sama), bukan
-- tengah malam (00:00 WIB = 17:00 UTC hari sebelumnya, tanggalnya bakal geser).
--
-- BENTUKNYA SENGAJA BERTAHAP — jangan digabung balik jadi satu join + SELECT DISTINCT.
-- Versi lama nge-join users×wishlists×items×item_products sekaligus lalu DISTINCT di
-- akhir, jadi:
--   - DISTINCT-nya nyortir baris LEBAR (user_id + name + email) buat SETIAP pasangan
--     (user, item), bukan cuma user_id;
--   - NOT EXISTS ke email_delivery_logs jalan sekali per PASANGAN (user, item) — user
--     yang wishlist 8 item diskon bayar cek cooldown itu 8x, padahal jawabannya sama.
-- Sekarang: saring item dulu (himpunan kecil), dedup ke user_id doang, baru sentuh
-- `users` dan cek cooldown SEKALI per user.
SELECT
  CAST(u.user_id AS CHAR)      AS user_id,
  u.name,
  u.email,
  TRUE AS is_active
FROM (
  -- Dedup ke user_id dulu: kolomnya sempit, dan ini yang bikin cek cooldown di bawah
  -- jalan sekali per user.
  SELECT DISTINCT w.user_id
  FROM wishlists w
  JOIN (
    -- Item diskon yang mulai hari ini. Kecil — ini yang nyetir query.
    SELECT i.item_id
    FROM items         i
    JOIN item_products ip ON ip.item_id = i.item_id
    WHERE i.discount_start_date = DATE(?)
      AND i.discount_end_date   >= DATE(?)
      AND i.discount_name IS NOT NULL AND i.discount_name != ''
      AND i.stock        >  0
      AND i.is_available =  1
      AND i.status       = 'ready'
      AND COALESCE(i.isAdult, 0) = 0
      AND i.discount_price > 0
      AND i.discount_price < ip.price
  ) d ON d.item_id = w.item_id
) eu
JOIN users u ON u.user_id = eu.user_id
WHERE u.email IS NOT NULL AND u.email <> ''
  AND NOT EXISTS (
    SELECT 1
    FROM email_delivery_logs edl
    WHERE edl.user_id    = CAST(u.user_id AS CHAR)
      AND edl.feature    = 'discounted_wishlist'
      AND edl.status     IN ('sent', 'queued', 'sending')
      AND edl.created_at >= DATE_SUB(NOW(), INTERVAL 7 DAY)
  )
