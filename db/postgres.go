package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

var PG *sql.DB

func InitPostgres() error {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_PORT", "5432"),
		getEnv("DB_USER", "postgres"),
		getEnv("DB_PASSWORD", "postgres"),
		getEnv("DB_NAME", "food_rescue"),
	)

	var err error
	PG, err = sql.Open("postgres", dsn)
	if err != nil {
		return err
	}

	if err = PG.Ping(); err != nil {
		return err
	}

	return createTables()
}

func ClosePostgres() {
	if PG != nil {
		PG.Close()
	}
}

func createTables() error {
	// 用户表 - 消费者
	_, err := PG.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			phone VARCHAR(20) UNIQUE NOT NULL,
			password VARCHAR(100) NOT NULL,
			nickname VARCHAR(50),
			avatar VARCHAR(255),
			
			-- 环保成就相关
			total_saved_weight DECIMAL(10,2) DEFAULT 0,
			total_saved_money DECIMAL(10,2) DEFAULT 0,
			total_orders INT DEFAULT 0,
			rank_level VARCHAR(20) DEFAULT 'newcomer',
			
			-- 状态
			status VARCHAR(20) DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 商家表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS merchants (
			id SERIAL PRIMARY KEY,
			phone VARCHAR(20) UNIQUE NOT NULL,
			password VARCHAR(100) NOT NULL,
			
			-- 店铺基本信息
			shop_name VARCHAR(100),
			shop_logo VARCHAR(255),
			shop_images TEXT[],
			description TEXT,
			
			-- 认证信息
			license_no VARCHAR(50),
			license_image VARCHAR(255),
			food_permit_no VARCHAR(50),
			food_permit_image VARCHAR(255),
			is_verified BOOLEAN DEFAULT FALSE,
			
			-- 位置信息
			province VARCHAR(50),
			city VARCHAR(50),
			district VARCHAR(50),
			address VARCHAR(255),
			latitude DECIMAL(10,7),
			longitude DECIMAL(10,7),
			
			-- 营业信息
			business_hours VARCHAR(100),
			pickup_start_time TIME,
			pickup_end_time TIME,
			contact_phone VARCHAR(20),
			
			-- 统计数据
			total_sales DECIMAL(12,2) DEFAULT 0,
			total_saved_loss DECIMAL(12,2) DEFAULT 0,
			total_products_sold INT DEFAULT 0,
			rating DECIMAL(2,1) DEFAULT 5.0,
			rating_count INT DEFAULT 0,
			
			-- 状态
			status VARCHAR(20) DEFAULT 'pending',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 用户收藏店铺表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS user_favorite_shops (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			merchant_id INT NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, merchant_id)
		)
	`)
	if err != nil {
		return err
	}

	// 用户地址表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS user_addresses (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name VARCHAR(50),
			phone VARCHAR(20),
			province VARCHAR(50),
			city VARCHAR(50),
			district VARCHAR(50),
			address VARCHAR(255),
			is_default BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 用户收藏商品表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS user_favorites (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			product_id INT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, product_id)
		)
	`)
	if err != nil {
		return err
	}

	// 购物车表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS cart_items (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			product_id INT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
			quantity INT DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, product_id)
		)
	`)
	if err != nil {
		return err
	}

	// 商品分类表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS categories (
			id SERIAL PRIMARY KEY,
			name VARCHAR(50) NOT NULL,
			icon VARCHAR(50),
			sort_order INT DEFAULT 0,
			is_active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 清理重复分类：只保留每个 name 最小 id 的记录
	_, _ = PG.Exec(`
		DELETE FROM categories WHERE id NOT IN (
			SELECT MIN(id) FROM categories GROUP BY name
		)
	`)
	// 加唯一约束（如果还没有）
	_, _ = PG.Exec(`
		DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'categories_name_key') THEN
				ALTER TABLE categories ADD CONSTRAINT categories_name_key UNIQUE (name);
			END IF;
		END $$
	`)

	// 插入默认分类（有唯一约束后 ON CONFLICT 才生效）
	_, _ = PG.Exec(`
		INSERT INTO categories (name, icon, sort_order) VALUES
		('面包糕点', 'cake', 1),
		('乳制品', 'milk', 2),
		('饮料', 'coffee', 3),
		('零食', 'cookie', 4),
		('熟食', 'utensils', 5),
		('水果', 'apple', 6),
		('蔬菜', 'carrot', 7),
		('其他', 'package', 8)
		ON CONFLICT (name) DO NOTHING
	`)

	// 商品表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS products (
			id SERIAL PRIMARY KEY,
			merchant_id INT NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
			category_id INT REFERENCES categories(id),
			
			name VARCHAR(100) NOT NULL,
			description TEXT,
			images TEXT[],
			
			original_price DECIMAL(10,2) NOT NULL,
			
			production_date DATE,
			expiry_date DATE NOT NULL,
			storage_condition VARCHAR(50),
			
			stock INT DEFAULT 0,
			sold_count INT DEFAULT 0,
			
			price_rule_type VARCHAR(20) DEFAULT 'fixed',
			price_rules JSONB,
			
			is_blind_box BOOLEAN DEFAULT FALSE,
			blind_box_min_value DECIMAL(10,2),
			blind_box_max_value DECIMAL(10,2),
			
			status VARCHAR(20) DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 商品索引
	_, _ = PG.Exec(`CREATE INDEX IF NOT EXISTS idx_products_merchant ON products(merchant_id)`)
	_, _ = PG.Exec(`CREATE INDEX IF NOT EXISTS idx_products_category ON products(category_id)`)
	_, _ = PG.Exec(`CREATE INDEX IF NOT EXISTS idx_products_expiry ON products(expiry_date)`)
	_, _ = PG.Exec(`CREATE INDEX IF NOT EXISTS idx_products_status ON products(status)`)

	// 订单表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS orders (
			id SERIAL PRIMARY KEY,
			order_no VARCHAR(32) UNIQUE NOT NULL,
			user_id INT NOT NULL REFERENCES users(id),
			merchant_id INT NOT NULL REFERENCES merchants(id),
			
			-- 金额
			total_amount DECIMAL(10,2) NOT NULL,
			discount_amount DECIMAL(10,2) DEFAULT 0,
			delivery_fee DECIMAL(10,2) DEFAULT 0,
			pay_amount DECIMAL(10,2) NOT NULL,
			
			-- 配送方式: pickup(到店自取) / delivery(配送到家)
			delivery_type VARCHAR(20) DEFAULT 'pickup',
			
			-- 配送地址（配送模式使用）
			address_name VARCHAR(50),
			address_phone VARCHAR(20),
			address_province VARCHAR(50),
			address_city VARCHAR(50),
			address_district VARCHAR(50),
			address_detail VARCHAR(255),
			
			-- 状态: pending(待支付) -> paid(已支付/待取货或配送中) -> completed(已完成) / cancelled(已取消) / expired(已过期)
			status VARCHAR(20) DEFAULT 'pending',
			
			-- 取货信息（自取模式使用）
			pickup_code VARCHAR(6),
			pickup_time TIMESTAMP,
			
			-- 备注
			remark TEXT,
			
			-- 时间
			pay_time TIMESTAMP,
			complete_time TIMESTAMP,
			cancel_time TIMESTAMP,
			expire_time TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 订单商品表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS order_items (
			id SERIAL PRIMARY KEY,
			order_id INT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
			product_id INT NOT NULL REFERENCES products(id),
			product_name VARCHAR(100) NOT NULL,
			product_image VARCHAR(255),
			original_price DECIMAL(10,2) NOT NULL,
			current_price DECIMAL(10,2) NOT NULL,
			quantity INT NOT NULL,
			subtotal DECIMAL(10,2) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 订单索引
	_, _ = PG.Exec(`CREATE INDEX IF NOT EXISTS idx_orders_user ON orders(user_id)`)
	_, _ = PG.Exec(`CREATE INDEX IF NOT EXISTS idx_orders_merchant ON orders(merchant_id)`)
	_, _ = PG.Exec(`CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status)`)

	// 添加订单表缺失的列（兼容旧表）
	_, _ = PG.Exec(`ALTER TABLE orders ADD COLUMN IF NOT EXISTS delivery_type VARCHAR(20) DEFAULT 'pickup'`)
	_, _ = PG.Exec(`ALTER TABLE orders ADD COLUMN IF NOT EXISTS delivery_fee DECIMAL(10,2) DEFAULT 0`)
	_, _ = PG.Exec(`ALTER TABLE orders ADD COLUMN IF NOT EXISTS address_name VARCHAR(50)`)
	_, _ = PG.Exec(`ALTER TABLE orders ADD COLUMN IF NOT EXISTS address_phone VARCHAR(20)`)
	_, _ = PG.Exec(`ALTER TABLE orders ADD COLUMN IF NOT EXISTS address_province VARCHAR(50)`)
	_, _ = PG.Exec(`ALTER TABLE orders ADD COLUMN IF NOT EXISTS address_city VARCHAR(50)`)
	_, _ = PG.Exec(`ALTER TABLE orders ADD COLUMN IF NOT EXISTS address_district VARCHAR(50)`)
	_, _ = PG.Exec(`ALTER TABLE orders ADD COLUMN IF NOT EXISTS address_detail VARCHAR(255)`)

	// 盲盒购买记录表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS blind_box_purchases (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL REFERENCES users(id),
			product_id INT NOT NULL REFERENCES products(id),
			merchant_id INT NOT NULL REFERENCES merchants(id),
			price DECIMAL(10,2) NOT NULL,
			status VARCHAR(20) DEFAULT 'unopened',
			opened_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 盲盒开盒内容表（记录用户开出的内容）
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS blind_box_items (
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
		)
	`)
	if err != nil {
		return err
	}

	// 盲盒商品内容表（商家设置的盲盒可能包含的商品）
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS blind_box_contents (
			id SERIAL PRIMARY KEY,
			blind_box_id INT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
			product_name VARCHAR(100) NOT NULL,
			product_value DECIMAL(10,2) NOT NULL,
			product_image VARCHAR(255),
			product_description TEXT,
			product_expiry_date DATE,
			product_storage_condition VARCHAR(50),
			quantity INT DEFAULT 1,
			probability INT DEFAULT 100,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 添加新列（兼容旧表）
	_, _ = PG.Exec(`ALTER TABLE blind_box_items ADD COLUMN IF NOT EXISTS item_description TEXT`)
	_, _ = PG.Exec(`ALTER TABLE blind_box_items ADD COLUMN IF NOT EXISTS item_expiry_date DATE`)
	_, _ = PG.Exec(`ALTER TABLE blind_box_items ADD COLUMN IF NOT EXISTS item_storage_condition VARCHAR(50)`)
	_, _ = PG.Exec(`ALTER TABLE blind_box_items ADD COLUMN IF NOT EXISTS item_quantity INT DEFAULT 1`)
	_, _ = PG.Exec(`ALTER TABLE blind_box_contents ADD COLUMN IF NOT EXISTS product_description TEXT`)
	_, _ = PG.Exec(`ALTER TABLE blind_box_contents ADD COLUMN IF NOT EXISTS product_expiry_date DATE`)
	_, _ = PG.Exec(`ALTER TABLE blind_box_contents ADD COLUMN IF NOT EXISTS product_storage_condition VARCHAR(50)`)
	_, _ = PG.Exec(`ALTER TABLE blind_box_contents ADD COLUMN IF NOT EXISTS source_product_id INT`)

	// 盲盒索引
	_, _ = PG.Exec(`CREATE INDEX IF NOT EXISTS idx_blind_box_user ON blind_box_purchases(user_id)`)
	_, _ = PG.Exec(`CREATE INDEX IF NOT EXISTS idx_blind_box_status ON blind_box_purchases(status)`)

	// 商品溯源信息表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS product_traceability (
			id SERIAL PRIMARY KEY,
			product_id INT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
			
			-- 供应商信息
			supplier_name VARCHAR(100),
			supplier_license VARCHAR(100),
			
			-- 进货信息
			purchase_date DATE,
			purchase_batch VARCHAR(50),
			purchase_quantity INT,
			purchase_certificate VARCHAR(255),
			
			-- 质检信息
			inspection_date DATE,
			inspection_result VARCHAR(20),
			inspection_certificate VARCHAR(255),
			
			-- 存储信息
			storage_temperature VARCHAR(50),
			storage_humidity VARCHAR(50),
			storage_location VARCHAR(100),
			
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(product_id)
		)
	`)
	if err != nil {
		return err
	}

	// 试吃官表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS tasters (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			
			-- 申请信息
			real_name VARCHAR(50),
			id_card VARCHAR(20),
			reason TEXT,
			
			-- 状态: pending(待审核) / approved(已通过) / rejected(已拒绝)
			status VARCHAR(20) DEFAULT 'pending',
			
			-- 试吃官等级: junior(初级) / senior(高级) / expert(专家)
			level VARCHAR(20) DEFAULT 'junior',
			
			-- 统计
			total_tasks INT DEFAULT 0,
			completed_tasks INT DEFAULT 0,
			total_reviews INT DEFAULT 0,
			avg_rating DECIMAL(2,1) DEFAULT 5.0,
			
			approved_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id)
		)
	`)
	if err != nil {
		return err
	}

	// 试吃任务表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS tasting_tasks (
			id SERIAL PRIMARY KEY,
			product_id INT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
			merchant_id INT NOT NULL REFERENCES merchants(id),
			
			-- 任务信息
			title VARCHAR(100) NOT NULL,
			description TEXT,
			requirements TEXT,
			
			-- 名额
			total_slots INT DEFAULT 5,
			filled_slots INT DEFAULT 0,
			
			-- 价格
			original_price DECIMAL(10,2),
			tasting_price DECIMAL(10,2) DEFAULT 0,
			
			-- 时间
			start_time TIMESTAMP,
			end_time TIMESTAMP,
			
			-- 状态: active(进行中) / completed(已结束) / cancelled(已取消)
			status VARCHAR(20) DEFAULT 'active',
			
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 试吃申请表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS tasting_applications (
			id SERIAL PRIMARY KEY,
			task_id INT NOT NULL REFERENCES tasting_tasks(id) ON DELETE CASCADE,
			user_id INT NOT NULL REFERENCES users(id),
			taster_id INT REFERENCES tasters(id),
			
			-- 状态: pending(待审核) / approved(已通过) / rejected(已拒绝) / completed(已完成)
			status VARCHAR(20) DEFAULT 'pending',
			
			-- 订单关联
			order_id INT REFERENCES orders(id),
			
			-- 评价
			review_id INT,
			
			approved_at TIMESTAMP,
			completed_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(task_id, user_id)
		)
	`)
	if err != nil {
		return err
	}

	// 商品评价表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS product_reviews (
			id SERIAL PRIMARY KEY,
			product_id INT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
			user_id INT NOT NULL REFERENCES users(id),
			order_id INT REFERENCES orders(id),
			
			-- 评价内容
			rating INT NOT NULL CHECK (rating >= 1 AND rating <= 5),
			content TEXT,
			images TEXT[],
			
			-- 是否试吃官评价
			is_taster_review BOOLEAN DEFAULT FALSE,
			tasting_application_id INT REFERENCES tasting_applications(id),
			
			-- 商家回复
			merchant_reply TEXT,
			reply_time TIMESTAMP,
			
			-- 状态
			status VARCHAR(20) DEFAULT 'visible',
			
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 评价索引
	_, _ = PG.Exec(`CREATE INDEX IF NOT EXISTS idx_reviews_product ON product_reviews(product_id)`)
	_, _ = PG.Exec(`CREATE INDEX IF NOT EXISTS idx_reviews_user ON product_reviews(user_id)`)
	_, _ = PG.Exec(`CREATE INDEX IF NOT EXISTS idx_tasting_tasks_merchant ON tasting_tasks(merchant_id)`)
	_, _ = PG.Exec(`CREATE INDEX IF NOT EXISTS idx_tasting_tasks_status ON tasting_tasks(status)`)

	// 管理员表
	_, err = PG.Exec(`
		CREATE TABLE IF NOT EXISTS admins (
			id SERIAL PRIMARY KEY,
			username VARCHAR(50) UNIQUE NOT NULL,
			password VARCHAR(100) NOT NULL,
			nickname VARCHAR(50),
			role VARCHAR(20) DEFAULT 'admin',
			status VARCHAR(20) DEFAULT 'active',
			last_login_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 插入默认管理员 admin/admin123
	_, _ = PG.Exec(`
		INSERT INTO admins (username, password, nickname, role)
		VALUES ('admin', 'admin123', '超级管理员', 'superadmin')
		ON CONFLICT (username) DO NOTHING
	`)

	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
