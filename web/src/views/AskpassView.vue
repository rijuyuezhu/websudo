<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ApiError, denyAskpass, getAskpass, submitAskpassPassword } from '../api'
import type { AskpassRequest } from '../types'

const props = defineProps<{ id: string }>()
const router = useRouter()
const request = ref<AskpassRequest | null>(null)
const password = ref('')
const loading = ref(true)
const saving = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    request.value = await getAskpass(props.id)
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      await router.replace('/login')
      return
    }
    error.value = err instanceof ApiError && err.status === 404 ? 'Password prompt not found.' : 'Unable to load password prompt.'
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
    if (err instanceof ApiError && err.status === 401) {
      await router.replace('/login')
      return
    }
    error.value = err instanceof ApiError && err.status === 409 ? 'This prompt is no longer pending.' : 'Unable to submit password.'
  } finally {
    saving.value = false
  }
}

async function cancel() {
  saving.value = true
  error.value = ''
  try {
    await denyAskpass(props.id)
    await router.replace('/')
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      await router.replace('/login')
      return
    }
    error.value = 'Unable to cancel prompt.'
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="detail-card panel">
    <p class="eyebrow">Sudo password</p>
    <h1>Password required</h1>
    <p v-if="loading" class="muted">Loading prompt...</p>
    <p v-if="error" class="notice error">{{ error }}</p>

    <template v-if="request">
      <span class="status">{{ request.status }}</span>
      <pre>{{ request.prompt }}</pre>

      <form @submit.prevent="submit">
        <label class="field">
          <span>Password</span>
          <input v-model="password" type="password" autocomplete="current-password" />
        </label>
        <div class="actions">
          <button class="primary-button" type="submit" :disabled="saving || password.length === 0">Submit Password</button>
          <button class="danger-button" type="button" :disabled="saving" @click="cancel">Cancel</button>
        </div>
      </form>
    </template>
  </section>
</template>
