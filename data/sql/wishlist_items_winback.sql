-- Winback-specific wishlist query. Sama seperti wishlist_items.sql TAPI LIMIT 12
-- (bukan LIMIT 1): grid winback ("wishlist kamu udah ready") harus nampilin
-- wishlist ASLI user sebanyak mungkin dulu, baru sisanya diisi most-popular.
-- wishlist_items.sql tetap LIMIT 1 karena dipakai birthday (teaser 1 item).
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
FROM wishlists w
JOIN items i ON i.item_id = w.item_id
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
WHERE w.user_id = ?
  AND i.is_available = 1
  AND i.stock > 0
  AND COALESCE(i.isAdult, 0) = 0
  AND (ip.po_deadline IS NULL OR ip.po_deadline >= CURRENT_DATE)
ORDER BY i.view_count DESC, w.created_at DESC
LIMIT 12;
