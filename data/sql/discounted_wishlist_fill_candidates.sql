-- Kandidat "fill" discounted-wishlist: item diskon LAIN di series yang sama dengan
-- item diskon yang udah ada di wishlist user. Reader ambil id-nya doang, lalu
-- hydrate cuma 12 pemenang lewat discounted_wishlist_hydrate.sql.
--
-- Sengaja ringan: cuma item id, TANPA subquery gambar per-baris dan TANPA join
-- manufactures/series. Join dieksekusi sebelum ORDER BY/LIMIT, jadi versi lama
-- (discounted_wishlist_fill.sql) nembak subquery gambar buat SETIAP kandidat di
-- series itu, bukan cuma 12 yang menang. Trade yang sama udah diambil di
-- wishlist_back_in_reco.sql, di sana ~10x lebih murah.
--
-- Digerakin dari item_products biar jalan lewat index series
-- (item_products_series_id_foreign), bukan scan items.
--
-- Filter diskon + sendability HIDUP DI SINI, bukan di query hydrate.
-- Params: ?1 = user id (sumber series), ?2 = user id (exclude wishlist dia sendiri).
SELECT CAST(i.item_id AS CHAR) AS id
FROM item_products ip
JOIN items         i  ON i.item_id = ip.item_id
WHERE ip.series_id IN (
  SELECT DISTINCT ip2.series_id
  FROM wishlists     w2
  JOIN items         i2  ON i2.item_id  = w2.item_id
  JOIN item_products ip2 ON ip2.item_id = i2.item_id
  WHERE w2.user_id           = ?
    AND i2.discount_name     IS NOT NULL AND i2.discount_name != ''
    AND i2.discount_end_date >= CURRENT_DATE
)
  AND i.status        = 'ready'
  AND i.stock         > 0
  AND i.is_available  = 1
  AND COALESCE(i.isAdult, 0) = 0
  AND i.discount_name IS NOT NULL AND i.discount_name != ''
  AND i.discount_end_date >= CURRENT_DATE
  AND i.discount_price > 0
  AND i.discount_price < ip.price
  AND NOT EXISTS (
    SELECT 1 FROM wishlists w WHERE w.user_id = ? AND w.item_id = i.item_id
  )
ORDER BY i.view_count DESC, i.updated_at DESC
LIMIT 12
