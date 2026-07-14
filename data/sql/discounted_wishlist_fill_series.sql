-- Series yang "disukai" tiap user, buat fill grid discounted-wishlist: series dari
-- item-item di wishlist dia yang lagi diskon. Ditarik SEKALI per run, bukan per user.
--
-- ⚠️ FILTERNYA SENGAJA LEBIH LONGGAR dari discounted_wishlist_items.sql, dan ini
-- MENIRU PERSIS perilaku query lama (discounted_wishlist_fill.sql). Di sini cuma dicek
-- `discount_name` + `discount_end_date`. TIDAK dicek stock / status ready / is_available
-- / kewajaran harga diskon.
--
-- Efeknya: item wishlist yang lagi diskon TAPI stoknya habis nggak muncul di email
-- (disaring discounted_wishlist_items.sql), tapi SERIES-nya tetep kepake buat nebak
-- selera user. Contoh: user wishlist Miku (Vocaloid, stok ada) + Rem (Re:Zero, stok
-- habis) → email nampilin Miku doang, tapi rekomendasinya tetep dari Vocaloid DAN
-- Re:Zero.
--
-- Entah itu disengaja (stok habis nggak ngubah selera) atau kelupaan waktu nulis —
-- belum ada yang tau. JANGAN diketatin di sini: itu keputusan produk, dan kalau
-- dibarengin sama perubahan performa, nggak ada yang bisa bedain penyebab email-nya
-- berubah. Kalau mau diubah, PR sendiri.
-- Tanpa parameter.
SELECT DISTINCT
  CAST(w.user_id AS CHAR) AS user_id,
  ip.series_id            AS series_id
FROM wishlists     w
JOIN items         i  ON i.item_id  = w.item_id
JOIN item_products ip ON ip.item_id = i.item_id
WHERE i.discount_name IS NOT NULL AND i.discount_name != ''
  AND i.discount_end_date >= CURRENT_DATE
  AND ip.series_id IS NOT NULL
