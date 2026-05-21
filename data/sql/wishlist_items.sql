SELECT
  i.item_id,
  i.name,
  CONCAT('/items/', i.item_id) AS url,
  0 AS price
FROM wishlists w
JOIN items i ON i.item_id = w.item_id
WHERE w.user_id = ?
  AND i.is_available = 1
ORDER BY w.created_at DESC
LIMIT 8;
