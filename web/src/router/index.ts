import { createRouter, createWebHistory, type RouteLocationNormalized } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { IMAGE_LIST_KINDS, IMAGE_LIST_TITLES } from '@/lib/api'

export async function routeGuard(to: RouteLocationNormalized) {
  const auth = useAuthStore()

  // Only check setup status when navigating to setup or login (avoids an API
  // call on every navigation). The result is cached in the auth store after the
  // first successful call.
  if (to.name === 'setup' || to.name === 'login' || to.name === 'landing' || to.name === 'dashboard' || to.name === 'not-found' || to.name === 'docs' || to.name === 'legal') {
    try {
      const setupRequired = await auth.checkSetupRequired()
      if (setupRequired && to.name !== 'setup') {
        auth.logout()
        return { name: 'setup' }
      }
      if (!setupRequired && to.name === 'setup') {
        return { name: 'login' }
      }
    } catch {
      // If we can't check, continue
    }
  }

  // Gate all public pages when DISABLE_PUBLIC_PAGES is set
  if (auth.disablePublicPages && !auth.isAuthenticated) {
    const publicPages = ['landing', 'legal', 'docs', 'not-found']
    if (publicPages.includes(to.name as string)) {
      return { name: 'login' }
    }
  }

  // Authenticated users hitting landing → redirect to admin dashboard
  if (to.name === 'landing') {
    if (auth.isAdminSession) {
      return { name: 'dashboard' }
    }
    if (auth.isApiKeySession) {
      return { name: 'individual-image-settings' }
    }
  }

  // Admin routes require admin session (not API key session)
  if (to.matched.some((r) => r.meta.requiresAuth)) {
    if (!auth.isAdminSession) {
      if (auth.isApiKeySession) {
        return { name: 'individual-image-settings' }
      }
      return { name: 'login' }
    }
  }

  // API key routes require API key session
  if (to.matched.some((r) => r.meta.requiresApiKey) && !auth.isApiKeySession) {
    return { name: 'login' }
  }

  // Redirect away from login/setup if already authenticated
  if (to.name === 'login' || to.name === 'setup') {
    if (auth.isAdminSession) {
      return { name: 'dashboard' }
    }
    if (auth.isApiKeySession) {
      return { name: 'individual-image-settings' }
    }
  }
}

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'landing',
      component: () => import('@/views/LandingView.vue'),
    },
    {
      path: '/admin',
      component: () => import('@/layouts/DashboardLayout.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: '',
          name: 'dashboard',
          component: () => import('@/views/DashboardView.vue'),
          meta: { title: 'Dashboard' },
        },
        // Admin image gallery — single route per kind, all sharing the
        // AdminListView wrapper + the per-kind config table in @/lib/api.
        // Route name + path are kept plural for backward compat with
        // existing router.push({ name: 'posters' }) callers.
        ...IMAGE_LIST_KINDS.map((kind) => ({
          path: `${kind === 'logo' ? 'logo' : kind}s`,
          name: `${kind === 'logo' ? 'logo' : kind}s`,
          component: () => import('@/views/AdminListView.vue'),
          props: { kind },
          meta: { title: IMAGE_LIST_TITLES[kind] },
        })),
        {
          path: 'keys',
          redirect: { name: 'settings-api' },
        },
        {
          path: 'settings',
          redirect: { name: 'settings-api' },
        },
        {
          path: 'settings/api',
          name: 'settings-api',
          component: () => import('@/views/SettingsView.vue'),
          meta: { title: 'API' },
          // /admin/settings/api without a ?tab= always lands on the default
          // tab so reload + bookmark + share-link all carry the active view in
          // the URL itself.
          beforeEnter: (to) => {
            if (!('tab' in to.query)) {
              return { name: 'settings-api', query: { ...to.query, tab: 'free-api-key' } }
            }
          },
        },
        {
          path: 'settings/backup',
          name: 'settings-backup',
          component: () => import('@/views/SettingsView.vue'),
          meta: { title: 'Backup' },
        },
        {
          path: 'settings/image',
          name: 'settings-image',
          component: () => import('@/views/SettingsView.vue'),
          meta: { title: 'Global Image Settings' },
          // /admin/settings/image without a ?tab= always lands on the default
          // form tab (image-settings) so reload + bookmark + share-link all
          // carry the active view in the URL itself.
          beforeEnter: (to) => {
            if (!('tab' in to.query)) {
              return { name: 'settings-image', query: { ...to.query, tab: 'image-settings' } }
            }
          },
        },
      ],
    },
    {
      path: '/individual-image-settings',
      component: () => import('@/layouts/DashboardLayout.vue'),
      meta: { requiresApiKey: true },
      children: [
        {
          path: '',
          name: 'individual-image-settings',
          component: () => import('@/views/KeySettingsView.vue'),
          meta: { title: 'Individual Image Settings' },
        },
      ],
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
    },
    {
      path: '/setup',
      name: 'setup',
      component: () => import('@/views/SetupView.vue'),
    },
    {
      path: '/docs',
      name: 'docs',
      component: () => import('@/views/DocsView.vue'),
    },
    {
      path: '/legal',
      name: 'legal',
      component: () => import('@/views/LegalView.vue'),
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/views/NotFoundView.vue'),
    },
  ],
})

router.beforeEach(routeGuard)

export default router
