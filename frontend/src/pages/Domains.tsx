import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { api } from '../api'
import { useDialog } from '../components/Dialog'

type Domain = {
  id: number; domain: string; dkim_enabled: boolean; dkim_record?: string; dkim_host?: string
  spf_record: string; dmarc_host: string; dmarc_record: string
}
type Check = { ok: boolean; found: string[] | null; detail: string }
type Checks = { spf: Check; dmarc: Check; dkim: Check }

async function copyText(t: string) {
  try {
    await navigator.clipboard.writeText(t)
  } catch {
    const ta = document.createElement('textarea')
    ta.value = t
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    ta.remove()
  }
}

function Record({ title, host, value, check, note }: { title: string; host: string; value?: string; check?: Check; note?: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="rec">
      <div className="row spread">
        <b>{title}</b>
        {check ? <span className={`badge ${check.ok ? 'b-ok' : 'b-bad'}`}>{check.ok ? 'publicado' : 'pendente'}</span> : <span className="badge b-info">não verificado</span>}
      </div>
      {value ? (
        <>
          <div className="small muted">TXT em <span className="mono">{host}</span></div>
          <code className="block">{value}</code>
          <div className="row">
            <button className="sm" onClick={async () => { await copyText(value); setCopied(true); setTimeout(() => setCopied(false), 1500) }}>{copied ? 'Copiado ✓' : 'Copiar valor'}</button>
            {note && <span className="help">{note}</span>}
          </div>
        </>
      ) : <span className="muted small">Gere a chave DKIM para obter o registro.</span>}
      {check && !check.ok && check.detail && <span className="help">{check.detail}</span>}
      {check?.found && check.found.length > 0 && <span className="help mono">Encontrado: {check.found.join(' | ')}</span>}
    </div>
  )
}

export default function Domains({ embedded = false }: { embedded?: boolean }) {
  const dlg = useDialog()
  const [list, setList] = useState<Domain[]>([])
  const [name, setName] = useState('')
  const [msg, setMsg] = useState<{ t: 'ok' | 'err'; s: string } | null>(null)
  const [checks, setChecks] = useState<Record<number, Checks>>({})

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
    } catch (e) { setMsg({ t: 'err', s: (e as Error).message }) }
  }

  return (
    <>
      {embedded ? <h3 style={{ margin: '8px 0 0' }}>Domínios, chave DKIM e registros de DNS</h3> : <div className="topbar"><h1>Domínios, DKIM, SPF e DMARC</h1></div>}
      <p className="muted">{embedded ? 'Cadastre os domínios de remetente, gere a chave DKIM e publique os registros TXT abaixo no DNS de cada domínio. Os registros usam as configurações salvas (IPs públicos e política DMARC): salve antes de copiá-los.' : 'Domínios de remetente autorizados no relay. Publique os registros TXT abaixo no DNS de cada domínio. A política DMARC e os IPs do SPF são ajustados em Configurações.'}</p>
      {msg && <div className={`alert ${msg.t}`} role="status" style={{ margin: '12px 0' }}>{msg.s}</div>}
      <form className="glass card row" onSubmit={add} style={{ marginBottom: 16 }}>
        <input style={{ flex: 1, minWidth: 220 }} placeholder="exemplo.com.br" value={name} onChange={(e) => setName(e.target.value)} required aria-label="Domínio" />
        <button className="primary">Adicionar domínio</button>
      </form>
      <div className="stack">
        {list.map((d) => {
          const c = checks[d.id]
          return (
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
                  <button className="sm danger" onClick={async () => (await dlg.confirm({ title: `Remover ${d.domain}?`, message: 'O domínio e a chave DKIM dele serão removidos. Remetentes desse domínio deixam de ser aceitos.', danger: true, confirmLabel: 'Remover' })) && act(() => api(`/api/admin/domains/${d.id}`, { method: 'DELETE' }), 'Domínio removido')}>Remover</button>
                </div>
              </div>
              <div className="grid cols-2">
                <Record title="SPF" host={d.domain} value={d.spf_record} check={c?.spf} note="Se já existir um SPF, mescle os mecanismos em um único registro." />
                <Record title="DKIM" host={d.dkim_host ?? ''} value={d.dkim_record} check={c?.dkim} />
                <Record title="DMARC" host={d.dmarc_host} value={d.dmarc_record} check={c?.dmarc} note="Comece com p=none e endureça após monitorar os relatórios." />
              </div>
            </div>
          )
        })}
        {list.length === 0 && <div className="glass card muted">Nenhum domínio cadastrado. Sem domínios, qualquer remetente é aceito das redes autorizadas.</div>}
      </div>
    </>
  )
}
