.PHONY: build build-agent run run-agent clean help

# 默认目标
.DEFAULT_GOAL := help

# 构建服务器
build:
	@echo "🔨 构建服务器..."
	@cd cmd/server && go build -o ../../bin/server
	@echo "✅ 服务器构建完成: bin/server"

# 构建 Agent
build-agent:
	@echo "🔨 构建 Agent..."
	@cd cmd/agent && go build -o ../../bin/agent
	@echo "✅ Agent 构建完成: bin/agent"

# 构建所有
build-all: build build-agent
	@echo "✅ 所有程序构建完成"

# 运行服务器
run:
	@echo "🚀 启动服务器..."
	@go run cmd/server/main.go

# 运行 Agent（开发模式）
run-agent:
	@echo "🚀 启动 Agent（开发模式）..."
	@go run cmd/agent/main.go -server http://localhost:8080 -interval 1m

# 运行 Agent（指定服务器）
run-agent-prod:
	@echo "🚀 启动 Agent（生产模式）..."
	@go run cmd/agent/main.go -server $(SERVER) -interval 20m

# 清理构建文件
clean:
	@echo "🧹 清理构建文件..."
	@rm -rf bin/
	@echo "✅ 清理完成"

# 创建 bin 目录
bin:
	@mkdir -p bin

# 帮助信息
help:
	@echo "可用命令："
	@echo "  make build           - 构建服务器"
	@echo "  make build-agent     - 构建 Agent"
	@echo "  make build-all       - 构建所有程序"
	@echo "  make run             - 运行服务器"
	@echo "  make run-agent       - 运行 Agent（开发模式，1分钟间隔）"
	@echo "  make run-agent-prod  - 运行 Agent（生产模式，需要指定 SERVER=http://...）"
	@echo "  make clean           - 清理构建文件"
	@echo "  make help            - 显示此帮助信息"
	@echo ""
	@echo "示例："
	@echo "  make build-all"
	@echo "  make run-agent"
	@echo "  SERVER=http://api.example.com make run-agent-prod"
