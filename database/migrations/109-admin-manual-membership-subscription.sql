-- Manual administrative membership grants must appear in the canonical
-- subscription projection without fabricating a paid order.

BEGIN;

ALTER TABLE xz_billing_subscriptions
  ALTER COLUMN source_order_id DROP NOT NULL;

COMMENT ON COLUMN xz_billing_subscriptions.source_order_id IS
  'Payment order id for checkout-originated subscriptions; NULL for non-payment administrative grants. source_order_no remains the provenance key.';

COMMIT;
