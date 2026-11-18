import { useEffect, useState } from 'react'
import { api } from '../api'
import { Footer } from '../components/Layout'
import Logo from '../components/Logo'
import ThemeToggle from '../components/ThemeToggle'

export default function ConfirmEmail() {
  const token = new URLSearchParams(window.location.search).get('t') ?? ''
  const [info, setInfo] = useState<{ username: string; email: string } | null>(null)
  const [err, setErr] = useState('')
  const [done, setDone] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api<{ username: string; email: string }>(`/api/auth/confirm-email?t=${encodeURIComponent(token)}`)
      .then(setInfo)
      .catch((e) => setErr((e as Error).message))
  }, [token])

  const confirm = async () => {
    setBusy(true)
    setErr('')
    try {
      await api('/api/auth/confirm-email', { method: 'POST', body: { token } })
      setDone(true)
    } catch (e) {
      setErr((e as Error).message)
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
          <h1>Confirmar novo e-mail</h1>
          <div className="stack">
            {done ? (
              <>
                <div className="alert ok" role="status">E-mail alterado com sucesso. A conta <b>{info?.username}</b> agora usa <b>{info?.email}</b>.</div>
                <button className="primary" style={{ justifyContent: 'center' }} onClick={() => window.location.assign('/')}>Ir para o sistema</button>
              </>
            ) : info ? (
              <>
                <p className="muted small" style={{ textAlign: 'center' }}>Confirme que o e-mail da conta <b>{info.username}</b> passa a ser:</p>
                <code className="block mono" style={{ textAlign: 'center', wordBreak: 'break-all' }}>{info.email}</code>
                {err && <div className="alert err" role="alert">{err}</div>}
                <button className="primary" disabled={busy} style={{ justifyContent: 'center' }} onClick={confirm}>{busy ? 'Confirmando…' : 'Confirmar alteração'}</button>
                <a className="small" style={{ textAlign: 'center' }} href="/">Cancelar</a>
              </>
            ) : err ? (
              <>
                <div className="alert err" role="alert">{err}</div>
                <button style={{ justifyContent: 'center' }} onClick={() => window.location.assign('/')}>Ir para o sistema</button>
              </>
            ) : <p className="muted" style={{ textAlign: 'center' }}>Verificando o link…</p>}
          </div>
        </div>
      </div>
      <div style={{ position: 'fixed', bottom: 0, left: 0, right: 0 }}><Footer /></div>
    </>
  )
}
