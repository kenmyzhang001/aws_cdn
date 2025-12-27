-- 分组管理功能迁移脚本
-- 适用于 MySQL 8+ / MariaDB 10.3+
-- 执行前请备份数据库！

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ============================================
-- 1. 创建分组表
-- ============================================
CREATE TABLE IF NOT EXISTS `groups` (
  `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `name`        VARCHAR(255) NOT NULL COMMENT '分组名称',
  `is_default`  TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否为默认分组',
  `created_at`  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`  DATETIME(3) NULL DEFAULT NULL,

  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_groups_name` (`name`, `deleted_at`),
  KEY `idx_groups_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='分组表';

-- ============================================
-- 2. 插入默认分组
-- ============================================
INSERT INTO `groups` (`name`, `is_default`, `created_at`, `updated_at`) 
VALUES ('默认分组', 1, NOW(), NOW())
ON DUPLICATE KEY UPDATE `name` = `name`;

-- 获取默认分组ID（用于后续数据迁移）
SET @default_group_id = (SELECT `id` FROM `groups` WHERE `is_default` = 1 LIMIT 1);

-- ============================================
-- 3. 修改 domains 表，添加 group_id 字段
-- ============================================
-- 检查字段是否已存在
SET @col_exists = (
  SELECT COUNT(*) 
  FROM INFORMATION_SCHEMA.COLUMNS 
  WHERE TABLE_SCHEMA = DATABASE() 
    AND TABLE_NAME = 'domains' 
    AND COLUMN_NAME = 'group_id'
);

SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `domains` ADD COLUMN `group_id` BIGINT UNSIGNED DEFAULT NULL COMMENT ''所属分组ID'' AFTER `registrar`',
  'SELECT ''group_id column already exists in domains table'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加索引
SET @idx_exists = (
  SELECT COUNT(*) 
  FROM INFORMATION_SCHEMA.STATISTICS 
  WHERE TABLE_SCHEMA = DATABASE() 
    AND TABLE_NAME = 'domains' 
    AND INDEX_NAME = 'idx_domains_group_id'
);

SET @sql = IF(@idx_exists = 0,
  'ALTER TABLE `domains` ADD KEY `idx_domains_group_id` (`group_id`)',
  'SELECT ''idx_domains_group_id already exists'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 将现有域名的 group_id 设置为默认分组
UPDATE `domains` 
SET `group_id` = @default_group_id 
WHERE `group_id` IS NULL AND `deleted_at` IS NULL;

-- 添加外键约束
SET @fk_exists = (
  SELECT COUNT(*) 
  FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE 
  WHERE TABLE_SCHEMA = DATABASE() 
    AND TABLE_NAME = 'domains' 
    AND CONSTRAINT_NAME = 'fk_domains_group'
);

SET @sql = IF(@fk_exists = 0,
  'ALTER TABLE `domains` ADD CONSTRAINT `fk_domains_group` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE SET NULL',
  'SELECT ''fk_domains_group already exists'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- ============================================
-- 4. 修改 redirect_rules 表，添加 group_id 字段
-- ============================================
-- 检查字段是否已存在
SET @col_exists = (
  SELECT COUNT(*) 
  FROM INFORMATION_SCHEMA.COLUMNS 
  WHERE TABLE_SCHEMA = DATABASE() 
    AND TABLE_NAME = 'redirect_rules' 
    AND COLUMN_NAME = 'group_id'
);

SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `redirect_rules` ADD COLUMN `group_id` BIGINT UNSIGNED DEFAULT NULL COMMENT ''所属分组ID'' AFTER `source_domain`',
  'SELECT ''group_id column already exists in redirect_rules table'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加索引
SET @idx_exists = (
  SELECT COUNT(*) 
  FROM INFORMATION_SCHEMA.STATISTICS 
  WHERE TABLE_SCHEMA = DATABASE() 
    AND TABLE_NAME = 'redirect_rules' 
    AND INDEX_NAME = 'idx_redirect_rules_group_id'
);

SET @sql = IF(@idx_exists = 0,
  'ALTER TABLE `redirect_rules` ADD KEY `idx_redirect_rules_group_id` (`group_id`)',
  'SELECT ''idx_redirect_rules_group_id already exists'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 将现有重定向规则的 group_id 设置为默认分组
UPDATE `redirect_rules` 
SET `group_id` = @default_group_id 
WHERE `group_id` IS NULL AND `deleted_at` IS NULL;

-- 添加外键约束
SET @fk_exists = (
  SELECT COUNT(*) 
  FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE 
  WHERE TABLE_SCHEMA = DATABASE() 
    AND TABLE_NAME = 'redirect_rules' 
    AND CONSTRAINT_NAME = 'fk_redirect_rules_group'
);

SET @sql = IF(@fk_exists = 0,
  'ALTER TABLE `redirect_rules` ADD CONSTRAINT `fk_redirect_rules_group` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE SET NULL',
  'SELECT ''fk_redirect_rules_group already exists'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- ============================================
-- 5. 修改 download_packages 表，添加 group_id 字段
-- ============================================
-- 检查字段是否已存在
SET @col_exists = (
  SELECT COUNT(*) 
  FROM INFORMATION_SCHEMA.COLUMNS 
  WHERE TABLE_SCHEMA = DATABASE() 
    AND TABLE_NAME = 'download_packages' 
    AND COLUMN_NAME = 'group_id'
);

SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `download_packages` ADD COLUMN `group_id` BIGINT UNSIGNED DEFAULT NULL COMMENT ''所属分组ID'' AFTER `domain_id`',
  'SELECT ''group_id column already exists in download_packages table'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加索引
SET @idx_exists = (
  SELECT COUNT(*) 
  FROM INFORMATION_SCHEMA.STATISTICS 
  WHERE TABLE_SCHEMA = DATABASE() 
    AND TABLE_NAME = 'download_packages' 
    AND INDEX_NAME = 'idx_download_packages_group_id'
);

SET @sql = IF(@idx_exists = 0,
  'ALTER TABLE `download_packages` ADD KEY `idx_download_packages_group_id` (`group_id`)',
  'SELECT ''idx_download_packages_group_id already exists'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 将现有下载包的 group_id 设置为对应域名的 group_id
-- 如果域名的 group_id 为空，则设置为默认分组
UPDATE `download_packages` dp
LEFT JOIN `domains` d ON dp.`domain_id` = d.`id`
SET dp.`group_id` = COALESCE(d.`group_id`, @default_group_id)
WHERE dp.`group_id` IS NULL AND dp.`deleted_at` IS NULL;

-- 添加外键约束
SET @fk_exists = (
  SELECT COUNT(*) 
  FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE 
  WHERE TABLE_SCHEMA = DATABASE() 
    AND TABLE_NAME = 'download_packages' 
    AND CONSTRAINT_NAME = 'fk_download_packages_group'
);

SET @sql = IF(@fk_exists = 0,
  'ALTER TABLE `download_packages` ADD CONSTRAINT `fk_download_packages_group` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE SET NULL',
  'SELECT ''fk_download_packages_group already exists'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET FOREIGN_KEY_CHECKS = 1;

-- ============================================
-- 迁移完成
-- ============================================
SELECT 'Migration completed successfully!' AS message;
SELECT 
  (SELECT COUNT(*) FROM `groups`) AS total_groups,
  (SELECT COUNT(*) FROM `domains` WHERE `group_id` IS NOT NULL) AS domains_with_group,
  (SELECT COUNT(*) FROM `redirect_rules` WHERE `group_id` IS NOT NULL) AS redirects_with_group,
  (SELECT COUNT(*) FROM `download_packages` WHERE `group_id` IS NOT NULL) AS packages_with_group;

