-- MySQL schema for aws_cdn project
-- 根据当前 GORM 模型生成，适用于 MySQL 8+ / MariaDB 10.3+
-- 使用前请根据实际需要调整库名、字符集等

-- 建议先手动创建数据库（如需）：
-- CREATE DATABASE aws_cdn CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
-- USE aws_cdn;

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

/*
 * 1. 用户表：users
 *    - 支持账号密码登录
 *    - 支持谷歌验证码（TOTP）二步验证
 */
CREATE TABLE IF NOT EXISTS `users` (
  `id`                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `username`           VARCHAR(191) NOT NULL COMMENT '用户名',
  `email`              VARCHAR(191) NOT NULL COMMENT '邮箱',
  `password`           VARCHAR(255) NOT NULL COMMENT 'bcrypt 哈希密码',
  `is_active`          TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否启用',

  `two_factor_secret`  VARCHAR(255) DEFAULT NULL COMMENT '谷歌验证码密钥',
  `is_two_factor_enabled` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否开启二步验证',

  `created_at`         DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`         DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`         DATETIME(3) NULL DEFAULT NULL,

  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_users_username` (`username`),
  UNIQUE KEY `idx_users_email` (`email`),
  KEY `idx_users_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户表';


/*
 * 2. 分组表：groups
 *    - 管理域名、重定向、下载包的分组
 */
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

/*
 * 3. 域名表：domains
 *    - 管理转入的域名、NS 和证书状态
 */
CREATE TABLE IF NOT EXISTS `domains` (
  `id`                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `domain_name`         VARCHAR(255) NOT NULL COMMENT '域名',
  `registrar`           VARCHAR(255) DEFAULT NULL COMMENT '原注册商',
  `group_id`            BIGINT UNSIGNED DEFAULT NULL COMMENT '所属分组ID',
  `status`              VARCHAR(32) NOT NULL DEFAULT 'pending' COMMENT '域名状态: pending/in_progress/completed/failed',
  `n_servers`           TEXT DEFAULT NULL COMMENT 'NS 服务器配置(JSON 字符串)',
  `certificate_status`  VARCHAR(32) NOT NULL DEFAULT 'pending' COMMENT '证书状态: pending/issued/failed',
  `certificate_arn`     VARCHAR(255) DEFAULT NULL COMMENT 'ACM 证书 ARN',
  `hosted_zone_id`      VARCHAR(255) DEFAULT NULL COMMENT 'Route53 Hosted Zone ID',

  `created_at`          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`          DATETIME(3) NULL DEFAULT NULL,

  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_domains_domain_name` (`domain_name`, `deleted_at`),
  KEY `idx_domains_deleted_at` (`deleted_at`),
  KEY `idx_domains_group_id` (`group_id`),

  CONSTRAINT `fk_domains_group`
    FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`)
    ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='域名表';


/*
 * 4. 重定向规则表：redirect_rules
 *    - 源域名以及对应的 CloudFront 分发
 */
CREATE TABLE IF NOT EXISTS `redirect_rules` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `source_domain` VARCHAR(255) NOT NULL COMMENT '源域名',
  `group_id`      BIGINT UNSIGNED DEFAULT NULL COMMENT '所属分组ID',
  `cloudfront_id` VARCHAR(255) DEFAULT NULL COMMENT 'CloudFront Distribution ID',

  `created_at`    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`    DATETIME(3) NULL DEFAULT NULL,

  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_redirect_rules_source_domain` (`source_domain`),
  KEY `idx_redirect_rules_deleted_at` (`deleted_at`),
  KEY `idx_redirect_rules_group_id` (`group_id`),

  CONSTRAINT `fk_redirect_rules_group`
    FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`)
    ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='重定向规则表';


/*
 * 5. 重定向目标表：redirect_targets
 *    - 每条规则下的多个目标 URL，支持权重与启用状态
 */
CREATE TABLE IF NOT EXISTS `redirect_targets` (
  `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `rule_id`     BIGINT UNSIGNED NOT NULL COMMENT '所属规则 ID',
  `target_url`  TEXT NOT NULL COMMENT '目标 URL',
  `weight`      INT NOT NULL DEFAULT 1 COMMENT '权重',
  `is_active`   TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否启用',

  `created_at`  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`  DATETIME(3) NULL DEFAULT NULL,

  PRIMARY KEY (`id`),
  KEY `idx_redirect_targets_rule_id` (`rule_id`),
  KEY `idx_redirect_targets_deleted_at` (`deleted_at`),

  CONSTRAINT `fk_redirect_targets_rule`
    FOREIGN KEY (`rule_id`) REFERENCES `redirect_rules` (`id`)
    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='重定向目标表';


/*
 * 6. 下载包表：download_packages
 *    - 管理下载包文件、S3存储、CloudFront分发
 */
CREATE TABLE IF NOT EXISTS `download_packages` (
  `id`                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `domain_id`           BIGINT UNSIGNED NOT NULL COMMENT '所属域名 ID',
  `group_id`            BIGINT UNSIGNED DEFAULT NULL COMMENT '所属分组ID',
  `domain_name`         VARCHAR(255) NOT NULL COMMENT '下载域名',
  `file_name`           VARCHAR(255) NOT NULL COMMENT '文件名',
  `file_size`           BIGINT NOT NULL COMMENT '文件大小（字节）',
  `file_type`           VARCHAR(100) DEFAULT NULL COMMENT '文件类型',
  `s3_key`              VARCHAR(500) NOT NULL COMMENT 'S3对象键',
  `cloudfront_id`       VARCHAR(255) DEFAULT NULL COMMENT 'CloudFront分发ID',
  `cloudfront_domain`   VARCHAR(255) DEFAULT NULL COMMENT 'CloudFront域名',
  `download_url`        VARCHAR(500) DEFAULT NULL COMMENT '下载URL',
  `status`              VARCHAR(32) NOT NULL DEFAULT 'pending' COMMENT '状态: pending/uploading/processing/completed/failed',
  `error_message`       TEXT DEFAULT NULL COMMENT '错误信息',

  `created_at`          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`          DATETIME(3) NULL DEFAULT NULL,

  PRIMARY KEY (`id`),
  KEY `idx_download_packages_domain_id` (`domain_id`),
  KEY `idx_download_packages_deleted_at` (`deleted_at`),
  KEY `idx_download_packages_group_id` (`group_id`),

  CONSTRAINT `fk_download_packages_domain`
    FOREIGN KEY (`domain_id`) REFERENCES `domains` (`id`)
    ON DELETE RESTRICT,
  CONSTRAINT `fk_download_packages_group`
    FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`)
    ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='下载包表';


/*
 * 7. 审计日志表：audit_logs
 *    - 记录所有后台操作的变更和操作时间
 */
CREATE TABLE IF NOT EXISTS `audit_logs` (
  `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `user_id`      BIGINT UNSIGNED NOT NULL COMMENT '操作用户ID',
  `username`     VARCHAR(191) NOT NULL COMMENT '操作用户名',
  `action`       VARCHAR(100) NOT NULL COMMENT '操作类型',
  `resource`     VARCHAR(100) DEFAULT NULL COMMENT '资源类型',
  `resource_id` VARCHAR(100) DEFAULT NULL COMMENT '资源ID',
  `method`       VARCHAR(10) DEFAULT NULL COMMENT 'HTTP方法',
  `path`         VARCHAR(500) DEFAULT NULL COMMENT '请求路径',
  `ip`           VARCHAR(50) DEFAULT NULL COMMENT '客户端IP',
  `user_agent`   VARCHAR(500) DEFAULT NULL COMMENT '用户代理',
  `request`      TEXT DEFAULT NULL COMMENT '请求数据（JSON格式）',
  `response`     TEXT DEFAULT NULL COMMENT '响应数据（JSON格式）',
  `status`       INT DEFAULT NULL COMMENT 'HTTP状态码',
  `message`      TEXT DEFAULT NULL COMMENT '操作描述',
  `error`        TEXT DEFAULT NULL COMMENT '错误信息',
  `duration`     BIGINT DEFAULT NULL COMMENT '操作耗时（毫秒）',
  `created_at`   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

  PRIMARY KEY (`id`),
  KEY `idx_audit_logs_user_id` (`user_id`),
  KEY `idx_audit_logs_username` (`username`),
  KEY `idx_audit_logs_action` (`action`),
  KEY `idx_audit_logs_resource` (`resource`),
  KEY `idx_audit_logs_resource_id` (`resource_id`),
  KEY `idx_audit_logs_ip` (`ip`),
  KEY `idx_audit_logs_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='审计日志表';


SET FOREIGN_KEY_CHECKS = 1;

-- 插入默认分组
INSERT INTO `groups` (`name`, `is_default`, `created_at`, `updated_at`) 
VALUES ('默认分组', 1, NOW(), NOW())
ON DUPLICATE KEY UPDATE `name` = `name`;

