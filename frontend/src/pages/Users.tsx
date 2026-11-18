import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { api, fmtDate, roleLabel, type User } from '../api'
import { useDialog } from '../components/Dialog'
import { copyText, randomPassword } from '../lib'

export default function Users({ me }: { me: User }) {
  const dlg = useDialog()
  const [list, setList] = useState<User[]>([])
  const [form, setForm] = useState({ username: '', display_name: '', email: '', role: 'viewer', password: '', confirm: '' })
  const [msg, setMsg] = useState<{ t: 'ok' | 'err'; s: string } | null>(null)
  const [gen, setGen] = useState('')
  const [copied, setCopied] = useState(false)

  const load = useCallback(() => api<User[]>('/api/admin/users').then(setList), [])
  useEffect(() => { load().catch((e) => setMsg({ t: 'err', s: e.message })) }, [load])

  const act = async (fn: () => Promise<unknown>, ok: string) => {
    try {
      await fn()
      setMsg({ t: 'ok', s: ok })
      await load()
    } catch (e) {
      setMsg({ t: 'err', s: (e as Error).message })
    }
  }

  const create = (e: FormEvent) => {
    e.preventDefault()
    if (form.password !== form.confirm) {
      setMsg({ t: 'err', s: 'A confirmação da senha não confere' })
      return
    }
    act(async () => {
      await api('/api/admin/users', { method: 'POST', body: { ...form, confirm: undefined } })
      setForm({ username: '', display_name: '', email: '', role: 'viewer', password: '', confirm: '' })
      setGen('')
    }, 'Usuário criado')
  }

  const patch = (u: User, body: object, ok: string) => act(() => api(`/api/admin/users/${u.id}`, { method: 'PATCH', body }), ok)

  return (
    <>
      <div className="topbar"><h1>Usuários</h1></div>
      {msg && <div className={`alert ${msg.t}`} role="status" style={{ marginBottom: 14 }}>{msg.s}</div>}
      <form className="glass card stack" onSubmit={create} style={{ marginBottom: 16 }}>
        <h3>Novo usuário local</h3>
        <div className="form-grid">
          <label className="f">Usuário<input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} required /></label>
          <label className="f">Nome<input value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} /></label>
          <label className="f">E-mail<input type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} required autoComplete="off" placeholder="usuario@empresa.com.br" /></label>
          <label className="f">Perfil
            <select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}>
              {Object.entries(roleLabel).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
            </select>
          </label>
          <label className="f">Senha (mín. 10)<input type="password" minLength={10} value={form.password} onChange={(e) => { setGen(''); setForm({ ...form, password: e.target.value }) }} required autoComplete="new-password" /></label>
          <label className="f">Confirme a senha<input type="password" minLength={10} value={form.confirm} onChange={(e) => { setGen(''); setForm({ ...form, confirm: e.target.value }) }} required autoComplete="new-password" aria-invalid={form.confirm !== '' && form.confirm !== form.password} /></label>
        </div>
        {gen && (
          <div className="row" style={{ gap: 10 }}>
            <span className="small muted">Senha gerada:</span>
            <code className="mono" style={{ padding: '6px 10px', letterSpacing: '0.05em', userSelect: 'all' }}>{gen}</code>
            <button type="button" className="sm" onClick={async () => { setCopied(await copyText(gen)); setTimeout(() => setCopied(false), 1800) }}>{copied ? 'Copiado ✓' : '⧉ Copiar senha'}</button>
          </div>
        )}
        <div className="row">
          <button className="primary">Criar usuário</button>
          <button type="button" onClick={() => { const p = randomPassword(16); setGen(p); setForm({ ...form, password: p, confirm: p }) }}>⚄ Gerar senha aleatória</button>
          {!gen && form.password && <button type="button" className="sm" onClick={async () => { setCopied(await copyText(form.password)); setTimeout(() => setCopied(false), 1800) }}>{copied ? 'Copiado ✓' : '⧉ Copiar senha'}</button>}
        </div>
      </form>
      <div className="glass card tablewrap">
        <table>
          <thead><tr><th>Usuário</th><th>Nome</th><th>Origem</th><th>Perfil</th><th>Último acesso</th><th>Ações</th></tr></thead>
          <tbody>
            {list.map((u) => (
              <tr key={u.id}>
                <td>{u.username} {u.disabled && <span className="badge b-bad">desativado</span>} {u.mfa && <span className="badge b-ok" title={u.mfa === 'totp' ? 'Aplicativo autenticador' : 'Código por e-mail'}>2FA</span>}</td>
                <td>{u.display_name}</td>
                <td><span className="badge b-info">{u.source}</span></td>
                <td>
                  <select value={u.role} disabled={u.id === me.id} onChange={(e) => patch(u, { role: e.target.value }, 'Perfil atualizado')} aria-label={`Perfil de ${u.username}`}>
                    {Object.entries(roleLabel).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
                  </select>
                </td>
                <td className="mono">{u.last_login ? fmtDate(u.last_login) : '—'}</td>
                <td>
                  <div className="row">
                    {u.source === 'local' && (
                      <button className="sm" onClick={async () => {
                        const r = await dlg.form({
                          title: `Redefinir a senha de ${u.username}`,
                          message: 'Todas as sessões abertas desse usuário serão encerradas.',
                          confirmLabel: 'Redefinir senha',
                          fields: [
                            { name: 'p', label: 'Nova senha (mín. 10 caracteres)', type: 'password', minLength: 10, autoComplete: 'new-password' },
                            { name: 'c', label: 'Confirme a nova senha', type: 'password', minLength: 10, autoComplete: 'new-password' },
                          ],
                          validate: (v) => (v.p !== v.c ? 'A confirmação da senha não confere' : null),
                        })
                        if (r) patch(u, { password: r.p }, 'Senha redefinida')
                      }}>Redefinir senha</button>
                    )}
                    {u.mfa && (
                      <button className="sm" onClick={async () => (await dlg.confirm({ title: `Redefinir o 2FA de ${u.username}?`, message: 'A verificação em duas etapas será removida e o usuário passará a entrar só com a senha até configurá-la de novo. Faça isso apenas se tiver certeza de que o pedido é do próprio usuário.', danger: true, confirmLabel: 'Redefinir 2FA' })) && act(() => api(`/api/admin/users/${u.id}/mfa/reset`, { method: 'POST' }), '2FA redefinido')}>Redefinir 2FA</button>
                    )}
                    {u.id !== me.id && (
                      <>
                        <button className="sm" onClick={() => patch(u, { disabled: !u.disabled }, u.disabled ? 'Usuário ativado' : 'Usuário desativado')}>{u.disabled ? 'Ativar' : 'Desativar'}</button>
                        <button className="sm danger" onClick={async () => (await dlg.confirm({ title: `Excluir ${u.username}?`, message: 'O usuário será removido e perderá o acesso. Esta ação não pode ser desfeita.', danger: true, confirmLabel: 'Excluir' })) && act(() => api(`/api/admin/users/${u.id}`, { method: 'DELETE' }), 'Usuário excluído')}>Excluir</button>
                      </>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  )
}
