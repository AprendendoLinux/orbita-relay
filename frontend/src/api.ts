export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

type Opts = { method?: string; body?: unknown; form?: FormData; headers?: Record<string, string> }

export async function api<T = unknown>(path: string, opts: Opts = {}): Promise<T> {
  const headers: Record<string, string> = { 'X-Requested-With': 'orbita', ...(opts.headers ?? {}) }
  let body: BodyInit | undefined
  if (opts.form) body = opts.form
  else if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(opts.body)
  }
  const res = await fetch(path, { method: opts.method ?? 'GET', headers, body, credentials: 'same-origin' })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new ApiError(res.status, (data as { error?: string }).error ?? `Erro ${res.status}`)
  return data as T
}

export type User = {
  id: number
  username: string
  display_name: string
  email: string
  role: 'viewer' | 'operator' | 'admin'
  source: string
  disabled: boolean
  created_at: string
  last_login: string | null
}

export type Theme = {
  id: number
  slug: string
  name: string
  builtin: boolean
  tokens: Record<string, string>
  logo: string
}

export type Branding = {
  version: string
  setup_required: boolean
  db_ready: boolean
  app_name: string
  timezone?: string
  theme: Theme
}

export const fmtDate = (iso: string, tz?: string) =>
  new Date(iso).toLocaleString('pt-BR', { timeZone: tz || undefined, dateStyle: 'short', timeStyle: 'medium' })

export const roleLabel: Record<string, string> = { admin: 'Administrador', operator: 'Operador', viewer: 'Leitura' }
export const statusLabel: Record<string, string> = {
  delivered: 'Entregue',
  failed: 'Falha',
  deferred: 'Adiada',
  queued: 'Em fila',
}
