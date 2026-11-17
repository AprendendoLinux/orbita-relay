import { useEffect, useState } from 'react'

type Mode = 'dark' | 'light'
const KEY = 'orbita-mode'

export function applyMode(m: Mode) {
  document.documentElement.dataset.theme = m
  document.querySelector('meta[name=color-scheme]')?.setAttribute('content', m)
}

export function initMode(): Mode {
  let m: Mode = 'dark'
  try {
    const s = localStorage.getItem(KEY)
    if (s === 'light' || s === 'dark') m = s
  } catch { /* sem armazenamento */ }
  applyMode(m)
  return m
}

export default function ThemeToggle({ className = '' }: { className?: string }) {
  const [mode, setMode] = useState<Mode>(() => (document.documentElement.dataset.theme === 'light' ? 'light' : 'dark'))
  useEffect(() => applyMode(mode), [mode])
  const toggle = () => {
    const next: Mode = mode === 'dark' ? 'light' : 'dark'
    try { localStorage.setItem(KEY, next) } catch { /* ignora */ }
    setMode(next)
  }
  return (
    <button className={`sm ${className}`} onClick={toggle} aria-pressed={mode === 'light'} title="Alternar modo claro/escuro">
      {mode === 'dark' ? '☀ Modo claro' : '☾ Modo escuro'}
    </button>
  )
}
