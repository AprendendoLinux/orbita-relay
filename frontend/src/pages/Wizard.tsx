import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import { useBranding } from '../branding'
import { Brand, Footer } from '../components/Layout'
import ThemeToggle from '../components/ThemeToggle'
import FieldInput, { applyChange, type Field, type Vals } from '../components/FieldInput'
import CollectPanel from '../components/CollectPanel'
import Tip from '../components/Tip'
import Ipv6Panel from '../components/Ipv6Panel'
import HostSendTip from '../components/HostSendTip'
import NetDetect from '../components/NetDetect'
import DmarcHelp from '../components/DmarcHelp'
import Domains from './Domains'
import { mergeLines } from '../lib'

const stepLabels: Record<string, string> = {
  nome: 'Nome', fuso: 'Fuso horário', impl: 'Implantação', central: 'Servidor central',
  modo: 'Modo', redes: 'Redes', entrega: 'Entrega e DNS', auth: 'Autenticação', logs: 'Logs', resumo: 'Resumo',
}
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
  const deploy = String(vals.deploy_mode ?? 'standalone')
  const mode = String(vals.relay_mode ?? 'relay')
  const steps = useMemo(
    () => ['nome', 'fuso', 'impl', ...(deploy === 'client' ? ['central'] : []), 'modo', 'redes', ...(mode === 'relay' ? ['entrega'] : []), 'auth', 'logs', 'resumo'],
    [deploy, mode],
  )
  const cur = steps[step]

  useEffect(() => {
    api<{ fields: Field[] }>('/api/admin/settings').then((r) => {
      setFields(r.fields)
      setVals(Object.fromEntries(r.fields.map((f) => [f.key, f.value])))
    }).catch((e) => setErr(e.message))
  }, [])

  const F = (key: string) => {
    const f = fields.find((x) => x.key === key)
    return f ? <FieldInput key={key} f={f} v={vals[key]} set={(x) => setVals(applyChange(vals, key, x))} /> : null
  }
  const set = (k: string, v: unknown) => setVals({ ...vals, [k]: v })
  const auth = String(vals.auth_mode ?? 'local')
  const blocked = cur === 'modo' && String(vals.mta_hostname ?? '').trim() === ''

  const [domVer, setDomVer] = useState(0)
  const saveEntrega = async () => {
    setBusy(true)
    setErr('')
    try {
      const body: Vals = {}
      for (const k of ['mta_hostname', 'dkim_selector', 'public_ips', 'dmarc_policy', 'dmarc_rua']) body[k] = vals[k]
      await api('/api/admin/settings', { method: 'PUT', body })
      setDomVer((n) => n + 1)
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const finish = async () => {
    setBusy(true)
    setErr('')
    try {
      const body: Vals = {}
      for (const f of fields) body[f.key] = vals[f.key]
      body.wizard_done = true
      await api('/api/admin/settings', { method: 'PUT', body })
      await refresh()
      nav(deploy === 'server' ? '/admin/servidores' : '/', { replace: true })
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
    ['Implantação', deploy === 'server' ? 'Servidor central' : deploy === 'client' ? 'Cliente' : 'Standalone'],
    ...(deploy === 'client' ? [['Servidor central', `${vals.collect_scheme}://${vals.collect_host || '—'}:${vals.collect_port}${vals.collect_path ?? ''}`] as [string, string]] : []),
    ['Modo', mode === 'relay' ? 'Relay SMTP (entrega direta)' : `Smarthost (${vals.smarthost_host || '—'}:${vals.smarthost_port})`],
    ['Hostname do relay', String(vals.mta_hostname || '(do container)')],
    ...(mode === 'relay' ? [['Seletor DKIM / DMARC', `${vals.dkim_selector} / ${vals.dmarc_policy}`] as [string, string]] : []),
    ['Redes autorizadas', `${String(vals.relay_nets ?? '').split('\n').filter((l) => l.trim()).length} na lista${vals.relay_nets_auto === true ? ' + detecção automática' : ''}`],
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
              <span key={s} className={`st ${i === step ? 'on' : i < step ? 'done' : ''}`}>{i < step ? '✓' : i + 1} {stepLabels[s]}</span>
            ))}
          </div>

          <div className="stack">
            {cur === 'nome' && (
              <>
                <Tip>Identifica esta instalação no painel e na aba do navegador. Em um servidor remoto, use um nome que o diferencie (ex.: <b>Filial São Paulo</b>).</Tip>
                <div className="form-grid">{F('app_name')}</div>
              </>
            )}

            {cur === 'fuso' && (
              <>
              <Tip>Define como datas, horas e gráficos são exibidos. Os dados são sempre armazenados em UTC, então trocar o fuso depois não altera o histórico.</Tip>
              <label className="f" style={{ maxWidth: 420 }}>Fuso horário
                <select value={String(vals.timezone)} onChange={(e) => set('timezone', e.target.value)}>
                  {tzList.map((z) => <option key={z}>{z}</option>)}
                </select>
                <span className="help">Usado em relatórios, gráficos e datas exibidas.</span>
              </label>
              </>
            )}

            {cur === 'impl' && (
              <>
                <Tip title="Como este sistema será usado?">
                  Escolha <b>Standalone</b> se há apenas uma instalação. Para consolidar relatórios de vários relays em um só painel, defina um deles como <b>Servidor central</b> e os demais como <b>Clientes</b>. Cada instalação continua completa, com banco e painel próprios.
                </Tip>
                <div className="choice">
                  <button type="button" className={deploy === 'standalone' ? 'on' : ''} onClick={() => set('deploy_mode', 'standalone')}>
                    <b>Standalone</b><span>Instalação única. Os relatórios mostram apenas este relay.</span>
                  </button>
                  <button type="button" className={deploy === 'server' ? 'on' : ''} onClick={() => set('deploy_mode', 'server')}>
                    <b>Servidor central</b><span>Recebe os eventos de outros relays e unifica os relatórios, com filtro por servidor.</span>
                  </button>
                  <button type="button" className={deploy === 'client' ? 'on' : ''} onClick={() => set('deploy_mode', 'client')}>
                    <b>Cliente</b><span>Envia os eventos deste relay para um servidor central (por HTTP ou HTTPS).</span>
                  </button>
                </div>
                {deploy === 'server' && (
                  <>
                    <Tip title="Próximos passos no servidor central">
                      <ol>
                        <li>Conclua o assistente e abra o menu <b>Servidores</b>.</li>
                        <li>Adicione um servidor para cada relay remoto e copie o <b>token</b> gerado (exibido uma única vez).</li>
                        <li>Os clientes precisam alcançar esta porta pela rede (diretamente ou por um proxy reverso).</li>
                      </ol>
                      Se houver um proxy reverso com HTTPS na frente, mantenha este sistema em HTTP interno e marque a opção abaixo.
                    </Tip>
                    <div className="form-grid">{F('trust_proxy')}</div>
                  </>
                )}
                {deploy === 'client' && (
                  <Tip title="Antes de continuar">Você vai precisar do <b>endereço</b> do servidor central e de um <b>token</b>, gerado no menu <b>Servidores</b> dele. O envio só precisa de conexão de saída deste servidor para o central.</Tip>
                )}
              </>
            )}

            {cur === 'central' && (
              <>
                <Tip title="Conexão com o servidor central">
                  <ol>
                    <li>Escolha o <b>protocolo</b>. Se o central está atrás de um proxy com HTTPS, use <b>HTTPS</b> e porta <b>443</b>.</li>
                    <li>Informe o <b>endereço</b> (nome ou IP) e a porta. Use o <b>caminho base</b> só se o proxy publicar o sistema em um subcaminho (ex.: <span className="mono">/orbita</span>).</li>
                    <li>Cole o <b>token</b> gerado no central e clique em <b>Testar conexão</b>.</li>
                  </ol>
                  Se o certificado for autoassinado, marque “Ignorar validação do certificado”. Se o central ficar indisponível, os eventos são retidos e enviados depois, sem perdas.
                </Tip>
                {vals.collect_scheme === 'http' && <div className="alert warn">Com HTTP o token e os dados trafegam sem criptografia. Use apenas em redes confiáveis.</div>}
                <div className="form-grid">
                  {['collect_scheme', 'collect_host', 'collect_port', 'collect_path', 'collect_token', 'collect_skip_verify', 'collect_interval'].map(F)}
                </div>
                <CollectPanel vals={vals} live={false} />
              </>
            )}

            {cur === 'modo' && (
              <>
                <Tip title="Como o relay entrega as mensagens?">
                  <b>Relay SMTP</b> entrega direto ao destino: exige IP público com DNS reverso e registros SPF, DKIM e DMARC nos seus domínios. <b>Smarthost</b> encaminha por um provedor (Office 365, Gmail, SES…), que cuida da reputação de entrega. Quem pode enviar por este relay é definido no próximo passo, <b>Redes</b>.
                </Tip>
                <div className="choice">
                  <button type="button" className={mode === 'relay' ? 'on' : ''} onClick={() => set('relay_mode', 'relay')}>
                    <b>Relay SMTP</b><span>Entrega direta aos servidores de destino. Habilita domínios, DKIM, SPF e DMARC.</span>
                  </button>
                  <button type="button" className={mode === 'smarthost' ? 'on' : ''} onClick={() => set('relay_mode', 'smarthost')}>
                    <b>Smarthost</b><span>Encaminha tudo por outro servidor SMTP (com autenticação). Sem gestão de domínios/DKIM.</span>
                  </button>
                </div>
                <div className="form-grid">
                  {F('mta_hostname')}{F('max_message_mb')}
                  {mode === 'smarthost' && <>{F('smarthost_host')}{F('smarthost_port')}{F('smarthost_tls')}{F('smarthost_user')}{F('smarthost_pass')}</>}
                </div>
              </>
            )}

            {cur === 'redes' && (
              <>
                <Tip title="Quem pode enviar e-mails por este relay?">
                  Liste os IPs e redes autorizados. O sistema já detecta a rede Docker em que está e a inclui automaticamente; você só precisa adicionar as demais redes (a LAN da empresa, outros servidores) e os IPs da própria máquina.
                </Tip>
                <div className="form-grid">
                  {F('relay_nets')}{F('relay_nets_auto')}{F('ip_mode')}
                </div>
                <NetDetect autoAdd current={String(vals.relay_nets ?? '')} auto={vals.relay_nets_auto === true} version={0} onAdd={(l) => setVals((v) => ({ ...v, relay_nets: mergeLines(String(v.relay_nets ?? ''), l) }))} />
                <HostSendTip />
                {vals.ip_mode === 'dual' && <Ipv6Panel />}
              </>
            )}

            {cur === 'entrega' && (
              <>
                <Tip title="Entrega direta: IPs, DKIM e DNS">
                  No modo Relay SMTP o destinatário confere se o seu domínio autoriza este servidor. Informe os <b>IPs públicos de saída</b> (para o SPF), escolha o <b>seletor DKIM</b> e a política DMARC, cadastre os domínios (a chave DKIM é criada junto) e publique os registros TXT no DNS de cada domínio.
                </Tip>
                <div className="form-grid">
                  {F('dkim_selector')}{F('public_ips')}{F('dmarc_policy')}{F('dmarc_rua')}
                  <DmarcHelp value={String(vals.dmarc_policy ?? 'none')} />
                </div>
                <div className="row">
                  <button type="button" onClick={saveEntrega} disabled={busy}>{busy ? 'Salvando…' : '↻ Salvar e atualizar os registros'}</button>
                  <span className="help">Os registros abaixo usam as configurações salvas.</span>
                </div>
                <Domains embedded selector={String(vals.dkim_selector ?? '')} version={domVer} />
              </>
            )}

            {cur === 'auth' && (
              <>
                <Tip title="Quem pode entrar no painel?">
                  <b>Local</b> usa usuários criados aqui (o primeiro é o administrador). Com <b>LDAP/AD</b>, os usuários entram com a senha do diretório; informe o <b>Domínio UPN</b> (ex.: <span className="mono">@dominio.local</span>), pois o login é feito como <span className="mono">usuario@dominio</span>. Deixe o grupo vazio para liberar todos os usuários do Base DN.
                </Tip>
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

            {cur === 'logs' && (
              <>
                <Tip title="Controle dos logs">
                  O log do Exim é rotacionado por dia ou ao atingir o tamanho definido, e os arquivos antigos são compactados. A <b>retenção no banco</b> define por quantos dias o histórico de mensagens fica disponível nos relatórios.
                </Tip>
                <div className="form-grid">{F('db_retention_days')}{F('log_rotate_mb')}{F('log_keep_files')}</div>
              </>
            )}

            {cur === 'resumo' && (
              <>
                <dl className="summary">
                  {summary.map(([k, v]) => (<><dt key={k}>{k}</dt><dd key={k + 'v'}>{v}</dd></>))}
                </dl>
                {deploy === 'server' && <div className="alert warn">Próximo passo: cadastrar os relays remotos em Servidores e copiar o token de cada um.</div>}
                {mode === 'relay' && deploy !== 'server' && <div className="alert info small">Domínios, chave DKIM e registros de DNS: cadastrados no passo <b>Entrega e DNS</b>. Depois, gerencie-os no menu <b>Domínios e DKIM</b>.</div>}
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
                ? <button className="primary" disabled={blocked} title={blocked ? 'Preencha o hostname do relay' : undefined} onClick={() => setStep(step + 1)}>Avançar →</button>
                : <button className="primary" disabled={busy} onClick={finish}>{busy ? 'Salvando…' : 'Concluir e aplicar'}</button>}
            </div>
          </div>
        </div>
      </div>
      <div className="footer-fixed"><Footer /></div>
    </>
  )
}
