SELECT DISTINCT
  CAST(u.user_id AS CHAR) AS user_id,
  u.name,
  u.email,
  u.created_at,
  TRUE AS is_active
FROM users u
WHERE DATE(u.last_login) = DATE_SUB(DATE(?), INTERVAL 90 DAY)
  AND u.email IS NOT NULL AND u.email <> ''
  AND (
    SELECT COALESCE(SUM(o.total_price + o.remaining), 0)
    FROM orders o
    WHERE o.user_id = u.user_id
      AND o.status NOT IN ('not paid', 'cancelled')
  ) > 300000
  AND NOT EXISTS (
    SELECT 1 FROM email_delivery_logs edl
    WHERE edl.user_id = CAST(u.user_id AS CHAR)
      AND edl.feature  = 'winback'
      AND edl.status   IN ('sent', 'queued', 'sending')
      AND edl.created_at >= DATE_SUB(NOW(), INTERVAL 1 YEAR)
  )
