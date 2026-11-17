import { useCallback, useEffect, useState } from 'react'
import { api, fmtDate } from '../api'
import { useBranding } from '../branding'
import type { Vals } from './FieldInput'

type Status = { last_ok?: string; last_error: string; sent: number; pending_bytes: number }

const keys = ['collect_scheme', 'collect_host', 'collect_port', 'collect_path', 'collect_token', 'collect_skip_verify', 'collect_interval']

const size = (b: number) => (b < 1024 ? `${b} B` : b < 1048576 ? `${(b / 1024).toFixed(1)} KB` : `${(b / 1048576).toFixed(1)} MB`)

export default function CollectPanel({ vals, live = true }: { vals: Vals; live?: boolean }) {
  const { branding } = useBranding()
  const [st, setSt] = useState<Status | null>(null)
  const [msg, setMsg] = useState<{ t: 'ok' | 'err'; s: string } | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(() => api<Status>('/api/admin/collect/status').then(setSt).catch(() => {}), [])
  useEffect(() => {
    if (!live) return
    load()
    const t = setInterval(load, 5000)
    return () => clearInterval(t)
  }, [load, live])

  const test = async () => {
    setBusy(true)
    setMsg(null)
    try {
      const body: Vals = {}
      for (const k of keys) body[k] = vals[k]
      await api('/api/admin/settings', { method: 'PUT', body })
      const r = await api<{ node: string; version: string }>('/api/admin/collect/test', { method: 'POST' })
      setMsg({ t: 'ok', s: `Conexão OK. Este servidor se identifica no central como “${r.node}” (versão ${r.version}).` })
    } catch (e) {
      setMsg({ t: 'err', s: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="stack">
      <div className="row">
        <button onClick={test} disabled={busy}>{busy ? 'Testando…' : 'Testar conexão'}</button>
        <span className="help">Salva os dados de conexão e testa o token no servidor central.</span>
      </div>
      {msg && <div className={`alert ${msg.t}`} role="status">{msg.s}</div>}
      {live && st && branding?.deploy_mode === 'client' && (
        <div className="glass card kv">
          <span className="muted">Último envio</span><span>{st.last_ok ? fmtDate(st.last_ok, branding.timezone) : '—'}</span>
          <span className="muted">Eventos enviados</span><span>{st.sent}</span>
          <span className="muted">Pendente no log</span><span>{size(st.pending_bytes)}</span>
          <span className="muted">Situação</span>
          <span>{st.last_error ? <span className="badge b-bad">{st.last_error}</span> : <span className="badge b-ok">sem erros</span>}</span>
        </div>
      )}
    </div>
  )
}
