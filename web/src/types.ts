export type AskpassStatus = 'pending' | 'completed' | 'denied' | 'expired'

export interface AskpassProvenance {
  command: string[]
  cwd: string
}

export interface AskpassRequest {
  id: string
  prompt: string
  provenance: AskpassProvenance
  createdAt: string
  finishedAt?: string
  status: AskpassStatus
}

export interface DashboardResponse {
  askpassPending: AskpassRequest[]
  askpassRecent: AskpassRequest[]
}

export interface SessionResponse {
  authenticated: boolean
  expiresAt?: string
}
