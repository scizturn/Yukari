SELECT
  u.user_id,
  u.name,
  u.email,
  u.birthdate,
  TRUE AS is_active
FROM users u
JOIN (
  SELECT c.user_id, MAX(ci.created_at) AS last_cart_at
  FROM carts c
  JOIN cart_items ci ON ci.cart_id = c.cart_id
  GROUP BY c.user_id
) ca ON ca.user_id = u.user_id
LEFT JOIN (
  SELECT user_id, MAX(created_at) AS last_order_at
  FROM orders
  GROUP BY user_id
) oa ON oa.user_id = u.user_id
WHERE u.email IS NOT NULL
  AND u.email <> ''
  AND EXISTS (
    SELECT 1
    FROM carts c2
    JOIN cart_items ci2 ON ci2.cart_id = c2.cart_id
    JOIN items i ON i.item_id = ci2.item_id
    WHERE c2.user_id = u.user_id
      AND i.is_available = 1
      AND i.stock > 0
      AND COALESCE(i.isAdult, 0) = 0
  )
  AND DATEDIFF(?, GREATEST(ca.last_cart_at, COALESCE(oa.last_order_at, '2000-01-01'))) = 14
  AND NOT EXISTS (
    SELECT 1
    FROM email_delivery_logs edl
    WHERE edl.user_id = u.user_id
      AND edl.feature = 'leftover_cart'
      AND edl.status IN ('sent', 'queued', 'sending')
      AND edl.created_at >= GREATEST(ca.last_cart_at, COALESCE(oa.last_order_at, '2000-01-01'))
  );
