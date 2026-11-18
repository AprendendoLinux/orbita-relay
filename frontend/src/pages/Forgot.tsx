import { useState, type FormEvent } from 'react'
import { api } from '../api'
import { Footer } from '../components/Layout'
import Logo from '../components/Logo'
import ThemeToggle from '../components/ThemeToggle'

export default function Forgot() {
  const [ident, setIdent] = useState('')
  const [err, setErr] = useState('')
  const [sent, setSent] = useState(false)
  const [busy, setBusy] = useState(false)

  const request = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setErr('')
    try {
      await api('/api/auth/forgot', { method: 'POST', body: { identifier: ident } })
      setSent(true)
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
          <h1>Recuperar minha senha</h1>
          {sent ? (
            <div className="stack">
              <div className="alert ok" role="status">Se existir uma conta local com esse usuário ou e-mail, enviamos um <b>link para redefinir a senha</b> para o e-mail cadastrado. O link vale por 30 minutos.</div>
              <p className="muted small" style={{ textAlign: 'center' }}>Não chegou? Confira a caixa de spam e aguarde um minuto antes de pedir de novo.</p>
              <button style={{ justifyContent: 'center' }} onClick={() => window.location.assign('/')}>← Voltar ao login</button>
            </div>
          ) : (
            <form className="stack" onSubmit={request}>
              <p className="muted small" style={{ textAlign: 'center', marginTop: -6 }}>Informe o seu usuário ou e-mail. Enviaremos um link para redefinir a senha ao e-mail cadastrado na conta.</p>
              <label className="f">Usuário ou e-mail
                <input value={ident} onChange={(e) => setIdent(e.target.value)} autoComplete="username" autoFocus required />
              </label>
              {err && <div className="alert err" role="alert">{err}</div>}
              <button className="primary" disabled={busy} style={{ justifyContent: 'center' }}>{busy ? 'Enviando…' : 'Enviar link'}</button>
              <a className="small" style={{ textAlign: 'center' }} href="/">← Voltar ao login</a>
              <span className="help">Usuários do LDAP/AD alteram a senha no diretório da empresa.</span>
            </form>
          )}
        </div>
      </div>
      <div style={{ position: 'fixed', bottom: 0, left: 0, right: 0 }}><Footer /></div>
    </>
  )
}
