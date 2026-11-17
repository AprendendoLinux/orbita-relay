import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { useBranding } from '../branding'
import FieldInput, { visible, type Field, type Vals } from '../components/FieldInput'

type Resp = { groups: { id: string; label: string }[]; fields: Field[] }

export default function Settings() {
  const { refresh } = useBranding()
  const [data, setData] = useState<Resp | null>(null)
  const [vals, setVals] = useState<Vals>({})
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
    const body: Vals = {}
    for (const f of data.fields) body[f.key] = vals[f.key]
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
  const groups = data.groups.filter((g) => data.fields.some((f) => f.group === g.id && visible(f, vals)))
  const current = groups.some((g) => g.id === tab) ? tab : groups[0]?.id
  const fields = data.fields.filter((f) => f.group === current && visible(f, vals))

  return (
    <>
      <div className="topbar">
        <h1>Configurações</h1>
        <Link className="btn primary" to="/assistente">✦ Alterar com o assistente</Link>
      </div>
      <div className="tabs" role="tablist">
        {groups.map((g) => (
          <button key={g.id} role="tab" className={current === g.id ? 'on' : ''} onClick={() => { setTab(g.id); setMsg(null) }}>{g.label}</button>
        ))}
      </div>
      <div className="glass card stack">
        <div className="form-grid">
          {fields.map((f) => (
            <FieldInput key={f.key} f={f} v={vals[f.key]} set={(x) => setVals({ ...vals, [f.key]: x })} />
          ))}
        </div>
        {msg && <div className={`alert ${msg.t}`} role="status">{msg.s}</div>}
        <div className="row">
          <button className="primary" onClick={save} disabled={busy}>{busy ? 'Salvando…' : 'Salvar'}</button>
          {current === 'ldap' && vals.auth_mode === 'ldap' && <button onClick={testLdap}>Testar conexão LDAP</button>}
        </div>
      </div>
    </>
  )
}
