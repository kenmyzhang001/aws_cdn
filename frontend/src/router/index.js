import { createRouter, createWebHistory } from 'vue-router'
import DomainList from '@/views/DomainList.vue'
import RedirectList from '@/views/RedirectList.vue'
import CloudFrontList from '@/views/CloudFrontList.vue'
import DownloadPackageList from '@/views/DownloadPackageList.vue'
import AuditLogList from '@/views/AuditLogList.vue'
import GroupList from '@/views/GroupList.vue'
import CfAccountList from '@/views/CfAccountList.vue'
import R2BucketList from '@/views/R2BucketList.vue'
import R2ApkLinkManager from '@/views/R2ApkLinkManager.vue'
import Login from '@/views/Login.vue'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: Login,
  },
  {
    path: '/',
    redirect: '/domains',
  },
  {
    path: '/domains',
    name: 'DomainList',
    component: DomainList,
  },
  {
    path: '/redirects',
    name: 'RedirectList',
    component: RedirectList,
  },
  {
    path: '/cloudfront',
    name: 'CloudFrontList',
    component: CloudFrontList,
  },
  {
    path: '/download-packages',
    name: 'DownloadPackageList',
    component: DownloadPackageList,
  },
  {
    path: '/audit-logs',
    name: 'AuditLogList',
    component: AuditLogList,
  },
  {
    path: '/groups',
    name: 'GroupList',
    component: GroupList,
  },
  {
    path: '/cf-accounts',
    name: 'CfAccountList',
    component: CfAccountList,
  },
  {
    path: '/r2-buckets',
    name: 'R2BucketList',
    component: R2BucketList,
  },
  {
    path: '/r2-apk-links',
    name: 'R2ApkLinkManager',
    component: R2ApkLinkManager,
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// 路由守卫：检查登录状态
router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('token')
  
  // 如果访问登录页且已登录，跳转到首页
  if (to.path === '/login' && token) {
    next('/domains')
    return
  }
  
  // 如果访问受保护页面且未登录，跳转到登录页
  if (to.path !== '/login' && !token) {
    next('/login')
    return
  }
  
  next()
})

export default router


