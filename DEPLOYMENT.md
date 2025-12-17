# 生产环境部署指南

本文档详细说明如何将 AWS CDN 管理平台部署到生产环境。

## 前置要求

1. **服务器要求**
   - CPU: 2 核以上
   - 内存: 4GB 以上
   - 磁盘: 20GB 以上
   - 操作系统: Linux (Ubuntu 20.04+ 推荐)

2. **软件要求**
   - Docker 20.10+
   - Docker Compose 2.0+
   - Git

3. **AWS 账户**
   - 有效的 AWS 账户
   - IAM 用户/角色（具有必要权限）
   - 已配置的 AWS 凭证

## 部署步骤

### 1. 准备服务器

```bash
# 更新系统
sudo apt update && sudo apt upgrade -y

# 安装 Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# 安装 Docker Compose
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose
```

### 2. 克隆项目

```bash
git clone <repository-url>
cd aws_cdn
```

### 3. 配置环境变量

```bash
cp .env.example .env
nano .env  # 或使用其他编辑器
```

**重要配置项：**

```env
# 数据库配置（生产环境建议使用外部数据库）
DB_HOST=your-db-host
DB_PORT=5432
DB_USER=your-db-user
DB_PASSWORD=your-strong-password
DB_NAME=aws_cdn
DB_SSLMODE=require

# 服务器配置
SERVER_PORT=8080
SERVER_MODE=release

# JWT 配置（使用强随机密钥）
JWT_SECRET=$(openssl rand -base64 32)
JWT_EXPIRE_HOURS=24

# AWS 配置
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=your-access-key-id
AWS_SECRET_ACCESS_KEY=your-secret-access-key

# CloudFront 配置
CLOUDFRONT_DISTRIBUTION_ID=your-distribution-id

# S3 配置
S3_BUCKET_NAME=your-bucket-name
```

### 4. 配置 AWS IAM 权限

创建 IAM 策略，包含以下权限：

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "route53:CreateHostedZone",
        "route53:GetHostedZone",
        "route53:ListHostedZones",
        "route53:ChangeResourceRecordSets"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "acm:RequestCertificate",
        "acm:DescribeCertificate",
        "acm:ListCertificates"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "cloudfront:CreateDistribution",
        "cloudfront:GetDistribution",
        "cloudfront:UpdateDistribution",
        "cloudfront:ListDistributions"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:CreateBucket",
        "s3:PutObject",
        "s3:GetObject",
        "s3:ListBucket"
      ],
      "Resource": "*"
    }
  ]
}
```

### 5. 部署应用

```bash
# 给部署脚本执行权限
chmod +x scripts/deploy.sh

# 执行部署
./scripts/deploy.sh production
```

### 6. 配置反向代理（Nginx）

创建 Nginx 配置文件 `/etc/nginx/sites-available/aws-cdn`:

```nginx
server {
    listen 80;
    server_name your-domain.com;

    # 重定向到 HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate /path/to/certificate.crt;
    ssl_certificate_key /path/to/private.key;

    # 前端
    location / {
        proxy_pass http://localhost:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # 后端 API
    location /api {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

启用配置：

```bash
sudo ln -s /etc/nginx/sites-available/aws-cdn /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

### 7. 配置防火墙

```bash
# 允许 HTTP 和 HTTPS
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# 允许 SSH（如果使用）
sudo ufw allow 22/tcp

# 启用防火墙
sudo ufw enable
```

### 8. 设置自动启动

```bash
# 创建 systemd 服务
sudo nano /etc/systemd/system/aws-cdn.service
```

服务文件内容：

```ini
[Unit]
Description=AWS CDN Management Platform
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/path/to/aws_cdn
ExecStart=/usr/local/bin/docker-compose up -d
ExecStop=/usr/local/bin/docker-compose down
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
```

启用服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable aws-cdn
sudo systemctl start aws-cdn
```

## 监控和日志

### 查看日志

```bash
# 查看所有服务日志
docker-compose logs -f

# 查看特定服务日志
docker-compose logs -f backend
docker-compose logs -f frontend
docker-compose logs -f postgres
```

### 健康检查

```bash
# 检查后端健康状态
curl http://localhost:8080/health

# 检查服务状态
docker-compose ps
```

## 备份和恢复

### 数据库备份

```bash
# 创建备份
docker-compose exec postgres pg_dump -U postgres aws_cdn > backup_$(date +%Y%m%d_%H%M%S).sql

# 恢复备份
docker-compose exec -T postgres psql -U postgres aws_cdn < backup_20240101_120000.sql
```

### 自动备份脚本

创建 `/etc/cron.daily/aws-cdn-backup`:

```bash
#!/bin/bash
BACKUP_DIR="/backup/aws-cdn"
DATE=$(date +%Y%m%d_%H%M%S)
mkdir -p $BACKUP_DIR
docker-compose exec -T postgres pg_dump -U postgres aws_cdn | gzip > $BACKUP_DIR/backup_$DATE.sql.gz
# 保留最近 30 天的备份
find $BACKUP_DIR -name "backup_*.sql.gz" -mtime +30 -delete
```

## 安全建议

1. **使用 HTTPS**: 配置 SSL/TLS 证书
2. **数据库加密**: 启用 PostgreSQL SSL 连接
3. **密钥管理**: 使用 AWS Secrets Manager 或 HashiCorp Vault
4. **访问控制**: 配置防火墙规则
5. **定期更新**: 保持 Docker 镜像和依赖更新
6. **监控告警**: 集成 CloudWatch 或 Prometheus
7. **日志审计**: 定期检查访问日志

## 故障排查

### 常见问题

1. **服务无法启动**
   - 检查 Docker 服务状态: `sudo systemctl status docker`
   - 查看日志: `docker-compose logs`

2. **数据库连接失败**
   - 检查数据库服务: `docker-compose ps postgres`
   - 验证连接字符串
   - 检查网络连接

3. **AWS API 调用失败**
   - 验证 AWS 凭证
   - 检查 IAM 权限
   - 查看 AWS CloudTrail 日志

4. **证书验证失败**
   - 检查 DNS 记录
   - 确认 NS 服务器已更新
   - 查看 ACM 控制台

## 性能优化

1. **数据库优化**
   - 调整 PostgreSQL 配置
   - 添加适当的索引
   - 定期执行 VACUUM

2. **应用优化**
   - 启用 Gzip 压缩
   - 配置 CDN 缓存
   - 使用连接池

3. **资源限制**
   - 设置 Docker 资源限制
   - 监控资源使用情况

## 更新部署

```bash
# 拉取最新代码
git pull

# 重新构建镜像
docker-compose build

# 运行迁移（如有）
docker-compose run --rm backend go run cmd/migrate/main.go

# 重启服务
docker-compose up -d
```

## 回滚

```bash
# 切换到之前的版本
git checkout <previous-commit>

# 重新构建和部署
docker-compose build
docker-compose up -d
```

