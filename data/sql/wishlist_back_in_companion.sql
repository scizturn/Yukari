SELECT
  CAST(i.item_id AS CHAR)                                         AS id,
  i.name,
  CONCAT('https://kyou.id/items/', i.item_id, '/')               AS url,
  COALESCE(CONCAT('https://kyoucdn.id/', img.path, '.webp'), '') AS image_url,
  oi.item_price                                                   AS price,
  i.status,
  COALESCE(m.name, '')                                            AS manufacturer,
  COALESCE(s.name, '')                                            AS series_name,
  COALESCE(c.name, '')                                            AS category_name,
  COALESCE(i.restocked_at, i.updated_at, i.created_at, CURRENT_TIMESTAMP) AS restocked_at
FROM orders o
JOIN order_items oi ON oi.order_id = o.order_id
JOIN items i ON i.item_id = oi.item_id
JOIN item_products ip ON ip.item_id = i.item_id
JOIN item_products target ON target.item_id = ?
LEFT JOIN manufactures m ON m.manufacture_id = ip.manufacture_id
LEFT JOIN series s ON s.series_id = ip.series_id
LEFT JOIN categories c ON c.category_id = ip.category_id
LEFT JOIN images img ON img.image_id = (
  SELECT image_id
  FROM images
  WHERE item_id = i.item_id
  ORDER BY sequence ASC, image_id ASC
  LIMIT 1
)
WHERE o.user_id = ?
  AND i.item_id != target.item_id
  AND oi.ordered = 1
  AND o.status NOT IN ('not paid', 'cancelled')
  AND oi.cancelled_quantity < oi.quantity
  AND (
    (target.series_id > 0 AND ip.series_id = target.series_id)
    OR (target.category_id IS NOT NULL AND ip.category_id = target.category_id)
  )
ORDER BY
  CASE WHEN target.series_id > 0 AND ip.series_id = target.series_id THEN 0 ELSE 1 END,
  o.created_at DESC
LIMIT 1
