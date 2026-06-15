SELECT DISTINCT
  CAST(u.user_id AS CHAR)      AS user_id,
  u.name,
  u.email,
  u.email_verified_at IS NOT NULL AS is_active
FROM users u
JOIN wishlists w  ON w.user_id  = u.user_id
JOIN items     i  ON i.item_id  = w.item_id
WHERE i.discount_start_date = DATE_SUB(DATE(?), INTERVAL 1 DAY)
  AND i.discount_end_date   >= DATE(?)
  AND i.discount_name IS NOT NULL AND i.discount_name != ''
  AND i.stock        >  0
  AND i.is_available =  1
  AND COALESCE(i.isAdult, 0) = 0
  AND u.email_verified_at IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM email_delivery_logs edl
    WHERE edl.user_id    = CAST(u.user_id AS CHAR)
      AND edl.feature    = 'discounted_wishlist'
      AND edl.status     IN ('sent', 'queued', 'sending')
      AND edl.created_at >= DATE_SUB(NOW(), INTERVAL 7 DAY)
  )
