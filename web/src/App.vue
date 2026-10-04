<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { logout } from './api'

const router = useRouter()
const route = useRoute()
const showLogout = computed(
  () => route.name !== undefined && route.meta.public !== true,
)

async function handleLogout() {
  try {
    await logout()
  } catch {
    return
  }
  await router.replace('/login')
}

function handleLoggedIn() {
  void router.replace('/')
}
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <RouterLink class="brand" to="/">websudo</RouterLink>
      <button
        v-if="showLogout"
        class="ghost-button"
        type="button"
        @click="handleLogout"
      >
        Logout
      </button>
    </header>

    <main class="page">
      <RouterView @logged-in="handleLoggedIn" />
    </main>
  </div>
</template>
