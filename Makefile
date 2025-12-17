.PHONY: build run test clean docker-up docker-down migrate

# 构建后端
build:
	cd backend && go build -o ../bin/server ./cmd/server

# 运行后端
run:
	cd backend && go run ./cmd/server

# 运行测试
test:
	cd backend && go test ./...

# 清理
clean:
	rm -rf bin/
	rm -rf frontend/dist/
	rm -rf frontend/node_modules/

# 启动 Docker 服务
docker-up:
	docker-compose up -d

# 停止 Docker 服务
docker-down:
	docker-compose down

# 数据库迁移
migrate:
	cd backend && go run ./cmd/migrate

# 安装前端依赖
frontend-install:
	cd frontend && npm install

# 运行前端开发服务器
frontend-dev:
	cd frontend && npm run dev

# 构建前端
frontend-build:
	cd frontend && npm run build

