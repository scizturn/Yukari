SELECT EXISTS (
  SELECT 1
  FROM orders o
  WHERE o.user_id = ?
    AND o.created_at >= ?
    AND o.created_at < ?
    AND COALESCE(o.status, '') NOT IN ('cancelled', 'canceled')
) AS converted;
