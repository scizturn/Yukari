SELECT
  COALESCE(i.name, 'Item Kyou') AS name,
  COALESCE(CONCAT('https://kyoucdn.id/', img.path, '.webp'), '') AS image_url,
  o.created_at AS order_date
FROM orders o
LEFT JOIN order_items oi ON oi.order_id = o.id
  AND oi.id = (SELECT MIN(oi2.id) FROM order_items oi2 WHERE oi2.order_id = o.order_id)
LEFT JOIN items i ON i.item_id = oi.item_id
LEFT JOIN images img ON img.image_id = (
  SELECT image_id FROM images WHERE item_id = i.item_id
  ORDER BY sequence ASC, image_id ASC LIMIT 1
)
WHERE o.user_id = ?
  AND o.status <> 'not paid'
  AND COALESCE(o.status, '') NOT IN ('cancelled', 'canceled')
ORDER BY o.created_at ASC;
