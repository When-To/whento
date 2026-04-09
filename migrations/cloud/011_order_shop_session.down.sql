DROP INDEX IF EXISTS idx_orders_shop_session_id;
ALTER TABLE orders DROP COLUMN IF EXISTS shop_session_id;
