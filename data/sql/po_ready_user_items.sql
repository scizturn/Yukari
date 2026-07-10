-- User-centric PO->ready feed: one row per (user, wishlisted item that just turned
-- ready) for every verified user, over the window [?, ?) (last 7 days .. now).
-- The reader runs weekly, on Saturday, so the window tiles exactly one week.
--
-- EVENT GATE: this campaign owns items with a PO->ready conversion row.
-- wishlist-back-in owns items with a 0->>0 restock row. An item that trips both
-- comes here -- wishlist_back_in_user_items.sql stands down on any item that is
-- currently `ready` and has a conversion row inside THIS query's 7-day window.
-- Change one window and you must change the other, or items fall between the two
-- campaigns and nobody announces them.
--
-- "Turned ready" = a stock_logs row from one of the three admin paths that convert
-- a PO item into ready stock. Verified against hanayo_prod (2026-07-10):
--
--   convert po by excel   221,717 rows all-time, still firing daily  <- the real path
--   convert po manual       8,586 rows, same semantics, manual entry
--   reconvert PO to ready   7,811 rows, small and 98% redundant (see below)
--
-- None of these are restock rows (is_restock = 0) and none carry before/after stock
-- in `information`, so stock availability CANNOT be read off the event. It is
-- verified against current item state instead: status='ready' AND stock > 0 AND
-- is_available = 1. An item that converted but sold out again is not "ready" to a
-- shopper, so it is excluded.
--
-- WHY NOT just `reconvert PO to ready`: of the 697 items it touched in the last 90
-- days, 682 also had an `Increased via Insert Stock (Adjusment)` 0->>0 restock, so
-- wishlist-back-in already emails those users. `convert po by excel` overlaps the
-- restock path for only 173 of 4,913 items (3.5%) -- that near-disjoint population
-- is this campaign's reason to exist. All three are kept because the cross-feature
-- dedup below collapses the overlap for free.
--
-- Dedup is per (user, item) with a 90-day cooldown and spans BOTH features: an item
-- announced by wishlist-back-in is not announced again by po-ready, and vice versa.
-- Prior sends record their item ids in the audit row's metadata.item_ids array
-- (reference_id = user_id), matched via JSON_CONTAINS. Keep the feature list here in
-- sync with wishlist_back_in_user_items.sql.
--
-- PERF: stock_logs drives the join (STRAIGHT_JOIN) because the 7-day window narrows
-- it to a few thousand rows; letting the planner start from items forces a full
-- ~200k-row scan. One item yields several stock_logs rows on the same timestamp (one
-- per branch: ALPHA, OP, SS, DELTA), so GROUP BY collapses them and MAX() picks the
-- newest conversion.
--
-- Ordered user_id, then newest conversion first so the reader can cap each user's
-- list to the 5 most recently readied items.
SELECT STRAIGHT_JOIN
  CAST(u.user_id AS CHAR)                                         AS user_id,
  u.name                                                          AS user_name,
  u.email                                                         AS user_email,
  u.email_verified_at IS NOT NULL                                 AS is_active,
  CAST(i.item_id AS CHAR)                                         AS id,
  i.name,
  CONCAT('https://kyou.id/items/', i.item_id, '/')               AS url,
  COALESCE(CONCAT('https://kyoucdn.id/', img.path, '.webp'), '') AS image_url,
  ip.price,
  MAX(sl.created_at)                                              AS ready_at,
  CASE WHEN i.discount_price > 0 AND i.discount_price < ip.price
       AND i.discount_name IS NOT NULL AND i.discount_name <> ''
       AND i.discount_qty > 0
       AND i.discount_start_date IS NOT NULL AND i.discount_end_date IS NOT NULL
       AND i.discount_start_date <= CURDATE() AND i.discount_end_date >= CURDATE()
     THEN i.discount_price ELSE 0 END                             AS discount_price
FROM stock_logs sl
JOIN items i ON i.item_id = sl.item_id
JOIN item_products ip ON ip.item_id = i.item_id
JOIN wishlists w ON w.item_id = i.item_id
JOIN users u ON u.user_id = w.user_id
LEFT JOIN images img ON img.image_id = (
  SELECT image_id
  FROM images
  WHERE item_id = i.item_id
  ORDER BY sequence ASC, image_id ASC
  LIMIT 1
)
WHERE sl.description IN ('convert po by excel', 'convert po manual', 'reconvert PO to ready')
  AND sl.created_at >= ?
  AND sl.created_at < ?
  AND i.status = 'ready'
  AND i.stock > 0
  AND i.is_available = 1
  AND COALESCE(i.isAdult, 0) = 0
  AND u.email_verified_at IS NOT NULL
  AND u.email IS NOT NULL AND u.email <> ''
  AND NOT EXISTS (
    SELECT 1
    FROM email_delivery_logs edl
    WHERE edl.feature IN ('po_ready', 'wishlist_back_in')
      AND edl.user_id = CAST(u.user_id AS CHAR)
      AND edl.status IN ('queued', 'sending', 'sent')
      AND edl.created_at >= DATE_SUB(NOW(), INTERVAL 90 DAY)
      AND JSON_CONTAINS(JSON_EXTRACT(edl.metadata, '$.item_ids'), JSON_QUOTE(CAST(i.item_id AS CHAR)))
  )
GROUP BY u.user_id, u.name, u.email, u.email_verified_at, i.item_id, i.name, img.path, ip.price
ORDER BY u.user_id, ready_at DESC, i.item_id
