import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { api, type Branding } from './api'
import { setSystemDefault } from './components/ThemeToggle'

type Ctx = { branding: Branding | null; refresh: () => Promise<void> }
const BrandingCtx = createContext<Ctx>({ branding: null, refresh: async () => {} })
export const useBranding = () => useContext(BrandingCtx)

export function BrandingProvider({ children }: { children: ReactNode }) {
  const [branding, setBranding] = useState<Branding | null>(null)

  const refresh = useCallback(async () => {
    const b = await api<Branding>('/api/public/branding')
    document.title = b.app_name
    setSystemDefault(b.default_theme)
    setBranding(b)
  }, [])

  useEffect(() => {
    refresh().catch(() => {})
  }, [refresh])

  return <BrandingCtx.Provider value={{ branding, refresh }}>{children}</BrandingCtx.Provider>
}
