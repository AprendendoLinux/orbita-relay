import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { api } from '../api'

type Domain = { id: number; domain: string; dkim_enabled: boolean; dkim_record?: string; dkim_host?: string }
type Check = { ok: boolean; found: string[] | null; detail: string }
type Checks = { spf: Check; dmarc: Check; dkim: Check }

export default function Domains() {
  const [list, setList] = useState<Domain[]>([])
  const [name, setName] = useState('')
  const [msg, setMsg] = useState<{ t: 'ok' | 'err'; s: string } | null>(null)
  const [checks, setChecks] = useState<Record<number, Checks>>({})
  const [open, setOpen] = useState<number | null>(null)

  const load = useCallback(() => api<Domain[]>('/api/admin/domains').then(setList), [])
  useEffect(() => { load().catch((e) => setMsg({ t: 'err', s: e.message })) }, [load])

  const act = async (fn: () => Promise<unknown>, ok = 'Concluído') => {
    try {
      await fn()
      setMsg({ t: 'ok', s: ok })
      await load()
    } catch (e) {
      setMsg({ t: 'err', s: (e as Error).message })
    }
  }

  const add = (e: FormEvent) => {
    e.preventDefault()
    act(async () => { await api('/api/admin/domains', { method: 'POST', body: { domain: name } }); setName('') }, 'Domínio adicionado')
  }

  const check = async (id: number) => {
    try {
      setChecks({ ...checks, [id]: await api<Checks>(`/api/admin/domains/${id}/check`) })
      setOpen(id)
    } catch (e) { setMsg({ t: 'err', s: (e as Error).message }) }
  }

  const Row = ({ label, c }: { label: string; c?: Check }) =>
    c ? (
      <div className="row small">
        <span className={`badge ${c.ok ? 'b-ok' : 'b-bad'}`}>{label}: {c.ok ? 'OK' : 'pendente'}</span>
        <span className="muted">{c.ok ? c.found?.join(' | ') : c.detail}</span>
      </div>
    ) : null

  return (
    <>
      <div className="topbar"><h1>Domínios e DKIM</h1></div>
      <p className="muted">Domínios de remetente autorizados no relay. Gere a chave DKIM e publique o registro TXT no DNS.</p>
      {msg && <div className={`alert ${msg.t}`} role="status" style={{ margin: '12px 0' }}>{msg.s}</div>}
      <form className="glass card row" onSubmit={add} style={{ marginBottom: 16 }}>
        <input style={{ flex: 1, minWidth: 220 }} placeholder="exemplo.com.br" value={name} onChange={(e) => setName(e.target.value)} required aria-label="Domínio" />
        <button className="primary">Adicionar domínio</button>
      </form>
      <div className="stack">
        {list.map((d) => (
          <div className="glass card stack" key={d.id}>
            <div className="row spread">
              <div className="row">
                <h3 style={{ margin: 0 }}>{d.domain}</h3>
                <span className={`badge ${d.dkim_enabled ? 'b-ok' : 'b-warn'}`}>DKIM {d.dkim_enabled ? 'ativo' : d.dkim_record ? 'pausado' : 'sem chave'}</span>
              </div>
              <div className="row">
                <button className="sm" onClick={() => check(d.id)}>Verificar DNS</button>
                <button className="sm" onClick={() => act(() => api(`/api/admin/domains/${d.id}/dkim`, { method: 'POST' }), 'Chave DKIM gerada')}>
                  {d.dkim_record ? 'Regerar chave' : 'Gerar chave DKIM'}
                </button>
                {d.dkim_record && (
                  <button className="sm" onClick={() => act(() => api(`/api/admin/domains/${d.id}/dkim`, { method: 'PUT', body: { enabled: !d.dkim_enabled } }))}>
                    {d.dkim_enabled ? 'Pausar' : 'Ativar'}
                  </button>
                )}
                <button className="sm danger" onClick={() => confirm(`Remover ${d.domain}?`) && act(() => api(`/api/admin/domains/${d.id}`, { method: 'DELETE' }), 'Domínio removido')}>Remover</button>
              </div>
            </div>
            {d.dkim_record && (
              <div>
                <div className="small muted">Registro TXT em <span className="mono">{d.dkim_host}</span></div>
                <code className="block">{d.dkim_record}</code>
              </div>
            )}
            {open === d.id && checks[d.id] && (
              <div className="stack">
                <Row label="SPF" c={checks[d.id].spf} />
                <Row label="DKIM" c={checks[d.id].dkim} />
                <Row label="DMARC" c={checks[d.id].dmarc} />
              </div>
            )}
          </div>
        ))}
        {list.length === 0 && <div className="glass card muted">Nenhum domínio cadastrado. Sem domínios, qualquer remetente é aceito das redes autorizadas.</div>}
      </div>
    </>
  )
}
