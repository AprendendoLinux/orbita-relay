import { useEffect, useState, type FormEvent } from 'react'
import { api } from '../api'
import { Footer } from '../components/Layout'
import { useBranding } from '../branding'
import Logo from '../components/Logo'
import ThemeToggle from '../components/ThemeToggle'

export default function Login() {
  const { branding } = useBranding()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [mfa, setMfa] = useState<{ token: string; method: string; hint?: string } | null>(null)
  const [code, setCode] = useState('')
  const [cool, setCool] = useState(0)
  const [info, setInfo] = useState('')
  const [remember, setRemember] = useState(false)

  useEffect(() => {
    if (cool <= 0) return
    const t = setTimeout(() => setCool(cool - 1), 1000)
    return () => clearTimeout(t)
  }, [cool])

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setErr('')
    try {
      const r = await api<{ mfa_required?: boolean; mfa_token?: string; method?: string; email_hint?: string }>('/api/auth/login', { method: 'POST', body: { username, password } })
      if (r.mfa_required && r.mfa_token) {
        setMfa({ token: r.mfa_token, method: r.method ?? 'totp', hint: r.email_hint })
        setPassword('')
        setCool(30)
        setBusy(false)
        return
      }
      window.location.assign('/')
    } catch (ex) {
      setErr((ex as Error).message)
      setBusy(false)
    }
  }

  const verify = async (e: FormEvent) => {
    e.preventDefault()
    if (!mfa) return
    setBusy(true)
    setErr('')
    try {
      await api('/api/auth/mfa', { method: 'POST', body: { mfa_token: mfa.token, code, remember } })
      window.location.assign('/')
    } catch (ex) {
      setErr((ex as Error).message)
      setBusy(false)
    }
  }

  const resend = async () => {
    if (!mfa) return
    setErr('')
    setInfo('')
    try {
      const r = await api<{ email_hint: string }>('/api/auth/mfa/resend', { method: 'POST', body: { mfa_token: mfa.token } })
      setInfo(`Novo código enviado para ${r.email_hint}.`)
      setCool(30)
    } catch (ex) {
      setErr((ex as Error).message)
    }
  }

  if (mfa) {
    return (
      <>
        <div className="floating-toggle"><ThemeToggle /></div>
        <div className="center">
          <form className="auth glass" onSubmit={verify}>
            <div className="logo"><Logo size={84} /></div>
            <h1>Verificação em duas etapas</h1>
            <p className="muted small" style={{ textAlign: 'center', marginTop: -6 }}>
              {mfa.method === 'email'
                ? <>Enviamos um código de 6 dígitos para <b>{mfa.hint}</b>.</>
                : 'Digite o código de 6 dígitos do seu aplicativo autenticador.'}
            </p>
            <div className="stack">
              <label className="f">Código
                <input className="otp" value={code} onChange={(e) => setCode(e.target.value.trim())} inputMode="text" autoComplete="one-time-code" placeholder="000000" maxLength={11} autoFocus required />
              </label>
              <span className="help">Sem acesso ao {mfa.method === 'email' ? 'e-mail' : 'aplicativo'}? Use um código de recuperação (ex.: abcde-fghij).</span>
              <label className="switch" style={{ alignSelf: 'flex-start' }}>
                <input type="checkbox" checked={remember} onChange={(e) => setRemember(e.target.checked)} />
                <span className="track" aria-hidden="true"><span className="thumb" /></span>
                Não pedir o código neste dispositivo por 30 dias
              </label>
              {info && <div className="alert ok" role="status">{info}</div>}
              {err && <div className="alert err" role="alert">{err}</div>}
              <button className="primary" disabled={busy || code.length < 6} style={{ justifyContent: 'center' }}>{busy ? 'Verificando…' : 'Verificar'}</button>
              <div className="row spread">
                <button type="button" className="sm" onClick={() => { setMfa(null); setCode(''); setErr(''); setInfo('') }}>← Voltar</button>
                {mfa.method === 'email' && <button type="button" className="sm" disabled={cool > 0} onClick={resend}>{cool > 0 ? `Reenviar em ${cool}s` : 'Reenviar código'}</button>}
              </div>
            </div>
          </form>
        </div>
        <div className="footer-fixed"><Footer /></div>
      </>
    )
  }

  return (
    <>
      <div className="floating-toggle"><ThemeToggle /></div>
      <div className="center">
        <form className="auth glass" onSubmit={submit}>
          <div className="logo"><Logo size={84} /></div>
          <h1>{branding?.app_name}</h1>
          <p className="muted small" style={{ textAlign: 'center', marginTop: -6 }}>Acesse com suas credenciais</p>
          <div className="stack">
            <label className="f">Usuário
              <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus required />
            </label>
            <label className="f">Senha
              <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required />
            </label>
            {err && <div className="alert err" role="alert">{err}</div>}
            <button className="primary" disabled={busy} style={{ justifyContent: 'center' }}>{busy ? 'Entrando…' : 'Entrar'}</button>
            {branding?.can_reset && <a href="/recuperar-senha" className="small" style={{ textAlign: 'center' }}>Esqueci minha senha</a>}
          </div>
        </form>
      </div>
      <div className="footer-fixed"><Footer /></div>
    </>
  )
}
