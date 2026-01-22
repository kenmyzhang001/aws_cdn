# 速度探测 Agent

独立的速度探测代理程序，用于定时探测所有链接的下载速度并上报结果到服务器。

## 功能特性

- ✅ 自动获取所有需要探测的链接（通过 `/api/v1/all-links` 接口）
- ✅ 并行探测所有链接的下载速度
- ✅ 批量上报探测结果到服务器
- ✅ 支持自定义探测间隔、超时时间等参数
- ✅ 详细的日志输出
- ✅ 自动去重URL

## 编译

```bash
# 在项目根目录下执行
cd cmd/agent
go build -o speed-probe-agent

# 或者使用 Makefile
make build-agent
```

## 使用方法

### 基本使用

```bash
./speed-probe-agent
```

### 指定服务器地址

```bash
./speed-probe-agent -server http://your-server.com:8080
```

### 完整参数示例

```bash
./speed-probe-agent \
  -server http://localhost:8080 \
  -interval 20m \
  -timeout 30s \
  -max-size 10485760 \
  -speed-threshold 100.0
```

## 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-server` | `http://localhost:8080` | 服务器地址 |
| `-interval` | `20m` | 探测间隔（支持: s秒, m分钟, h小时） |
| `-timeout` | `30s` | 单次探测超时时间 |
| `-max-size` | `10485760` | 最大下载文件大小（10MB） |
| `-speed-threshold` | `100.0` | 速度阈值（KB/s），低于此值视为失败 |

## 运行示例

### 开发环境

```bash
# 连接到本地服务器，每20分钟探测一次
./speed-probe-agent -server http://localhost:8080 -interval 20m
```

### 生产环境

```bash
# 连接到生产服务器，每15分钟探测一次
./speed-probe-agent \
  -server https://api.example.com \
  -interval 15m \
  -timeout 45s \
  -speed-threshold 150.0
```

## 后台运行

### 使用 nohup

```bash
nohup ./speed-probe-agent -server http://your-server.com:8080 > agent.log 2>&1 &
```

### 使用 systemd（推荐）

创建服务文件 `/etc/systemd/system/speed-probe-agent.service`:

```ini
[Unit]
Description=Speed Probe Agent
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/path/to/agent
ExecStart=/path/to/agent/speed-probe-agent \
  -server http://your-server.com:8080 \
  -interval 20m \
  -timeout 30s \
  -speed-threshold 100.0
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable speed-probe-agent
sudo systemctl start speed-probe-agent
sudo systemctl status speed-probe-agent
```

查看日志：

```bash
sudo journalctl -u speed-probe-agent -f
```

### 使用 Docker

创建 `Dockerfile`:

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN cd cmd/agent && go build -o /speed-probe-agent

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /speed-probe-agent /usr/local/bin/
ENTRYPOINT ["speed-probe-agent"]
```

构建并运行：

```bash
docker build -t speed-probe-agent .
docker run -d --name agent \
  --restart=always \
  speed-probe-agent \
  -server http://your-server.com:8080 \
  -interval 20m
```

## 日志示例

```
2026/01/22 12:00:00 🚀 Agent 启动
   服务器地址: http://localhost:8080
   探测间隔: 20m0s
   探测超时: 30s
   最大文件大小: 10 MB
   速度阈值: 100.00 KB/s
2026/01/22 12:00:00 ⏰ 开始首次探测...
2026/01/22 12:00:00 📋 获取到 45 个链接
2026/01/22 12:00:00 🔍 去重后需要探测 38 个URL
2026/01/22 12:00:01    [1/38] 探测: https://example.com/file1.apk
2026/01/22 12:00:03    ✓ 成功 | 速度: 523.45 KB/s | 耗时: 1234 ms
2026/01/22 12:00:03    [2/38] 探测: https://example.com/file2.apk
2026/01/22 12:00:05    ✓ 成功 | 速度: 432.18 KB/s | 耗时: 1456 ms
...
2026/01/22 12:05:30 📤 上报探测结果...
2026/01/22 12:05:31 ✅ 上报成功
2026/01/22 12:05:31 📊 本次探测完成
   总耗时: 5m31s
   探测总数: 38
   成功: 35 (92.1%)
   失败: 3 (7.9%)

2026/01/22 12:20:00 ⏰ 开始定时探测...
...
```

## 工作流程

1. **获取链接列表**
   - 调用 `GET /api/v1/all-links` 获取所有链接
   - 包括：下载包、自定义下载链接、R2自定义域名

2. **URL去重**
   - 将所有类型的链接合并
   - 去除重复的URL

3. **逐个探测**
   - 下载每个URL（最多10MB）
   - 记录下载速度、文件大小、耗时
   - 根据速度阈值判断成功/失败

4. **批量上报**
   - 调用 `POST /api/v1/speed-probe/report-batch` 上报所有结果
   - 服务器会自动识别客户端IP

5. **等待下次探测**
   - 按设定的间隔等待
   - 重复上述流程

## 多实例部署

可以在不同网络环境或地区部署多个 Agent 实例，以便：

- 从不同IP测试链接可达性
- 评估不同地区的访问速度
- 提高监控覆盖范围

每个实例可以使用不同的配置：

```bash
# 实例1 - 快速探测
./speed-probe-agent -server http://api.com -interval 10m -timeout 20s

# 实例2 - 深度探测
./speed-probe-agent -server http://api.com -interval 30m -timeout 60s -max-size 52428800
```

## 故障排查

### Agent 无法连接服务器

```bash
# 检查网络连通性
curl http://your-server.com:8080/health

# 检查防火墙规则
# 确保服务器允许 Agent 的IP访问
```

### 探测超时频繁

```bash
# 增加超时时间
./speed-probe-agent -timeout 60s

# 减小最大文件大小
./speed-probe-agent -max-size 5242880
```

### 上报失败

```bash
# 检查 API 是否可访问（注意：该接口无需认证）
curl -X POST http://your-server.com:8080/api/v1/speed-probe/report-batch \
  -H "Content-Type: application/json" \
  -d '{"results":[]}'
```

## 监控建议

1. **日志监控**：定期检查 agent 日志，确认探测正常执行
2. **告警设置**：在服务器端设置告警阈值
3. **多点部署**：在多个地区部署 agent，全面评估链接质量
4. **定期重启**：建议每周重启一次 agent（通过 cron 或 systemd timer）

## 注意事项

1. Agent 会下载链接内容进行速度测试，会产生流量消耗
2. 建议根据链接数量和网络状况调整探测间隔
3. 速度阈值应根据实际业务需求设置
4. Agent 会自动识别客户端IP，无需手动配置
5. 确保 Agent 所在网络能够访问目标链接
