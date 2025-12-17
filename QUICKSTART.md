# 快速启动指南

本指南帮助您快速启动和运行 AWS CDN 管理平台。

## 5 分钟快速开始

### 1. 克隆项目

```bash
git clone <repository-url>
cd aws_cdn
```

### 2. 配置环境变量

```bash
cp .env.example .env
```

编辑 `.env` 文件，至少配置以下必需项：

```env
# AWS 配置（必需）
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=your-access-key-id
AWS_SECRET_ACCESS_KEY=your-secret-access-key

# 数据库配置（如果使用 Docker Compose，可以保持默认值）
DB_HOST=postgres
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=aws_cdn
DB_SSLMODE=disable
```

### 3. 启动服务

```bash
# 使用 Docker Compose（推荐）
docker-compose up -d

# 查看日志
docker-compose logs -f
```

### 4. 访问应用

- **前端界面**: http://localhost:3000
- **后端 API**: http://localhost:8080
- **健康检查**: http://localhost:8080/health

## 本地开发模式

### 后端开发

```bash
# 1. 启动数据库（如果还没有）
docker-compose up -d postgres

# 2. 安装 Go 依赖
go mod download

# 3. 运行数据库迁移
go run cmd/migrate/main.go

# 4. 启动后端服务
go run cmd/server/main.go
```

### 前端开发

```bash
cd frontend

# 1. 安装依赖
npm install

# 2. 启动开发服务器
npm run dev
```

前端开发服务器会在 http://localhost:3000 启动，并自动代理 API 请求到后端。

## 测试功能

### 1. 测试域名转入

1. 打开前端界面: http://localhost:3000
2. 点击"转入域名"
3. 输入域名和原注册商
4. 提交后查看 NS 服务器配置

### 2. 测试重定向

1. 点击"重定向管理"
2. 创建新的重定向规则
3. 添加多个目标 URL
4. 绑定到 CloudFront（需要先配置）

## 常见问题

### Q: 数据库连接失败？

**A:** 确保 PostgreSQL 服务正在运行：

```bash
docker-compose ps postgres
docker-compose logs postgres
```

### Q: AWS API 调用失败？

**A:** 检查以下内容：

1. AWS 凭证是否正确
2. IAM 权限是否足够
3. 区域设置是否正确

### Q: 前端无法访问后端 API？

**A:** 检查：

1. 后端服务是否运行: `curl http://localhost:8080/health`
2. 前端代理配置是否正确
3. CORS 配置是否正确

### Q: 证书验证失败？

**A:** 证书验证需要：

1. 域名已正确转入 Route53
2. NS 服务器已更新
3. DNS 记录已传播（可能需要几分钟到几小时）

## 下一步

- 查看 [README.md](README.md) 了解完整功能
- 查看 [DEPLOYMENT.md](DEPLOYMENT.md) 了解生产环境部署
- 查看 [ARCHITECTURE.md](ARCHITECTURE.md) 了解系统架构

## 获取帮助

- 提交 Issue: [GitHub Issues](https://github.com/your-repo/issues)
- 查看文档: [完整文档](README.md)

