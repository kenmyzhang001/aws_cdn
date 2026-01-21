<template>
  <el-container class="app-container">
    <el-header class="app-header">
      <div class="header-content">
        <h1>CDN 管理平台</h1>
      </div>
    </el-header>
    <el-container>
      <el-aside width="220px" class="app-sidebar">
        <el-menu
          :default-active="activeMenu"
          router
          class="sidebar-menu"
          :default-openeds="['aws-cdn', 'cf-cdn']"
        >
          <!-- 分组管理（一级菜单，AWS 和 CF 共用） -->
          <el-menu-item index="/groups">
            <el-icon><Folder /></el-icon>
            <span>分组管理</span>
          </el-menu-item>

          <el-menu-item index="/cf-accounts">
              <el-icon><User /></el-icon>
              <span>CF 账号管理</span>
          </el-menu-item>



          <!-- AWS-CDN 一级菜单 -->
          <el-sub-menu index="aws-cdn">
            <template #title>
              <el-icon><Connection /></el-icon>
              <span>AWS-CDN</span>
            </template>
            <el-menu-item index="/domains">
              <el-icon><Link /></el-icon>
              <span>域名管理</span>
            </el-menu-item>
            <el-menu-item index="/download-packages">
              <el-icon><Download /></el-icon>
              <span>下载包管理</span>
            </el-menu-item>
          </el-sub-menu>

          <!-- CF-CDN 一级菜单 -->
          <el-sub-menu index="cf-cdn">
            <template #title>
              <el-icon><Box /></el-icon>
              <span>CF-CDN</span>
            </template>
            <el-menu-item index="/r2-buckets">
              <el-icon><Box /></el-icon>
              <span>R2 存储桶管理</span>
            </el-menu-item>
            <el-menu-item index="/r2-apk-links">
              <el-icon><Link /></el-icon>
              <span>下载包管理</span>
            </el-menu-item>
          </el-sub-menu>

          <!-- 自定义下载链接管理 -->
          <el-menu-item index="/custom-download-links">
            <el-icon><Link /></el-icon>
            <span>自定义下载链接</span>
          </el-menu-item>

          <!-- 审计日志 -->
          <el-menu-item index="/audit-logs">
            <el-icon><Document /></el-icon>
            <span>审计日志</span>
          </el-menu-item>
        </el-menu>
      </el-aside>
      <el-main class="app-main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { Link, RefreshRight, Connection, Download, Document, Folder, User, Box } from '@element-plus/icons-vue'

const route = useRoute()
const activeMenu = computed(() => route.path)
</script>

<style scoped>
.app-container {
  height: 100vh;
}

.app-header {
  background-color: #409eff;
  color: white;
  display: flex;
  align-items: center;
  padding: 0 20px;
}

.header-content h1 {
  margin: 0;
  font-size: 20px;
}

.app-sidebar {
  background-color: #f5f5f5;
  border-right: 1px solid #e4e7ed;
}

.sidebar-menu {
  border-right: none;
}

.app-main {
  padding: 20px;
  background-color: #fafafa;
}
</style>


