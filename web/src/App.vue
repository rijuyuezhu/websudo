<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { ApiError, getSession, logout } from './api'

const router = useRouter()
const route = useRoute()
const ready = ref(false)
const authenticated = ref(false)
const message = ref('')

async function refreshSession() {
  try {
    await getSession()
    authenticated.value = true
    if (route.path === '/login') {
      await router.replace('/')
    }
  } catch (error) {
    authenticated.value = false
    if (!(error instanceof ApiError) || error.status !== 401) {
      message.value = 'Unable to reach websudo.'
    }
    if (route.path !== '/login') {
      await router.replace('/login')
    }
  } finally {
    ready.value = true
  }
}

async function handleLogout() {
  await logout().catch(() => undefined)
  authenticated.value = false
  await router.replace('/login')
}

function handleLoggedIn() {
  authenticated.value = true
  message.value = ''
  router.replace('/')
}

onMounted(refreshSession)
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <RouterLink class="brand" to="/">websudo</RouterLink>
      <button v-if="authenticated" class="ghost-button" type="button" @click="handleLogout">Logout</button>
    </header>

    <main v-if="ready" class="page">
      <p v-if="message" class="notice error">{{ message }}</p>
      <RouterView @logged-in="handleLoggedIn" />
    </main>
    <main v-else class="page loading">Loading websudo...</main>
  </div>
</template>
