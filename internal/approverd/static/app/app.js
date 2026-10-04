'use strict'

class ApiError extends Error {
  constructor(status) {
    super(`HTTP ${status}`)
    this.status = status
  }
}

const sessionKey = 'websudo_session'
const app = document.getElementById('app')
const logoutButton = document.getElementById('logout')

async function request(path, options = {}) {
  const token = window.localStorage.getItem(sessionKey)
  const response = await fetch(path, {
    ...options,
    headers: {
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers,
    },
  })
  if (response.status === 401 && path !== '/api/login') {
    window.localStorage.removeItem(sessionKey)
  }
  if (!response.ok) throw new ApiError(response.status)
  if (response.status === 204) return undefined
  const text = await response.text()
  return text ? JSON.parse(text) : undefined
}

function replace(path) {
  window.location.replace(path)
}

function element(tag, className, text) {
  const node = document.createElement(tag)
  if (className) node.className = className
  if (text !== undefined) node.textContent = text
  return node
}

function formatCommand(command) {
  return command.map((arg) => JSON.stringify(arg)).join(' ')
}

function showError(node, message) {
  node.textContent = message
  node.hidden = false
}

function hideError(node) {
  node.textContent = ''
  node.hidden = true
}

async function logout() {
  try {
    await request('/api/logout', { method: 'POST', body: '{}' })
  } catch {
    return
  }
  window.localStorage.removeItem(sessionKey)
  replace('/login')
}

function renderLogin() {
  logoutButton.hidden = true
  app.innerHTML = `
    <section class="login-wrap">
      <form class="login-card">
        <h1>Sign in to websudo</h1>
        <label class="field">
          <span>Password</span>
          <input type="password" autocomplete="current-password" autofocus />
        </label>
        <p class="notice error" hidden></p>
        <button class="primary-button" type="submit" disabled>Sign in</button>
      </form>
    </section>`

  const form = app.querySelector('form')
  const input = form.querySelector('input')
  const error = form.querySelector('.error')
  const button = form.querySelector('button')
  let saving = false

  function updateButton() {
    button.disabled = saving || input.value.length === 0
    button.textContent = saving ? 'Signing in...' : 'Sign in'
  }

  input.addEventListener('input', updateButton)
  form.addEventListener('submit', async (event) => {
    event.preventDefault()
    if (saving || input.value.length === 0) return
    saving = true
    hideError(error)
    updateButton()
    try {
      const session = await request('/api/login', {
        method: 'POST',
        body: JSON.stringify({ password: input.value }),
      })
      window.localStorage.setItem(sessionKey, session.token)
      input.value = ''
      replace('/')
    } catch (err) {
      showError(
        error,
        err instanceof ApiError && err.status === 401
          ? 'Password rejected.'
          : 'Unable to log in.',
      )
    } finally {
      saving = false
      updateButton()
    }
  })
}

function requestCard(item) {
  const link = element('a', 'item-card')
  link.href = `/askpass/${encodeURIComponent(item.id)}`
  link.append(element('span', 'status', item.status))
  link.append(element('h3', '', formatCommand(item.provenance.command)))
  link.append(element('p', 'muted', item.provenance.cwd))
  if (item.finishedAt) link.append(element('p', 'muted', item.finishedAt))
  return link
}

function renderRequestList(container, empty, items) {
  container.replaceChildren()
  if (items.length === 0) {
    empty.hidden = false
    return
  }
  empty.hidden = true
  for (const item of items) container.append(requestCard(item))
}

function renderDashboard() {
  app.innerHTML = `
    <section class="grid">
      <h1>Requests</h1>
      <p class="notice error" hidden></p>
      <p class="muted loading">Loading requests...</p>
      <section class="panel">
        <h2>Pending</h2>
        <div class="cards pending"></div>
        <p class="muted pending-empty" hidden>No pending requests.</p>
      </section>
      <section class="panel">
        <h2>Recent</h2>
        <div class="cards recent"></div>
        <p class="muted recent-empty" hidden>No recent requests.</p>
      </section>
    </section>`

  const error = app.querySelector('.error')
  const loading = app.querySelector('.loading')
  const pending = app.querySelector('.pending')
  const pendingEmpty = app.querySelector('.pending-empty')
  const recent = app.querySelector('.recent')
  const recentEmpty = app.querySelector('.recent-empty')
  let polling = false

  async function load(initial = false) {
    if (polling) return
    polling = true
    if (initial) loading.hidden = false
    try {
      const data = await request('/api/dashboard')
      renderRequestList(pending, pendingEmpty, data.askpassPending)
      renderRequestList(recent, recentEmpty, data.askpassRecent)
      hideError(error)
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        replace('/login')
        return
      }
      showError(error, 'Unable to load requests.')
    } finally {
      if (initial) loading.hidden = true
      polling = false
    }
  }

  void load(true)
  window.setInterval(() => void load(), 1000)
}

function renderAskpass(id) {
  app.innerHTML = `
    <section class="detail-card panel">
      <h1>Sudo request</h1>
      <p class="muted loading">Loading request...</p>
      <p class="notice error" hidden></p>
      <div class="detail" hidden></div>
    </section>`

  const loading = app.querySelector('.loading')
  const error = app.querySelector('.error')
  const detail = app.querySelector('.detail')
  let status
  let timer
  let loadGeneration = 0

  function schedulePoll() {
    timer = window.setTimeout(() => void load(), 1000)
  }

  async function load() {
    window.clearTimeout(timer)
    timer = undefined
    const generation = ++loadGeneration
    hideError(error)
    try {
      const item = await request(`/api/askpass/${encodeURIComponent(id)}`)
      if (generation !== loadGeneration) return
      if (status === undefined || item.status !== status) renderDetail(item)
      status = item.status
      if (item.status === 'pending') schedulePoll()
    } catch (err) {
      if (generation !== loadGeneration) return
      if (err instanceof ApiError && err.status === 401) {
        replace('/login')
        return
      }
      if (err instanceof ApiError && err.status === 404) {
        detail.replaceChildren()
        detail.hidden = true
        showError(error, 'Request not found.')
        return
      }
      showError(error, 'Unable to load request.')
      schedulePoll()
    } finally {
      if (generation === loadGeneration) loading.hidden = true
    }
  }

  function renderDetail(item) {
    detail.innerHTML = `
      <span class="status"></span>
      <div>
        <p><strong>Command</strong></p>
        <pre class="command-value"></pre>
        <p class="muted cwd"></p>
      </div>
      <pre class="prompt"></pre>
      <form>
        <label class="field">
          <span>Password</span>
          <input type="password" autocomplete="current-password" />
        </label>
        <div class="actions">
          <button class="primary-button" type="submit" disabled>Submit password</button>
          <button class="danger-button" type="button">Deny</button>
        </div>
      </form>`
    detail.hidden = false
    detail.querySelector('.status').textContent = item.status
    detail.querySelector('.command-value').textContent = formatCommand(
      item.provenance.command,
    )
    detail.querySelector('.cwd').textContent =
      `Working directory: ${JSON.stringify(item.provenance.cwd)}`
    detail.querySelector('.prompt').textContent = item.prompt

    const form = detail.querySelector('form')
    if (item.status !== 'pending') {
      form.remove()
      return
    }

    const input = form.querySelector('input')
    const submit = form.querySelector('.primary-button')
    const deny = form.querySelector('.danger-button')
    let saving = false

    function updateButtons() {
      submit.disabled = saving || input.value.length === 0
      deny.disabled = saving
    }
    input.addEventListener('input', updateButtons)

    form.addEventListener('submit', async (event) => {
      event.preventDefault()
      if (saving || input.value.length === 0) return
      saving = true
      hideError(error)
      updateButtons()
      try {
        await request(`/api/askpass/${encodeURIComponent(id)}/complete`, {
          method: 'POST',
          body: JSON.stringify({ password: input.value }),
        })
        input.value = ''
        replace('/')
      } catch (err) {
        input.value = ''
        if (err instanceof ApiError && err.status === 401) {
          replace('/login')
          return
        }
        if (err instanceof ApiError && err.status === 409) {
          await load()
          return
        }
        showError(error, 'Unable to submit password.')
      } finally {
        saving = false
        if (form.isConnected) updateButtons()
      }
    })

    deny.addEventListener('click', async () => {
      if (saving) return
      saving = true
      input.value = ''
      hideError(error)
      updateButtons()
      try {
        await request(`/api/askpass/${encodeURIComponent(id)}/deny`, {
          method: 'POST',
          body: '{}',
        })
        replace('/')
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) {
          replace('/login')
          return
        }
        if (err instanceof ApiError && err.status === 409) {
          await load()
          return
        }
        showError(error, 'Unable to deny request.')
      } finally {
        saving = false
        if (form.isConnected) updateButtons()
      }
    })
  }

  void load()
}

function renderNotFound() {
  logoutButton.hidden = true
  app.replaceChildren(element('section', 'panel', 'Page not found.'))
}

function route() {
  if (window.location.pathname === '/login') return { kind: 'login' }
  if (window.location.pathname === '/') return { kind: 'dashboard' }

  const match = window.location.pathname.match(/^\/askpass\/([^/]+)$/)
  if (!match) return { kind: 'not-found' }
  try {
    return { kind: 'askpass', id: decodeURIComponent(match[1]) }
  } catch {
    return { kind: 'not-found' }
  }
}

async function main() {
  const current = route()
  if (current.kind === 'login') {
    renderLogin()
    return
  }
  try {
    await request('/api/session')
  } catch {
    replace('/login')
    return
  }

  if (current.kind === 'not-found') {
    renderNotFound()
    return
  }

  logoutButton.hidden = false
  if (current.kind === 'dashboard') renderDashboard()
  else renderAskpass(current.id)
}

logoutButton.addEventListener('click', () => void logout())
void main()
