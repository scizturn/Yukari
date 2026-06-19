SELECT
  CAST(i.item_id AS CHAR)                                          AS id,
  i.name,
  CONCAT('https://kyou.id/items/', i.item_id, '/')                AS url,
  COALESCE(CONCAT('https://kyoucdn.id/', img.path, '.webp'), '')  AS image_url,
  ip.price                                                         AS original_price,
  i.discount_price,
  i.discount_name,
  i.discount_end_date,
  i.status,
  m.name                                                           AS manufacturer,
  s.name                                                           AS series_name
FROM items         i
JOIN item_products ip ON ip.item_id        = i.item_id
LEFT JOIN manufactures m ON m.manufacture_id = ip.manufacture_id
LEFT JOIN series       s ON s.series_id      = ip.series_id
LEFT JOIN images     img ON img.image_id     = (
  SELECT image_id FROM images
  WHERE item_id = i.item_id
  ORDER BY sequence ASC, image_id ASC
  LIMIT 1
)
WHERE ip.series_id IN (
  SELECT DISTINCT ip2.series_id
  FROM wishlists     w2
  JOIN items         i2  ON i2.item_id  = w2.item_id
  JOIN item_products ip2 ON ip2.item_id = i2.item_id
  WHERE w2.user_id           = ?
    AND i2.discount_name     IS NOT NULL AND i2.discount_name != ''
    AND i2.discount_end_date >= CURRENT_DATE
)
AND i.status        = 'ready'
AND i.stock         > 0
AND i.is_available  = 1
AND COALESCE(i.isAdult, 0) = 0
AND i.discount_name IS NOT NULL AND i.discount_name != ''
AND i.discount_end_date >= CURRENT_DATE
AND i.discount_price > 0
AND i.discount_price < ip.price
AND i.item_id NOT IN (
  SELECT item_id FROM wishlists WHERE user_id = ?
)
ORDER BY i.view_count DESC, i.updated_at DESC
LIMIT 12
