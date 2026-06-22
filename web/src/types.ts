export type AskpassStatus = 'pending' | 'completed' | 'denied' | 'expired'

export interface AskpassRequest {
  id: string
  prompt: string
  createdAt: string
  status: AskpassStatus
}

export interface DashboardResponse {
  askpassPending: AskpassRequest[]
}

export interface SessionResponse {
  authenticated: boolean
  expiresAt?: string
}
