import { useCallback, useEffect, useState, type FormEvent } from 'react'
import QRCode from 'qrcode'
import { api } from '../api'
import { useDialog } from '../components/Dialog'

type Status = { method: '' | 'totp' | 'email'; email: string; recovery_left: number; smtp_ready: boolean; needs_password: boolean; user_email: string; trusted_devices: number }
type Msg = { t: 'ok' | 'err'; s: string } | null

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

function Recovery({ codes, onDone }: { codes: string[]; onDone: () => void }) {
  const [copied, setCopied] = useState(false)
  const text = codes.join('\n')
  const download = () => {
    const url = URL.createObjectURL(new Blob([`Códigos de recuperação do 2FA\n\n${text}\n\nCada código vale uma única vez.\n`], { type: 'text/plain' }))
    const a = document.createElement('a')
    a.href = url
    a.download = 'codigos-de-recuperacao.txt'
    a.click()
    URL.revokeObjectURL(url)
  }
  return (
    <div className="glass card stack">
      <h3>2FA ativado. Guarde seus códigos de recuperação</h3>
      <div className="alert warn">Cada código vale <b>uma única vez</b> e substitui o código do aplicativo ou do e-mail se você perder o acesso. Eles <b>não serão exibidos novamente</b>. Guarde-os em um lugar seguro.</div>
      <div className="codes">{codes.map((c) => <code key={c} className="mono">{c}</code>)}</div>
      <div className="row">
        <button className="sm" onClick={async () => { await copyText(text); setCopied(true); setTimeout(() => setCopied(false), 1500) }}>{copied ? 'Copiado ✓' : 'Copiar'}</button>
        <button className="sm" onClick={download}>Baixar .txt</button>
        <button className="primary" onClick={onDone}>Já guardei os códigos</button>
      </div>
    </div>
  )
}

export default function Security() {
  const dlg = useDialog()
  const [st, setSt] = useState<Status | null>(null)
  const [msg, setMsg] = useState<Msg>(null)
  const [mode, setMode] = useState<'' | 'totp' | 'email' | 'recovery'>('')
  const [pw, setPw] = useState('')
  const [code, setCode] = useState('')
  const [totp, setTotp] = useState<{ secret: string; qr: string } | null>(null)
  const [sent, setSent] = useState('')
  const [codes, setCodes] = useState<string[] | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(() => api<Status>('/api/me/mfa').then(setSt), [])
  useEffect(() => { load().catch((e) => setMsg({ t: 'err', s: e.message })) }, [load])

  const reset = () => { setMode(''); setPw(''); setCode(''); setTotp(null); setSent(''); setMsg(null) }
  const run = async (fn: () => Promise<void>) => {
    setBusy(true)
    setMsg(null)
    try { await fn() } catch (e) { setMsg({ t: 'err', s: (e as Error).message }) } finally { setBusy(false) }
  }

  const startTotp = (e: FormEvent) => {
    e.preventDefault()
    run(async () => {
      const r = await api<{ secret: string; uri: string }>('/api/me/mfa/totp/setup', { method: 'POST', body: { password: pw } })
      setTotp({ secret: r.secret, qr: await QRCode.toDataURL(r.uri, { margin: 1, width: 400, errorCorrectionLevel: 'M' }) })
    })
  }
  const enableTotp = (e: FormEvent) => {
    e.preventDefault()
    run(async () => {
      const r = await api<{ recovery: string[] }>('/api/me/mfa/totp/enable', { method: 'POST', body: { code } })
      setCodes(r.recovery)
      reset()
      await load()
    })
  }
  const sendEmail = (e: FormEvent) => {
    e.preventDefault()
    run(async () => {
      const r = await api<{ email_hint: string }>('/api/me/mfa/email/setup', { method: 'POST', body: { password: pw } })
      setSent(r.email_hint)
      setMsg({ t: 'ok', s: `Código enviado para ${r.email_hint}. Ele vale por 10 minutos.` })
    })
  }
  const enableEmail = (e: FormEvent) => {
    e.preventDefault()
    run(async () => {
      const r = await api<{ recovery: string[] }>('/api/me/mfa/email/enable', { method: 'POST', body: { code } })
      setCodes(r.recovery)
      reset()
      await load()
    })
  }
  const disable = async () => {
    if (!st) return
    const r = await dlg.form({
      title: 'Desativar a verificação em duas etapas?',
      danger: true,
      confirmLabel: 'Sim, desativar o 2FA',
      message: (
        <>
          <p style={{ margin: '0 0 10px' }}><b>Isso é arriscado.</b> Sem o 2FA, qualquer pessoa que descubra a sua senha consegue entrar na sua conta.</p>
          <p style={{ margin: 0 }}>Os códigos de recuperação e os dispositivos confiáveis também serão apagados.{st.needs_password ? ' Para confirmar, informe a sua senha.' : ''}</p>
        </>
      ),
      fields: st.needs_password ? [{ name: 'password', label: 'Sua senha atual', type: 'password', autoComplete: 'current-password' }] : [],
    })
    if (!r) return
    run(async () => {
      await api('/api/me/mfa/disable', { method: 'POST', body: { password: r.password ?? '' } })
      reset()
      setMsg({ t: 'ok', s: '2FA desativado.' })
      await load()
    })
  }
  const revokeTrusted = async () => {
    if (!(await dlg.confirm({ title: 'Remover os dispositivos confiáveis?', message: 'O código do 2FA voltará a ser pedido em todos os dispositivos no próximo login.', confirmLabel: 'Remover' }))) return
    run(async () => {
      await api('/api/me/mfa/trusted/revoke', { method: 'POST' })
      setMsg({ t: 'ok', s: 'Dispositivos confiáveis removidos.' })
      await load()
    })
  }
  const regen = (e: FormEvent) => {
    e.preventDefault()
    run(async () => {
      const r = await api<{ recovery: string[] }>('/api/me/mfa/recovery', { method: 'POST', body: { code, password: pw } })
      setCodes(r.recovery)
      reset()
      await load()
    })
  }

  if (!st) return <p className="muted">Carregando…</p>
  const pwField = st.needs_password && (
    <label className="f">Sua senha atual<input type="password" value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="current-password" required /></label>
  )
  const codeField = (label = 'Código de 6 dígitos') => (
    <label className="f">{label}
      <input className="otp" value={code} onChange={(e) => setCode(e.target.value.replace(/[^0-9]/g, '').slice(0, 6))} inputMode="numeric" autoComplete="one-time-code" placeholder="000000" maxLength={6} required autoFocus />
    </label>
  )
  const anyCode = (
    <label className="f">Código do 2FA ou código de recuperação
      <input value={code} onChange={(e) => setCode(e.target.value)} autoComplete="one-time-code" required autoFocus />
    </label>
  )

  return (
    <>
      <div className="topbar"><h1>Segurança (2FA)</h1></div>
      <p className="muted">A verificação em duas etapas protege a sua conta mesmo que a senha vaze. Vale para usuários locais e do LDAP/AD. Ao entrar, além da senha, você informa um código.</p>
      {msg && <div className={`alert ${msg.t}`} role="status" style={{ margin: '12px 0' }}>{msg.s}</div>}
      {codes && <Recovery codes={codes} onDone={() => setCodes(null)} />}

      {!codes && st.method !== '' && (
        <div className="glass card stack" style={{ maxWidth: 560 }}>
          <div className="row spread">
            <h3>Verificação em duas etapas</h3>
            <span className="badge b-ok">ativa</span>
          </div>
          <div className="small">Método: <b>{st.method === 'totp' ? 'Aplicativo autenticador' : `Código por e-mail (${st.email})`}</b></div>
          <div className="small muted">Códigos de recuperação restantes: <b>{st.recovery_left}</b></div>
          <div className="small muted">
            Dispositivos confiáveis (sem pedir o código por 30 dias): <b>{st.trusted_devices}</b>{' '}
            {st.trusted_devices > 0 && <button type="button" className="sm" onClick={revokeTrusted}>Remover todos</button>}
          </div>
          {mode === '' && (
            <div className="row">
              <button onClick={() => { reset(); setMode('recovery') }}>Gerar novos códigos de recuperação</button>
              <button className="danger" onClick={disable}>Desativar 2FA</button>
            </div>
          )}
          {mode === 'recovery' && (
            <form className="stack" onSubmit={regen}>
              <b>Confirme para gerar novos códigos (os antigos deixam de valer)</b>
              {pwField}
              {anyCode}
              <div className="row">
                <button className="primary" disabled={busy}>Gerar códigos</button>
                <button type="button" onClick={reset}>Cancelar</button>
              </div>
            </form>
          )}
        </div>
      )}

      {!codes && st.method === '' && (
        <>
          <div className="alert warn" style={{ marginBottom: 12 }}>O 2FA está <b>desativado</b>. Escolha um método abaixo.</div>
          {mode === '' && (
            <div className="grid cols-2">
              <div className="glass card stack">
                <h3>Aplicativo autenticador <span className="badge b-ok">recomendado</span></h3>
                <p className="muted small">Google Authenticator, Microsoft Authenticator, Authy, 1Password, Bitwarden… Você escaneia um QR Code e passa a usar códigos de 6 dígitos que mudam a cada 30 segundos.</p>
                <div><button className="primary" onClick={() => { reset(); setMode('totp') }}>Configurar aplicativo</button></div>
              </div>
              <div className="glass card stack">
                <h3>Código por e-mail</h3>
                <p className="muted small">A cada acesso enviamos um código de 6 dígitos para o seu e-mail.</p>
                {!st.smtp_ready && <div className="alert warn small">O envio de e-mails ainda não foi configurado. Um administrador precisa preencher o SMTP em Configurações &gt; E-mail do sistema (2FA).</div>}
                <div><button disabled={!st.smtp_ready} onClick={() => { reset(); setMode('email') }}>Configurar e-mail</button></div>
              </div>
            </div>
          )}

          {mode === 'totp' && (
            <div className="glass card stack" style={{ maxWidth: 560 }}>
              <h3>Configurar aplicativo autenticador</h3>
              {!totp ? (
                <form className="stack" onSubmit={startTotp}>
                  <p className="muted small">Passo 1 de 2. {st.needs_password ? 'Confirme a sua senha para gerar o QR Code.' : 'Clique para gerar o QR Code.'}</p>
                  {pwField}
                  <div className="row"><button className="primary" disabled={busy}>Gerar QR Code</button><button type="button" onClick={reset}>Cancelar</button></div>
                </form>
              ) : (
                <form className="stack" onSubmit={enableTotp}>
                  <ol className="guide">
                    <li>Abra o aplicativo autenticador e escolha <b>adicionar conta</b>.</li>
                    <li>Escaneie o QR Code ou digite a chave manualmente.</li>
                    <li>Digite abaixo o código de 6 dígitos que o aplicativo mostrar.</li>
                  </ol>
                  <div className="qr"><img src={totp.qr} alt="QR Code para o aplicativo autenticador" /></div>
                  <div className="stack" style={{ gap: 6 }}>
                    <span className="small muted">Não consegue escanear? Digite esta chave (tipo: baseada em tempo):</span>
                    <code className="block mono" style={{ wordBreak: 'break-all' }}>{totp.secret.replace(/(.{4})/g, '$1 ').trim()}</code>
                    <div><button type="button" className="sm" onClick={() => copyText(totp.secret)}>Copiar chave</button></div>
                  </div>
                  {codeField()}
                  <div className="row"><button className="primary" disabled={busy || code.length !== 6}>Ativar 2FA</button><button type="button" onClick={reset}>Cancelar</button></div>
                </form>
              )}
            </div>
          )}

          {mode === 'email' && (
            <div className="glass card stack" style={{ maxWidth: 560 }}>
              <h3>Configurar código por e-mail</h3>
              {!sent ? (
                <form className="stack" onSubmit={sendEmail}>
                  {st.user_email ? (
                    <p className="muted small">O código será enviado para o e-mail cadastrado na sua conta: <b>{st.user_email}</b>. Para usar outro endereço, altere o e-mail em {st.needs_password ? <b>Minha conta</b> : 'seu cadastro'} antes.</p>
                  ) : (
                    <div className="alert warn small">A sua conta não tem e-mail cadastrado. {st.needs_password ? 'Cadastre um em Minha conta' : 'Peça a um administrador para cadastrar'} antes de usar o código por e-mail.</div>
                  )}
                  {pwField}
                  <div className="row"><button className="primary" disabled={busy || !st.user_email}>Enviar código</button><button type="button" onClick={reset}>Cancelar</button></div>
                </form>
              ) : (
                <form className="stack" onSubmit={enableEmail}>
                  <p className="muted small">Digite o código enviado para <b>{sent}</b>.</p>
                  {codeField('Código recebido')}
                  <div className="row">
                    <button className="primary" disabled={busy || code.length !== 6}>Ativar 2FA</button>
                    <button type="button" disabled={busy} onClick={(e) => { setCode(''); sendEmail(e as unknown as FormEvent) }}>Reenviar código</button>
                    <button type="button" onClick={reset}>Cancelar</button>
                  </div>
                </form>
              )}
            </div>
          )}
        </>
      )}
    </>
  )
}
