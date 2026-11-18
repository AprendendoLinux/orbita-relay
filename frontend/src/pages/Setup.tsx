import { useState, type FormEvent } from 'react'
import { api } from '../api'
import { Footer } from '../components/Layout'
import { useBranding } from '../branding'
import Logo from '../components/Logo'
import ThemeToggle from '../components/ThemeToggle'

export default function Setup() {
  const { branding, refresh } = useBranding()
  const [step, setStep] = useState(branding?.db_ready ? 2 : 1)
  const [token, setToken] = useState('')
  const [db, setDb] = useState({ host: 'orbitarelay-db', port: '3306', user: 'orbita', password: '', name: 'orbita', sslmode: 'preferred' })
  const [adm, setAdm] = useState({ app_name: 'OrbitaRelay', username: 'admin', email: '', password: '', password_confirm: '' })
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const hdr = { 'X-Setup-Token': token.trim() }

  const run = async (fn: () => Promise<void>) => {
    setBusy(true)
    setErr('')
    try {
      await fn()
    } catch (ex) {
      setErr((ex as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const saveDb = (e: FormEvent) => {
    e.preventDefault()
    run(async () => {
      await api('/api/setup/db', { method: 'POST', headers: hdr, body: { ...db, port: Number(db.port) } })
      setStep(2)
    })
  }

  const mismatch = adm.password_confirm !== '' && adm.password !== adm.password_confirm
  const saveAdmin = (e: FormEvent) => {
    e.preventDefault()
    if (adm.password !== adm.password_confirm) {
      setErr('A confirmação da senha não confere')
      return
    }
    run(async () => {
      await api('/api/setup/admin', { method: 'POST', headers: hdr, body: adm })
      await refresh()
      window.location.assign('/')
    })
  }

  return (
    <>
      <div className="floating-toggle"><ThemeToggle /></div>
      <div className="center">
        <div className="auth glass" style={{ maxWidth: 520 }}>
          <div className="logo"><Logo size={72} /></div>
          <div className="steps"><span className="on" /><span className={step === 2 ? 'on' : ''} /></div>
          <h1>Configuração inicial</h1>
          <p className="muted small" style={{ textAlign: 'center', marginTop: -6 }}>
            O token está nos logs do container: <code className="mono">docker compose logs orbitarelay-app</code>
          </p>
          {step === 1 ? (
            <form className="stack" onSubmit={saveDb}>
              <label className="f">Token de configuração
                <input value={token} onChange={(e) => setToken(e.target.value)} required autoFocus />
              </label>
              <h3>Banco de dados (MySQL)</h3>
              <div className="form-grid">
                <label className="f">Host<input value={db.host} onChange={(e) => setDb({ ...db, host: e.target.value })} required /></label>
                <label className="f">Porta<input value={db.port} onChange={(e) => setDb({ ...db, port: e.target.value })} required /></label>
                <label className="f">Usuário<input value={db.user} onChange={(e) => setDb({ ...db, user: e.target.value })} required /></label>
                <label className="f">Senha<input type="password" value={db.password} onChange={(e) => setDb({ ...db, password: e.target.value })} /></label>
                <label className="f">Banco<input value={db.name} onChange={(e) => setDb({ ...db, name: e.target.value })} required /></label>
                <label className="f">SSL
                  <select value={db.sslmode} onChange={(e) => setDb({ ...db, sslmode: e.target.value })}>
                    {['disable', 'preferred', 'required', 'skip-verify'].map((m) => <option key={m}>{m}</option>)}
                  </select>
                </label>
              </div>
              {err && <div className="alert err" role="alert">{err}</div>}
              <button className="primary" disabled={busy} style={{ justifyContent: 'center' }}>{busy ? 'Testando…' : 'Testar e continuar'}</button>
            </form>
          ) : (
            <form className="stack" onSubmit={saveAdmin}>
              {branding?.db_ready && (
                <label className="f">Token de configuração
                  <input value={token} onChange={(e) => setToken(e.target.value)} required />
                </label>
              )}
              <label className="f">Nome do sistema<input value={adm.app_name} onChange={(e) => setAdm({ ...adm, app_name: e.target.value })} required /></label>
              <h3>Administrador</h3>
              <div className="form-grid">
                <label className="f">Usuário<input value={adm.username} onChange={(e) => setAdm({ ...adm, username: e.target.value })} required /></label>
                <label className="f">E-mail<input type="email" value={adm.email} onChange={(e) => setAdm({ ...adm, email: e.target.value })} /></label>
              </div>
              <label className="f">Senha (mín. 10 caracteres)
                <input type="password" value={adm.password} onChange={(e) => setAdm({ ...adm, password: e.target.value })} minLength={10} required autoComplete="new-password" />
              </label>
              <label className="f">Confirme a senha
                <input type="password" value={adm.password_confirm} onChange={(e) => setAdm({ ...adm, password_confirm: e.target.value })} minLength={10} required autoComplete="new-password" aria-invalid={mismatch} />
                {mismatch && <span className="help" style={{ color: 'var(--danger)' }}>As senhas não conferem</span>}
                {adm.password_confirm !== '' && !mismatch && <span className="help" style={{ color: 'var(--success)' }}>As senhas conferem ✓</span>}
              </label>
              {err && <div className="alert err" role="alert">{err}</div>}
              <button className="primary" disabled={busy || mismatch} style={{ justifyContent: 'center' }}>{busy ? 'Criando…' : 'Concluir'}</button>
            </form>
          )}
        </div>
      </div>
      <div style={{ position: 'fixed', bottom: 0, left: 0, right: 0 }}><Footer /></div>
    </>
  )
}
