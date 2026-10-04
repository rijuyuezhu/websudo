<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  ApiError,
  denyAskpass,
  getAskpass,
  submitAskpassPassword,
} from '../api'
import type { AskpassRequest } from '../types'

const props = defineProps<{ id: string }>()
const router = useRouter()
const request = ref<AskpassRequest | null>(null)
const password = ref('')
const loading = ref(true)
const saving = ref(false)
const error = ref('')

function formatCommand(command: string[]) {
  return command.map((arg) => JSON.stringify(arg)).join(' ')
}

function formatValue(value: string) {
  return JSON.stringify(value)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    request.value = await getAskpass(props.id)
  } catch (err) {
    password.value = ''
    if (err instanceof ApiError && err.status === 401) {
      await router.replace('/login')
      return
    }
    error.value =
      err instanceof ApiError && err.status === 404
        ? 'Request not found.'
        : 'Unable to load request.'
  } finally {
    loading.value = false
  }
}

async function submit() {
  saving.value = true
  error.value = ''
  try {
    await submitAskpassPassword(props.id, password.value)
    password.value = ''
    await router.replace('/')
  } catch (err) {
    password.value = ''
    if (err instanceof ApiError && err.status === 401) {
      await router.replace('/login')
      return
    }
    error.value =
      err instanceof ApiError && err.status === 409
        ? 'This request is no longer pending.'
        : 'Unable to submit password.'
  } finally {
    saving.value = false
  }
}

async function deny() {
  saving.value = true
  error.value = ''
  password.value = ''
  try {
    await denyAskpass(props.id)
    await router.replace('/')
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      await router.replace('/login')
      return
    }
    error.value = 'Unable to deny request.'
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="detail-card panel">
    <h1>Sudo request</h1>
    <p v-if="loading" class="muted">Loading request...</p>
    <p v-if="error" class="notice error">{{ error }}</p>

    <template v-if="request">
      <span class="status">{{ request.status }}</span>
      <div>
        <p><strong>Command</strong></p>
        <pre>{{ formatCommand(request.provenance.command) }}</pre>
        <p class="muted">
          Working directory: {{ formatValue(request.provenance.cwd) }}
        </p>
      </div>
      <pre>{{ request.prompt }}</pre>

      <form v-if="request.status === 'pending'" @submit.prevent="submit">
        <label class="field">
          <span>Password</span>
          <input
            v-model="password"
            type="password"
            autocomplete="current-password"
          />
        </label>
        <div class="actions">
          <button
            class="primary-button"
            type="submit"
            :disabled="saving || password.length === 0"
          >
            Submit password
          </button>
          <button
            class="danger-button"
            type="button"
            :disabled="saving"
            @click="deny"
          >
            Deny
          </button>
        </div>
      </form>
    </template>
  </section>
</template>
