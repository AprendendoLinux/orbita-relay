import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'

type Item = { cidr: string; label: string; source: string }
type Resp = { auto: boolean; items: Item[]; suggestions: Item[]; effective: string[] }

export default function NetDetect({ auto, version, current, onAdd }: { auto: boolean; version: number; current: string; onAdd: (list: string[]) => void }) {
  const inField = new Set(current.split('\n').map((l) => l.trim()).filter(Boolean))
  const [d, setD] = useState<Resp | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [manual, setManual] = useState('')
  const [added, setAdded] = useState('')

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

  const pend = (d?.suggestions ?? []).filter((i) => !inField.has(i.cidr))

  const add = (list: string[], msg: string) => {
    onAdd(list)
    setAdded(msg)
    setTimeout(() => setAdded(''), 2500)
  }
  const addManual = () => {
    const list = manual.split(/[\s,;]+/).filter(Boolean)
    if (list.length) {
      add(list, `${list.length} item(ns) adicionado(s) ao campo acima`)
      setManual('')
    }
  }

  return (
    <div className="glass card stack" style={{ background: 'transparent' }}>
      <div className="row spread">
        <b>Redes detectadas (máquina e Docker)</b>
        <div className="row">
          {added && <span className="muted small">{added} ✓</span>}
          <button type="button" className="sm" onClick={run} disabled={busy}>{busy ? 'Detectando…' : '↻ Detectar novamente'}</button>
        </div>
      </div>
      {err && <div className="alert err">{err}</div>}
      {d && (
        <>
          <span className="help">
            {auto && d.auto
              ? 'Estas redes já valem para o envio (inclusão automática ligada). Use o botão para também listá-las no campo “Redes autorizadas a enviar”, acima.'
              : 'A inclusão automática está desligada: estas redes só valem se estiverem no campo “Redes autorizadas a enviar”.'}
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
          <div>
            {d.items.every((i) => inField.has(i.cidr))
              ? <span className="muted small">✓ Todas as redes detectadas já estão no campo acima.</span>
              : <button type="button" className="sm" onClick={() => add(d.items.map((i) => i.cidr), 'Redes detectadas adicionadas ao campo acima')}>＋ Adicionar as redes detectadas ao campo</button>}
          </div>

          {pend.length > 0 && (
            <>
              <div className="row spread" style={{ marginTop: 6 }}>
                <b>Outros IPs e redes encontrados <span className="muted small">(confira antes de adicionar)</span></b>
                <button type="button" className="sm" onClick={() => add(pend.map((i) => i.cidr), `${pend.length} item(ns) adicionado(s) ao campo acima`)}>＋ Adicionar todos ({pend.length})</button>
              </div>
              <div className="tablewrap">
                <table>
                  <tbody>
                    {pend.map((i) => (
                      <tr key={i.cidr}>
                        <td className="mono">{i.cidr}</td>
                        <td className="wrap">{i.label}</td>
                        <td><button type="button" className="sm" onClick={() => add([i.cidr], `${i.cidr} adicionado ao campo acima`)}>Adicionar</button></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}

          <div className="stack" style={{ gap: 6 }}>
            <b>IPs da máquina (host)</b>
            <span className="help">
              De dentro do contêiner não dá para ler as interfaces da máquina. Rode <span className="mono">hostname -I</span> no servidor e cole o resultado aqui (aceita vários IPs IPv4 e IPv6 separados por espaço). Também dá para fixá-los na variável <span className="mono">ORBITA_HOST_IPS</span> do serviço <span className="mono">orbitarelay-app</span>.
            </span>
            <div className="row">
              <input style={{ flex: '1 1 320px' }} value={manual} onChange={(e) => setManual(e.target.value)} placeholder="192.168.0.10 2804:14c:... 172.18.0.1" aria-label="IPs da máquina" />
              <button type="button" className="sm" onClick={addManual} disabled={!manual.trim()}>Adicionar</button>
            </div>
          </div>
          <span className="help">Conexões vindas pelo gateway do Docker continuam bloqueadas por segurança, mesmo dentro de uma rede autorizada.</span>
        </>
      )}
    </div>
  )
}
