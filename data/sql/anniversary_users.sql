SELECT
  u.user_id,
  u.name,
  u.email,
  u.birthdate,
  u.email_verified_at IS NOT NULL AS is_active,
  TIMESTAMPDIFF(YEAR, u.created_at, NOW()) AS years
FROM users u
WHERE DATE_FORMAT(u.created_at, '%m-%d') = ?
  AND u.email_verified_at IS NOT NULL
  AND u.email IS NOT NULL
  AND u.email <> ''
  AND TIMESTAMPDIFF(YEAR, u.created_at, NOW()) > 0
  AND (
    SELECT COALESCE(SUM(o.total_price + o.remaining), 0)
    FROM orders o
    WHERE o.user_id = u.user_id
      AND o.status <> 'not paid'
      AND COALESCE(o.status, '') NOT IN ('cancelled', 'canceled')
  ) > 300000
  AND EXISTS (
    SELECT 1
    FROM orders o2
    WHERE o2.user_id = u.user_id
      AND o2.status <> 'not paid'
      AND COALESCE(o2.status, '') NOT IN ('cancelled', 'canceled')
      AND o2.created_at >= DATE_SUB(NOW(), INTERVAL 1 YEAR)
  );
