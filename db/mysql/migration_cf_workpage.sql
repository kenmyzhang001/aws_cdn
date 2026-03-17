-- CF-WorkPage 模版与站点表迁移脚本
-- 适用于 MySQL 8+ / MariaDB 10.3+
-- 执行前请备份数据库！

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ============================================
-- 1. 模版表：cf_workpage_templates
-- ============================================
CREATE TABLE IF NOT EXISTS `cf_workpage_templates` (
  `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `name_zh`      VARCHAR(255) NOT NULL DEFAULT '' COMMENT '模版名称-中文',
  `name_my`      VARCHAR(255) NOT NULL DEFAULT '' COMMENT '模版名称-缅甸文',
  `default_lang` VARCHAR(8)   NOT NULL DEFAULT 'zh' COMMENT '落地页默认语言: zh | my',
  `created_at`   DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`   DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`   DATETIME(3)   NULL DEFAULT NULL,

  PRIMARY KEY (`id`),
  KEY `idx_cf_workpage_templates_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='CF-WorkPage 模版（中/缅双语言）';

-- ============================================
-- 2. 站点表：cf_workpage_sites
-- ============================================
CREATE TABLE IF NOT EXISTS `cf_workpage_sites` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `cf_account_id` BIGINT UNSIGNED NOT NULL COMMENT 'CF 账号 ID',
  `template_id`   BIGINT UNSIGNED NOT NULL COMMENT '模版 ID',
  `zone_id`       VARCHAR(64)   NOT NULL COMMENT 'CF Zone ID（主域名所属）',
  `main_domain`   VARCHAR(255)   NOT NULL COMMENT '主域名',
  `subdomain`     VARCHAR(128)   NOT NULL DEFAULT '' COMMENT '子域名前缀，如 www、app',
  `status`        VARCHAR(32)   NOT NULL DEFAULT 'pending' COMMENT 'pending | deployed | failed',
  `created_at`    DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`    DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`    DATETIME(3)   NULL DEFAULT NULL,

  PRIMARY KEY (`id`),
  KEY `idx_cf_workpage_sites_deleted_at` (`deleted_at`),
  KEY `idx_cf_workpage_sites_cf_account_id` (`cf_account_id`),
  KEY `idx_cf_workpage_sites_template_id` (`template_id`),

  CONSTRAINT `fk_cf_workpage_sites_cf_account`
    FOREIGN KEY (`cf_account_id`) REFERENCES `cf_accounts` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_cf_workpage_sites_template`
    FOREIGN KEY (`template_id`) REFERENCES `cf_workpage_templates` (`id`) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='CF-WorkPage 站点（关联账号、模版，绑定主域名与子域名）';

SET FOREIGN_KEY_CHECKS = 1;

-- ============================================
-- 迁移完成
-- ============================================
SELECT 'CF-WorkPage migration completed successfully!' AS message;
