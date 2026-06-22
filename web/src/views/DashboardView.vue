<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { ApiError, getDashboard } from '../api'
import type { AskpassRequest } from '../types'

const router = useRouter()
const loading = ref(true)
const error = ref('')
const askpassPending = ref<AskpassRequest[]>([])

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await getDashboard()
    askpassPending.value = data.askpassPending
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      await router.replace('/login')
      return
    }
    error.value = 'Unable to load password prompts.'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="grid">
    <div>
      <p class="eyebrow">Approval queue</p>
      <h1>Local websudo password prompts</h1>
      <p class="muted">
        Password prompts appear here when sudo invokes websudo-askpass.
      </p>
    </div>

    <p v-if="error" class="notice error">{{ error }}</p>
    <p v-if="loading" class="muted">Loading prompts...</p>

    <section class="panel">
      <h2>Password Prompts</h2>
      <div v-if="askpassPending.length" class="cards">
        <RouterLink
          v-for="item in askpassPending"
          :key="item.id"
          class="item-card"
          :to="`/askpass/${item.id}`"
        >
          <span class="status">{{ item.status }}</span>
          <h3>{{ item.id }}</h3>
          <pre>{{ item.prompt }}</pre>
        </RouterLink>
      </div>
      <p v-else class="muted">No pending password prompts.</p>
    </section>
  </section>
</template>
