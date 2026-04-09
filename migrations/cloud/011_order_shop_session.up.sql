-- Add shop_session_id to orders for session ownership validation
ALTER TABLE orders ADD COLUMN shop_session_id TEXT;
CREATE INDEX idx_orders_shop_session_id ON orders(shop_session_id) WHERE shop_session_id IS NOT NULL;
