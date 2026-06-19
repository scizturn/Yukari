SELECT
  CAST(i.item_id AS CHAR)                                         AS id,
  i.name,
  CONCAT('https://kyou.id/items/', i.item_id, '/')               AS url,
  COALESCE(CONCAT('https://kyoucdn.id/', img.path, '.webp'), '') AS image_url,
  ip.price,
  i.status,
  COALESCE(m.name, '')                                            AS manufacturer,
  COALESCE(s.name, '')                                            AS series_name,
  COALESCE(c.name, '')                                            AS category_name,
  COUNT(DISTINCT w.user_id)                                       AS popular_score,
  MAX(sl.created_at)                                              AS restocked_at
FROM stock_logs sl
JOIN items i ON i.item_id = sl.item_id
JOIN item_products ip ON ip.item_id = i.item_id
JOIN wishlists w ON w.item_id = i.item_id
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
WHERE sl.is_restock = 1
  AND sl.type = 'increase'
  AND sl.description = 'Increased via Insert Stock (Adjusment)'
  AND CAST(JSON_UNQUOTE(JSON_EXTRACT(sl.information, '$.before_all_stock')) AS SIGNED) = 0
  AND CAST(JSON_UNQUOTE(JSON_EXTRACT(sl.information, '$.after_all_stock')) AS SIGNED) > 0
  AND sl.created_at >= ?
  AND sl.created_at < ?
  AND i.status = 'ready'
  AND i.stock > 0
  AND i.is_available = 1
  AND COALESCE(i.isAdult, 0) = 0
  AND NOT EXISTS (
    SELECT 1
    FROM email_delivery_logs edl
    WHERE edl.feature = 'wishlist_back_in'
      AND edl.reference_id = CAST(i.item_id AS CHAR)
      AND edl.status IN ('queued', 'sending', 'sent')
  )
GROUP BY i.item_id, i.name, img.path, ip.price, i.status, m.name, s.name, c.name
ORDER BY popular_score DESC, restocked_at ASC
LIMIT 5
