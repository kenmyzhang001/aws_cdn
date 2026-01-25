# AWS CDN 管理平台

一个完整的 AWS CDN 管理平台，支持域名管理和重定向管理功能。使用 Golang + Vue.js 构建，可部署到生产环境。

## 功能特性

### 1. 域名管理模块

- ✅ 支持将其他域名提供商购买的域名转入到 AWS Route53
- ✅ 查看已转入的域名列表
- ✅ 生成域名的 SSL 证书（通过 AWS ACM）
- ✅ 查看域名转入后对应的 NS 服务器配置
- ✅ 查看域名的转入状态（待转入、转入中、已完成、失败）
- ✅ 查看域名的证书状态（未申请、验证中、已签发、失败）

### 2. 域名重定向管理

- ✅ 支持设置域名的重定向规则，输入源域名和目标域名
- ✅ 一个源域名可以对应多个目标域名
- ✅ 采用轮询方式分配流量（基于客户端 IP 和时间的哈希算法）
- ✅ 浏览器端缓存机制（通过 HTTP 缓存头）
- ✅ 部署在 AWS S3 + CloudFront 上
- ✅ 支持将域名绑定到 CloudFront 分发

### 3. Cloudflare R2 存储管理 ⭐ 新功能

- ✅ **R2 存储桶管理**
  - 创建和管理 Cloudflare R2 存储桶
  - 支持多 Cloudflare 账号管理
  - 自动生成 R2 访问凭证
  
- ✅ **R2 文件管理**
  - 上传/下载/删除文件
  - 文件列表查看和搜索
  - 支持大文件上传（分片上传）
  
- ✅ **自定义域名配置**
  - 为 R2 存储桶添加自定义域名
  - 自动创建 DNS CNAME 记录
  - 自动配置 CORS Transform Rule（跨域支持）
  - **自动创建 WAF 安全规则** 🔥
    - VPN 白名单（跳过验证码）
    - IDM 高频下载豁免（支持多线程下载）
    - 智能威胁分过滤（威胁分 ≤ 50）
  - **自动创建 Page Rule（缓存优化）** 🔥
    - Cache Everything（强制缓存所有内容）
    - Edge Cache TTL: 30 天（节点缓存）
    - Browser Cache TTL: 1 年（浏览器缓存）
    - **节省 95-99% 源站流量费用** 💰
  - **自动启用网络优化** 🚀
    - Smart Tiered Cache（智能分层缓存，节点接力）
    - HTTP/3 (QUIC)（抗丢包，移动网络优化）
    - 0-RTT（秒连，回头客优化）
  
- ✅ **缓存规则管理**
  - 配置自定义缓存规则
  - 按文件扩展名设置缓存时间
  - 支持 Browser TTL 和 Edge TTL

## 技术栈

### 后端
- **Golang 1.21+**
- **Gin** - Web 框架
- **GORM** - ORM 框架
- **PostgreSQL** - 数据库
- **AWS SDK for Go** - AWS 服务集成

### 前端
- **Vue 3** - 前端框架
- **Vite** - 构建工具
- **Element Plus** - UI 组件库
- **Vue Router** - 路由管理
- **Pinia** - 状态管理
- **Axios** - HTTP 客户端

### 基础设施
- **Docker & Docker Compose** - 容器化部署
- **Nginx** - 前端 Web 服务器
- **AWS Services**:
  - Route53 - DNS 管理
  - ACM - SSL 证书管理
  - CloudFront - CDN 分发
  - S3 - 静态资源存储
- **Cloudflare Services**:
  - R2 - 对象存储
  - DNS - 域名解析
  - WAF - Web 应用防火墙
  - Transform Rules - 请求/响应转换

## 项目结构

```
aws_cdn/
├── cmd/
│   └── server/
│       └── main.go              # 应用入口
├── internal/
│   ├── config/                  # 配置管理
│   ├── database/                # 数据库初始化
│   ├── models/                  # 数据模型
│   ├── handlers/                # HTTP 处理器
│   ├── services/                # 业务逻辑层
│   │   └── aws/                 # AWS 服务集成
│   └── router/                  # 路由配置
├── frontend/                    # Vue.js 前端应用
│   ├── src/
│   │   ├── api/                 # API 接口
│   │   ├── views/               # 页面组件
│   │   ├── router/              # 路由配置
│   │   └── App.vue              # 根组件
│   ├── package.json
│   └── vite.config.js
├── docker-compose.yml           # Docker Compose 配置
├── Dockerfile.backend          # 后端 Dockerfile
├── go.mod                      # Go 模块定义
└── README.md                   # 项目文档
```

## 快速开始

### 前置要求

- Go 1.21+
- Node.js 18+
- Docker & Docker Compose
- PostgreSQL 15+ (或使用 Docker)
- AWS 账户及访问凭证

### 1. 克隆项目

```bash
git clone <repository-url>
cd aws_cdn
```

### 2. 配置环境变量

复制 `.env.example` 为 `.env` 并填写配置：

```bash
cp .env.example .env
```

编辑 `.env` 文件，配置以下内容：

```env
# 数据库配置
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=aws_cdn
DB_SSLMODE=disable

# 服务器配置
SERVER_PORT=8080
SERVER_MODE=release

# JWT 配置
JWT_SECRET=your-secret-key-change-in-production
JWT_EXPIRE_HOURS=24

# AWS 配置
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=your-access-key-id
AWS_SECRET_ACCESS_KEY=your-secret-access-key

# CloudFront 配置
CLOUDFRONT_DISTRIBUTION_ID=your-distribution-id

# S3 配置
S3_BUCKET_NAME=your-bucket-name

# Cloudflare 配置（可选，用于 R2 功能）
CLOUDFLARE_API_TOKEN=your-cloudflare-api-token
CLOUDFLARE_API_EMAIL=your-cloudflare-email
CLOUDFLARE_API_KEY=your-cloudflare-api-key
```

### 3. 使用 Docker Compose 启动（推荐）

```bash
# 启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down
```

服务启动后：
- 后端 API: http://localhost:8080
- 前端界面: http://localhost:3000
- PostgreSQL: localhost:5432

### 4. 本地开发

#### 后端开发

```bash
# 安装依赖
go mod download

# 运行服务
go run cmd/server/main.go
```

#### 前端开发

```bash
cd frontend

# 安装依赖
npm install

# 启动开发服务器
npm run dev
```

## API 文档

### 域名管理 API

#### 转入域名
```http
POST /api/v1/domains
Content-Type: application/json

{
  "domain_name": "example.com",
  "registrar": "GoDaddy"
}
```

#### 获取域名列表
```http
GET /api/v1/domains?page=1&page_size=10
```

#### 获取域名详情
```http
GET /api/v1/domains/{id}
```

#### 获取 NS 服务器
```http
GET /api/v1/domains/{id}/ns-servers
```

#### 获取域名状态
```http
GET /api/v1/domains/{id}/status
```

#### 生成证书
```http
POST /api/v1/domains/{id}/certificate
```

#### 获取证书状态
```http
GET /api/v1/domains/{id}/certificate/status
```

### 重定向管理 API

#### 创建重定向规则
```http
POST /api/v1/redirects
Content-Type: application/json

{
  "source_domain": "example.com",
  "target_urls": [
    "https://target1.com",
    "https://target2.com"
  ]
}
```

#### 获取重定向规则列表
```http
GET /api/v1/redirects?page=1&page_size=10
```

#### 获取重定向规则详情
```http
GET /api/v1/redirects/{id}
```

#### 添加目标 URL
```http
POST /api/v1/redirects/{id}/targets
Content-Type: application/json

{
  "target_url": "https://target3.com"
}
```

#### 删除目标 URL
```http
DELETE /api/v1/redirects/targets/{id}
```

#### 绑定域名到 CloudFront
```http
POST /api/v1/redirects/{id}/bind-cloudfront
Content-Type: application/json

{
  "distribution_id": "E1234567890ABC",
  "domain_name": "example.com"
}
```

#### 删除重定向规则
```http
DELETE /api/v1/redirects/{id}
```

## 生产环境部署

### 1. AWS 资源准备

在部署前，需要准备以下 AWS 资源：

1. **IAM 用户/角色**：具有以下权限
   - Route53: 创建和管理托管区域
   - ACM: 请求和管理证书
   - CloudFront: 创建和管理分发
   - S3: 创建和管理存储桶

2. **S3 存储桶**：用于存储静态资源

3. **VPC 和子网**（可选）：如果使用私有部署

### 2. 数据库迁移

```bash
# 使用 Docker 运行迁移
docker-compose exec backend go run cmd/migrate/main.go

# 或本地运行
go run cmd/migrate/main.go
```

### 3. 构建和部署

#### 使用 Docker

```bash
# 构建镜像
docker-compose build

# 启动服务
docker-compose up -d
```

#### 使用 Kubernetes

```bash
# 创建命名空间
kubectl create namespace aws-cdn

# 部署配置
kubectl apply -f k8s/

# 查看状态
kubectl get pods -n aws-cdn
```

### 4. 环境变量配置

生产环境需要设置以下环境变量：

- `SERVER_MODE=release` - 生产模式
- `JWT_SECRET` - 强随机密钥
- AWS 凭证（建议使用 IAM 角色而非访问密钥）
- 数据库连接信息

### 5. 安全建议

1. **使用 HTTPS**：配置 SSL/TLS 证书
2. **API 认证**：实现 JWT 认证（当前版本未实现，需要添加）
3. **CORS 配置**：限制允许的源
4. **数据库加密**：启用 PostgreSQL SSL 连接
5. **密钥管理**：使用 AWS Secrets Manager 或类似服务
6. **日志监控**：集成 CloudWatch 或类似服务

## 工作原理

### 域名转入流程

1. 用户提交域名转入请求
2. 系统在 Route53 创建托管区域
3. 返回 NS 服务器配置给用户
4. 用户在原注册商处更新 NS 服务器
5. 系统异步请求 ACM 证书
6. 等待 DNS 验证完成
7. 证书签发后更新状态

### 重定向轮询机制

1. 用户访问源域名
2. 系统基于客户端 IP 和时间生成哈希值
3. 根据哈希值选择目标 URL（轮询）
4. 设置 HTTP 缓存头（Cache-Control）
5. 浏览器缓存选择结果 1 小时
6. 重定向到选定的目标 URL

## 故障排查

### 常见问题

1. **证书验证失败**
   - 检查 DNS 记录是否正确配置
   - 确认 NS 服务器已更新
   - 查看 ACM 控制台的验证状态

2. **CloudFront 分发创建失败**
   - 检查证书是否已签发
   - 确认 S3 存储桶存在
   - 验证 IAM 权限

3. **数据库连接失败**
   - 检查数据库服务是否运行
   - 验证连接字符串
   - 确认网络可达性

## 开发计划

- [ ] 实现用户认证和授权
- [ ] 添加操作日志记录
- [ ] 实现更精确的轮询算法（基于浏览器 localStorage）
- [ ] 添加监控和告警
- [ ] 支持批量域名操作
- [ ] 实现域名转出功能
- [ ] 添加 API 文档（Swagger）

## 文档索引

### 快速入门
- [QUICKSTART.md](./QUICKSTART.md) - 快速开始指南
- [DEPLOYMENT.md](./DEPLOYMENT.md) - 部署文档

### Cloudflare R2 相关
- [CLOUDFLARE_R2_CDN_TUTORIAL.md](./CLOUDFLARE_R2_CDN_TUTORIAL.md) - Cloudflare R2 CDN 完整教程
- [R2_FILE_MANAGEMENT.md](./R2_FILE_MANAGEMENT.md) - R2 文件管理指南
- [AUTO_CACHE_RULE_CONFIGURATION.md](./AUTO_CACHE_RULE_CONFIGURATION.md) - 自动缓存规则配置
- [CORS_CONFIGURATION.md](./CORS_CONFIGURATION.md) - CORS 跨域配置

### 安全配置 🔥
- [AUTO_WAF_RULE_CONFIGURATION.md](./AUTO_WAF_RULE_CONFIGURATION.md) - 自动 WAF 规则配置（新）
- [CLOUDFLARE_WAF_CONFIGURATION.md](./CLOUDFLARE_WAF_CONFIGURATION.md) - Cloudflare WAF 手动配置指南
- [CLOUDFLARE_PERMISSIONS.md](./CLOUDFLARE_PERMISSIONS.md) - Cloudflare API 权限配置

### 缓存优化 💰
- [CLOUDFLARE_PAGE_RULES.md](./CLOUDFLARE_PAGE_RULES.md) - Page Rules 缓存优化（新）
- [AUTO_CACHE_RULE_CONFIGURATION.md](./AUTO_CACHE_RULE_CONFIGURATION.md) - 自动缓存规则配置

### 网络优化 🚀
- [CLOUDFLARE_NETWORK_OPTIMIZATION.md](./CLOUDFLARE_NETWORK_OPTIMIZATION.md) - 网络与缓存优化配置（新）
  - Smart Tiered Cache（智能分层缓存）
  - HTTP/3 (QUIC) 抗丢包优化
  - 0-RTT 秒连优化

### 监控和运维
- [MONITOR_QUICKSTART.md](./MONITOR_QUICKSTART.md) - 监控快速入门
- [MONITOR_CONFIGURATION.md](./MONITOR_CONFIGURATION.md) - 监控配置详解
- [CHANGELOG_MONITOR.md](./CHANGELOG_MONITOR.md) - 变更日志监控
- [SPEED_PROBE_API.md](./SPEED_PROBE_API.md) - 速度探测 API

### 系统优化
- [SYSTEM_OPTIMIZATION.md](./SYSTEM_OPTIMIZATION.md) - 系统优化指南
- [SCHEDULED_TASKS_CONFIGURATION.md](./SCHEDULED_TASKS_CONFIGURATION.md) - 定时任务配置

## 亮点功能 🌟

### 1. 自动 WAF 安全规则

当你添加 R2 自定义域名时，系统会**自动创建 WAF 安全规则**，无需手动配置！

**自动配置内容:**
- ✅ VPN 白名单 - 威胁分 ≤ 50 的用户无需验证码
- ✅ IDM 高频下载豁免 - 支持 IDM/ADM 多线程下载
- ✅ 智能安全防护 - 恶意流量仍然被拦截
- ✅ 零配置 - 添加域名即可自动生效

**规则详情:**
```
触发条件: (cf.threat_score le 50) and (http.host eq "你的域名") and (http.request.uri.path.extension eq "apk")
执行动作: Skip - Rate Limiting, Bot Fight Mode, WAF Managed Rules
效果: VPN 用户可直接下载，IDM 可开启 32 线程
```

详见 [AUTO_WAF_RULE_CONFIGURATION.md](./AUTO_WAF_RULE_CONFIGURATION.md)

### 2. 自动 Page Rule（缓存优化）🔥

当你添加 R2 自定义域名时，系统会**自动创建 Page Rule**，强制缓存文件，大幅节省流量费用！

**自动配置内容:**
- ✅ **Cache Everything** - 强制缓存所有内容
- ✅ **Edge Cache TTL: 30 天** - 文件在 Cloudflare 节点存 30 天
- ✅ **Browser Cache TTL: 1 年** - 用户浏览器缓存 1 年
- ✅ **Rocket Loader: Off** - 防止文件损坏
- ✅ **节省 95-99% 流量费用** 💰

**效果示例:**

| 场景 | 无缓存 | 有 Page Rule | 节省 |
|-----|--------|-------------|------|
| 100MB APK, 1000 次下载/天 | 100GB/天 | ~100MB/天 | 99.9% |
| 月流量费用（¥0.5/GB） | ¥1,500 | ¥1.5 | **¥1,498.5** 💰 |

详见 [CLOUDFLARE_PAGE_RULES.md](./CLOUDFLARE_PAGE_RULES.md)


## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！

## 联系方式

如有问题，请提交 Issue 或联系维护者。


