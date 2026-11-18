import { useState, type KeyboardEvent } from 'react'

const v4 = /^(\d{1,3}\.){3}\d{1,3}(\/\d{1,2})?$/
const v6 = /^[0-9A-Fa-f:.]+(\/\d{1,3})?$/

export function validEntry(s: string, cidr: boolean): boolean {
  const [ip, mask] = s.split('/')
  if (!cidr && mask !== undefined) return false
  if (v4.test(s)) {
    return ip.split('.').every((n) => Number(n) <= 255) && (mask === undefined || Number(mask) <= 32)
  }
  return s.includes(':') && v6.test(s) && (mask === undefined || Number(mask) <= 128) && ip.split(':').length <= 8
}

export default function ListEditor({ label, help, example, value, onChange, cidr, placeholder }: {
  label: string
  help?: string
  example?: string
  value: string
  onChange: (v: string) => void
  cidr: boolean
  placeholder: string
}) {
  const items = value.split('\n').map((l) => l.trim()).filter(Boolean)
  const [txt, setTxt] = useState('')
  const [err, setErr] = useState('')

  const add = () => {
    const parts = txt.split(/[\s,;]+/).filter(Boolean)
    if (!parts.length) return
    const bad = parts.filter((p) => !validEntry(p, cidr))
    if (bad.length) {
      setErr(`Valor inválido: ${bad.join(', ')}${cidr ? '' : ' (informe apenas IPs, sem máscara)'}`)
      return
    }
    const set = new Set(items)
    parts.forEach((p) => set.add(p))
    onChange([...set].join('\n'))
    setTxt('')
    setErr('')
  }
  const remove = (x: string) => onChange(items.filter((i) => i !== x).join('\n'))
  const key = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      add()
    }
  }

  return (
    <div className="f" style={{ gridColumn: '1 / -1' }}>
      <span>{label} <span className="muted small">({items.length})</span></span>
      <div className="listed">
        {items.length === 0 && <span className="muted small" style={{ padding: '6px 4px' }}>Nenhum item. Adicione abaixo.</span>}
        {items.map((i) => (
          <div className="item" key={i}>
            <span className="mono">{i}</span>
            <button type="button" className="sm danger" onClick={() => remove(i)} aria-label={`Remover ${i}`} title="Remover">✕ Remover</button>
          </div>
        ))}
      </div>
      <div className="row" style={{ marginTop: 8 }}>
        <input style={{ flex: '1 1 260px' }} value={txt} onChange={(e) => { setTxt(e.target.value); setErr('') }} onKeyDown={key} placeholder={placeholder} aria-label={`Adicionar em ${label}`} autoComplete="off" />
        <button type="button" className="sm" onClick={add} disabled={!txt.trim()}>＋ Adicionar</button>
      </div>
      {err && <span className="help" style={{ color: 'var(--danger)' }}>{err}</span>}
      {help && <span className="help">{help}</span>}
      {example && <span className="help">Ex.: <span className="mono">{example}</span></span>}
    </div>
  )
}
