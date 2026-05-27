WITH item_scores AS (
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
    ip.po_release_date,
    SUM(a.weight) AS item_weight,
    MAX(a.created_at) AS last_action_at
  FROM user_item_actions a
  JOIN items i ON i.item_id = a.item_id
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
  WHERE a.user_id = ?
    AND ip.series_id IS NOT NULL
    AND i.is_available = 1
    AND i.stock > 0
    AND COALESCE(i.isAdult, 0) = 0
    AND (ip.po_deadline IS NULL OR ip.po_deadline >= CURRENT_DATE)
  GROUP BY i.item_id, i.name, i.character_name, ip.series_id, img.path, ip.price, i.status, m.name, s.name, ip.po_deadline, ip.po_release_date
),
series_scores AS (
  SELECT
    item_scores.*,
    SUM(item_weight) OVER (PARTITION BY series_id) AS series_weight,
    MAX(last_action_at) OVER (PARTITION BY series_id) AS series_last_action_at
  FROM item_scores
),
ranked AS (
  SELECT
    series_scores.*,
    DENSE_RANK() OVER (ORDER BY series_weight DESC, series_last_action_at DESC, series_id DESC) AS series_rank,
    ROW_NUMBER() OVER (PARTITION BY series_id ORDER BY item_weight DESC, last_action_at DESC, item_id DESC) AS item_rank
  FROM series_scores
)
SELECT
  item_id,
  name,
  kind,
  series_id,
  image_url,
  price,
  status,
  manufacturer,
  series_name,
  po_deadline,
  po_release_date
FROM ranked
WHERE series_rank <= 3
  AND item_rank = 1
ORDER BY series_rank ASC, item_rank ASC
LIMIT 3;
