#!/bin/bash

# AWS CDN 部署脚本
# 使用方法: ./scripts/deploy.sh [production|staging]

set -e

ENV=${1:-staging}

echo "开始部署到 $ENV 环境..."

# 检查环境变量文件
if [ ! -f .env ]; then
    echo "错误: 未找到 .env 文件"
    echo "请复制 .env.example 为 .env 并填写配置"
    exit 1
fi

# 构建 Docker 镜像
echo "构建 Docker 镜像..."
docker-compose build

# 运行数据库迁移
echo "运行数据库迁移..."
docker-compose run --rm backend go run cmd/migrate/main.go

# 启动服务
echo "启动服务..."
if [ "$ENV" = "production" ]; then
    docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
else
    docker-compose up -d
fi

echo "部署完成！"
echo "后端 API: http://localhost:8080"
echo "前端界面: http://localhost:3000"

