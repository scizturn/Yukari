-- Hydrate pemenang fill discounted-wishlist: dikasih ≤12 item id yang udah diranking
-- discounted_wishlist_fill_candidates.sql, ambil kolom display lengkapnya. Cuma ~12
-- baris, jadi subquery gambar per-baris — biaya yang sengaja dilewatin query kandidat
-- — cuma jalan segelintir kali.
--
-- Token IN-list di bawah diganti jumlah `?` yang pas di Go (harus jadi satu-satunya
-- token itu di file ini). IN nggak jaga urutan, jadi reader ngurutin ulang hasilnya
-- balik ke urutan ranked id.
--
-- SENGAJA nggak ada filter diskon/sendability di sini — itu tugas query kandidat.
-- Jangan panggil ini pakai id dari sumber lain.
-- Kolom + urutannya harus sama persis dengan discounted_wishlist_items.sql: dua-duanya
-- di-scan discountedWishlistRows.
SELECT
  CAST(i.item_id AS CHAR)                                          AS id,
  i.name,
  COALESCE(i.character_name, '')                                   AS character_name,
  CONCAT('https://kyou.id/items/', i.item_id, '/')                AS url,
  COALESCE(CONCAT('https://kyoucdn.id/', img.path, '.webp'), '')  AS image_url,
  ip.price                                                         AS original_price,
  i.discount_price,
  i.discount_name,
  i.discount_end_date,
  i.status,
  m.name                                                           AS manufacturer,
  s.name                                                           AS series_name
FROM items         i
JOIN item_products ip ON ip.item_id        = i.item_id
LEFT JOIN manufactures m ON m.manufacture_id = ip.manufacture_id
LEFT JOIN series       s ON s.series_id      = ip.series_id
LEFT JOIN images     img ON img.image_id     = (
  SELECT image_id FROM images
  WHERE item_id = i.item_id
  ORDER BY sequence ASC, image_id ASC
  LIMIT 1
)
WHERE i.item_id IN (/*IDS*/)
