-- 公益联动系统数据库表
-- 运行方式: psql -U postgres -d food_rescue -f seed_charity.sql

-- 公益捐赠记录表
CREATE TABLE IF NOT EXISTS donations (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id),
    order_id INT REFERENCES orders(id),
    amount DECIMAL(10,2) NOT NULL DEFAULT 1.00,
    food_bank_id INT, -- 关联的食物银行
    message VARCHAR(200), -- 用户留言
    status VARCHAR(20) DEFAULT 'completed', -- completed, pending
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_donations_user ON donations(user_id);
CREATE INDEX IF NOT EXISTS idx_donations_time ON donations(created_at DESC);

-- 食物银行表
CREATE TABLE IF NOT EXISTS food_banks (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    logo VARCHAR(255),
    address VARCHAR(255),
    contact_phone VARCHAR(20),
    total_received DECIMAL(12,2) DEFAULT 0, -- 累计收到捐赠
    people_helped INT DEFAULT 0, -- 帮助人数
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 公益统计表（平台级别）
CREATE TABLE IF NOT EXISTS charity_stats (
    id SERIAL PRIMARY KEY,
    stat_date DATE NOT NULL UNIQUE,
    total_donations DECIMAL(12,2) DEFAULT 0,
    donation_count INT DEFAULT 0,
    donor_count INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 在users表添加公益相关字段（如果不存在）
ALTER TABLE users ADD COLUMN IF NOT EXISTS total_donations DECIMAL(10,2) DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS donation_count INT DEFAULT 0;

-- 插入测试食物银行数据
INSERT INTO food_banks (name, description, logo, address, contact_phone, total_received, people_helped)
VALUES 
    ('城市食物银行', '致力于减少食物浪费，帮助有需要的家庭获得营养食物', '', '深圳市福田区益田路', '0755-12345678', 15680.00, 3420),
    ('爱心餐桌公益', '为社区独居老人和困难家庭提供免费餐食', '', '深圳市南山区科技园', '0755-87654321', 8920.00, 1850),
    ('校园午餐计划', '帮助贫困地区学生获得营养午餐', '', '深圳市罗湖区东门', '0755-11112222', 23500.00, 5200)
ON CONFLICT DO NOTHING;

-- 查看创建的数据
SELECT 'donations' as table_name, COUNT(*) as count FROM donations;
SELECT 'food_banks' as table_name, COUNT(*) as count FROM food_banks;
