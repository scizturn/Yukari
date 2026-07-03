-- Ready-but-unpaid PO items for a single order (the arrived items to list in the
-- pelunasan email). Price comes from order_items.item_price (what the user
-- actually committed to), name falls back to items.name, image from the item's
-- first image. No item_products join, so no row multiplication.
SELECT
  CAST(i.item_id AS CHAR)                                          AS id,
  COALESCE(NULLIF(oi.item_name, ''), i.name, 'Item Kyou')         AS name,
  CONCAT('https://kyou.id/items/', i.item_id, '/')                AS url,
  COALESCE(CONCAT('https://kyoucdn.id/', img.path, '.webp'), '')  AS image_url,
  COALESCE(oi.item_price, 0)                                       AS price,
  oi.quantity
FROM order_items oi
JOIN items i ON i.item_id = oi.item_id
LEFT JOIN images img ON img.image_id = (
  SELECT image_id FROM images
  WHERE item_id = i.item_id
  ORDER BY sequence ASC, image_id ASC
  LIMIT 1
)
WHERE oi.order_id      = ?
  AND oi.status        = 'po'
  AND i.status         = 'ready'
  AND oi.cancelled_at IS NULL
  AND oi.refund_status = 'none'
ORDER BY oi.id ASC
