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
