WITH historical_item AS (
  SELECT
    oi.item_id,
    ip.series_id,
    ip.category_id
  FROM orders o
  JOIN order_items oi ON oi.order_id = o.order_id
  LEFT JOIN item_products ip ON ip.item_id = oi.item_id
  WHERE o.user_id = ?
    AND o.status <> 'not paid'
    AND COALESCE(o.status, '') NOT IN ('cancelled', 'canceled')
  ORDER BY o.created_at DESC, oi.id ASC
  LIMIT 1
)
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
WHERE i.is_available = 1
  AND i.stock > 0
  AND COALESCE(i.isAdult, 0) = 0
  AND (ip.po_deadline IS NULL OR ip.po_deadline >= CURRENT_DATE)
  AND EXISTS (
    SELECT 1
    FROM historical_item hi
    WHERE (hi.series_id IS NOT NULL AND ip.series_id = hi.series_id)
      OR (hi.category_id IS NOT NULL AND ip.category_id = hi.category_id)
  )
  AND i.item_id <> (SELECT hi.item_id FROM historical_item hi)
  AND i.item_id NOT IN (
    SELECT ci.item_id
    FROM carts c
    JOIN cart_items ci ON ci.cart_id = c.cart_id
    WHERE c.user_id = ?
  )
  AND i.item_id NOT IN (
    SELECT oi.item_id
    FROM orders o
    JOIN order_items oi ON oi.order_id = o.order_id
    WHERE o.user_id = ?
      AND o.status <> 'not paid'
      AND COALESCE(o.status, '') NOT IN ('cancelled', 'canceled')
  )
ORDER BY
  CASE
    WHEN ip.series_id = (SELECT hi.series_id FROM historical_item hi) THEN 0
    ELSE 1
  END,
  i.view_count DESC,
  i.updated_at DESC
LIMIT 3;
