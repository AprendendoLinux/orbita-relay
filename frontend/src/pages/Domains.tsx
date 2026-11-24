import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { api } from '../api'
import { useDialog } from '../components/Dialog'
import { copyText } from '../lib'

type Domain = {
  id: number; domain: string; dkim_enabled: boolean; dkim_record?: string; dkim_host?: string
  spf_record: string; spf_note?: string; dmarc_host: string; dmarc_record: string
}
type Check = { ok: boolean; found: string[] | null; detail: string }
type Checks = { spf: Check; dmarc: Check; dkim: Check }

function CopyIcon({ text, title }: { text: string; title: string }) {
  const [ok, setOk] = useState(false)
  return (
    <button type="button" className="copy-ico" title={title} aria-label={title} onClick={async () => { setOk(await copyText(text)); setTimeout(() => setOk(false), 1400) }}>{ok ? '✓' : '⧉'}</button>
  )
}

function Record({ title, host, value, check, note, stale }: { title: string; host: string; value?: string; check?: Check; note?: string; stale?: boolean }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="rec">
      <div className="row spread">
        <b>{title}</b>
        {check ? <span className={`badge ${check.ok ? 'b-ok' : 'b-bad'}`}>{check.ok ? 'publicado' : 'pendente'}</span> : <span className="badge b-info">não verificado</span>}
      </div>
      {value ? (
        <>
          <div className="small muted">TXT em <span className="mono">{host}</span> <CopyIcon text={host} title="Copiar o nome do registro" /></div>
          {stale && <span className="help" style={{ color: 'var(--warning)' }}>O seletor foi alterado e ainda não foi salvo: clique em Salvar acima para aplicá-lo ao relay.</span>}
          <code className="block">{value}</code>
          <div className="row">
            <button className="sm" onClick={async () => { setCopied(await copyText(value)); setTimeout(() => setCopied(false), 1500) }}>{copied ? 'Copiado ✓' : 'Copiar valor'}</button>
            {note && <span className="help">{note}</span>}
          </div>
        </>
      ) : <span className="muted small">Gere a chave DKIM para obter o registro.</span>}
      {check?.ok && value && (
        <span className="help" style={{ color: 'var(--success)' }}>
          ✓ {title === 'DKIM' ? 'Encontrado no DNS e igual à chave gerada' : 'Registro encontrado no DNS'}
        </span>
      )}
      {check && !check.ok && check.detail && <span className="help">{check.detail}</span>}
      {check && !check.ok && check.found && check.found.length > 0 && (
        <details className="found">
          <summary>Ver o que foi encontrado no DNS</summary>
          {check.found.map((f, i) => <code className="block" key={i}>{f}</code>)}
        </details>
      )}
    </div>
  )
}

export default function Domains({ embedded = false, selector, version = 0 }: { embedded?: boolean; selector?: string; version?: number }) {
  const dlg = useDialog()
  const [list, setList] = useState<Domain[]>([])
  const [name, setName] = useState('')
  const [msg, setMsg] = useState<{ t: 'ok' | 'err'; s: string } | null>(null)
  const [checks, setChecks] = useState<Record<number, Checks>>({})

  const load = useCallback(() => api<Domain[]>('/api/admin/domains').then(setList), [])
  useEffect(() => { load().catch((e) => setMsg({ t: 'err', s: e.message })) }, [load, version])

  const act = async (fn: () => Promise<unknown>, ok = 'Concluído') => {
    try {
      await fn()
      setMsg({ t: 'ok', s: ok })
      await load()
    } catch (e) {
      setMsg({ t: 'err', s: (e as Error).message })
    }
  }

  const dkimHost = (d: Domain) => (selector ? `${selector}._domainkey.${d.domain}` : d.dkim_host ?? '')

  const add = (e: FormEvent) => {
    e.preventDefault()
    act(async () => { await api('/api/admin/domains', { method: 'POST', body: { domain: name } }); setName('') }, 'Domínio adicionado com a chave DKIM já gerada')
  }

  const check = async (id: number) => {
    try {
      setChecks({ ...checks, [id]: await api<Checks>(`/api/admin/domains/${id}/check`) })
    } catch (e) { setMsg({ t: 'err', s: (e as Error).message }) }
  }

  return (
    <>
      {embedded ? <h3 style={{ margin: '8px 0 0' }}>Domínios, chave DKIM e registros de DNS</h3> : <div className="topbar"><h1>Domínios, DKIM, SPF e DMARC</h1></div>}
      <p className="muted">{embedded ? 'Cadastre os domínios de remetente, gere a chave DKIM e publique os registros TXT abaixo no DNS de cada domínio. Os registros usam a política DMARC salva.' : 'Domínios de remetente autorizados no relay. Publique os registros TXT abaixo no DNS de cada domínio. A política DMARC é ajustada em Configurações.'}</p>
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
                <Record title="SPF" host={d.domain} value={d.spf_record} check={c?.spf} note={`${d.spf_note ? d.spf_note + ' ' : ''}Se já existir um SPF, mescle os mecanismos em um único registro.`} />
                <Record title="DKIM" host={dkimHost(d)} stale={selector !== undefined && selector !== '' && !!d.dkim_host && !d.dkim_host.startsWith(selector + '.')} value={d.dkim_record} check={c?.dkim} />
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
