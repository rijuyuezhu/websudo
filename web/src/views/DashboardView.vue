<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { ApiError, getDashboard } from '../api'
import type { ApprovalRequest, AskpassRequest } from '../types'

const router = useRouter()
const loading = ref(true)
const error = ref('')
const askpassPending = ref<AskpassRequest[]>([])
const pending = ref<ApprovalRequest[]>([])
const recent = ref<ApprovalRequest[]>([])

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await getDashboard()
    askpassPending.value = data.askpassPending
    pending.value = data.pending
    recent.value = data.recent
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      await router.replace('/login')
      return
    }
    error.value = 'Unable to load approval requests.'
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
      <h1>Local websudo requests</h1>
      <p class="muted">Password prompts are shown first because they block the waiting sudo command.</p>
    </div>

    <p v-if="error" class="notice error">{{ error }}</p>
    <p v-if="loading" class="muted">Loading requests...</p>

    <section class="panel">
      <h2>Password Prompts</h2>
      <div v-if="askpassPending.length" class="cards">
        <RouterLink v-for="item in askpassPending" :key="item.id" class="item-card" :to="`/askpass/${item.id}`">
          <span class="status">{{ item.status }}</span>
          <h3>{{ item.id }}</h3>
          <pre>{{ item.prompt }}</pre>
        </RouterLink>
      </div>
      <p v-else class="muted">No pending password prompts.</p>
    </section>

    <section class="panel">
      <h2>Pending Requests</h2>
      <div v-if="pending.length" class="cards">
        <RouterLink v-for="item in pending" :key="item.id" class="item-card" :to="`/requests/${item.id}`">
          <span class="status">{{ item.status }}</span>
          <h3>{{ item.command.ResolvedPath }}</h3>
          <p class="muted">{{ item.requestedBy.Username }} · {{ item.command.Cwd }}</p>
        </RouterLink>
      </div>
      <p v-else class="muted">No pending command requests.</p>
    </section>

    <section class="panel">
      <h2>Recent Requests</h2>
      <div v-if="recent.length" class="cards">
        <RouterLink v-for="item in recent" :key="item.id" class="item-card" :to="`/requests/${item.id}`">
          <span class="status">{{ item.status }}</span>
          <h3>{{ item.command.ResolvedPath }}</h3>
          <p class="muted">{{ item.id }}</p>
        </RouterLink>
      </div>
      <p v-else class="muted">No recent requests.</p>
    </section>
  </section>
</template>
