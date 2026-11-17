export type Field = {
  key: string; group: string; label: string; help?: string; type: string
  options?: string[]; value: unknown; secret?: boolean; has_value?: boolean; depends?: string
}
export type Vals = Record<string, unknown>

const optionLabels: Record<string, string> = {
  relay: 'Relay SMTP (entrega direta)', smarthost: 'Smarthost (encaminhar)',
  local: 'Local', ldap: 'LDAP / Active Directory',
  none: 'Nenhuma', starttls: 'STARTTLS', tls: 'TLS implícito (SMTPS)', ldaps: 'LDAPS',
  dual: 'IPv4 + IPv6', ipv4: 'Somente IPv4', quarantine: 'quarantine', reject: 'reject',
}

export const visible = (f: Field, vals: Vals) => {
  if (!f.depends) return true
  const [k, v] = f.depends.split('=')
  return String(vals[k]) === v
}

export default function FieldInput({ f, v, set }: { f: Field; v: unknown; set: (x: unknown) => void }) {
  if (f.type === 'bool')
    return (
      <label className="check" style={{ alignSelf: 'end' }}>
        <input type="checkbox" checked={v === true} onChange={(e) => set(e.target.checked)} /> {f.label}
      </label>
    )
  return (
    <label className="f" style={f.type === 'textarea' ? { gridColumn: '1 / -1' } : undefined}>
      {f.label}
      {f.type === 'select' ? (
        <select value={String(v)} onChange={(e) => set(e.target.value)}>
          {f.options?.map((o) => <option key={o} value={o}>{optionLabels[o] ?? o}</option>)}
        </select>
      ) : f.type === 'textarea' ? (
        <textarea value={String(v ?? '')} onChange={(e) => set(e.target.value)} />
      ) : (
        <input
          type={f.type === 'password' ? 'password' : f.type === 'number' ? 'number' : 'text'}
          value={String(v ?? '')}
          placeholder={f.secret && f.has_value ? '•••••••• (mantida)' : ''}
          autoComplete="off"
          onChange={(e) => set(e.target.value)}
        />
      )}
      {f.help && <span className="help">{f.help}</span>}
    </label>
  )
}
