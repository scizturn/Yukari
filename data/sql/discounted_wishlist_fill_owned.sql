-- Item pool yang UDAH ada di wishlist tiap user — dipakai buat ngeluarin mereka dari
-- fill grid (fill itu "item diskon LAIN", bukan yang udah dia wishlist). Ditarik SEKALI
-- per run, bukan per user.
--
-- Query lama ngeluarinnya pakai `i.item_id NOT IN (SELECT item_id FROM wishlists WHERE
-- user_id = ?)` — seluruh isi wishlist user. Di sini sengaja cuma yang irisannya sama
-- POOL (filternya identik dengan discounted_wishlist_fill_pool.sql): item yang nggak
-- ada di pool nggak mungkin jadi kandidat, jadi ngeluarin dia nggak ada gunanya.
-- Hasil akhirnya sama persis, tapi yang ditahan di memory jauh lebih sedikit —
-- bukan seluruh wishlist 32rb user.
-- Tanpa parameter.
SELECT DISTINCT
  CAST(w.user_id AS CHAR) AS user_id,
  CAST(w.item_id AS CHAR) AS item_id
FROM wishlists     w
JOIN items         i  ON i.item_id  = w.item_id
JOIN item_products ip ON ip.item_id = i.item_id
WHERE i.status        = 'ready'
  AND i.stock         > 0
  AND i.is_available  = 1
  AND COALESCE(i.isAdult, 0) = 0
  AND i.discount_name IS NOT NULL AND i.discount_name != ''
  AND i.discount_end_date >= CURRENT_DATE
  AND i.discount_price > 0
  AND i.discount_price < ip.price
  AND ip.series_id IS NOT NULL
