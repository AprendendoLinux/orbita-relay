import { useEffect, useState } from 'react'
import { api } from '../api'
import { useBranding } from '../branding'

export function useServerMode() {
  const b = useBranding().branding
  return b?.deploy_mode === 'server' || !!b?.has_mailcow
}

export default function NodeSelect({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const [nodes, setNodes] = useState<{ id: number; name: string }[]>([])
  useEffect(() => { api<{ id: number; name: string }[]>('/api/nodes').then(setNodes).catch(() => {}) }, [])
  return (
    <label className="f">Servidor
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">Todos</option>
        <option value="local">Local</option>
        {nodes.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
      </select>
    </label>
  )
}
