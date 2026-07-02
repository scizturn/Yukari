-- Eligible users for the discounted-wishlist campaign: user punya item di
-- wishlist yang diskonnya MULAI di hari run ("pas mulai"), masih aktif, beneran
-- lebih murah, in-stock/ready/non-adult, email verified, dan belum dikirimin
-- discounted_wishlist dalam 7 hari terakhir (cooldown).
--
-- `?` = tanggal run (now). `discount_start_date = DATE(?)` → nembak diskon yang
-- start-nya hari itu. Jalanin scheduled task-nya SETELAH diskon go-live di hari
-- yang sama (mis. 20:00 WIB = 13:00 UTC, tanggal UTC & WIB masih sama), bukan
-- tengah malam (00:00 WIB = 17:00 UTC hari sebelumnya, tanggalnya bakal geser).
SELECT DISTINCT
  CAST(u.user_id AS CHAR)      AS user_id,
  u.name,
  u.email,
  u.email_verified_at IS NOT NULL AS is_active
FROM users u
JOIN wishlists w  ON w.user_id  = u.user_id
JOIN items     i  ON i.item_id  = w.item_id
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
  AND u.email_verified_at IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM email_delivery_logs edl
    WHERE edl.user_id    = CAST(u.user_id AS CHAR)
      AND edl.feature    = 'discounted_wishlist'
      AND edl.status     IN ('sent', 'queued', 'sending')
      AND edl.created_at >= DATE_SUB(NOW(), INTERVAL 7 DAY)
  )
