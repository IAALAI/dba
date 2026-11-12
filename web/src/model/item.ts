export interface Target {
  id: string
  name: string
  type: number
  type_name: string
  endpoint: string
  options: string[]
  status: 'unknown' | 'online' | 'offline' | string
  bytes: number
  last_at: number
  error: string
  dsn?: string
}

export interface StatPoint {
  at: number
  online: boolean
  bytes: number
  error: string
}
