import { createContext, useCallback, useContext, useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'

export type DialogField = { name: string; label: string; type?: 'text' | 'password' | 'email'; required?: boolean; minLength?: number; placeholder?: string; autoComplete?: string; value?: string }
export type DialogOpts = {
  title: string
  message?: ReactNode
  danger?: boolean
  confirmLabel?: string
  cancelLabel?: string
  fields?: DialogField[]
  validate?: (v: Record<string, string>) => string | null
}
type Ctx = { ask: (o: DialogOpts) => Promise<Record<string, string> | null> }

const DialogCtx = createContext<Ctx>({ ask: async () => null })

export function useDialog() {
  const { ask } = useContext(DialogCtx)
  const confirm = useCallback((o: Omit<DialogOpts, 'fields' | 'validate'>) => ask(o).then((r) => r !== null), [ask])
  return { confirm, form: ask }
}

export function DialogProvider({ children }: { children: ReactNode }) {
  const [cur, setCur] = useState<{ o: DialogOpts; resolve: (v: Record<string, string> | null) => void } | null>(null)
  const [vals, setVals] = useState<Record<string, string>>({})
  const [err, setErr] = useState('')
  const first = useRef<HTMLElement | null>(null)

  const ask = useCallback((o: DialogOpts) => new Promise<Record<string, string> | null>((resolve) => {
    setVals(Object.fromEntries((o.fields ?? []).map((f) => [f.name, f.value ?? ''])))
    setErr('')
    setCur({ o, resolve })
  }), [])

  const close = useCallback((v: Record<string, string> | null) => {
    setCur((c) => { c?.resolve(v); return null })
  }, [])

  useEffect(() => {
    if (!cur) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') close(null) }
    document.addEventListener('keydown', onKey)
    const t = setTimeout(() => first.current?.focus(), 30)
    return () => { document.removeEventListener('keydown', onKey); clearTimeout(t) }
  }, [cur, close])

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!cur) return
    const msg = cur.o.validate?.(vals) ?? null
    if (msg) {
      setErr(msg)
      return
    }
    close(vals)
  }

  const o = cur?.o
  return (
    <DialogCtx.Provider value={{ ask }}>
      {children}
      {o && (
        <div className="modal-bg" style={{ zIndex: 80 }} onClick={() => close(null)}>
          <form className="modal glass stack dialog" role="alertdialog" aria-modal="true" aria-label={o.title} onClick={(e) => e.stopPropagation()} onSubmit={submit}>
            <div className="row" style={{ gap: 12, alignItems: 'center' }}>
              <span className={`dlg-icon ${o.danger ? 'danger' : ''}`} aria-hidden="true">{o.danger ? '⚠' : '?'}</span>
              <h2 style={{ margin: 0 }}>{o.title}</h2>
            </div>
            {o.message && <div className="small" style={{ lineHeight: 1.6 }}>{o.message}</div>}
            {(o.fields ?? []).map((f, i) => (
              <label className="f" key={f.name}>{f.label}
                <input
                  ref={i === 0 ? (el) => { first.current = el } : undefined}
                  type={f.type ?? 'text'} value={vals[f.name] ?? ''} placeholder={f.placeholder}
                  required={f.required ?? true} minLength={f.minLength} autoComplete={f.autoComplete}
                  onChange={(e) => setVals({ ...vals, [f.name]: e.target.value })}
                />
              </label>
            ))}
            {err && <div className="alert err" role="alert">{err}</div>}
            <div className="row" style={{ justifyContent: 'flex-end' }}>
              <button type="button" onClick={() => close(null)}>{o.cancelLabel ?? 'Cancelar'}</button>
              <button
                className={o.danger ? 'danger' : 'primary'}
                ref={(o.fields ?? []).length === 0 ? (el) => { first.current = el } : undefined}
              >{o.confirmLabel ?? 'Confirmar'}</button>
            </div>
          </form>
        </div>
      )}
    </DialogCtx.Provider>
  )
}
