-- 挑战赛系统数据库表
-- 运行方式: psql -U postgres -d food_rescue -f seed_challenge.sql

-- 用户表添加挑战相关字段
ALTER TABLE users ADD COLUMN IF NOT EXISTS checkin_streak INT DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_checkin_date DATE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS total_checkins INT DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS challenge_points INT DEFAULT 0;

-- 用户任务表
CREATE TABLE IF NOT EXISTS user_tasks (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id),
    task_type VARCHAR(20) NOT NULL DEFAULT 'daily',
    title VARCHAR(100) NOT NULL,
    goal_type VARCHAR(30) NOT NULL,
    goal_value DECIMAL(10,2) NOT NULL,
    current_value DECIMAL(10,2) DEFAULT 0,
    reward_points INT DEFAULT 0,
    reward_desc VARCHAR(50),
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_user_tasks_user ON user_tasks(user_id);
CREATE INDEX IF NOT EXISTS idx_user_tasks_date ON user_tasks(created_at);

-- 紧急清仓事件表
CREATE TABLE IF NOT EXISTS urgent_events (
    id SERIAL PRIMARY KEY,
    merchant_id INT NOT NULL REFERENCES merchants(id),
    product_id INT REFERENCES products(id),
    title VARCHAR(100) NOT NULL,
    description TEXT,
    extra_discount DECIMAL(5,2) DEFAULT 10,
    total_slots INT DEFAULT 10,
    remaining_slots INT DEFAULT 10,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_urgent_events_status ON urgent_events(status);
CREATE INDEX IF NOT EXISTS idx_urgent_events_time ON urgent_events(end_time);

-- 事件参与者表
CREATE TABLE IF NOT EXISTS event_participants (
    id SERIAL PRIMARY KEY,
    event_id INT NOT NULL REFERENCES urgent_events(id),
    user_id INT NOT NULL REFERENCES users(id),
    joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    order_id INT REFERENCES orders(id),
    UNIQUE(event_id, user_id)
);

-- 清除旧的测试事件
DELETE FROM event_participants;
DELETE FROM urgent_events;

-- 插入测试紧急事件（使用动态时间，确保事件有效）
INSERT INTO urgent_events (merchant_id, product_id, title, description, extra_discount, total_slots, remaining_slots, start_time, end_time)
SELECT 
    m.id,
    p.id,
    '限时抢购: ' || p.name || '（小份）',
    '商家临时清仓，前10名下单额外享受折上折！',
    20,
    5,
    3,
    NOW(),
    NOW() + INTERVAL '10 hours'
FROM merchants m
JOIN products p ON p.merchant_id = m.id
WHERE m.status = 'active' AND p.status = 'active'
ORDER BY p.id
LIMIT 1;

INSERT INTO urgent_events (merchant_id, product_id, title, description, extra_discount, total_slots, remaining_slots, start_time, end_time)
SELECT 
    m.id,
    p.id,
    '紧急清仓: ' || p.name,
    '仅剩少量库存，手慢无！',
    15,
    10,
    8,
    NOW(),
    NOW() + INTERVAL '10 hours'
FROM merchants m
JOIN products p ON p.merchant_id = m.id
WHERE m.status = 'active' AND p.status = 'active'
ORDER BY p.id
OFFSET 1 LIMIT 1;

-- 给测试用户添加一些打卡数据
UPDATE users SET 
    checkin_streak = 1,
    total_checkins = 1,
    challenge_points = 5
WHERE status = 'active' AND checkin_streak = 0;

-- 查看创建的数据
SELECT 'urgent_events' as table_name, COUNT(*) as count FROM urgent_events;
SELECT id, title, extra_discount, remaining_slots, total_slots, 
       start_time, end_time, 
       EXTRACT(EPOCH FROM (end_time - NOW())) as remaining_seconds
FROM urgent_events WHERE status = 'active';
