-- 社交拼救系统数据库表
-- 运行方式: psql -U postgres -d food_rescue -f seed_social.sql

-- 拼救团表
CREATE TABLE IF NOT EXISTS group_buys (
    id SERIAL PRIMARY KEY,
    creator_id INT NOT NULL REFERENCES users(id),
    product_id INT NOT NULL REFERENCES products(id),
    merchant_id INT NOT NULL REFERENCES merchants(id),
    target_count INT NOT NULL DEFAULT 2,
    current_count INT DEFAULT 1,
    original_price DECIMAL(10,2) NOT NULL,
    group_price DECIMAL(10,2) NOT NULL,
    invite_code VARCHAR(10) UNIQUE NOT NULL,
    status VARCHAR(20) DEFAULT 'pending',
    expire_time TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_group_buys_status ON group_buys(status);
CREATE INDEX IF NOT EXISTS idx_group_buys_invite ON group_buys(invite_code);

-- 拼团成员表
CREATE TABLE IF NOT EXISTS group_buy_members (
    id SERIAL PRIMARY KEY,
    group_id INT NOT NULL REFERENCES group_buys(id),
    user_id INT NOT NULL REFERENCES users(id),
    is_creator BOOLEAN DEFAULT false,
    joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(group_id, user_id)
);

-- 小队表
CREATE TABLE IF NOT EXISTS teams (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    captain_id INT NOT NULL REFERENCES users(id),
    invite_code VARCHAR(10) UNIQUE NOT NULL,
    member_count INT DEFAULT 1,
    total_saved DECIMAL(10,2) DEFAULT 0,
    weekly_saved DECIMAL(10,2) DEFAULT 0,
    weekly_rank INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_teams_invite ON teams(invite_code);

-- 小队成员表
CREATE TABLE IF NOT EXISTS team_members (
    id SERIAL PRIMARY KEY,
    team_id INT NOT NULL REFERENCES teams(id),
    user_id INT NOT NULL REFERENCES users(id),
    is_captain BOOLEAN DEFAULT false,
    contribution DECIMAL(10,2) DEFAULT 0,
    joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id)
);

-- 好友动态表
CREATE TABLE IF NOT EXISTS friend_activities (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id),
    activity_type VARCHAR(30) NOT NULL,
    product_id INT REFERENCES products(id),
    discount DECIMAL(5,2),
    saved_money DECIMAL(10,2),
    message VARCHAR(200),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_activities_user ON friend_activities(user_id);
CREATE INDEX IF NOT EXISTS idx_activities_time ON friend_activities(created_at DESC);

-- 插入测试数据

-- 创建测试小队（使用users表，不需要role字段）
INSERT INTO teams (name, captain_id, invite_code, member_count, total_saved, weekly_saved)
SELECT '食物拯救小分队', id, 'T00001', 1, 128.5, 45.0
FROM users WHERE status = 'active' LIMIT 1
ON CONFLICT DO NOTHING;

-- 队长加入小队
INSERT INTO team_members (team_id, user_id, is_captain, contribution)
SELECT t.id, t.captain_id, true, 128.5
FROM teams t WHERE t.invite_code = 'T00001'
ON CONFLICT DO NOTHING;

-- 创建测试拼团
INSERT INTO group_buys (creator_id, product_id, merchant_id, target_count, current_count, original_price, group_price, invite_code, expire_time)
SELECT 
    u.id,
    p.id,
    p.merchant_id,
    3,
    1,
    p.original_price,
    p.original_price * 0.9,
    '123456',
    NOW() + INTERVAL '12 hours'
FROM users u, products p
WHERE u.status = 'active' AND p.status = 'active'
LIMIT 1
ON CONFLICT DO NOTHING;

-- 创建者加入拼团
INSERT INTO group_buy_members (group_id, user_id, is_creator)
SELECT g.id, g.creator_id, true
FROM group_buys g WHERE g.invite_code = '123456'
ON CONFLICT DO NOTHING;

-- 插入测试好友动态
INSERT INTO friend_activities (user_id, activity_type, product_id, discount, saved_money, message)
SELECT u.id, 'purchase', p.id, 0.3, 15.0, '抢到了3折草莓'
FROM users u, products p
WHERE u.status = 'active' AND p.status = 'active'
LIMIT 1;

INSERT INTO friend_activities (user_id, activity_type, product_id, discount, saved_money, message)
SELECT u.id, 'purchase', p.id, 0.5, 8.5, '抢到了5折面包'
FROM users u, products p
WHERE u.status = 'active' AND p.status = 'active'
OFFSET 1 LIMIT 1;

-- 查看创建的数据
SELECT 'teams' as table_name, COUNT(*) as count FROM teams;
SELECT 'group_buys' as table_name, COUNT(*) as count FROM group_buys;
SELECT 'friend_activities' as table_name, COUNT(*) as count FROM friend_activities;
