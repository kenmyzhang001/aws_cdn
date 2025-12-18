<template>
  <el-container class="app-container">
    <el-header class="app-header">
      <div class="header-content">
        <h1>AWS CDN 管理平台</h1>
      </div>
    </el-header>
    <el-container>
      <el-aside width="200px" class="app-sidebar">
        <el-menu
          :default-active="activeMenu"
          router
          class="sidebar-menu"
        >
          <el-menu-item index="/domains">
            <el-icon><Link /></el-icon>
            <span>域名管理</span>
          </el-menu-item>
          <el-menu-item index="/redirects">
            <el-icon><RefreshRight /></el-icon>
            <span>重定向管理</span>
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
import { Link, RefreshRight } from '@element-plus/icons-vue'

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


