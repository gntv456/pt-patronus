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
          path: 'plugins',
          name: 'plugins',
          component: () => import('@/views/Plugins.vue'),
        },
        {
          path: 'plugins/view/:pluginId/:viewId',
          name: 'plugin-view',
          component: () => import('@/views/PluginView.vue'),
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
    // 404 兜底：未匹配的路径统一回到仪表盘
    { path: '/:pathMatch(.*)*', redirect: { name: 'dashboard' } },
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

// 懒加载 chunk 加载失败兜底（限流 429 / 部署后旧 hash / 网络抖动）：路由组件
// import() 失败时 Vue Router 抛 ChunkLoadError，页面会停在白屏。检测到后自动
// 刷新一次（令牌桶/缓存窗口通常已过，刷新即恢复）；仅刷一次避免死循环。
let reloadedForChunkError = false
router.onError((error) => {
  if (
    !reloadedForChunkError &&
    /Loading chunk|Failed to fetch dynamically imported module|Loading CSS chunk|Importing a module script failed/i.test(
      error.message,
    )
  ) {
    reloadedForChunkError = true
    location.reload()
  }
})

export default router
