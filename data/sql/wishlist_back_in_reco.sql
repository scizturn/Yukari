-- Cross-sell CANDIDATES for the wishlist-back-in "Gas, nemenin yang udah kamu beli"
-- section: buyable item ids in the SAME SERIES as an item the user already bought
-- (the anchor, `?1`). The reader ranks these by the once-per-run popularity map and
-- keeps the top 6, then hydrates just those via wishlist_back_in_reco_hydrate.sql.
--
-- This is deliberately lightweight: only the item id and a recency tiebreak, NO
-- image lookup, NO manufacturer/series/category joins, NO popularity aggregate.
-- Series can be large (measured max ~3,465 buyable items); pulling full display
-- rows + a per-row image subquery for all of them was the ~700ms-per-user cost.
-- Fetching bare ids for the whole series then hydrating only the 6 winners is
-- ~10x cheaper. Driven by the series index (item_products_series_id_foreign).
--
-- Ready/in-stock/non-adult/non-admin-only/non-wakeari only, excluding items the user
-- already bought or wishlisted. Params: ?1 = anchor item id, ?2 and ?3 = user id.
SELECT
  CAST(i.item_id AS CHAR)                                 AS id,
  COALESCE(i.updated_at, i.created_at, CURRENT_TIMESTAMP) AS raw_updated_at
FROM item_products target
JOIN item_products ip ON ip.series_id = target.series_id
JOIN items i ON i.item_id = ip.item_id
LEFT JOIN item_access_limit ial ON ial.item_id = i.item_id
WHERE target.item_id = ?
  AND target.series_id > 0
  AND i.item_id != target.item_id
  AND i.status = 'ready'
  AND i.is_available = 1
  AND i.stock > 0
  AND COALESCE(i.isAdult, 0) = 0
  AND COALESCE(ial.is_admin_only, 0) = 0
  AND i.name NOT LIKE '%Wakeari%'
  AND NOT EXISTS (
    SELECT 1 FROM order_items oi
    JOIN orders o ON o.order_id = oi.order_id
    WHERE o.user_id = ? AND oi.item_id = i.item_id
      AND oi.ordered = 1 AND o.status NOT IN ('not paid', 'cancelled')
  )
  AND NOT EXISTS (
    SELECT 1 FROM wishlists w WHERE w.user_id = ? AND w.item_id = i.item_id
  )
