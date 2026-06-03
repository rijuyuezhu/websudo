import type {
  ApprovalRequest,
  AskpassRequest,
  DashboardResponse,
  SessionResponse,
} from './types'

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    credentials: 'same-origin',
    ...init,
    headers: {
      ...(init.body ? { 'Content-Type': 'application/json' } : {}),
      ...init.headers,
    },
  })
  if (!response.ok) {
    throw new ApiError(response.status, response.statusText)
  }
  if (response.status === 204) {
    return undefined as T
  }
  const text = await response.text()
  return text ? (JSON.parse(text) as T) : (undefined as T)
}

export function getSession(): Promise<SessionResponse> {
  return request<SessionResponse>('/api/session')
}

export function login(password: string): Promise<SessionResponse> {
  return request<SessionResponse>('/api/login', {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
}

export function logout(): Promise<SessionResponse> {
  return request<SessionResponse>('/api/logout', {
    method: 'POST',
    body: JSON.stringify({}),
  })
}

export function getDashboard(): Promise<DashboardResponse> {
  return request<DashboardResponse>('/api/dashboard')
}

export function getAskpass(id: string): Promise<AskpassRequest> {
  return request<AskpassRequest>(`/api/askpass/${encodeURIComponent(id)}`)
}

export function submitAskpassPassword(
  id: string,
  password: string,
): Promise<void> {
  return request<void>(`/api/askpass/${encodeURIComponent(id)}/complete`, {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
}

export function denyAskpass(id: string): Promise<void> {
  return request<void>(`/api/askpass/${encodeURIComponent(id)}/deny`, {
    method: 'POST',
    body: JSON.stringify({}),
  })
}

export function getRequest(id: string): Promise<ApprovalRequest> {
  return request<ApprovalRequest>(
    `/api/browser/requests/${encodeURIComponent(id)}`,
  )
}

export function approveRequest(id: string): Promise<void> {
  return request<void>(`/api/requests/${encodeURIComponent(id)}/approve`, {
    method: 'POST',
    body: JSON.stringify({}),
  })
}

export function denyRequest(id: string): Promise<void> {
  return request<void>(`/api/requests/${encodeURIComponent(id)}/deny`, {
    method: 'POST',
    body: JSON.stringify({}),
  })
}
