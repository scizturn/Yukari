-- Forcejob-only: a user's currently-available wishlist items (up to 5, newest
-- restock first), bypassing the restock-window + dedup eligibility so a test
-- send can be produced for any chosen user. Same column shape as
-- wishlist_back_in_user_items.sql (minus the user columns) so it scans into
-- WishlistBackInItem, including the discount/DP price fields.
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
  CASE WHEN i.status IN ('PO', 'LPO', 'BO', 'BPO') AND ip.po_down_payment > 0
     THEN ip.po_down_payment ELSE 0 END                           AS down_payment,
  -- See wishlist_back_in_user_items.sql for why this mirrors hanayo's formula.
  CASE WHEN ist.cogs IS NULL OR ip.price <= 0 THEN NULL
       ELSE ((CAST(ip.price AS SIGNED) - CAST(ist.cogs AS SIGNED)) / ip.price) * 100
  END                                                             AS gp_ratio
FROM wishlists w
JOIN items i ON i.item_id = w.item_id
JOIN item_products ip ON ip.item_id = i.item_id
LEFT JOIN item_states ist ON ist.item_id = i.item_id
LEFT JOIN manufactures m ON m.manufacture_id = ip.manufacture_id
LEFT JOIN series s ON s.series_id = ip.series_id
LEFT JOIN categories c ON c.category_id = ip.category_id
LEFT JOIN images img ON img.image_id = (
  SELECT image_id FROM images
  WHERE item_id = i.item_id
  ORDER BY sequence ASC, image_id ASC
  LIMIT 1
)
WHERE w.user_id = ?
  AND i.status IN ('ready', 'PO')
  AND (ip.po_deadline IS NULL OR ip.po_deadline >= CURRENT_DATE)
  AND i.stock > 0
  AND i.is_available = 1
  AND COALESCE(i.isAdult, 0) = 0
GROUP BY i.item_id, i.name, img.path, ip.price, i.status, m.name, s.name, c.name, ist.cogs
ORDER BY restocked_at DESC, i.item_id
LIMIT 5
