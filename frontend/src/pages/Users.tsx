import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { api, fmtDate, roleLabel, type User } from '../api'

export default function Users({ me }: { me: User }) {
  const [list, setList] = useState<User[]>([])
  const [form, setForm] = useState({ username: '', display_name: '', email: '', role: 'viewer', password: '' })
  const [msg, setMsg] = useState<{ t: 'ok' | 'err'; s: string } | null>(null)

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
    act(async () => {
      await api('/api/admin/users', { method: 'POST', body: form })
      setForm({ username: '', display_name: '', email: '', role: 'viewer', password: '' })
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
          <label className="f">E-mail<input type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} /></label>
          <label className="f">Perfil
            <select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}>
              {Object.entries(roleLabel).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
            </select>
          </label>
          <label className="f">Senha (mín. 10)<input type="password" minLength={10} value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required autoComplete="new-password" /></label>
        </div>
        <div><button className="primary">Criar usuário</button></div>
      </form>
      <div className="glass card tablewrap">
        <table>
          <thead><tr><th>Usuário</th><th>Nome</th><th>Origem</th><th>Perfil</th><th>Último acesso</th><th>Ações</th></tr></thead>
          <tbody>
            {list.map((u) => (
              <tr key={u.id}>
                <td>{u.username} {u.disabled && <span className="badge b-bad">desativado</span>}</td>
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
                      <button className="sm" onClick={() => { const p = prompt('Nova senha (mín. 10 caracteres)'); if (p) patch(u, { password: p }, 'Senha redefinida') }}>Redefinir senha</button>
                    )}
                    {u.id !== me.id && (
                      <>
                        <button className="sm" onClick={() => patch(u, { disabled: !u.disabled }, u.disabled ? 'Usuário ativado' : 'Usuário desativado')}>{u.disabled ? 'Ativar' : 'Desativar'}</button>
                        <button className="sm danger" onClick={() => confirm(`Excluir ${u.username}?`) && act(() => api(`/api/admin/users/${u.id}`, { method: 'DELETE' }), 'Usuário excluído')}>Excluir</button>
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
