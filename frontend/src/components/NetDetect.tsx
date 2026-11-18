import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'

type Item = { cidr: string; label: string; source: string }
type Resp = { auto: boolean; items: Item[]; effective: string[] }

export default function NetDetect({ auto, version }: { auto: boolean; version: number }) {
  const [d, setD] = useState<Resp | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const run = useCallback(async () => {
    setBusy(true)
    setErr('')
    try {
      setD(await api<Resp>('/api/admin/networks/detect', { method: 'POST' }))
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }, [])
  useEffect(() => { run() }, [run, version])

  return (
    <div className="glass card stack" style={{ background: 'transparent' }}>
      <div className="row spread">
        <b>Redes detectadas automaticamente</b>
        <button type="button" className="sm" onClick={run} disabled={busy}>{busy ? 'Detectando…' : '↻ Detectar novamente'}</button>
      </div>
      {err && <div className="alert err">{err}</div>}
      {d && (
        <>
          <span className="help">
            {auto && d.auto
              ? 'Estas redes são somadas à lista acima e liberadas para enviar e-mails automaticamente. Elas são recalculadas sempre que o sistema inicia ou as configurações são salvas.'
              : 'A inclusão automática está desligada: as redes abaixo são apenas informativas e não estão liberadas.'}
          </span>
          <div className="tablewrap">
            <table>
              <thead><tr><th>Rede / IP</th><th>Origem</th></tr></thead>
              <tbody>
                {d.items.map((i) => (
                  <tr key={i.cidr}><td className="mono">{i.cidr}</td><td className="wrap">{i.label}</td></tr>
                ))}
              </tbody>
            </table>
          </div>
          <span className="help">
            Dentro do contêiner só é possível enxergar as interfaces dele (a rede Docker). O IP da máquina é aprendido quando você acessa o painel por ele (por exemplo, <span className="mono">http://192.168.4.129:8080</span>) ou informado na variável <span className="mono">ORBITA_HOST_IPS</span>. Conexões vindas pelo gateway do Docker continuam bloqueadas por segurança.
          </span>
        </>
      )}
    </div>
  )
}
