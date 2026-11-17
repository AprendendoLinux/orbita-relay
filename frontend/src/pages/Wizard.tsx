import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import { useBranding } from '../branding'
import { Brand, Footer } from '../components/Layout'
import ThemeToggle from '../components/ThemeToggle'
import FieldInput, { type Field, type Vals } from '../components/FieldInput'

const steps = ['Nome', 'Fuso horário', 'Modo', 'Autenticação', 'Logs', 'Resumo']
const fallbackZones = ['America/Sao_Paulo', 'America/Manaus', 'America/Fortaleza', 'America/Bahia', 'America/Cuiaba', 'America/Rio_Branco', 'America/Noronha', 'UTC']

function zones(): string[] {
  const fn = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf
  const z = fn ? fn('timeZone') : fallbackZones
  return z.includes('UTC') ? z : [...z, 'UTC']
}

export default function Wizard({ first }: { first: boolean }) {
  const { refresh } = useBranding()
  const nav = useNavigate()
  const [fields, setFields] = useState<Field[]>([])
  const [vals, setVals] = useState<Vals>({})
  const [step, setStep] = useState(0)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const tzList = useMemo(zones, [])

  useEffect(() => {
    api<{ fields: Field[] }>('/api/admin/settings').then((r) => {
      setFields(r.fields)
      setVals(Object.fromEntries(r.fields.map((f) => [f.key, f.value])))
    }).catch((e) => setErr(e.message))
  }, [])

  const F = (key: string) => {
    const f = fields.find((x) => x.key === key)
    return f ? <FieldInput key={key} f={f} v={vals[key]} set={(x) => setVals({ ...vals, [key]: x })} /> : null
  }
  const set = (k: string, v: unknown) => setVals({ ...vals, [k]: v })
  const mode = String(vals.relay_mode ?? 'relay')
  const auth = String(vals.auth_mode ?? 'local')

  const finish = async () => {
    setBusy(true)
    setErr('')
    try {
      const body: Vals = {}
      for (const f of fields) body[f.key] = vals[f.key]
      body.wizard_done = true
      await api('/api/admin/settings', { method: 'PUT', body })
      await refresh()
      nav(mode === 'relay' ? '/dominios' : '/', { replace: true })
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const label = (key: string) => fields.find((f) => f.key === key)?.label ?? key
  const summary: [string, string][] = [
    ['Nome do sistema', String(vals.app_name)],
    ['Fuso horário', String(vals.timezone)],
    ['Modo', mode === 'relay' ? 'Relay SMTP (entrega direta)' : `Smarthost (${vals.smarthost_host || '—'}:${vals.smarthost_port})`],
    ['Hostname do relay', String(vals.mta_hostname || '(do container)')],
    ['Autenticação do painel', auth === 'ldap' ? `LDAP/AD (${vals.ldap_host || '—'})` : 'Local'],
    ['Retenção no banco', `${vals.db_retention_days} dias`],
    ['Rotação do log Exim', `${vals.log_rotate_mb} MB, ${vals.log_keep_files} arquivos`],
  ]

  return (
    <>
      <div className="floating-toggle"><ThemeToggle /></div>
      <div className="center" style={{ justifyContent: 'flex-start', paddingTop: 48, paddingBottom: 80 }}>
        <div className="glass card" style={{ width: '100%', maxWidth: 860, padding: 28 }}>
          <Brand />
          <h1>{first ? 'Bem-vindo! Vamos configurar o sistema' : 'Assistente de configuração'}</h1>
          <p className="muted small" style={{ marginTop: -6 }}>Passo {step + 1} de {steps.length}. Tudo pode ser alterado depois em Configurações.</p>
          <div className="wiz-steps">
            {steps.map((s, i) => (
              <span key={s} className={`st ${i === step ? 'on' : i < step ? 'done' : ''}`}>{i < step ? '✓' : i + 1} {s}</span>
            ))}
          </div>

          <div className="stack">
            {step === 0 && <div className="form-grid">{F('app_name')}</div>}

            {step === 1 && (
              <label className="f" style={{ maxWidth: 420 }}>Fuso horário
                <select value={String(vals.timezone)} onChange={(e) => set('timezone', e.target.value)}>
                  {tzList.map((z) => <option key={z}>{z}</option>)}
                </select>
                <span className="help">Usado em relatórios, gráficos e datas exibidas.</span>
              </label>
            )}

            {step === 2 && (
              <>
                <div className="choice">
                  <button type="button" className={mode === 'relay' ? 'on' : ''} onClick={() => set('relay_mode', 'relay')}>
                    <b>Relay SMTP</b><span>Entrega direta aos servidores de destino. Habilita domínios, DKIM, SPF e DMARC.</span>
                  </button>
                  <button type="button" className={mode === 'smarthost' ? 'on' : ''} onClick={() => set('relay_mode', 'smarthost')}>
                    <b>Smarthost</b><span>Encaminha tudo por outro servidor SMTP (com autenticação). Sem gestão de domínios/DKIM.</span>
                  </button>
                </div>
                <div className="form-grid">
                  {F('mta_hostname')}{F('ip_mode')}{F('max_message_mb')}
                  {F('relay_nets')}
                  {mode === 'relay' && <>{F('public_ips')}{F('dmarc_policy')}{F('dmarc_rua')}</>}
                  {mode === 'smarthost' && <>{F('smarthost_host')}{F('smarthost_port')}{F('smarthost_tls')}{F('smarthost_user')}{F('smarthost_pass')}</>}
                </div>
              </>
            )}

            {step === 3 && (
              <>
                <div className="choice">
                  <button type="button" className={auth === 'local' ? 'on' : ''} onClick={() => set('auth_mode', 'local')}>
                    <b>Local</b><span>Usuários e senhas cadastrados no próprio sistema.</span>
                  </button>
                  <button type="button" className={auth === 'ldap' ? 'on' : ''} onClick={() => set('auth_mode', 'ldap')}>
                    <b>LDAP / Active Directory</b><span>Login com credenciais do diretório. Usuários locais continuam funcionando.</span>
                  </button>
                </div>
                {auth === 'ldap' && (
                  <div className="form-grid">
                    {['ldap_host', 'ldap_port', 'ldap_tls', 'ldap_skip_verify', 'ldap_domain', 'ldap_base_dn', 'ldap_user_filter', 'ldap_group_dn', 'ldap_admin_group_dn'].map(F)}
                  </div>
                )}
              </>
            )}

            {step === 4 && <div className="form-grid">{F('db_retention_days')}{F('log_rotate_mb')}{F('log_keep_files')}</div>}

            {step === 5 && (
              <>
                <dl className="summary">
                  {summary.map(([k, v]) => (<><dt key={k}>{k}</dt><dd key={k + 'v'}>{v}</dd></>))}
                </dl>
                {mode === 'relay' && <div className="alert warn">Próximo passo: cadastrar seus domínios e gerar o DKIM. Os registros SPF e DMARC recomendados serão exibidos lá.</div>}
                <span className="help">{label('wizard_done')}: ao concluir, as configurações são salvas e aplicadas ao relay.</span>
              </>
            )}

            {err && <div className="alert err" role="alert">{err}</div>}
            <div className="row spread" style={{ marginTop: 8 }}>
              <div className="row">
                <button disabled={step === 0} onClick={() => setStep(step - 1)}>← Voltar</button>
                {!first && <button onClick={() => nav('/admin/configuracoes')}>Cancelar</button>}
              </div>
              {step < steps.length - 1
                ? <button className="primary" onClick={() => setStep(step + 1)}>Avançar →</button>
                : <button className="primary" disabled={busy} onClick={finish}>{busy ? 'Salvando…' : 'Concluir e aplicar'}</button>}
            </div>
          </div>
        </div>
      </div>
      <div style={{ position: 'fixed', bottom: 0, left: 0, right: 0 }}><Footer /></div>
    </>
  )
}
