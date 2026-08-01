import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/store/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/views/Login.vue'),
      meta: { requiresGuest: true }
    },
    {
      path: '/register',
      name: 'Register',
      component: () => import('@/views/Register.vue'),
      meta: { requiresGuest: true }
    },
    {
      path: '/',
      component: () => import('@/layout/DashboardLayout.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: '',
          name: 'Devices',
          component: () => import('@/views/Devices.vue')
        },
        {
          path: 'settings',
          name: 'Settings',
          component: () => import('@/views/Settings.vue')
        }
      ]
    }
  ]
})

export function resolveAuthRedirect(
  meta: { requiresAuth?: unknown; requiresGuest?: unknown },
  authenticated: boolean,
) {
  if (meta.requiresAuth && !authenticated) {
    return { name: 'Login' }
  }
  if (meta.requiresGuest && authenticated) {
    return { name: 'Devices' }
  }
}

router.beforeEach((to) => {
  const auth = useAuthStore()
  return resolveAuthRedirect(to.meta, auth.isAuthenticated)
})

export default router
