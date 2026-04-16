-- 盲盒测试数据
-- 运行方式: psql -U postgres -d food_rescue -f seed_blindbox.sql

-- 先创建/更新盲盒相关表
CREATE TABLE IF NOT EXISTS blind_box_purchases (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id),
    product_id INT NOT NULL REFERENCES products(id),
    merchant_id INT NOT NULL REFERENCES merchants(id),
    price DECIMAL(10,2) NOT NULL,
    status VARCHAR(20) DEFAULT 'unopened',
    opened_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 删除旧表重建（确保字段完整）
DROP TABLE IF EXISTS blind_box_items CASCADE;
CREATE TABLE blind_box_items (
    id SERIAL PRIMARY KEY,
    purchase_id INT NOT NULL REFERENCES blind_box_purchases(id) ON DELETE CASCADE,
    item_name VARCHAR(100) NOT NULL,
    item_value DECIMAL(10,2) NOT NULL,
    item_image VARCHAR(255),
    item_description TEXT,
    item_expiry_date DATE,
    item_storage_condition VARCHAR(50),
    item_quantity INT DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

DROP TABLE IF EXISTS blind_box_contents CASCADE;
CREATE TABLE blind_box_contents (
    id SERIAL PRIMARY KEY,
    blind_box_id INT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    source_product_id INT,
    product_name VARCHAR(100) NOT NULL,
    product_value DECIMAL(10,2) NOT NULL,
    product_image VARCHAR(255),
    product_description TEXT,
    product_expiry_date DATE,
    product_storage_condition VARCHAR(50),
    quantity INT DEFAULT 1,
    probability INT DEFAULT 100,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_blind_box_user ON blind_box_purchases(user_id);
CREATE INDEX IF NOT EXISTS idx_blind_box_status ON blind_box_purchases(status);

-- 删除旧的盲盒商品
DELETE FROM products WHERE is_blind_box = true;

-- 确保有测试商家
INSERT INTO merchants (phone, password, shop_name, shop_logo, description, is_verified, province, city, district, address, business_hours, status)
VALUES 
  ('13800001001', '123456', '甜蜜时光烘焙坊', '', '专注手工烘焙，每日新鲜出炉', true, '广东省', '深圳市', '南山区', '科技园南路88号', '08:00-22:00', 'active'),
  ('13800001002', '123456', '鲜果物语', '', '新鲜水果，产地直供', true, '广东省', '深圳市', '福田区', '华强北路168号', '09:00-21:00', 'active')
ON CONFLICT (phone) DO NOTHING;

-- 获取商家ID并创建盲盒
DO $$
DECLARE
  merchant1_id INT;
  merchant2_id INT;
  box1_id INT;
  box2_id INT;
  box3_id INT;
  expiry_3days DATE := CURRENT_DATE + INTERVAL '3 days';
  expiry_5days DATE := CURRENT_DATE + INTERVAL '5 days';
BEGIN
  SELECT id INTO merchant1_id FROM merchants WHERE phone = '13800001001';
  SELECT id INTO merchant2_id FROM merchants WHERE phone = '13800001002';

  IF merchant1_id IS NULL THEN merchant1_id := 1; END IF;
  IF merchant2_id IS NULL THEN merchant2_id := 2; END IF;

  -- 创建盲盒商品1: 烘焙惊喜袋（小份）
  INSERT INTO products (
    merchant_id, name, description, original_price, expiry_date, 
    stock, sold_count, is_blind_box, blind_box_min_value, blind_box_max_value, status
  ) VALUES (
    merchant1_id, 
    '烘焙惊喜袋（小份）', 
    '包含3-5件精选烘焙商品，可能有面包、蛋糕、饼干等惊喜',
    9.9, 
    expiry_3days,
    20, 5, true, 15, 30, 'active'
  ) RETURNING id INTO box1_id;

  -- 创建盲盒商品2: 烘焙惊喜袋（大份）
  INSERT INTO products (
    merchant_id, name, description, original_price, expiry_date,
    stock, sold_count, is_blind_box, blind_box_min_value, blind_box_max_value, status
  ) VALUES (
    merchant1_id,
    '烘焙惊喜袋（大份）',
    '包含6-10件精选烘焙商品，超值大满足',
    19.9,
    expiry_3days,
    15, 3, true, 35, 60, 'active'
  ) RETURNING id INTO box2_id;

  -- 创建盲盒商品3: 水果惊喜盒
  INSERT INTO products (
    merchant_id, name, description, original_price, expiry_date,
    stock, sold_count, is_blind_box, blind_box_min_value, blind_box_max_value, status
  ) VALUES (
    merchant2_id,
    '水果惊喜盒',
    '当季新鲜水果随机组合，营养美味',
    15.9,
    expiry_5days,
    10, 2, true, 25, 45, 'active'
  ) RETURNING id INTO box3_id;

  -- 为盲盒1添加内容（带完整信息）
  INSERT INTO blind_box_contents (blind_box_id, product_name, product_value, product_description, product_expiry_date, product_storage_condition, quantity, probability) VALUES
    (box1_id, '法式牛角包', 6.0, '外酥内软，奶香浓郁，采用进口黄油制作', expiry_3days, '常温保存，避免阳光直射', 1, 80),
    (box1_id, '巧克力麦芬', 5.0, '浓郁巧克力风味，松软可口，内含巧克力豆', expiry_3days, '常温保存，开封后尽快食用', 1, 70),
    (box1_id, '红豆面包', 4.5, '精选红豆馅，甜而不腻，手工揉制', expiry_3days, '常温保存', 1, 90),
    (box1_id, '蛋黄酥', 8.0, '咸蛋黄配红豆沙，层层酥脆，传统工艺', expiry_5days, '常温密封保存', 1, 50),
    (box1_id, '椰蓉面包', 4.0, '椰香四溢，口感丰富，新鲜椰蓉', expiry_3days, '常温保存', 1, 85),
    (box1_id, '肉松小贝', 5.5, '酥脆肉松搭配沙拉酱，咸香可口', expiry_3days, '常温保存，避免挤压', 1, 60);

  -- 为盲盒2添加内容
  INSERT INTO blind_box_contents (blind_box_id, product_name, product_value, product_description, product_expiry_date, product_storage_condition, quantity, probability) VALUES
    (box2_id, '提拉米苏切块', 12.0, '意式经典甜点，咖啡与芝士的完美融合', expiry_3days, '冷藏保存 0-4°C', 1, 70),
    (box2_id, '芝士蛋糕', 10.0, '进口芝士制作，香浓细腻，入口即化', expiry_3days, '冷藏保存 0-4°C', 1, 75),
    (box2_id, '法式牛角包', 6.0, '外酥内软，奶香浓郁', expiry_3days, '常温保存', 2, 90),
    (box2_id, '草莓慕斯', 8.0, '新鲜草莓制作，口感轻盈细腻', expiry_3days, '冷藏保存 0-4°C', 1, 60),
    (box2_id, '抹茶卷', 7.0, '日式抹茶风味，清新不腻，抹茶粉来自日本', expiry_3days, '冷藏保存', 1, 80),
    (box2_id, '奶油泡芙', 5.0, '新鲜奶油填充，外脆内软', expiry_3days, '冷藏保存，当日食用最佳', 2, 85),
    (box2_id, '蓝莓马芬', 6.0, '新鲜蓝莓，松软可口，低糖配方', expiry_3days, '常温保存', 1, 70),
    (box2_id, '黑森林蛋糕', 15.0, '樱桃巧克力经典搭配，德式传统配方', expiry_3days, '冷藏保存 0-4°C', 1, 40);

  -- 为盲盒3添加内容
  INSERT INTO blind_box_contents (blind_box_id, product_name, product_value, product_description, product_expiry_date, product_storage_condition, quantity, probability) VALUES
    (box3_id, '红富士苹果', 8.0, '山东烟台红富士，脆甜多汁，果香浓郁', expiry_5days, '常温或冷藏保存', 2, 90),
    (box3_id, '香蕉', 5.0, '进口香蕉，香甜软糯，富含钾元素', expiry_3days, '常温保存，避免冷藏', 3, 95),
    (box3_id, '橙子', 6.0, '赣南脐橙，皮薄多汁，维C丰富', expiry_5days, '常温或冷藏保存', 2, 85),
    (box3_id, '猕猴桃', 10.0, '新西兰奇异果，酸甜可口，营养丰富', expiry_5days, '冷藏保存，待软后食用', 2, 60),
    (box3_id, '葡萄', 12.0, '新疆无籽葡萄，颗粒饱满，甜度高', expiry_3days, '冷藏保存', 1, 50),
    (box3_id, '草莓', 15.0, '丹东99草莓，个大饱满，香甜多汁', expiry_3days, '冷藏保存，轻拿轻放', 1, 40);

  RAISE NOTICE '盲盒测试数据创建成功！';
  RAISE NOTICE '盲盒1 ID: %, 盲盒2 ID: %, 盲盒3 ID: %', box1_id, box2_id, box3_id;

END $$;

-- 查看创建的盲盒
SELECT 
  p.id,
  p.name,
  p.original_price as price,
  p.blind_box_min_value as min_value,
  p.blind_box_max_value as max_value,
  p.stock,
  m.shop_name
FROM products p
JOIN merchants m ON p.merchant_id = m.id
WHERE p.is_blind_box = true AND p.status = 'active';

-- 查看盲盒内容（验证数据完整性）
SELECT 
  p.name as blind_box_name,
  bc.product_name,
  bc.product_value,
  bc.product_description,
  bc.product_expiry_date,
  bc.product_storage_condition,
  bc.quantity,
  bc.probability
FROM blind_box_contents bc
JOIN products p ON bc.blind_box_id = p.id
WHERE p.is_blind_box = true
ORDER BY p.id, bc.probability DESC;
