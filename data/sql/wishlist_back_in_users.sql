SELECT DISTINCT
  CAST(u.user_id AS CHAR) AS user_id,
  u.name,
  u.email,
  u.email_verified_at IS NOT NULL AS is_active
FROM wishlists w
JOIN users u ON u.user_id = w.user_id
WHERE w.item_id = ?
  AND u.email_verified_at IS NOT NULL
ORDER BY u.user_id
