<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ApiError, approveRequest, denyRequest, getRequest } from '../api'
import type { ApprovalRequest } from '../types'

const props = defineProps<{ id: string }>()
const router = useRouter()
const request = ref<ApprovalRequest | null>(null)
const loading = ref(true)
const saving = ref(false)
const error = ref('')

const argv = computed(() => request.value?.command.Argv.join('\n') ?? '')

async function load() {
  loading.value = true
  error.value = ''
  try {
    request.value = await getRequest(props.id)
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      await router.replace('/login')
      return
    }
    error.value = err instanceof ApiError && err.status === 404 ? 'Request not found.' : 'Unable to load request.'
  } finally {
    loading.value = false
  }
}

async function act(kind: 'approve' | 'deny') {
  saving.value = true
  error.value = ''
  try {
    if (kind === 'approve') {
      await approveRequest(props.id)
    } else {
      await denyRequest(props.id)
    }
    await load()
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      await router.replace('/login')
      return
    }
    error.value = err instanceof ApiError && err.status === 409 ? 'This request is no longer pending.' : 'Unable to update request.'
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="detail-card panel">
    <p class="eyebrow">Command request</p>
    <h1>{{ request?.command.ResolvedPath || id }}</h1>
    <p v-if="loading" class="muted">Loading request...</p>
    <p v-if="error" class="notice error">{{ error }}</p>

    <template v-if="request">
      <span class="status">{{ request.status }}</span>
      <p class="muted">{{ request.requestedBy.Username }} on {{ request.requestedBy.Hostname }}</p>
      <p><strong>Working directory:</strong> {{ request.command.Cwd }}</p>
      <h2>Arguments</h2>
      <pre>{{ argv }}</pre>

      <div v-if="request.status === 'pending'" class="actions">
        <button class="primary-button" type="button" :disabled="saving" @click="act('approve')">Approve</button>
        <button class="danger-button" type="button" :disabled="saving" @click="act('deny')">Deny</button>
      </div>

      <section v-if="request.result">
        <h2>Result</h2>
        <p><strong>Exit code:</strong> {{ request.result.exitCode }}</p>
        <p v-if="request.result.signal"><strong>Signal:</strong> {{ request.result.signal }}</p>
        <h3 v-if="request.result.stdout">Stdout</h3>
        <pre v-if="request.result.stdout">{{ request.result.stdout }}</pre>
        <h3 v-if="request.result.stderr">Stderr</h3>
        <pre v-if="request.result.stderr">{{ request.result.stderr }}</pre>
      </section>
    </template>
  </section>
</template>
