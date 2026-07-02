-- Most-popular READY items used to fill the winback "wishlist ready" grid up to
-- 12. Same column shape as popular_items.sql (reuses the fypRows scanner), but
-- restricted to status='ready' and a larger LIMIT. Kept separate from
-- popular_items.sql (LIMIT 3, no ready filter) so birthday/anniversary are
-- unaffected.
SELECT
  i.item_id,
  i.name AS name,
  CASE WHEN i.character_name IS NULL OR i.character_name = '' THEN 'series' ELSE 'character' END AS kind,
  CAST(ip.series_id AS CHAR) AS series_id,
  COALESCE(CONCAT('https://kyoucdn.id/', img.path, '.webp'), '') AS image_url,
  ip.price,
  i.status,
  m.name AS manufacturer,
  s.name AS series_name,
  ip.po_deadline,
  ip.po_release_date
FROM items i
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
WHERE i.status = 'ready'
  AND i.is_available = 1
  AND i.stock > 0
  AND COALESCE(i.isAdult, 0) = 0
  -- skip "wakeari" (訳あり / imperfect-grade) items: no dedicated column, only the
  -- name tag marks them, so match the bracketed "[Wakeari]" label.
  AND i.name NOT LIKE '%Wakeari%'
ORDER BY i.view_count DESC, i.updated_at DESC
LIMIT 15;
