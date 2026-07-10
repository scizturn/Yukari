-- Hydrates the wishlist-back-in cross-sell winners: given the up-to-6 item ids the
-- reader picked from wishlist_back_in_reco.sql (ranked by the popularity map), fetch
-- the full display columns. Only ~6 rows, so the per-row image subquery — the cost
-- the candidate query deliberately skips — runs just a handful of times.
--
-- The IN-list placeholder token below is replaced with the right number of `?`
-- placeholders in Go (it must be the only such token in this file). IN does not
-- preserve order, so the reader reorders the results to the ranked ids.
-- Columns/shape must match wishlist_back_in_reco_category.sql so both feed the same
-- scan.
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
  COALESCE(i.restocked_at, i.updated_at, i.created_at, CURRENT_TIMESTAMP) AS restocked_at,
  CASE WHEN i.discount_price > 0 AND i.discount_price < ip.price
       AND i.discount_name IS NOT NULL AND i.discount_name <> ''
       AND i.discount_qty > 0
       AND i.discount_start_date IS NOT NULL AND i.discount_end_date IS NOT NULL
       AND i.discount_start_date <= CURDATE() AND i.discount_end_date >= CURDATE()
     THEN i.discount_price ELSE 0 END                             AS discount_price,
  0                                                               AS down_payment
FROM items i
JOIN item_products ip ON ip.item_id = i.item_id
LEFT JOIN manufactures m ON m.manufacture_id = ip.manufacture_id
LEFT JOIN series s ON s.series_id = ip.series_id
LEFT JOIN categories c ON c.category_id = ip.category_id
LEFT JOIN images img ON img.image_id = (
  SELECT image_id FROM images
  WHERE item_id = i.item_id
  ORDER BY sequence ASC, image_id ASC
  LIMIT 1
)
WHERE i.item_id IN (/*IDS*/)
