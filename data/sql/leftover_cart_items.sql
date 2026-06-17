SELECT
  i.item_id,
  i.name AS name,
  CONCAT('https://kyou.id/items/', i.item_id, '/') AS url,
  COALESCE(CONCAT('https://kyoucdn.id/', img.path, '.webp'), '') AS image_url,
  ip.price,
  i.status,
  m.name AS manufacturer,
  s.name AS series_name,
  ip.po_deadline,
  ip.po_release_date
FROM carts c
JOIN cart_items ci ON ci.cart_id = c.cart_id
JOIN items i ON i.item_id = ci.item_id
JOIN item_products ip ON ip.item_id = i.item_id
LEFT JOIN manufactures m ON m.manufacture_id = ip.manufacture_id
LEFT JOIN series s ON s.series_id = ip.series_id
LEFT JOIN images img ON img.image_id = (
  SELECT image_id
  FROM images
  WHERE item_id = i.item_id
  ORDER BY sequence ASC, image_id ASC
  LIMIT 1
)
WHERE c.user_id = ?
  AND i.is_available = 1
  AND i.stock > 0
  AND COALESCE(i.isAdult, 0) = 0
ORDER BY i.view_count DESC, ci.created_at DESC
LIMIT 4;
