import { useEffect, useState, type FormEvent } from 'react'
import { api } from '../api'
import { Footer } from '../components/Layout'
import Logo from '../components/Logo'
import ThemeToggle from '../components/ThemeToggle'

export default function ResetPassword() {
  const token = new URLSearchParams(window.location.search).get('t') ?? ''
  const [user, setUser] = useState<string | null>(null)
  const [pw, setPw] = useState('')
  const [cf, setCf] = useState('')
  const [err, setErr] = useState('')
  const [done, setDone] = useState(false)
  const [busy, setBusy] = useState(false)
  const mismatch = cf !== '' && pw !== cf

  useEffect(() => {
    api<{ username: string }>(`/api/auth/reset?t=${encodeURIComponent(token)}`)
      .then((r) => setUser(r.username))
      .catch((e) => setErr((e as Error).message))
  }, [token])

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (pw !== cf) {
      setErr('A confirmação da senha não confere')
      return
    }
    setBusy(true)
    setErr('')
    try {
      await api('/api/auth/reset', { method: 'POST', body: { token, password: pw, password_confirm: cf } })
      setDone(true)
    } catch (ex) {
      setErr((ex as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <div className="floating-toggle"><ThemeToggle /></div>
      <div className="center">
        <div className="auth glass">
          <div className="logo"><Logo size={72} /></div>
          <h1>Redefinir senha</h1>
          {done ? (
            <div className="stack">
              <div className="alert ok" role="status">Senha redefinida com sucesso. Todas as sessões abertas foram encerradas.</div>
              <p className="muted small" style={{ textAlign: 'center' }}>Se a sua conta usa 2FA, o código continuará sendo pedido no login.</p>
              <button className="primary" style={{ justifyContent: 'center' }} onClick={() => window.location.assign('/')}>Ir para o login</button>
            </div>
          ) : user ? (
            <form className="stack" onSubmit={submit}>
              <p className="muted small" style={{ textAlign: 'center', marginTop: -6 }}>Escolha a nova senha da conta <b>{user}</b>.</p>
              <label className="f">Nova senha (mín. 10 caracteres)
                <input type="password" value={pw} onChange={(e) => setPw(e.target.value)} minLength={10} autoComplete="new-password" autoFocus required />
              </label>
              <label className="f">Confirme a nova senha
                <input type="password" value={cf} onChange={(e) => setCf(e.target.value)} minLength={10} autoComplete="new-password" required aria-invalid={mismatch} />
                {mismatch && <span className="help" style={{ color: 'var(--danger)' }}>As senhas não conferem</span>}
                {cf !== '' && !mismatch && <span className="help" style={{ color: 'var(--success)' }}>As senhas conferem ✓</span>}
              </label>
              {err && <div className="alert err" role="alert">{err}</div>}
              <button className="primary" disabled={busy || mismatch} style={{ justifyContent: 'center' }}>{busy ? 'Salvando…' : 'Redefinir senha'}</button>
            </form>
          ) : err ? (
            <div className="stack">
              <div className="alert err" role="alert">{err}</div>
              <button style={{ justifyContent: 'center' }} onClick={() => window.location.assign('/recuperar-senha')}>Pedir um novo link</button>
            </div>
          ) : <p className="muted" style={{ textAlign: 'center' }}>Verificando o link…</p>}
        </div>
      </div>
      <div className="footer-fixed"><Footer /></div>
    </>
  )
}
