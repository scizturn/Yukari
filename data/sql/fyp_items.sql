SELECT
  i.item_id,
  COALESCE(i.character_name, i.name) AS name,
  CASE WHEN i.character_name IS NULL OR i.character_name = '' THEN 'series' ELSE 'character' END AS kind,
  CAST(i.item_id AS CHAR) AS series_id
FROM user_item_actions a
JOIN items i ON i.item_id = a.item_id
WHERE a.user_id = ?
  AND i.is_available = 1
GROUP BY i.item_id, i.name, i.character_name
ORDER BY SUM(a.weight) DESC, MAX(a.created_at) DESC
LIMIT 4;
