import { useCallback, useEffect, useRef, useState } from 'react'

const read = (k: string, d: string) => {
  try { return localStorage.getItem(k) ?? d } catch { return d }
}
const write = (k: string, v: string) => {
  try { localStorage.setItem(k, v) } catch { /* ignorado */ }
}

export function useRefresh(load: () => Promise<unknown>) {
  const [auto, setAutoState] = useState(() => read('orbita.auto', '1') === '1')
  const [sec, setSecState] = useState(() => Number(read('orbita.auto.sec', '30')) || 30)
  const [busy, setBusy] = useState(false)
  const [updated, setUpdated] = useState<Date | null>(null)
  const loadRef = useRef(load)
  loadRef.current = load

  const run = useCallback(async () => {
    setBusy(true)
    try {
      await loadRef.current()
      setUpdated(new Date())
    } catch { /* o erro é exibido pela própria tela */ } finally {
      setBusy(false)
    }
  }, [])

  useEffect(() => {
    if (!auto) return
    const tick = () => { if (!document.hidden) run() }
    const t = setInterval(tick, sec * 1000)
    document.addEventListener('visibilitychange', tick)
    return () => { clearInterval(t); document.removeEventListener('visibilitychange', tick) }
  }, [auto, sec, run])

  const setAuto = (v: boolean) => { setAutoState(v); write('orbita.auto', v ? '1' : '0') }
  const setSec = (v: number) => { setSecState(v); write('orbita.auto.sec', String(v)) }
  return { auto, setAuto, sec, setSec, busy, updated, run }
}

export default function RefreshBar({ r, tz }: { r: ReturnType<typeof useRefresh>; tz?: string }) {
  return (
    <div className="refresh noprint">
      <button className="sm" onClick={r.run} disabled={r.busy} title="Buscar os dados agora">
        <span className={r.busy ? 'spin' : ''} aria-hidden="true">↻</span> {r.busy ? 'Atualizando…' : 'Atualizar'}
      </button>
      <label className="switch" title="Atualiza a lista sozinha enquanto esta aba estiver visível">
        <input type="checkbox" checked={r.auto} onChange={(e) => r.setAuto(e.target.checked)} />
        <span className="track" aria-hidden="true"><span className="thumb" /></span>
        Atualização automática
      </label>
      {r.auto && (
        <select value={r.sec} onChange={(e) => r.setSec(Number(e.target.value))} aria-label="Intervalo de atualização" style={{ width: 'auto' }}>
          {[15, 30, 60, 120].map((s) => <option key={s} value={s}>a cada {s} s</option>)}
        </select>
      )}
      {r.updated && <span className="muted small">Atualizado às {r.updated.toLocaleTimeString('pt-BR', { timeZone: tz || undefined })}</span>}
    </div>
  )
}
