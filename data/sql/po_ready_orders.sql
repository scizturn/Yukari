-- Eligible orders for the "PO ready → pelunasan" campaign.
-- One row per order: a DP-paid order that still owes a balance and has at least
-- one PO item that has arrived (item.status='ready') while the order-item is
-- still 'po' (not yet fulfilled/settled). State-based, so past-ready orders are
-- naturally caught on the first run; per-order dedup keeps it one-shot.
SELECT
  CAST(o.order_id AS CHAR)          AS order_id,
  CAST(o.user_id AS CHAR)           AS user_id,
  u.name,
  u.email,
  u.email_verified_at IS NOT NULL   AS is_active,
  o.remaining,
  COALESCE(SUM(oi.down_payment), 0) AS down_payment,
  COALESCE(o.eta, '')               AS eta
FROM orders o
JOIN order_items oi ON oi.order_id = o.order_id
JOIN items i        ON i.item_id   = oi.item_id
JOIN users u        ON u.user_id   = o.user_id
WHERE o.status        = 'dp paid'
  AND o.remaining     > 0
  AND oi.status       = 'po'
  AND i.status        = 'ready'
  AND oi.cancelled_at IS NULL
  AND oi.refund_status = 'none'
  AND u.email_verified_at IS NOT NULL
  AND u.email IS NOT NULL AND u.email <> ''
  AND NOT EXISTS (
    SELECT 1
    FROM email_delivery_logs edl
    WHERE edl.reference_id = CAST(o.order_id AS CHAR)
      AND edl.feature      = 'po_ready'
      AND edl.status       IN ('queued', 'sending', 'sent')
  )
GROUP BY o.order_id, o.user_id, u.name, u.email, u.email_verified_at, o.remaining, o.eta
ORDER BY o.remaining DESC
