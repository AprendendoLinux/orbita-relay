import { useEffect, useState } from 'react'
import { api } from '../api'
import { useBranding } from '../branding'

type Field = {
  key: string; group: string; label: string; help?: string; type: string
  options?: string[]; value: unknown; secret?: boolean; has_value?: boolean
}
type Resp = { groups: { id: string; label: string }[]; fields: Field[] }

export default function Settings() {
  const { refresh } = useBranding()
  const [data, setData] = useState<Resp | null>(null)
  const [vals, setVals] = useState<Record<string, unknown>>({})
  const [tab, setTab] = useState('geral')
  const [msg, setMsg] = useState<{ t: 'ok' | 'err'; s: string } | null>(null)
  const [busy, setBusy] = useState(false)

  const load = async () => {
    const r = await api<Resp>('/api/admin/settings')
    setData(r)
    setVals(Object.fromEntries(r.fields.map((f) => [f.key, f.value])))
  }
  useEffect(() => { load().catch((e) => setMsg({ t: 'err', s: e.message })) }, [])

  const save = async () => {
    if (!data) return
    setBusy(true)
    setMsg(null)
    const body: Record<string, unknown> = {}
    for (const f of data.fields.filter((x) => x.group === tab)) body[f.key] = vals[f.key]
    try {
      await api('/api/admin/settings', { method: 'PUT', body })
      setMsg({ t: 'ok', s: 'Configurações salvas. O relay recarrega a configuração automaticamente.' })
      await load()
      await refresh()
    } catch (e) {
      setMsg({ t: 'err', s: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }

  const testLdap = async () => {
    setMsg(null)
    try {
      await api('/api/admin/ldap/test', { method: 'POST' })
      setMsg({ t: 'ok', s: 'Conexão LDAP bem-sucedida (salve antes de testar).' })
    } catch (e) {
      setMsg({ t: 'err', s: `Falha LDAP: ${(e as Error).message}` })
    }
  }

  if (!data) return <p className="muted">Carregando…</p>
  const fields = data.fields.filter((f) => f.group === tab)

  return (
    <>
      <div className="topbar"><h1>Configurações</h1></div>
      <div className="tabs" role="tablist">
        {data.groups.map((g) => (
          <button key={g.id} role="tab" className={tab === g.id ? 'on' : ''} onClick={() => { setTab(g.id); setMsg(null) }}>{g.label}</button>
        ))}
      </div>
      <div className="glass card stack">
        <div className="form-grid">
          {fields.map((f) => {
            const v = vals[f.key]
            const set = (x: unknown) => setVals({ ...vals, [f.key]: x })
            if (f.type === 'bool')
              return (
                <label key={f.key} className="check" style={{ alignSelf: 'end' }}>
                  <input type="checkbox" checked={v === true} onChange={(e) => set(e.target.checked)} /> {f.label}
                </label>
              )
            return (
              <label key={f.key} className="f" style={f.type === 'textarea' ? { gridColumn: '1 / -1' } : undefined}>
                {f.label}
                {f.type === 'select' ? (
                  <select value={String(v)} onChange={(e) => set(e.target.value)}>
                    {f.options?.map((o) => <option key={o}>{o}</option>)}
                  </select>
                ) : f.type === 'textarea' ? (
                  <textarea value={String(v ?? '')} onChange={(e) => set(e.target.value)} />
                ) : (
                  <input
                    type={f.type === 'password' ? 'password' : f.type === 'number' ? 'number' : 'text'}
                    value={String(v ?? '')}
                    placeholder={f.secret && f.has_value ? '•••••••• (mantida)' : ''}
                    autoComplete="off"
                    onChange={(e) => set(e.target.value)}
                  />
                )}
                {f.help && <span className="help">{f.help}</span>}
              </label>
            )
          })}
        </div>
        {msg && <div className={`alert ${msg.t}`} role="status">{msg.s}</div>}
        <div className="row">
          <button className="primary" onClick={save} disabled={busy}>{busy ? 'Salvando…' : 'Salvar'}</button>
          {tab === 'ldap' && <button onClick={testLdap}>Testar conexão LDAP</button>}
        </div>
      </div>
    </>
  )
}
