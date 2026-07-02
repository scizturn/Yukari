SELECT
  COALESCE(i.name, 'Item Kyou') AS name,
  -- series name via correlated subquery so a multi-row item_products join can't
  -- duplicate the order row.
  COALESCE((
    SELECT s.name
    FROM item_products ip
    JOIN series s ON s.series_id = ip.series_id
    WHERE ip.item_id = i.item_id
    LIMIT 1
  ), '') AS series_name,
  COALESCE(CONCAT('https://kyoucdn.id/', img.path, '.webp'), '') AS image_url,
  COALESCE(CONCAT('https://kyou.id/items/', i.item_id, '/'), '') AS url,
  o.created_at AS order_date
FROM orders o
LEFT JOIN order_items oi ON oi.order_id = o.order_id
  AND oi.id = (SELECT MIN(oi2.id) FROM order_items oi2 WHERE oi2.order_id = o.order_id)
LEFT JOIN items i ON i.item_id = oi.item_id
LEFT JOIN images img ON img.image_id = (
  SELECT image_id FROM images WHERE item_id = i.item_id
  ORDER BY sequence ASC, image_id ASC LIMIT 1
)
WHERE o.user_id = ?
  -- only orders that have already reached the customer ("sudah sampai"):
  -- shipping/shipped are the fulfilled states; PO/waiting/cancelled are excluded.
  AND o.status IN ('shipping', 'shipped')
ORDER BY o.created_at ASC;
