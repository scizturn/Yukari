-- User-centric wishlist-back-in feed: one row per (user, restocked wishlist item)
-- for every verified user whose wishlisted item came back in stock during the
-- carry-over window [?, ?) (last ~21 days .. this Friday 00:00). The window is
-- wider than a week so items that overflowed a user's 5-item cap earlier still
-- surface on a later Friday; the per-(user,item) dedup below drains that queue.
--
-- "Back in stock" = a stock_logs restock event that moved all-stock 0 -> >0
-- (Insert Stock adjustment). Covers BOTH ready items (status='ready') and PO
-- items (status='PO') whose slot reopened, guarded so the PO is still open
-- (po_deadline in the future or open-ended) -- an expired PO is not "buyable".
--
-- PERF: stock_logs is the driving table (STRAIGHT_JOIN) because the 7-day window
-- narrows it to a handful of rows; letting the planner start from items instead
-- forces a full 200k-row scan (74s+). No popularity self-join -- user-centric
-- ranks by newest restock, so global popularity is not needed.
--
-- Dedup is per (user, item) with a 90-day cooldown: a user is not re-notified
-- about an item they were emailed for within the last 90 days, but a genuine
-- restock after a longer absence re-engages them. Prior sends record their item
-- ids in the audit row's metadata.item_ids array (reference_id = user_id),
-- matched via JSON_CONTAINS.
--
-- The cooldown spans BOTH item-news features: an item already announced by
-- po-ready ("your PO item is now ready") is not announced again here as a
-- restock, and vice versa. Keep the feature list in sync with
-- po_ready_user_items.sql.
--
-- Separately from the cooldown, the two campaigns are partitioned by EVENT TYPE
-- (the "gate" below): a 0->>0 restock row means the item is ours; a PO->ready
-- conversion row means it is po-ready's. Items that trip both -- a conversion
-- that also ended a stockout, or two nearby events -- go to po-ready. The gate
-- is what makes the split deterministic; it does not depend on which cron runs
-- first, and it is what keeps po-ready from stealing items that overflowed a
-- user's 5-item cap here and are still queued for a later Friday.
--
-- Ordered user_id, then newest restock first so the reader can cap each user's
-- list to the 5 most recently returned items.
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
  i.status,
  COALESCE(m.name, '')                                            AS manufacturer,
  COALESCE(s.name, '')                                            AS series_name,
  COALESCE(c.name, '')                                            AS category_name,
  MAX(sl.created_at)                                              AS restocked_at,
  CASE WHEN i.discount_price > 0 AND i.discount_price < ip.price
       AND i.discount_name IS NOT NULL AND i.discount_name <> ''
       AND i.discount_qty > 0
       AND i.discount_start_date IS NOT NULL AND i.discount_end_date IS NOT NULL
       AND i.discount_start_date <= CURDATE() AND i.discount_end_date >= CURDATE()
     THEN i.discount_price ELSE 0 END                             AS discount_price,
  CASE WHEN i.status IN ('PO', 'LPO', 'BO', 'BPO') AND ip.po_down_payment > 0
     THEN ip.po_down_payment ELSE 0 END                           AS down_payment,
  -- Gross profit ratio, in percent. MUST stay identical to hanayo's
  -- VoucherValidationService::getItemGpRatio(), which is what actually gates the
  -- voucher at checkout: ((price - cogs) / price) * 100, price from item_products,
  -- cogs from item_states. NULL when hanayo would also bail (cogs unknown or
  -- price <= 0) -- such an item passes no gp_ratio rule, so it earns no discount.
  -- cogs = 0 deliberately yields 100.0 here, exactly as hanayo computes it.
  -- CAST to SIGNED first: both columns are UNSIGNED and cogs > price underflows.
  CASE WHEN ist.cogs IS NULL OR ip.price <= 0 THEN NULL
       ELSE ((CAST(ip.price AS SIGNED) - CAST(ist.cogs AS SIGNED)) / ip.price) * 100
  END                                                             AS gp_ratio
FROM stock_logs sl
JOIN items i ON i.item_id = sl.item_id
JOIN item_products ip ON ip.item_id = i.item_id
JOIN wishlists w ON w.item_id = i.item_id
JOIN users u ON u.user_id = w.user_id
LEFT JOIN item_states ist ON ist.item_id = i.item_id
LEFT JOIN manufactures m ON m.manufacture_id = ip.manufacture_id
LEFT JOIN series s ON s.series_id = ip.series_id
LEFT JOIN categories c ON c.category_id = ip.category_id
LEFT JOIN images img ON img.image_id = (
  SELECT image_id
  FROM images
  WHERE item_id = i.item_id
  ORDER BY sequence ASC, image_id ASC
  LIMIT 1
)
WHERE sl.is_restock = 1
  AND sl.type = 'increase'
  AND sl.description = 'Increased via Insert Stock (Adjusment)'
  AND CAST(JSON_UNQUOTE(JSON_EXTRACT(sl.information, '$.before_all_stock')) AS SIGNED) = 0
  AND CAST(JSON_UNQUOTE(JSON_EXTRACT(sl.information, '$.after_all_stock')) AS SIGNED) > 0
  AND sl.created_at >= ?
  AND sl.created_at < ?
  AND i.status IN ('ready', 'PO')
  AND (ip.po_deadline IS NULL OR ip.po_deadline >= CURRENT_DATE)
  AND i.stock > 0
  AND i.is_available = 1
  AND COALESCE(i.isAdult, 0) = 0
  AND u.email_verified_at IS NOT NULL
  AND u.email IS NOT NULL AND u.email <> ''
  -- EVENT GATE: an item whose PO stock was just converted belongs to po-ready,
  -- not here. Some items trip both triggers (a conversion that also ended a
  -- stockout, or a separate restock nearby); the conversion wins.
  --
  -- The condition must mirror po_ready_user_items.sql's eligibility EXACTLY --
  -- same 7-day conversion window, same `status = 'ready'` -- not merely "has ever
  -- been converted". Widen it and items fall between the two campaigns: an item
  -- converted 20 days ago and restocked today would be rejected here while
  -- sitting outside po-ready's window, and nobody would announce it.
  AND NOT (
    i.status = 'ready'
    AND EXISTS (
      SELECT 1
      FROM stock_logs conv
      WHERE conv.item_id = i.item_id
        AND conv.description IN ('convert po by excel', 'convert po manual', 'reconvert PO to ready')
        AND conv.created_at >= DATE_SUB(NOW(), INTERVAL 7 DAY)
    )
  )
  AND NOT EXISTS (
    SELECT 1
    FROM email_delivery_logs edl
    WHERE edl.feature IN ('wishlist_back_in', 'po_ready')
      AND edl.user_id = CAST(u.user_id AS CHAR)
      AND edl.status IN ('queued', 'sending', 'sent')
      AND edl.created_at >= DATE_SUB(NOW(), INTERVAL 90 DAY)
      AND JSON_CONTAINS(JSON_EXTRACT(edl.metadata, '$.item_ids'), JSON_QUOTE(CAST(i.item_id AS CHAR)))
  )
GROUP BY u.user_id, u.name, u.email, i.item_id, i.name, img.path, ip.price, i.status, m.name, s.name, c.name, ist.cogs
ORDER BY u.user_id, restocked_at DESC, i.item_id
