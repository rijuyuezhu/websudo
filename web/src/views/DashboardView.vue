<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { ApiError, getDashboard } from '../api'
import type { AskpassRequest } from '../types'

const router = useRouter()
const loading = ref(true)
const error = ref('')
const askpassPending = ref<AskpassRequest[]>([])
const askpassRecent = ref<AskpassRequest[]>([])
let polling = false
let pollTimer: number | undefined

async function load(initial = false) {
  if (polling) return
  polling = true
  if (initial) loading.value = true
  try {
    const data = await getDashboard()
    askpassPending.value = data.askpassPending
    askpassRecent.value = data.askpassRecent
    error.value = ''
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      await router.replace('/login')
      return
    }
    error.value = 'Unable to load password prompts.'
  } finally {
    if (initial) loading.value = false
    polling = false
  }
}

function formatCommand(command: string[]) {
  return command.map((arg) => JSON.stringify(arg)).join(' ')
}

onMounted(() => {
  void load(true)
  pollTimer = window.setInterval(() => void load(), 1000)
})

onBeforeUnmount(() => {
  if (pollTimer !== undefined) window.clearInterval(pollTimer)
})
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

    <section class="panel">
      <h2>Recent Requests</h2>
      <div v-if="askpassRecent.length" class="cards">
        <RouterLink
          v-for="item in askpassRecent"
          :key="item.id"
          class="item-card"
          :to="`/askpass/${item.id}`"
        >
          <span class="status">{{ item.status }}</span>
          <h3>{{ formatCommand(item.provenance.command) }}</h3>
          <p class="muted">{{ item.provenance.cwd }}</p>
          <p v-if="item.finishedAt" class="muted">{{ item.finishedAt }}</p>
        </RouterLink>
      </div>
      <p v-else class="muted">No recent requests.</p>
    </section>
  </section>
</template>
