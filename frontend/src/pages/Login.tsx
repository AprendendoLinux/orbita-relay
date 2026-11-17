import { useState, type FormEvent } from 'react'
import { api } from '../api'
import { Brand, Footer } from '../components/Layout'
import { useBranding } from '../branding'

export default function Login() {
  const { branding } = useBranding()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setErr('')
    try {
      await api('/api/auth/login', { method: 'POST', body: { username, password } })
      window.location.assign('/')
    } catch (ex) {
      setErr((ex as Error).message)
      setBusy(false)
    }
  }

  return (
    <>
      <div className="center">
        <form className="auth glass" onSubmit={submit}>
          {branding?.theme.logo ? (
            <div className="logo"><img src={branding.theme.logo} alt="" /></div>
          ) : (
            <Brand />
          )}
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
          </div>
        </form>
      </div>
      <div style={{ position: 'fixed', bottom: 0, left: 0, right: 0 }}><Footer /></div>
    </>
  )
}
