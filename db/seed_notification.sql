-- 通知系统数据库表
-- 运行方式: psql -U postgres -d food_rescue -f seed_notification.sql

-- 用户通知表
CREATE TABLE IF NOT EXISTS notifications (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id),
    type VARCHAR(30) NOT NULL, -- price_drop, flash_sale, pickup_remind, order_status, donation, system
    title VARCHAR(100) NOT NULL,
    content VARCHAR(500) NOT NULL,
    related_id INT, -- 关联的商品/订单ID
    related_type VARCHAR(20), -- product, order, donation
    is_read BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, is_read);
CREATE INDEX IF NOT EXISTS idx_notifications_time ON notifications(created_at DESC);

-- 用户通知设置表
CREATE TABLE IF NOT EXISTS notification_settings (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id) UNIQUE,
    price_drop_enabled BOOLEAN DEFAULT true,      -- 降价提醒
    flash_sale_enabled BOOLEAN DEFAULT true,      -- 闪购提醒
    pickup_remind_enabled BOOLEAN DEFAULT true,   -- 取货提醒
    order_status_enabled BOOLEAN DEFAULT true,    -- 订单状态
    donation_enabled BOOLEAN DEFAULT true,        -- 公益通知
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 商品关注表（用于降价提醒）
CREATE TABLE IF NOT EXISTS product_watches (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id),
    product_id INT NOT NULL REFERENCES products(id),
    target_price DECIMAL(10,2), -- 目标价格，低于此价格时提醒
    notified_price DECIMAL(10,2), -- 上次提醒时的价格，避免重复提醒
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_product_watches_product ON product_watches(product_id);

-- 闪购活动表
CREATE TABLE IF NOT EXISTS flash_sales (
    id SERIAL PRIMARY KEY,
    title VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    status VARCHAR(20) DEFAULT 'pending', -- pending, active, ended
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 闪购商品关联表
CREATE TABLE IF NOT EXISTS flash_sale_products (
    id SERIAL PRIMARY KEY,
    flash_sale_id INT NOT NULL REFERENCES flash_sales(id),
    product_id INT NOT NULL REFERENCES products(id),
    flash_price DECIMAL(10,2) NOT NULL,
    flash_stock INT NOT NULL,
    sold_count INT DEFAULT 0,
    UNIQUE(flash_sale_id, product_id)
);

-- 插入测试数据

-- 创建今日闪购活动
INSERT INTO flash_sales (title, description, start_time, end_time, status)
VALUES (
    '今日限时闪购',
    '每晚8点，超低价清仓！',
    CURRENT_DATE + INTERVAL '20 hours',
    CURRENT_DATE + INTERVAL '21 hours',
    'pending'
) ON CONFLICT DO NOTHING;

-- 查看创建的数据
SELECT 'notifications' as table_name, COUNT(*) as count FROM notifications;
SELECT 'flash_sales' as table_name, COUNT(*) as count FROM flash_sales;
