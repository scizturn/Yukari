-- Forcejob/previewjob-only: a user's wishlist items that are currently ready and
-- buyable (up to 5, newest conversion first), bypassing the conversion-window +
-- dedup eligibility so a test send can be produced for any chosen user. Same
-- column shape as po_ready_user_items.sql (minus the user columns) so it scans
-- into PoReadyItem.
--
-- ready_at falls back through the item's own timestamps when the user has no
-- stock_logs conversion row in range: the forced job still needs a plausible
-- date, and the real campaign never reaches this query.
SELECT
  CAST(i.item_id AS CHAR)                                         AS id,
  i.name,
  CONCAT('https://kyou.id/items/', i.item_id, '/')               AS url,
  COALESCE(CONCAT('https://kyoucdn.id/', img.path, '.webp'), '') AS image_url,
  ip.price,
  COALESCE(
    (SELECT MAX(sl.created_at) FROM stock_logs sl
     WHERE sl.item_id = i.item_id
       AND sl.description IN ('convert po by excel', 'convert po manual', 'reconvert PO to ready')),
    i.restocked_at, i.updated_at, i.created_at, CURRENT_TIMESTAMP
  )                                                               AS ready_at,
  CASE WHEN i.discount_price > 0 AND i.discount_price < ip.price
       AND i.discount_name IS NOT NULL AND i.discount_name <> ''
       AND i.discount_qty > 0
       AND i.discount_start_date IS NOT NULL AND i.discount_end_date IS NOT NULL
       AND i.discount_start_date <= CURDATE() AND i.discount_end_date >= CURDATE()
     THEN i.discount_price ELSE 0 END                             AS discount_price
FROM wishlists w
JOIN items i ON i.item_id = w.item_id
JOIN item_products ip ON ip.item_id = i.item_id
LEFT JOIN images img ON img.image_id = (
  SELECT image_id FROM images
  WHERE item_id = i.item_id
  ORDER BY sequence ASC, image_id ASC
  LIMIT 1
)
WHERE w.user_id = ?
  AND i.status = 'ready'
  AND i.stock > 0
  AND i.is_available = 1
  AND COALESCE(i.isAdult, 0) = 0
GROUP BY i.item_id, i.name, img.path, ip.price
ORDER BY ready_at DESC, i.item_id
LIMIT 5
