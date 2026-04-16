-- 时间降价商品测试数据
-- 运行方式: psql -U postgres -d food_rescue -f seed_time_based_products.sql

-- 确保有测试商家
INSERT INTO merchants (phone, password, shop_name, shop_logo, description, is_verified, province, city, district, address, business_hours, status)
VALUES 
  ('13800001001', '123456', '甜蜜时光烘焙坊', '', '专注手工烘焙，每日新鲜出炉', true, '广东省', '深圳市', '南山区', '科技园南路88号', '08:00-22:00', 'active'),
  ('13800001002', '123456', '鲜果物语', '', '新鲜水果，产地直供', true, '广东省', '深圳市', '福田区', '华强北路168号', '09:00-21:00', 'active'),
  ('13800001003', '123456', '优鲜超市', '', '社区便利店，新鲜每一天', true, '广东省', '深圳市', '南山区', '后海大道66号', '07:00-23:00', 'active')
ON CONFLICT (phone) DO NOTHING;

-- 确保有分类
INSERT INTO categories (name, icon, is_active) VALUES
  ('面包糕点', 'bread', true),
  ('乳制品', 'milk', true),
  ('水果', 'apple', true),
  ('饮料', 'coffee', true),
  ('零食', 'cookie', true),
  ('熟食', 'meat', true)
ON CONFLICT DO NOTHING;

-- 商品1: 全麦面包 - 2天后过期
INSERT INTO products (
  merchant_id, category_id, name, description, images,
  original_price, production_date, expiry_date, storage_condition,
  stock, sold_count, price_rule_type, price_rules, status
) 
SELECT 
  m.id, c.id,
  '全麦吐司面包 450g',
  '精选优质全麦粉，低糖低脂，健康早餐首选。富含膳食纤维，口感松软有嚼劲。',
  '{}',
  18.00,
  CURRENT_DATE - INTERVAL '5 days',
  CURRENT_DATE + INTERVAL '2 days',
  '常温避光保存',
  15, 8,
  'time_based',
  '[{"discount": 80, "hours_left": 72}, {"discount": 60, "hours_left": 48}, {"discount": 40, "hours_left": 24}, {"discount": 20, "hours_left": 6}]'::jsonb,
  'active'
FROM merchants m, categories c
WHERE m.phone = '13800001001' AND c.name = '面包糕点'
LIMIT 1;

-- 商品2: 牛角包 - 1天后过期
INSERT INTO products (
  merchant_id, category_id, name, description, images,
  original_price, production_date, expiry_date, storage_condition,
  stock, sold_count, price_rule_type, price_rules, status
)
SELECT 
  m.id, c.id,
  '法式黄油牛角包 4个装',
  '采用进口黄油，层层酥脆，奶香浓郁。每日新鲜出炉，外酥内软。',
  '{}',
  22.00,
  CURRENT_DATE - INTERVAL '2 days',
  CURRENT_DATE + INTERVAL '1 day',
  '常温保存，避免挤压',
  5, 12,
  'time_based',
  '[{"discount": 70, "hours_left": 48}, {"discount": 50, "hours_left": 24}, {"discount": 30, "hours_left": 12}, {"discount": 15, "hours_left": 3}]'::jsonb,
  'active'
FROM merchants m, categories c
WHERE m.phone = '13800001001' AND c.name = '面包糕点'
LIMIT 1;

-- 商品3: 酸奶 - 3天后过期
INSERT INTO products (
  merchant_id, category_id, name, description, images,
  original_price, production_date, expiry_date, storage_condition,
  stock, sold_count, price_rule_type, price_rules, status
)
SELECT 
  m.id, c.id,
  '原味酸奶 200g*6杯',
  '新鲜牛奶发酵，无添加防腐剂，富含活性益生菌。口感醇厚，酸甜适中。',
  '{}',
  28.00,
  CURRENT_DATE - INTERVAL '10 days',
  CURRENT_DATE + INTERVAL '3 days',
  '冷藏保存 2-6°C',
  20, 5,
  'time_based',
  '[{"discount": 85, "hours_left": 96}, {"discount": 70, "hours_left": 72}, {"discount": 50, "hours_left": 48}, {"discount": 30, "hours_left": 24}]'::jsonb,
  'active'
FROM merchants m, categories c
WHERE m.phone = '13800001003' AND c.name = '乳制品'
LIMIT 1;

-- 商品4: 草莓 - 今天过期，最低价
INSERT INTO products (
  merchant_id, category_id, name, description, images,
  original_price, production_date, expiry_date, storage_condition,
  stock, sold_count, price_rule_type, price_rules, status
)
SELECT 
  m.id, c.id,
  '丹东99草莓 300g',
  '当季新鲜草莓，个大饱满，香甜多汁。产地直供，品质保证。',
  '{}',
  35.00,
  CURRENT_DATE - INTERVAL '3 days',
  CURRENT_DATE,
  '冷藏保存，轻拿轻放',
  3, 15,
  'time_based',
  '[{"discount": 80, "hours_left": 48}, {"discount": 60, "hours_left": 24}, {"discount": 40, "hours_left": 12}, {"discount": 20, "hours_left": 6}]'::jsonb,
  'active'
FROM merchants m, categories c
WHERE m.phone = '13800001002' AND c.name = '水果'
LIMIT 1;

-- 商品5: 鲜榨果汁 - 明天过期
INSERT INTO products (
  merchant_id, category_id, name, description, images,
  original_price, production_date, expiry_date, storage_condition,
  stock, sold_count, price_rule_type, price_rules, status
)
SELECT 
  m.id, c.id,
  '鲜榨橙汁 500ml',
  '100%鲜榨，无添加糖和防腐剂。富含维生素C，清爽解渴。',
  '{}',
  15.00,
  CURRENT_DATE - INTERVAL '1 day',
  CURRENT_DATE + INTERVAL '1 day',
  '冷藏保存 0-4°C',
  8, 6,
  'time_based',
  '[{"discount": 80, "hours_left": 36}, {"discount": 60, "hours_left": 24}, {"discount": 40, "hours_left": 12}, {"discount": 25, "hours_left": 4}]'::jsonb,
  'active'
FROM merchants m, categories c
WHERE m.phone = '13800001002' AND c.name = '饮料'
LIMIT 1;

-- 商品6: 薯片 - 5天后过期
INSERT INTO products (
  merchant_id, category_id, name, description, images,
  original_price, production_date, expiry_date, storage_condition,
  stock, sold_count, price_rule_type, price_rules, status
)
SELECT 
  m.id, c.id,
  '原味薯片 大包装 200g',
  '精选优质土豆，薄脆可口，原味经典。派对聚会必备零食。',
  '{}',
  12.00,
  CURRENT_DATE - INTERVAL '25 days',
  CURRENT_DATE + INTERVAL '5 days',
  '常温避光保存',
  30, 3,
  'time_based',
  '[{"discount": 90, "hours_left": 168}, {"discount": 75, "hours_left": 120}, {"discount": 60, "hours_left": 72}, {"discount": 40, "hours_left": 24}]'::jsonb,
  'active'
FROM merchants m, categories c
WHERE m.phone = '13800001003' AND c.name = '零食'
LIMIT 1;

-- 商品7: 蛋糕卷 - 后天过期
INSERT INTO products (
  merchant_id, category_id, name, description, images,
  original_price, production_date, expiry_date, storage_condition,
  stock, sold_count, price_rule_type, price_rules, status
)
SELECT 
  m.id, c.id,
  '瑞士卷蛋糕 原味 200g',
  '松软蛋糕卷配奶油夹心，入口即化。下午茶甜点首选。',
  '{}',
  25.00,
  CURRENT_DATE - INTERVAL '3 days',
  CURRENT_DATE + INTERVAL '2 days',
  '冷藏保存 2-8°C',
  10, 4,
  'time_based',
  '[{"discount": 85, "hours_left": 72}, {"discount": 65, "hours_left": 48}, {"discount": 45, "hours_left": 24}, {"discount": 25, "hours_left": 8}]'::jsonb,
  'active'
FROM merchants m, categories c
WHERE m.phone = '13800001001' AND c.name = '面包糕点'
LIMIT 1;

-- 查看创建的时间降价商品
SELECT 
  p.id,
  p.name,
  p.original_price,
  p.price_rule_type,
  p.expiry_date,
  round(EXTRACT(EPOCH FROM (p.expiry_date + TIME '23:59:59' - NOW())) / 3600) as hours_left,
  m.shop_name
FROM products p
JOIN merchants m ON p.merchant_id = m.id
WHERE p.price_rule_type = 'time_based' AND p.status = 'active'
ORDER BY p.expiry_date;
