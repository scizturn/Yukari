-- The 14-day "Most Popular" score (kyou_search_score) per item, computed ONCE per
-- wishlist-back-in run and held in a Go map. The series reco query used to rebuild
-- this aggregate on every user (~134ms each, ~1,290 times); loading it once and
-- ranking candidates in Go removes that repeated work.
--
-- Same weights and 14-day window as winback_fill_items.sql and the category
-- fallback (wishlist_back_in_reco_category.sql): view=1, wishlist=3, cart=5,
-- bought=10. Keep all three in sync so /search and the email quote one ordering.
SELECT
  CAST(uia.item_id AS CHAR) AS id,
  SUM(CASE uia.action
    WHEN 'view' THEN 1 WHEN 'wishlist' THEN 3 WHEN 'cart' THEN 5 WHEN 'bought' THEN 10 ELSE 0 END) AS search_score
FROM user_item_actions uia
WHERE uia.action IN ('view', 'wishlist', 'cart', 'bought')
  AND uia.created_at > (NOW() - INTERVAL 14 DAY)
GROUP BY uia.item_id
