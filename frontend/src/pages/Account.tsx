import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { api, fmtDate } from '../api'
import { useBranding } from '../branding'

type Msg = { t: 'ok' | 'err'; s: string } | null
type EmailInfo = { email: string; smtp_ready: boolean; pending?: { email: string; expires_at: string } }

export default function Account() {
  const { branding } = useBranding()
  const [cur, setCur] = useState('')
  const [nw, setNw] = useState('')
  const [cf, setCf] = useState('')
  const [msg, setMsg] = useState<Msg>(null)

  const [em, setEm] = useState<EmailInfo | null>(null)
  const [newEmail, setNewEmail] = useState('')
  const [emPw, setEmPw] = useState('')
  const [emMsg, setEmMsg] = useState<Msg>(null)
  const [busy, setBusy] = useState(false)

  const loadEmail = useCallback(() => api<EmailInfo>('/api/me/email').then(setEm), [])
  useEffect(() => { loadEmail().catch((e) => setEmMsg({ t: 'err', s: e.message })) }, [loadEmail])

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (nw !== cf) {
      setMsg({ t: 'err', s: 'A confirmação da nova senha não confere' })
      return
    }
    try {
      await api('/api/me/password', { method: 'POST', body: { current: cur, new: nw } })
      setMsg({ t: 'ok', s: 'Senha alterada. Outras sessões foram encerradas.' })
      setCur('')
      setNw('')
      setCf('')
    } catch (ex) {
      setMsg({ t: 'err', s: (ex as Error).message })
    }
  }

  const changeEmail = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setEmMsg(null)
    try {
      const r = await api<{ email_hint: string }>('/api/me/email/change', { method: 'POST', body: { email: newEmail, password: emPw } })
      setEmMsg({ t: 'ok', s: `Enviamos um e-mail para ${r.email_hint}. Clique no botão dentro dele para confirmar. Até lá, o e-mail atual continua valendo.` })
      setNewEmail('')
      setEmPw('')
      await loadEmail()
    } catch (ex) {
      setEmMsg({ t: 'err', s: (ex as Error).message })
    } finally {
      setBusy(false)
    }
  }

  const cancel = async () => {
    await api('/api/me/email/change', { method: 'DELETE' })
    setEmMsg({ t: 'ok', s: 'Alteração cancelada.' })
    await loadEmail()
  }

  return (
    <>
      <div className="topbar"><h1>Minha conta</h1></div>
      <div className="stack" style={{ maxWidth: 520 }}>
        <form className="glass card stack" onSubmit={changeEmail}>
          <h3>E-mail da conta</h3>
          <div className="small">Atual: <b>{em?.email || <span className="muted">Sem e-mail cadastrado</span>}</b></div>
          <span className="help">É para este e-mail que vão a recuperação de senha e as notificações de segurança.</span>
          {em && !em.smtp_ready && <div className="alert warn small">O envio de e-mails ainda não foi configurado. Um administrador precisa preencher o SMTP em Configurações &gt; E-mail do sistema para você poder alterar o e-mail.</div>}
          {em?.pending && (
            <div className="alert warn small">
              Aguardando confirmação de <b>{em.pending.email}</b> (o link vale até {fmtDate(em.pending.expires_at, branding?.timezone)}).{' '}
              <button type="button" className="sm" onClick={cancel}>Cancelar alteração</button>
            </div>
          )}
          <label className="f">Novo e-mail<input type="email" value={newEmail} onChange={(e) => setNewEmail(e.target.value)} required disabled={!em?.smtp_ready} autoComplete="email" /></label>
          <label className="f">Sua senha atual<input type="password" value={emPw} onChange={(e) => setEmPw(e.target.value)} required disabled={!em?.smtp_ready} autoComplete="current-password" /></label>
          {emMsg && <div className={`alert ${emMsg.t}`} role="status">{emMsg.s}</div>}
          <div><button className="primary" disabled={busy || !em?.smtp_ready}>{busy ? 'Enviando…' : 'Enviar e-mail de confirmação'}</button></div>
        </form>

        <form className="glass card stack" onSubmit={submit}>
          <h3>Alterar senha</h3>
          <label className="f">Senha atual<input type="password" value={cur} onChange={(e) => setCur(e.target.value)} autoComplete="current-password" required /></label>
          <label className="f">Nova senha (mín. 10)<input type="password" value={nw} onChange={(e) => setNw(e.target.value)} minLength={10} autoComplete="new-password" required /></label>
          <label className="f">Confirme a nova senha<input type="password" value={cf} onChange={(e) => setCf(e.target.value)} minLength={10} autoComplete="new-password" required aria-invalid={cf !== '' && cf !== nw} /></label>
          {msg && <div className={`alert ${msg.t}`} role="status">{msg.s}</div>}
          <div><button className="primary">Alterar senha</button></div>
        </form>
      </div>
    </>
  )
}
