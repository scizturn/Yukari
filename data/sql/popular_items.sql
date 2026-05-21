SELECT
  i.item_id,
  COALESCE(i.character_name, i.name) AS name,
  CASE WHEN i.character_name IS NULL OR i.character_name = '' THEN 'series' ELSE 'character' END AS kind,
  CAST(i.item_id AS CHAR) AS series_id
FROM items i
WHERE i.is_available = 1
ORDER BY i.view_count DESC, i.updated_at DESC
LIMIT 4;
