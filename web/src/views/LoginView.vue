<script setup lang="ts">
import { ref } from 'vue'
import { ApiError, login } from '../api'

const emit = defineEmits<{ 'logged-in': [] }>()

const password = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await login(password.value)
    password.value = ''
    emit('logged-in')
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      error.value = 'Password rejected.'
    } else {
      error.value = 'Unable to log in.'
    }
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="login-wrap">
    <form class="login-card" @submit.prevent="submit">
      <p class="eyebrow">Local approval</p>
      <h1>Authorize this browser for websudo</h1>
      <p class="muted">
        Enter the current machine password. Approval actions will stay unlocked
        in this browser for up to 72 hours.
      </p>

      <label class="field">
        <span>Password</span>
        <input
          v-model="password"
          type="password"
          autocomplete="current-password"
          autofocus
        />
      </label>

      <p v-if="error" class="notice error">{{ error }}</p>
      <button
        class="primary-button"
        type="submit"
        :disabled="loading || password.length === 0"
      >
        {{ loading ? 'Checking...' : 'Login' }}
      </button>
    </form>
  </section>
</template>
