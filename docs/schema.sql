-- ============================================================
-- 供应链管理系统 - 数据库建表 SQL
-- 数据库: supply_chain
-- 字符集: utf8mb4
-- ============================================================

CREATE DATABASE IF NOT EXISTS supply_chain
  DEFAULT CHARACTER SET utf8mb4
  DEFAULT COLLATE utf8mb4_unicode_ci;

USE supply_chain;

-- ------------------------------------------------------------
-- 1. 权限模块
-- ------------------------------------------------------------

-- 账号表
CREATE TABLE `account` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `username`   VARCHAR(64)     NOT NULL COMMENT '用户名/登录名',
  `password`   VARCHAR(255)    NOT NULL COMMENT '密码(bcrypt)',
  `real_name`  VARCHAR(64)     NOT NULL DEFAULT '' COMMENT '真实姓名',
  `role`       TINYINT UNSIGNED NOT NULL COMMENT '角色: 0=超级管理员, 1=团队负责人, 2=主管, 3=员工',
  `created_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='账号表';

-- 模块表（动态维护）
CREATE TABLE `module` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `name`       VARCHAR(64)     NOT NULL COMMENT '模块名称, 如: 产品管理, 订单管理',
  `code`       VARCHAR(64)     NOT NULL COMMENT '模块编码, 如: product, order',
  `created_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='系统模块表';

-- 账号-模块权限表
CREATE TABLE `account_permission` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `account_id` BIGINT UNSIGNED NOT NULL COMMENT '账号ID',
  `module_id`  BIGINT UNSIGNED NOT NULL COMMENT '模块ID',
  `can_view`   TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '是否有查看权限: 0=否, 1=是',
  `can_edit`   TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '是否有编辑权限: 0=否, 1=是',
  `created_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_account_module` (`account_id`, `module_id`),
  KEY `idx_module_id` (`module_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='账号模块权限表';

-- 初始化模块数据
INSERT INTO `module` (`name`, `code`) VALUES
  ('产品管理', 'product'),
  ('订单管理', 'order');

-- ------------------------------------------------------------
-- 3. 订单管理模块
-- ------------------------------------------------------------

-- 订单主表（从万里牛ERP同步）
CREATE TABLE `order_trade` (
  `id`                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `uid`               VARCHAR(64)     NOT NULL COMMENT '万里牛订单唯一ID',
  `order_id`          VARCHAR(64)     NOT NULL COMMENT '平台订单号',
  `platform`          VARCHAR(64)     NOT NULL DEFAULT '' COMMENT '平台(如: 淘宝, 京东, 拼多多)',
  `shop_name`         VARCHAR(128)    NOT NULL DEFAULT '' COMMENT '店铺名称',
  `status`            VARCHAR(32)     NOT NULL DEFAULT '' COMMENT '订单状态',
  `trade_status`      VARCHAR(64)     NOT NULL DEFAULT '' COMMENT '交易状态',
  `buyer_nick`        VARCHAR(128)    NOT NULL DEFAULT '' COMMENT '买家昵称',
  `receiver_name`     VARCHAR(128)    NOT NULL DEFAULT '' COMMENT '收货人姓名',
  `receiver_phone`    VARCHAR(32)     NOT NULL DEFAULT '' COMMENT '收货人手机',
  `receiver_province` VARCHAR(64)     NOT NULL DEFAULT '' COMMENT '收货省份',
  `receiver_city`     VARCHAR(64)     NOT NULL DEFAULT '' COMMENT '收货城市',
  `receiver_district` VARCHAR(64)     NOT NULL DEFAULT '' COMMENT '收货区县',
  `receiver_address`  VARCHAR(512)    NOT NULL DEFAULT '' COMMENT '收货详细地址',
  `total_amount`      DECIMAL(12,2)   NOT NULL DEFAULT 0.00 COMMENT '订单总金额',
  `pay_amount`        DECIMAL(12,2)   NOT NULL DEFAULT 0.00 COMMENT '实付金额',
  `post_fee`          DECIMAL(12,2)   NOT NULL DEFAULT 0.00 COMMENT '邮费',
  `discount_fee`      DECIMAL(12,2)   NOT NULL DEFAULT 0.00 COMMENT '优惠金额',
  `logistics_name`    VARCHAR(64)     NOT NULL DEFAULT '' COMMENT '物流公司',
  `logistics_no`      VARCHAR(128)    NOT NULL DEFAULT '' COMMENT '物流单号',
  `buyer_message`     TEXT                                 COMMENT '买家留言',
  `seller_remark`     TEXT                                 COMMENT '卖家备注',
  `pay_time`          DATETIME                             COMMENT '付款时间',
  `send_time`         DATETIME                             COMMENT '发货时间',
  `trade_time`        DATETIME                             COMMENT '交易时间',
  `modify_time`       DATETIME                             COMMENT '最后修改时间(万里牛)',
  `created_at`        DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at`        DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_uid` (`uid`),
  KEY `idx_order_id` (`order_id`),
  KEY `idx_shop_name` (`shop_name`),
  KEY `idx_status` (`status`),
  KEY `idx_trade_time` (`trade_time`),
  KEY `idx_modify_time` (`modify_time`),
  KEY `idx_logistics_no` (`logistics_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='订单主表(万里牛同步)';

-- 订单明细表
CREATE TABLE `order_item` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `trade_uid`     VARCHAR(64)     NOT NULL COMMENT '订单UID(关联order_trade.uid)',
  `item_id`       VARCHAR(64)     NOT NULL DEFAULT '' COMMENT '商品ID',
  `sku_id`        VARCHAR(64)     NOT NULL DEFAULT '' COMMENT 'SKU ID',
  `product_name`  VARCHAR(255)    NOT NULL DEFAULT '' COMMENT '商品名称',
  `sku_name`      VARCHAR(255)    NOT NULL DEFAULT '' COMMENT 'SKU名称',
  `quantity`      INT             NOT NULL DEFAULT 0 COMMENT '数量',
  `price`         DECIMAL(12,2)   NOT NULL DEFAULT 0.00 COMMENT '单价',
  `total_fee`     DECIMAL(12,2)   NOT NULL DEFAULT 0.00 COMMENT '小计',
  `refund_status` VARCHAR(32)     NOT NULL DEFAULT '' COMMENT '退款状态',
  `pic_url`       VARCHAR(512)    NOT NULL DEFAULT '' COMMENT '商品图片URL',
  `created_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_trade_uid` (`trade_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='订单明细表';

-- 店铺表（从订单自动提取）
CREATE TABLE `shop` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `shop_name`  VARCHAR(128)    NOT NULL COMMENT '店铺名称',
  `platform`   VARCHAR(64)     NOT NULL DEFAULT '' COMMENT '所属平台',
  `created_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_shop_name` (`shop_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='店铺表';

-- 账号-店铺权限表
CREATE TABLE `account_shop` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `account_id` BIGINT UNSIGNED NOT NULL COMMENT '账号ID',
  `shop_id`    BIGINT UNSIGNED NOT NULL COMMENT '店铺ID',
  `created_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_account_shop` (`account_id`, `shop_id`),
  KEY `idx_shop_id` (`shop_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='账号店铺权限表';

-- 同步状态表
CREATE TABLE `sync_state` (
  `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `sync_type`      VARCHAR(64)     NOT NULL COMMENT '同步类型',
  `last_sync_time` DATETIME        NOT NULL COMMENT '最后同步时间',
  `updated_at`     DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_sync_type` (`sync_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='同步状态表';

-- ------------------------------------------------------------
-- 2. 产品管理模块
-- ------------------------------------------------------------

-- 产品表
-- 分表建议：
--   当数据量达到亿级时，按 id 范围水平分表，如:
--   product_0000 (id 1 ~ 10,000,000)
--   product_0001 (id 10,000,001 ~ 20,000,000)
--   路由规则: table_suffix = id / 10,000,000
--   也可使用 TiDB / Vitess 等中间件实现自动分片
CREATE TABLE `product` (
  `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `image_url`       VARCHAR(512)    NOT NULL DEFAULT '' COMMENT '产品主图OSS URL',
  `name`            VARCHAR(255)    NOT NULL DEFAULT '' COMMENT '产品名称',
  `product_code`    VARCHAR(128)    NOT NULL DEFAULT '' COMMENT '货号(内部编号, 非SKU, 无唯一约束)',
  `supplier`        VARCHAR(255)    NOT NULL DEFAULT '' COMMENT '供应商',
  `status`          TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '状态: 0=待上架, 1=正常在售, 2=停售',
  `brand`           VARCHAR(128)    NOT NULL DEFAULT '' COMMENT '品牌',
  `category`        VARCHAR(128)    NOT NULL DEFAULT '' COMMENT '类目',
  `group_name`      VARCHAR(128)    NOT NULL DEFAULT '' COMMENT '分组',
  `material`        VARCHAR(255)    NOT NULL DEFAULT '' COMMENT '面料材质',
  `patent_status`   VARCHAR(128)    NOT NULL DEFAULT '' COMMENT '外观专利状态',
  `factory_price`   DECIMAL(12,2)   NOT NULL DEFAULT 0.00 COMMENT '出厂价格(人民币元)',
  `created_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间(即上架时间)',
  `updated_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_status` (`status`),
  KEY `idx_created_at` (`created_at`),
  KEY `idx_created_at_id` (`created_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='产品表';

-- 规格参数表
CREATE TABLE `product_spec` (
  `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `product_id`      BIGINT UNSIGNED NOT NULL COMMENT '产品ID',
  `size_model`      VARCHAR(128)    NOT NULL DEFAULT '' COMMENT '尺码/型号',
  `dimension`       VARCHAR(128)    NOT NULL DEFAULT '' COMMENT '尺寸(cm)',
  `weight`          DECIMAL(10,3)   NOT NULL DEFAULT 0.000 COMMENT '重量(kg)',
  `box_spec`        VARCHAR(128)    NOT NULL DEFAULT '' COMMENT '箱规(cm)',
  `packing_qty`     INT UNSIGNED    NOT NULL DEFAULT 0 COMMENT '装箱数量',
  `created_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_product_id` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='产品规格参数表';

-- 平台管控价表
CREATE TABLE `product_platform_price` (
  `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `product_id`      BIGINT UNSIGNED NOT NULL COMMENT '产品ID',
  `platform_name`   VARCHAR(64)     NOT NULL DEFAULT '' COMMENT '平台名称',
  `control_price`   DECIMAL(12,2)   NOT NULL DEFAULT 0.00 COMMENT '管控价格',
  `currency`        VARCHAR(8)      NOT NULL DEFAULT 'CNY' COMMENT '货币类型: CNY / USD',
  `created_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_product_id` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='产品平台管控价表';

-- SKU 信息表
CREATE TABLE `product_sku` (
  `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `product_id`      BIGINT UNSIGNED NOT NULL COMMENT '产品ID',
  `model`           VARCHAR(128)    NOT NULL DEFAULT '' COMMENT '型号(如: 黑色, 灰色)',
  `size`            VARCHAR(64)     NOT NULL DEFAULT '' COMMENT '尺码(如: S, M, L)',
  `sku_code`        VARCHAR(128)    NOT NULL DEFAULT '' COMMENT 'SKU编码(商品唯一编码)',
  `created_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_product_id` (`product_id`),
  KEY `idx_sku_code` (`sku_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='产品SKU信息表';

-- 产品详情图表
CREATE TABLE `product_detail_image` (
  `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `product_id`      BIGINT UNSIGNED NOT NULL COMMENT '产品ID',
  `image_url`       VARCHAR(512)    NOT NULL DEFAULT '' COMMENT '图片OSS URL',
  `sort_order`      INT UNSIGNED    NOT NULL DEFAULT 0 COMMENT '排序号',
  `created_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_product_id` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='产品详情图表';

-- 产品实拍视频表
CREATE TABLE `product_video` (
  `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `product_id`      BIGINT UNSIGNED NOT NULL COMMENT '产品ID',
  `video_url`       VARCHAR(512)    NOT NULL DEFAULT '' COMMENT '视频OSS URL',
  `cover_url`       VARCHAR(512)    NOT NULL DEFAULT '' COMMENT '封面图OSS URL',
  `created_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_product_id` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='产品实拍视频表';
