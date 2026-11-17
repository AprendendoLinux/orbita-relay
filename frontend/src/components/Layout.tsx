import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { api, roleLabel, type User } from '../api'
import { useBranding } from '../branding'
import Logo from './Logo'
import ThemeToggle from './ThemeToggle'

export function Footer() {
  const { branding } = useBranding()
  return (
    <footer className="footer">
      <b>{branding?.app_name ?? 'OrbitaRelay'}</b> · Relay SMTP e relatórios
      <span className="ver">v{__APP_VERSION__}</span>
    </footer>
  )
}

export function Brand({ size = 40 }: { size?: number }) {
  const { branding } = useBranding()
  return (
    <div className="brand">
      <Logo size={size} />
      <span>{branding?.app_name ?? 'OrbitaRelay'}</span>
    </div>
  )
}

export default function Layout({ user }: { user: User }) {
  const nav = useNavigate()
  const admin = user.role === 'admin'
  const logout = async () => {
    await api('/api/auth/logout', { method: 'POST' }).catch(() => {})
    nav(0)
  }
  return (
    <>
      <div className="shell">
        <aside className="side glass">
          <Brand />
          <nav className="nav" aria-label="Principal">
            <NavLink to="/" end>◈ Painel</NavLink>
            <NavLink to="/mensagens">✉ Mensagens</NavLink>
            {admin && (
              <>
                <div className="sep">Administração</div>
                <NavLink to="/dominios">⌖ Domínios e DKIM</NavLink>
                <NavLink to="/admin/configuracoes">⚙ Configurações</NavLink>
                <NavLink to="/admin/usuarios">☺ Usuários</NavLink>
                <NavLink to="/admin/auditoria">☰ Auditoria</NavLink>
              </>
            )}
            <div className="sep">Conta</div>
            <NavLink to="/conta">⚿ Minha conta</NavLink>
          </nav>
          <div className="grow" />
          <div className="small muted" style={{ padding: '0 10px' }}>
            {user.username}
            <br />
            {roleLabel[user.role]}
          </div>
          <ThemeToggle />
          <button className="sm" onClick={logout}>Sair</button>
        </aside>
        <div className="main">
          <main className="content">
            <Outlet />
          </main>
          <Footer />
        </div>
      </div>
    </>
  )
}
