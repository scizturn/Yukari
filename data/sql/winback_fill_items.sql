-- Most-popular READY items used to fill the winback "wishlist ready" grid up to
-- 12. Same column shape as popular_items.sql (reuses the fypRows scanner), but
-- restricted to status='ready' and a larger LIMIT. Kept separate from
-- popular_items.sql (LIMIT 3, no ready filter) so birthday/anniversary are
-- unaffected.
--
-- "Most popular" here mirrors the /search "Most Popular" sort exactly. On the
-- site that sort is `kyou_search_score`, which the mitsuha search backend
-- precomputes daily and stores as `search_score` in Elasticsearch. Its formula
-- is a weighted sum of user_item_actions over the trailing 14 days:
--     view=1, wishlist=3, cart=5, bought=10   (mitsuha/src/core/mitsuha/mitsuha.go)
-- We recompute the same score live from user_item_actions and order by it, so
-- the winback fill grid ranks items the way the user sees them on /search.
-- (The old `ORDER BY i.view_count DESC` was meaningless: items.view_count is
-- unpopulated / 0, so it silently fell through to updated_at = newest.)
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
LEFT JOIN (
  SELECT
    uia.item_id,
    SUM(
      CASE uia.action
        WHEN 'view'     THEN 1
        WHEN 'wishlist' THEN 3
        WHEN 'cart'     THEN 5
        WHEN 'bought'   THEN 10
        ELSE 0
      END
    ) AS search_score
  FROM user_item_actions uia
  WHERE uia.action IN ('view', 'wishlist', 'cart', 'bought')
    AND uia.created_at > (NOW() - INTERVAL 14 DAY)
  GROUP BY uia.item_id
) ss ON ss.item_id = i.item_id
-- admin-only items (e.g. "[Free] Paperbag/Totebag Kyou" checkout add-ons) rank
-- high on search_score from cart-adds but are hidden from public /search. Mirror
-- that: public search filters is_admin_only=false via item_access_limit, so we
-- exclude them here too.
LEFT JOIN item_access_limit ial ON ial.item_id = i.item_id
WHERE i.status = 'ready'
  AND i.is_available = 1
  AND i.stock > 0
  AND COALESCE(i.isAdult, 0) = 0
  AND COALESCE(ial.is_admin_only, 0) = 0
  -- skip "wakeari" (訳あり / imperfect-grade) items: no dedicated column, only the
  -- name tag marks them, so match the bracketed "[Wakeari]" label.
  AND i.name NOT LIKE '%Wakeari%'
ORDER BY COALESCE(ss.search_score, 0) DESC, i.updated_at DESC
LIMIT 15;
