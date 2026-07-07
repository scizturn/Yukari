-- Cross-sell recommendations for the wishlist-back-in "Gas, nemenin yang udah
-- kamu beli" section: the 6 most-popular Kyou items in the same series/category
-- as an item the user ALREADY BOUGHT (the anchor, `?1`), to drive new sales.
--
-- NOT from the user's wishlist/purchases -- these are fresh items to buy. We
-- exclude items the user already bought or already wishlisted so we never
-- recommend something they own or are already tracking.
--
-- "Most popular" mirrors /search "Most Popular" = kyou_search_score, recomputed
-- live from user_item_actions over the trailing 14 days (view=1, wishlist=3,
-- cart=5, bought=10) -- same formula as winback_fill_items.sql. Ready/in-stock/
-- non-adult/non-admin-only/non-wakeari only (directly buyable).
--
-- The reader only keeps the section when this returns a FULL 6; otherwise the
-- whole "Gas, nemenin..." block is hidden (N/A). Params: ?1 = anchor item id,
-- ?2 and ?3 = user id (bought-exclude, wishlist-exclude).
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
JOIN item_products target ON target.item_id = ?
LEFT JOIN manufactures m ON m.manufacture_id = ip.manufacture_id
LEFT JOIN series s ON s.series_id = ip.series_id
LEFT JOIN categories c ON c.category_id = ip.category_id
LEFT JOIN images img ON img.image_id = (
  SELECT image_id FROM images
  WHERE item_id = i.item_id
  ORDER BY sequence ASC, image_id ASC
  LIMIT 1
)
LEFT JOIN item_access_limit ial ON ial.item_id = i.item_id
LEFT JOIN (
  SELECT
    uia.item_id,
    SUM(CASE uia.action
      WHEN 'view' THEN 1 WHEN 'wishlist' THEN 3 WHEN 'cart' THEN 5 WHEN 'bought' THEN 10 ELSE 0 END) AS search_score
  FROM user_item_actions uia
  WHERE uia.action IN ('view', 'wishlist', 'cart', 'bought')
    AND uia.created_at > (NOW() - INTERVAL 14 DAY)
  GROUP BY uia.item_id
) ss ON ss.item_id = i.item_id
WHERE i.item_id != target.item_id
  AND i.status = 'ready'
  AND i.is_available = 1
  AND i.stock > 0
  AND COALESCE(i.isAdult, 0) = 0
  AND COALESCE(ial.is_admin_only, 0) = 0
  AND i.name NOT LIKE '%Wakeari%'
  AND (
    (target.series_id > 0 AND ip.series_id = target.series_id)
    OR (target.category_id IS NOT NULL AND ip.category_id = target.category_id)
  )
  AND NOT EXISTS (
    SELECT 1 FROM order_items oi
    JOIN orders o ON o.order_id = oi.order_id
    WHERE o.user_id = ? AND oi.item_id = i.item_id
      AND oi.ordered = 1 AND o.status NOT IN ('not paid', 'cancelled')
  )
  AND NOT EXISTS (
    SELECT 1 FROM wishlists w WHERE w.user_id = ? AND w.item_id = i.item_id
  )
GROUP BY i.item_id, i.name, img.path, ip.price, i.status, m.name, s.name, c.name
ORDER BY
  CASE WHEN target.series_id > 0 AND ip.series_id = target.series_id THEN 0 ELSE 1 END,
  COALESCE(ss.search_score, 0) DESC,
  i.updated_at DESC
LIMIT 6
