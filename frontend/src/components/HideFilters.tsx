import { useBranding } from '../branding'

const has = (v: string, k: string) => v.split(',').includes(k)
const toggle = (v: string, k: string, on: boolean) => {
  const s = new Set(v.split(',').filter(Boolean))
  if (on) s.add(k)
  else s.delete(k)
  return ['unknown', 'rspamd'].filter((x) => s.has(x)).join(',')
}

export const hideParam = (v: string) => v || 'none'

export function useHideDefault() {
  return useBranding().branding?.hide_default ?? ''
}

export default function HideFilters({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { branding } = useBranding()
  return (
    <div className="row" style={{ gap: '8px 20px' }}>
      <label className="switch" title="Esconde mensagens que falharam só porque o destinatário não existe (ex.: 550 5.1.1 user unknown)">
        <input type="checkbox" checked={has(value, 'unknown')} onChange={(e) => onChange(toggle(value, 'unknown', e.target.checked))} />
        <span className="track" aria-hidden="true"><span className="thumb" /></span>
        Ocultar falhas por caixa inexistente
      </label>
      {(branding?.has_mailcow || has(value, 'rspamd')) && (
        <label className="switch" title="Esconde mensagens barradas como spam pelo Rspamd do Mailcow">
          <input type="checkbox" checked={has(value, 'rspamd')} onChange={(e) => onChange(toggle(value, 'rspamd', e.target.checked))} />
          <span className="track" aria-hidden="true"><span className="thumb" /></span>
          Ocultar rejeições do Rspamd
        </label>
      )}
    </div>
  )
}
