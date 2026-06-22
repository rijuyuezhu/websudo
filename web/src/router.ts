import { createRouter, createWebHistory } from 'vue-router'
import { getSession } from './api'
import DashboardView from './views/DashboardView.vue'
import LoginView from './views/LoginView.vue'
import AskpassView from './views/AskpassView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: LoginView,
      meta: { public: true },
    },
    { path: '/', name: 'dashboard', component: DashboardView },
    {
      path: '/askpass/:id',
      name: 'askpass',
      component: AskpassView,
      props: true,
    },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.public) {
    return true
  }
  try {
    await getSession()
    return true
  } catch {
    return '/login'
  }
})

export default router
