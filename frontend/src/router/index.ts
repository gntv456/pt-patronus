import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('@/views/Login.vue') },
    {
      path: '/',
      component: () => import('@/layouts/MainLayout.vue'),
      children: [
        { path: '', redirect: '/dashboard' },
        {
          path: 'dashboard',
          name: 'dashboard',
          component: () => import('@/views/Dashboard.vue'),
        },
        {
          path: 'sites',
          name: 'sites',
          component: () => import('@/views/Sites.vue'),
        },
        {
          path: 'search',
          name: 'search',
          component: () => import('@/views/Search.vue'),
        },
        {
          path: 'subscriptions',
          name: 'subscriptions',
          component: () => import('@/views/Subscriptions.vue'),
        },
        {
          path: 'workflow',
          name: 'workflow',
          component: () => import('@/views/Workflow.vue'),
        },
        {
          path: 'workflow-editor',
          name: 'workflow-editor',
          component: () => import('@/views/WorkflowEditor.vue'),
        },
        {
          path: 'rss',
          name: 'rss',
          component: () => import('@/views/Rss.vue'),
        },
        {
          path: 'media-settings',
          name: 'media-settings',
          component: () => import('@/views/MediaSettings.vue'),
        },
        {
          path: 'media-detail',
          name: 'media-detail',
          component: () => import('@/views/MediaDetail.vue'),
        },
        {
          path: 'downloaders',
          name: 'downloaders',
          component: () => import('@/views/Downloaders.vue'),
        },
        {
          path: 'reseed',
          name: 'reseed',
          component: () => import('@/views/Reseed.vue'),
        },
        {
          path: 'nas-server',
          name: 'nas-server',
          component: () => import('@/views/NasServer.vue'),
        },
        {
          path: 'entertainment',
          name: 'entertainment',
          component: () => import('@/views/Entertainment.vue'),
        },
        {
          path: 'freeleech',
          name: 'freeleech',
          component: () => import('@/views/Freeleech.vue'),
        },
        {
          path: 'announcements',
          name: 'announcements',
          component: () => import('@/views/Announcements.vue'),
        },
        {
          path: 'savept',
          name: 'savept',
          component: () => import('@/views/Savept.vue'),
        },
        {
          path: 'harem',
          name: 'harem',
          component: () => import('@/views/Harem.vue'),
        },
        {
          path: 'assessment',
          name: 'assessment',
          component: () => import('@/views/Assessment.vue'),
        },
        {
          path: 'medal-wall',
          name: 'medal-wall',
          component: () => import('@/views/MedalWall.vue'),
        },
        {
          path: 'messages',
          name: 'messages',
          component: () => import('@/views/Messages.vue'),
        },
        {
          path: 'recommend',
          name: 'recommend',
          component: () => import('@/views/Recommend.vue'),
        },
        {
          path: 'files',
          name: 'files',
          component: () => import('@/views/FileManager.vue'),
        },
        {
          path: 'backup',
          name: 'backup',
          component: () => import('@/views/Backup.vue'),
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('@/views/Settings.vue'),
        },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (to.name !== 'login' && !auth.authenticated) return { name: 'login' }
  // 刷新后内存 user 丢失（只 token 在 localStorage）：token 有效则拉 status 恢复 user，
  // 否则顶栏会回退到 fallback 用户名、VIP 徽标也会失真。
  if (auth.authenticated && !auth.user) {
    await auth.restoreUser()
  }
})

export default router
