import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { useBranding } from '../branding'
import FieldInput, { applyChange, visible, type Field, type Vals } from '../components/FieldInput'
import CollectPanel from '../components/CollectPanel'
import Tip from '../components/Tip'
import Ipv6Panel from '../components/Ipv6Panel'
import HostSendTip from '../components/HostSendTip'
import NetDetect from '../components/NetDetect'
import Domains from './Domains'
import DmarcHelp from '../components/DmarcHelp'
import { mergeLines } from '../lib'
import { clearPersonalMode } from '../components/ThemeToggle'

type Resp = { groups: { id: string; label: string }[]; fields: Field[] }

export default function Settings() {
  const { refresh } = useBranding()
  const [data, setData] = useState<Resp | null>(null)
  const [vals, setVals] = useState<Vals>({})
  const [tab, setTab] = useState('geral')
  const [msg, setMsg] = useState<{ t: 'ok' | 'err'; s: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [saves, setSaves] = useState(0)
  const [editSmtp, setEditSmtp] = useState(false)
  const [testTo, setTestTo] = useState('')
  const [testing, setTesting] = useState(false)

  const load = async () => {
    const r = await api<Resp>('/api/admin/settings')
    setData(r)
    setVals(Object.fromEntries(r.fields.map((f) => [f.key, f.value])))
  }
  useEffect(() => { load().catch((e) => setMsg({ t: 'err', s: e.message })) }, [])

  const save = async () => {
    if (!data) return
    const miss = data.fields.find((f) => f.required && visible(f, vals) && String(vals[f.key] ?? '').trim() === '')
    if (miss) {
      setMsg({ t: 'err', s: `Preencha o campo obrigatório: ${miss.label}` })
      return
    }
    setBusy(true)
    setMsg(null)
    const body: Vals = {}
    for (const f of data.fields) body[f.key] = vals[f.key]
    try {
      const themeChanged = data.fields.find((f) => f.key === 'default_theme')?.value !== vals.default_theme
      await api('/api/admin/settings', { method: 'PUT', body })
      if (themeChanged) clearPersonalMode()
      setMsg({ t: 'ok', s: 'Configurações salvas. O relay recarrega a configuração automaticamente.' })
      await load()
      setSaves((n) => n + 1)
      setEditSmtp(false)
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

  const testSmtp = async (e: React.FormEvent) => {
    e.preventDefault()
    setMsg(null)
    setTesting(true)
    try {
      await api('/api/admin/smtp/test', { method: 'POST', body: {
        to: testTo, host: vals.smtp_host, port: Number(vals.smtp_port), tls: vals.smtp_tls, user: vals.smtp_user,
        pass: typeof vals.smtp_pass === 'string' ? vals.smtp_pass : '', from: vals.smtp_from, skip_verify: vals.smtp_skip_verify === true,
      } })
      setMsg({ t: 'ok', s: `E-mail de teste enviado para ${testTo}. Confira a caixa de entrada (e o spam).` })
    } catch (e2) {
      setMsg({ t: 'err', s: `Falha no envio: ${(e2 as Error).message}` })
    } finally {
      setTesting(false)
    }
  }

  if (!data) return <p className="muted">Carregando…</p>
  const groups = data.groups.filter((g) => data.fields.some((f) => f.group === g.id && visible(f, vals)))
  const current = groups.some((g) => g.id === tab) ? tab : (['smarthost', 'entrega'].includes(tab) ? 'mta' : groups[0]?.id)
  const fields = data.fields.filter((f) => f.group === current && visible(f, vals))
  const saved = (k: string) => data.fields.find((f) => f.key === k)
  const smtpConfigured = !!saved('smtp_host')?.value && !!saved('smtp_from')?.value
  const smtpSummary = current === 'seguranca' && smtpConfigured && !editSmtp
  const tlsLabel: Record<string, string> = { none: 'Nenhuma', starttls: 'STARTTLS', tls: 'TLS (SMTPS)' }
  const publicUrl = String(saved('public_url')?.value ?? '')

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
        {smtpSummary && (
          <div className="stack">
            <div className="row spread">
              <b>Servidor SMTP configurado</b>
              <span className="badge b-ok">salvo</span>
            </div>
            <div className="kv">
              <span>Servidor</span><span className="mono">{String(saved('smtp_host')?.value)}:{String(saved('smtp_port')?.value)}</span>
              <span>Criptografia</span><span>{tlsLabel[String(saved('smtp_tls')?.value)] ?? String(saved('smtp_tls')?.value)}{saved('smtp_skip_verify')?.value === true ? ' (certificado não validado)' : ''}</span>
              <span>Usuário</span><span>{String(saved('smtp_user')?.value || '—')}</span>
              <span>Senha</span><span>{saved('smtp_pass')?.has_value ? '•••••••• (salva, criptografada)' : '—'}</span>
              <span>Remetente</span><span className="mono">{String(saved('smtp_from')?.value)}</span>
            </div>
            <div><button onClick={() => setEditSmtp(true)}>✎ Editar</button></div>
          </div>
        )}
        {current === 'seguranca' && !publicUrl && <div className="alert warn small">Para o “Esqueci minha senha” e os links dos e-mails funcionarem, defina o <b>Endereço público do painel</b> em Configurações &gt; Geral. Ele é preenchido sozinho quando você acessa o painel pelo endereço real (não por localhost).</div>}
        <div className="form-grid" style={smtpSummary ? { display: 'none' } : undefined}>
          {fields.map((f) => (
            <FieldInput key={f.key} f={f} v={vals[f.key]} set={(x) => setVals(applyChange(vals, f.key, x))} />
          ))}
          {current === 'entrega' && <DmarcHelp value={String(vals.dmarc_policy ?? 'none')} />}
        </div>
        {current === 'geral' && vals.deploy_mode === 'server' && (
          <Tip title="Servidor central">Cadastre cada relay remoto em <Link to="/admin/servidores">Servidores</Link> para gerar o token dele. Se este sistema estiver atrás de um proxy com HTTPS, ative “Estou atrás de um proxy reverso”.</Tip>
        )}
        {current === 'geral' && vals.deploy_mode === 'client' && (
          <Tip title="Cliente">Os dados do servidor central ficam na aba “Envio para o servidor central”.</Tip>
        )}
        {current === 'redes' && <NetDetect current={String(vals.relay_nets ?? '')} auto={vals.relay_nets_auto === true} version={saves} onAdd={(l) => setVals((v) => ({ ...v, relay_nets: mergeLines(String(v.relay_nets ?? ''), l) }))} />}
        {current === 'redes' && <HostSendTip />}
        {current === 'redes' && vals.ip_mode === 'dual' && <Ipv6Panel />}
        {current === 'mta' && (
          <div className="alert info small">
            {vals.relay_mode === 'smarthost'
              ? <>No modo <b>Smarthost</b>, configure o servidor de saída na aba <b>Smarthost</b>. Quem pode enviar por este relay fica em <b>Redes e acesso</b>.</>
              : <>No modo <b>Relay SMTP</b>, ajuste IPs públicos, DKIM e DMARC na aba <b>Entrega e DNS</b>. Quem pode enviar por este relay fica em <b>Redes e acesso</b>.</>}
          </div>
        )}
        {current === 'coleta' && (
          <>
            {vals.collect_scheme === 'http' && <div className="alert warn">Com HTTP o token e os dados trafegam sem criptografia. Use apenas em redes confiáveis.</div>}
            <CollectPanel vals={vals} />
          </>
        )}
        {current === 'seguranca' && (
          <form className="row" onSubmit={testSmtp} style={{ alignItems: 'flex-end' }}>
            <label className="f" style={{ flex: '1 1 280px', maxWidth: 420 }}>Enviar e-mail de teste para
              <input type="email" value={testTo} onChange={(e) => setTestTo(e.target.value)} placeholder="voce@exemplo.com.br" required />
            </label>
            <button disabled={testing || !testTo}>{testing ? 'Enviando…' : '✉ Enviar teste'}</button>
            <span className="help" style={{ flexBasis: '100%' }}>{smtpSummary ? 'O teste usa a configuração salva.' : 'O teste usa os valores digitados acima, mesmo sem salvar. Para que valham no sistema, clique em Salvar.'}</span>
          </form>
        )}
        {msg && <div className={`alert ${msg.t}`} role="status">{msg.s}</div>}
        <div className="row">
          {!smtpSummary && <button className="primary" onClick={save} disabled={busy}>{busy ? 'Salvando…' : 'Salvar'}</button>}
          {current === 'seguranca' && editSmtp && <button onClick={() => { setEditSmtp(false); setMsg(null); load() }}>Cancelar</button>}
          {current === 'ldap' && vals.auth_mode === 'ldap' && <button onClick={testLdap}>Testar conexão LDAP</button>}
        </div>
      </div>
      {current === 'entrega' && <div style={{ marginTop: 18 }}><Domains embedded selector={String(vals.dkim_selector ?? '')} version={saves} /></div>}
    </>
  )
}
