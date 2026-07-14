-- POOL buat fill grid discounted-wishlist: SEMUA item diskon yang beneran bisa dibeli,
-- lengkap sama series_id dan kolom ranking-nya. Ditarik SEKALI per run, bukan per user.
--
-- Kenapa: kandidat fill itu sama buat semua user — bedanya cuma (a) series mana yang
-- si user demen dan (b) item mana yang udah dia wishlist. Dua-duanya bisa disaring di
-- memory. Versi lama nembak query ini sekali per user; di run 13 Jul 2026 itu 32.771
-- user, jadi ~32.771 query buat ngitung ulang jawaban yang sama terus-terusan.
--
-- Subquery gambar per-baris tetep ada DI SINI, dan itu nggak apa-apa: dia jalan sekali
-- per item di katalog diskon (ratusan/ribuan), bukan per user × kandidat.
--
-- Filternya HARUS sama persis dengan discounted_wishlist_items.sql — ini yang nentuin
-- "item ini layak dikirim". Beda dari filter series di discounted_wishlist_fill_series.sql,
-- yang sengaja lebih longgar; baca komentar di file itu.
--
-- Urutan di sini cuma biar deterministik; ranking sebenernya per user diulang di Go
-- (item bisa nyangkut di lebih dari satu series si user).
-- Tanpa parameter.
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
  s.name                                                           AS series_name,
  ip.series_id                                                     AS series_id,
  COALESCE(i.view_count, 0)                                        AS view_count,
  COALESCE(i.updated_at, i.created_at, CURRENT_TIMESTAMP)          AS updated_at
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
WHERE i.status        = 'ready'
  AND i.stock         > 0
  AND i.is_available  = 1
  AND COALESCE(i.isAdult, 0) = 0
  AND i.discount_name IS NOT NULL AND i.discount_name != ''
  AND i.discount_end_date >= CURRENT_DATE
  AND i.discount_price > 0
  AND i.discount_price < ip.price
  AND ip.series_id IS NOT NULL
ORDER BY COALESCE(i.view_count, 0) DESC, i.updated_at DESC
