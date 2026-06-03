export type RequestStatus =
  | 'pending'
  | 'approved'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'denied'
  | 'expired'

export type AskpassStatus = 'pending' | 'completed' | 'denied' | 'expired'

export interface Requester {
  UID: number
  GID: number
  Username: string
  Hostname: string
}

export interface Command {
  ResolvedPath: string
  Argv: string[]
  Cwd: string
}

export interface Result {
  exitCode: number
  signal?: number
  stdout?: string
  stderr?: string
}

export interface ApprovalRequest {
  id: string
  createdAt: string
  requestedBy: Requester
  command: Command
  status: RequestStatus
  result?: Result
}

export interface AskpassRequest {
  id: string
  prompt: string
  createdAt: string
  status: AskpassStatus
}

export interface DashboardResponse {
  askpassPending: AskpassRequest[]
  pending: ApprovalRequest[]
  recent: ApprovalRequest[]
}

export interface SessionResponse {
  authenticated: boolean
  expiresAt?: string
}
