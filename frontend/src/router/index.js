import { createRouter, createWebHistory } from 'vue-router'
import DomainList from '@/views/DomainList.vue'
import RedirectList from '@/views/RedirectList.vue'

const routes = [
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
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router

