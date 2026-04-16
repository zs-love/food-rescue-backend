-- 信任机制测试数据：溯源信息 + 试吃官计划
-- 包含完整的依赖数据（用户、商家、商品）

-- =====================================================
-- 0. 基础数据：用户、商家、商品
-- =====================================================

-- 创建测试用户
INSERT INTO users (id, phone, password, nickname, avatar, total_saved_weight, total_saved_money, total_orders, rank_level, status)
VALUES 
(1, '13800000001', '$2a$10$abcdefghijklmnopqrstuv', '张小明', NULL, 25.5, 180.00, 15, 'expert', 'active'),
(2, '13800000002', '$2a$10$abcdefghijklmnopqrstuv', '李小红', NULL, 12.0, 85.00, 8, 'expert', 'active'),
(3, '13800000003', '$2a$10$abcdefghijklmnopqrstuv', '王小华', NULL, 5.0, 35.00, 3, 'newcomer', 'active')
ON CONFLICT (id) DO UPDATE SET
    nickname = EXCLUDED.nickname,
    total_saved_weight = EXCLUDED.total_saved_weight,
    total_saved_money = EXCLUDED.total_saved_money,
    total_orders = EXCLUDED.total_orders;

-- 重置用户ID序列
SELECT setval('users_id_seq', (SELECT MAX(id) FROM users));

-- 创建测试商家
INSERT INTO merchants (id, phone, password, shop_name, shop_logo, description, 
    province, city, district, address, latitude, longitude,
    business_hours, is_verified, total_sales, total_saved_loss, rating, status)
VALUES 
(1, '13900000001', '$2a$10$abcdefghijklmnopqrstuv', '鲜味烘焙坊', NULL, 
 '专注临期烘焙食品，每日新鲜出炉，品质保证',
 '浙江省', '杭州市', '西湖区', '文三路123号', 30.2741, 120.1551,
 '08:00-21:00', true, 12580.00, 8650.00, 4.8, 'active')
ON CONFLICT (id) DO UPDATE SET
    shop_name = EXCLUDED.shop_name,
    description = EXCLUDED.description,
    is_verified = EXCLUDED.is_verified,
    status = EXCLUDED.status;

-- 重置商家ID序列
SELECT setval('merchants_id_seq', (SELECT MAX(id) FROM merchants));

-- 创建测试商品
INSERT INTO products (id, merchant_id, category_id, name, description, original_price, 
    production_date, expiry_date, storage_condition, stock, sold_count, 
    price_rule_type, status)
VALUES 
-- 商品1: 全麦面包
(1, 1, 1, '全麦吐司面包', '优质全麦粉制作，低糖低脂，营养健康', 15.00,
 CURRENT_DATE - INTERVAL '2 days', CURRENT_DATE + INTERVAL '3 days', '常温避光保存',
 50, 120, 'time_based', 'active'),

-- 商品2: 鲜牛奶
(2, 1, 2, '新鲜纯牛奶 1L', '绿野牧场直供，当日配送，营养丰富', 12.00,
 CURRENT_DATE - INTERVAL '1 day', CURRENT_DATE + INTERVAL '5 days', '2-6°C冷藏',
 30, 85, 'time_based', 'active'),

-- 商品3: 酸奶
(3, 1, 2, '希腊风味酸奶', '进口原料，浓郁口感，高蛋白低脂肪', 8.00,
 CURRENT_DATE - INTERVAL '3 days', CURRENT_DATE + INTERVAL '4 days', '2-8°C冷藏',
 40, 65, 'fixed', 'active'),

-- 商品4: 蛋糕
(4, 1, 1, '手工芝士蛋糕', '每日新鲜制作，入口即化，甜而不腻', 28.00,
 CURRENT_DATE, CURRENT_DATE + INTERVAL '2 days', '冷藏保存，尽快食用',
 15, 42, 'time_based', 'active'),

-- 商品5: 三明治
(5, 1, 5, '金枪鱼三明治', '新鲜食材，现做现卖，营养美味', 18.00,
 CURRENT_DATE, CURRENT_DATE + INTERVAL '1 day', '0-4°C冷藏，当日食用',
 20, 38, 'fixed', 'active')

ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    original_price = EXCLUDED.original_price,
    production_date = EXCLUDED.production_date,
    expiry_date = EXCLUDED.expiry_date,
    storage_condition = EXCLUDED.storage_condition,
    stock = EXCLUDED.stock,
    sold_count = EXCLUDED.sold_count,
    status = EXCLUDED.status;

-- 重置商品ID序列
SELECT setval('products_id_seq', (SELECT MAX(id) FROM products));

-- =====================================================
-- 1. 商品溯源信息 (product_traceability)
-- =====================================================

INSERT INTO product_traceability (
    product_id, supplier_name, supplier_license, 
    purchase_date, purchase_batch, purchase_quantity, purchase_certificate,
    inspection_date, inspection_result, inspection_certificate,
    storage_temperature, storage_humidity, storage_location
) VALUES 
-- 商品1: 全麦面包
(1, '优质面粉供应商', 'FOOD-2024-001234', 
 CURRENT_DATE - INTERVAL '3 days', 'BATCH-20260217-001', 100, '/uploads/certificates/cert_001.jpg',
 CURRENT_DATE - INTERVAL '2 days', 'passed', '/uploads/certificates/inspect_001.jpg',
 '15-25°C', '40-60%', '常温仓库A区'),

-- 商品2: 鲜牛奶
(2, '绿野牧场', 'DAIRY-2024-005678', 
 CURRENT_DATE - INTERVAL '1 day', 'BATCH-20260219-002', 50, '/uploads/certificates/cert_002.jpg',
 CURRENT_DATE - INTERVAL '1 day', 'passed', '/uploads/certificates/inspect_002.jpg',
 '2-6°C', '70-80%', '冷藏库B区'),

-- 商品3: 酸奶
(3, '优格乳业', 'DAIRY-2024-009012', 
 CURRENT_DATE - INTERVAL '2 days', 'BATCH-20260218-003', 80, '/uploads/certificates/cert_003.jpg',
 CURRENT_DATE - INTERVAL '2 days', 'passed', '/uploads/certificates/inspect_003.jpg',
 '2-8°C', '65-75%', '冷藏库B区'),

-- 商品4: 蛋糕
(4, '甜蜜烘焙坊', 'BAKERY-2024-003456', 
 CURRENT_DATE - INTERVAL '1 day', 'BATCH-20260219-004', 30, '/uploads/certificates/cert_004.jpg',
 CURRENT_DATE - INTERVAL '1 day', 'passed', '/uploads/certificates/inspect_004.jpg',
 '18-22°C', '50-60%', '常温仓库A区'),

-- 商品5: 三明治
(5, '新鲜食材配送', 'FOOD-2024-007890', 
 CURRENT_DATE, 'BATCH-20260220-005', 40, '/uploads/certificates/cert_005.jpg',
 CURRENT_DATE, 'passed', '/uploads/certificates/inspect_005.jpg',
 '0-4°C', '60-70%', '冷藏库C区')

ON CONFLICT (product_id) DO UPDATE SET
    supplier_name = EXCLUDED.supplier_name,
    supplier_license = EXCLUDED.supplier_license,
    purchase_date = EXCLUDED.purchase_date,
    purchase_batch = EXCLUDED.purchase_batch,
    purchase_quantity = EXCLUDED.purchase_quantity,
    purchase_certificate = EXCLUDED.purchase_certificate,
    inspection_date = EXCLUDED.inspection_date,
    inspection_result = EXCLUDED.inspection_result,
    inspection_certificate = EXCLUDED.inspection_certificate,
    storage_temperature = EXCLUDED.storage_temperature,
    storage_humidity = EXCLUDED.storage_humidity,
    storage_location = EXCLUDED.storage_location,
    updated_at = CURRENT_TIMESTAMP;

-- =====================================================
-- 2. 试吃官 (tasters)
-- =====================================================

INSERT INTO tasters (user_id, real_name, reason, status, level, total_tasks, completed_tasks, total_reviews, avg_rating, approved_at)
VALUES 
-- 用户1: 已通过的高级试吃官
(1, '张小明', '热爱美食，希望帮助更多人了解临期食品的真实品质', 'approved', 'senior', 15, 12, 12, 4.8, CURRENT_TIMESTAMP - INTERVAL '30 days'),

-- 用户2: 已通过的初级试吃官
(2, '李小红', '经常购买临期食品，想分享真实体验', 'approved', 'junior', 5, 3, 3, 4.5, CURRENT_TIMESTAMP - INTERVAL '10 days'),

-- 用户3: 待审核的申请者
(3, '王小华', '对食品安全很关注，想通过试吃帮助大家', 'pending', 'junior', 0, 0, 0, 5.0, NULL)

ON CONFLICT (user_id) DO UPDATE SET
    real_name = EXCLUDED.real_name,
    reason = EXCLUDED.reason,
    status = EXCLUDED.status,
    level = EXCLUDED.level,
    total_tasks = EXCLUDED.total_tasks,
    completed_tasks = EXCLUDED.completed_tasks,
    total_reviews = EXCLUDED.total_reviews,
    avg_rating = EXCLUDED.avg_rating,
    approved_at = EXCLUDED.approved_at,
    updated_at = CURRENT_TIMESTAMP;

-- =====================================================
-- 3. 试吃任务 (tasting_tasks)
-- =====================================================

-- 先删除旧数据避免冲突
DELETE FROM tasting_applications;
DELETE FROM tasting_tasks;

INSERT INTO tasting_tasks (id, product_id, merchant_id, title, description, requirements, total_slots, filled_slots, original_price, tasting_price, start_time, end_time, status)
VALUES 
-- 任务1: 免费试吃面包
(1, 1, 1, '新品全麦面包免费试吃', 
 '我们新推出的全麦面包，使用优质全麦粉制作，口感松软，营养丰富。诚邀试吃官品尝并分享真实评价。', 
 '需在3天内完成试吃并提交评价，评价需包含口感、新鲜度等方面的描述',
 10, 3, 15.00, 0.00, 
 CURRENT_TIMESTAMP - INTERVAL '2 days', CURRENT_TIMESTAMP + INTERVAL '5 days', 'active'),

-- 任务2: 超低价试吃酸奶
(2, 3, 1, '进口酸奶1元试吃', 
 '原价8元的进口希腊酸奶，现仅需1元即可试吃！口感浓郁，蛋白质含量高。', 
 '需在2天内完成试吃并提交带图评价',
 5, 2, 8.00, 1.00, 
 CURRENT_TIMESTAMP - INTERVAL '1 day', CURRENT_TIMESTAMP + INTERVAL '3 days', 'active'),

-- 任务3: 已结束的任务
(3, 2, 1, '鲜牛奶品质体验', 
 '来自绿野牧场的新鲜牛奶，当日配送，品质保证。', 
 '需在24小时内完成试吃',
 8, 8, 12.00, 0.00, 
 CURRENT_TIMESTAMP - INTERVAL '10 days', CURRENT_TIMESTAMP - INTERVAL '3 days', 'completed'),

-- 任务4: 蛋糕试吃
(4, 4, 1, '手工蛋糕免费品尝', 
 '店内招牌手工蛋糕，每日新鲜制作，临期特惠中。欢迎试吃官来体验！', 
 '需到店自取，并在当天完成评价',
 6, 1, 28.00, 0.00, 
 CURRENT_TIMESTAMP, CURRENT_TIMESTAMP + INTERVAL '7 days', 'active');

-- 重置任务ID序列
SELECT setval('tasting_tasks_id_seq', (SELECT MAX(id) FROM tasting_tasks));

-- =====================================================
-- 4. 试吃申请 (tasting_applications)
-- =====================================================

INSERT INTO tasting_applications (task_id, user_id, taster_id, status, approved_at, completed_at)
VALUES 
-- 任务1的申请
(1, 1, (SELECT id FROM tasters WHERE user_id = 1), 'completed', CURRENT_TIMESTAMP - INTERVAL '2 days', CURRENT_TIMESTAMP - INTERVAL '1 day'),
(1, 2, (SELECT id FROM tasters WHERE user_id = 2), 'approved', CURRENT_TIMESTAMP - INTERVAL '1 day', NULL),

-- 任务2的申请
(2, 1, (SELECT id FROM tasters WHERE user_id = 1), 'approved', CURRENT_TIMESTAMP - INTERVAL '1 day', NULL),
(2, 2, (SELECT id FROM tasters WHERE user_id = 2), 'completed', CURRENT_TIMESTAMP - INTERVAL '1 day', CURRENT_TIMESTAMP - INTERVAL '12 hours'),

-- 任务3的申请（已结束任务）
(3, 1, (SELECT id FROM tasters WHERE user_id = 1), 'completed', CURRENT_TIMESTAMP - INTERVAL '9 days', CURRENT_TIMESTAMP - INTERVAL '8 days'),
(3, 2, (SELECT id FROM tasters WHERE user_id = 2), 'completed', CURRENT_TIMESTAMP - INTERVAL '9 days', CURRENT_TIMESTAMP - INTERVAL '7 days'),

-- 任务4的申请
(4, 1, (SELECT id FROM tasters WHERE user_id = 1), 'pending', NULL, NULL)

ON CONFLICT (task_id, user_id) DO UPDATE SET
    status = EXCLUDED.status,
    approved_at = EXCLUDED.approved_at,
    completed_at = EXCLUDED.completed_at;

-- =====================================================
-- 5. 商品评价 (product_reviews)
-- =====================================================

-- 先删除旧评价数据
DELETE FROM product_reviews WHERE product_id IN (1, 2, 3);

INSERT INTO product_reviews (product_id, user_id, rating, content, images, is_taster_review, merchant_reply, reply_time, status)
VALUES 
-- 商品1(面包)的评价
(1, 1, 5, '面包非常新鲜，口感松软，全麦的香气很浓郁。虽然是临期商品，但品质完全没问题，性价比超高！', 
 ARRAY['/uploads/reviews/review_001.jpg'], true, 
 '感谢您的认可！我们每天都会严格把控品质，欢迎再次光临~', CURRENT_TIMESTAMP - INTERVAL '12 hours', 'visible'),

(1, 2, 4, '面包味道不错，就是感觉稍微有点干，可能是因为临近保质期的原因。总体来说还是值得购买的。', 
 NULL, true, NULL, NULL, 'visible'),

-- 商品2(牛奶)的评价
(2, 1, 5, '牛奶很新鲜，奶香味十足，完全感觉不出是临期商品。冷藏保存得很好，推荐购买！', 
 ARRAY['/uploads/reviews/review_002.jpg', '/uploads/reviews/review_003.jpg'], true, 
 '谢谢支持！我们的冷链配送全程保鲜~', CURRENT_TIMESTAMP - INTERVAL '6 days', 'visible'),

(2, 2, 5, '第一次买临期牛奶，没想到品质这么好！以后会经常来买的。', 
 NULL, true, NULL, NULL, 'visible'),

-- 商品3(酸奶)的评价
(3, 2, 4, '酸奶口感浓郁，酸甜适中。包装完好，日期也在保质期内。唯一的小遗憾是口味选择不多。', 
 ARRAY['/uploads/reviews/review_004.jpg'], true, 
 '感谢反馈！我们会尽快增加更多口味选择~', CURRENT_TIMESTAMP - INTERVAL '6 hours', 'visible'),

-- 普通用户评价（非试吃官）
(1, 3, 4, '买了两个面包，味道还可以，价格很实惠。', NULL, false, NULL, NULL, 'visible'),

(3, 3, 5, '酸奶很好喝，会回购！', NULL, false, NULL, NULL, 'visible');

-- =====================================================
-- 6. 更新统计数据
-- =====================================================

-- 更新试吃任务的已填充名额
UPDATE tasting_tasks SET filled_slots = (
    SELECT COUNT(*) FROM tasting_applications 
    WHERE tasting_applications.task_id = tasting_tasks.id 
    AND tasting_applications.status IN ('approved', 'completed')
);

-- 更新试吃官的统计数据
UPDATE tasters SET 
    total_tasks = (SELECT COUNT(*) FROM tasting_applications WHERE tasting_applications.taster_id = tasters.id),
    completed_tasks = (SELECT COUNT(*) FROM tasting_applications WHERE tasting_applications.taster_id = tasters.id AND status = 'completed'),
    total_reviews = (SELECT COUNT(*) FROM product_reviews WHERE product_reviews.user_id = tasters.user_id AND is_taster_review = true),
    avg_rating = COALESCE((SELECT AVG(rating)::DECIMAL(2,1) FROM product_reviews WHERE product_reviews.user_id = tasters.user_id AND is_taster_review = true), 5.0);

-- =====================================================
-- 完成提示
-- =====================================================
DO $$
BEGIN
    RAISE NOTICE '测试数据导入完成！';
    RAISE NOTICE '- 用户: 3 条';
    RAISE NOTICE '- 商家: 1 条';
    RAISE NOTICE '- 商品: 5 条';
    RAISE NOTICE '- 溯源信息: 5 条';
    RAISE NOTICE '- 试吃官: 3 条';
    RAISE NOTICE '- 试吃任务: 4 条';
    RAISE NOTICE '- 试吃申请: 7 条';
    RAISE NOTICE '- 商品评价: 7 条';
END $$;
