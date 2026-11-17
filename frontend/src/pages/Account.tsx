import { useState, type FormEvent } from 'react'
import { api } from '../api'

export default function Account() {
  const [cur, setCur] = useState('')
  const [nw, setNw] = useState('')
  const [msg, setMsg] = useState<{ t: 'ok' | 'err'; s: string } | null>(null)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    try {
      await api('/api/me/password', { method: 'POST', body: { current: cur, new: nw } })
      setMsg({ t: 'ok', s: 'Senha alterada. Outras sessões foram encerradas.' })
      setCur('')
      setNw('')
    } catch (ex) {
      setMsg({ t: 'err', s: (ex as Error).message })
    }
  }

  return (
    <>
      <div className="topbar"><h1>Minha conta</h1></div>
      <form className="glass card stack" style={{ maxWidth: 460 }} onSubmit={submit}>
        <h3>Alterar senha</h3>
        <label className="f">Senha atual<input type="password" value={cur} onChange={(e) => setCur(e.target.value)} autoComplete="current-password" required /></label>
        <label className="f">Nova senha (mín. 10)<input type="password" value={nw} onChange={(e) => setNw(e.target.value)} minLength={10} autoComplete="new-password" required /></label>
        {msg && <div className={`alert ${msg.t}`} role="status">{msg.s}</div>}
        <div><button className="primary">Alterar senha</button></div>
      </form>
    </>
  )
}
