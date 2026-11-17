import { useEffect, useState } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { api, ApiError, type User } from './api'
import { useBranding } from './branding'
import Layout from './components/Layout'
import Setup from './pages/Setup'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Messages from './pages/Messages'
import Domains from './pages/Domains'
import Settings from './pages/Settings'
import Themes from './pages/Themes'
import Users from './pages/Users'
import Audit from './pages/Audit'
import Account from './pages/Account'

export default function App() {
  const { branding } = useBranding()
  const [user, setUser] = useState<User | null | undefined>(undefined)

  useEffect(() => {
    if (!branding || branding.setup_required) return
    api<User>('/api/me')
      .then(setUser)
      .catch((e) => {
        if (e instanceof ApiError && e.status === 401) setUser(null)
        else setUser(null)
      })
  }, [branding])

  if (!branding) return null
  if (branding.setup_required) return <Setup />
  if (user === undefined) return null
  if (user === null) return <Login />

  const admin = user.role === 'admin'
  return (
    <Routes>
      <Route element={<Layout user={user} />}>
        <Route index element={<Dashboard />} />
        <Route path="mensagens" element={<Messages />} />
        <Route path="conta" element={<Account />} />
        {admin && (
          <>
            <Route path="dominios" element={<Domains />} />
            <Route path="admin/configuracoes" element={<Settings />} />
            <Route path="admin/temas" element={<Themes />} />
            <Route path="admin/usuarios" element={<Users me={user} />} />
            <Route path="admin/auditoria" element={<Audit />} />
          </>
        )}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
