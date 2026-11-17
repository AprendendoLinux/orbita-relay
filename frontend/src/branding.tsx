import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { api, type Branding } from './api'

type Ctx = { branding: Branding | null; refresh: () => Promise<void> }
const BrandingCtx = createContext<Ctx>({ branding: null, refresh: async () => {} })
export const useBranding = () => useContext(BrandingCtx)

const fontFamily: Record<string, string> = {
  'space-grotesk': "'Space Grotesk Variable', system-ui, sans-serif",
  inter: "'Inter Variable', system-ui, sans-serif",
  'jetbrains-mono': "'JetBrains Mono', ui-monospace, monospace",
}

export function applyTokens(tokens: Record<string, string>) {
  const s = document.documentElement.style
  for (const [k, v] of Object.entries(tokens)) {
    if (k === 'font') s.setProperty('--font', fontFamily[v] ?? fontFamily['space-grotesk'])
    else if (k === 'radius' || k === 'blur') s.setProperty(`--${k}`, `${v}px`)
    else s.setProperty(`--${k}`, v)
  }
}

export function BrandingProvider({ children }: { children: ReactNode }) {
  const [branding, setBranding] = useState<Branding | null>(null)

  const refresh = useCallback(async () => {
    const b = await api<Branding>('/api/public/branding')
    applyTokens(b.theme.tokens)
    document.title = b.app_name
    setBranding(b)
  }, [])

  useEffect(() => {
    refresh().catch(() => {})
  }, [refresh])

  return <BrandingCtx.Provider value={{ branding, refresh }}>{children}</BrandingCtx.Provider>
}
