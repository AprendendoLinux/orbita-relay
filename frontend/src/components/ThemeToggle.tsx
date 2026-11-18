import { useEffect, useState } from 'react'

type Mode = 'dark' | 'light'
const KEY = 'orbita-mode'

const DEF_KEY = 'orbita-default-mode'

export function applyMode(m: Mode) {
  document.documentElement.dataset.theme = m
  document.querySelector('meta[name=color-scheme]')?.setAttribute('content', m)
  window.dispatchEvent(new CustomEvent('orbita-mode', { detail: m }))
}

const read = (k: string) => {
  try { const v = localStorage.getItem(k); return v === 'light' || v === 'dark' ? (v as Mode) : null } catch { return null }
}

export function initMode(): Mode {
  const m = read(KEY) ?? read(DEF_KEY) ?? 'dark'
  applyMode(m)
  return m
}

export function setSystemDefault(d: string | undefined) {
  if (d !== 'light' && d !== 'dark') return
  try { localStorage.setItem(DEF_KEY, d) } catch { /* ignora */ }
  if (!read(KEY)) applyMode(d)
}

export function currentMode(): Mode {
  return document.documentElement.dataset.theme === 'light' ? 'light' : 'dark'
}

export function toggleMode(): Mode {
  const next: Mode = currentMode() === 'dark' ? 'light' : 'dark'
  try { localStorage.setItem(KEY, next) } catch { /* ignora */ }
  applyMode(next)
  return next
}

export function clearPersonalMode() {
  try { localStorage.removeItem(KEY) } catch { /* ignora */ }
}

export default function ThemeToggle({ className = '' }: { className?: string }) {
  const [mode, setMode] = useState<Mode>(() => (document.documentElement.dataset.theme === 'light' ? 'light' : 'dark'))
  useEffect(() => {
    const on = (e: Event) => setMode((e as CustomEvent<Mode>).detail)
    window.addEventListener('orbita-mode', on)
    return () => window.removeEventListener('orbita-mode', on)
  }, [])
  const toggle = () => {
    const next: Mode = mode === 'dark' ? 'light' : 'dark'
    try { localStorage.setItem(KEY, next) } catch { /* ignora */ }
    applyMode(next)
  }
  return (
    <button className={`sm ${className}`} onClick={toggle} aria-pressed={mode === 'light'} title="Alternar modo claro/escuro">
      {mode === 'dark' ? '☀ Modo claro' : '☾ Modo escuro'}
    </button>
  )
}
