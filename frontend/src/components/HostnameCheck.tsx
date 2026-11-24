import { useState } from 'react'
import { api } from '../api'

type Check = { hostname: string; ok: boolean; ips: string[]; ptr: Record<string, string>; tracked: string[]; cloudflare: string[]; cf_source: string; cf_stale: boolean; cf_updated?: string }
type Authorized = { relay_nets: string; tracked: string[] }

// Verifica o hostname do relay (A/AAAA e PTR) e autoriza a enviar só os IPs escolhidos.
// Não bloqueia o avanço: sem DNS pronto, o administrador pode seguir e verificar depois.
export default function HostnameCheck({ hostname, mode, onNets, onVerified }: { hostname: string; mode: string; onNets: (relayNets: string) => void; onVerified?: (host: string) => void }) {
  const [res, setRes] = useState<Check | null>(null)
  const [sel, setSel] = useState<Set<string>>(new Set())
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState<{ t: 'ok' | 'err'; s: string } | null>(null)

  if (mode !== 'relay') return null

  const check = async () => {
    setBusy(true)
    setMsg(null)
    try {
      const r = await api<Check>('/api/admin/hostname/check', { method: 'POST', body: { hostname } })
      setRes(r)
      onVerified?.(r.hostname)
      setSel(new Set((r.tracked ?? []).map((t) => t.replace(/\/(32|128)$/, ''))))
    } catch (e) {
      setMsg({ t: 'err', s: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }

  const authorize = async () => {
    if (!res) return
    setBusy(true)
    setMsg(null)
    try {
      const r = await api<Authorized>('/api/admin/hostname/authorize', { method: 'POST', body: { hostname: res.hostname, ips: [...sel] } })
      onNets(r.relay_nets)
      setRes({ ...res, tracked: r.tracked })
      setMsg({ t: 'ok', s: `${r.tracked.length} IP(s) autorizado(s) a enviar a partir de ${res.hostname}.` })
    } catch (e) {
      setMsg({ t: 'err', s: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }

  const toggle = (ip: string) => {
    const next = new Set(sel)
    if (next.has(ip)) next.delete(ip)
    else next.add(ip)
    setSel(next)
  }

  return (
    <div className="glass card stack" style={{ background: 'transparent' }}>
      <div className="row spread">
        <b>Hostname do relay no DNS</b>
        <button type="button" className="sm" onClick={check} disabled={busy || !hostname.trim()}>{busy ? 'Verificando…' : 'Verificar hostname'}</button>
      </div>
      <span className="help">O hostname precisa apontar (A/AAAA) para os IPs desta máquina. Só entram em <b>Redes autorizadas a enviar</b> os IPs que você marcar.</span>

      {res && res.cf_stale && (
        <div className="alert warn">
          A lista de faixas da Cloudflare pode estar desatualizada ({res.cf_source}{res.cf_updated ? `, última atualização ${new Date(res.cf_updated).toLocaleString('pt-BR')}` : ''}). A detecção de proxy pode não reconhecer IPs novos.
        </div>
      )}
      {res && res.cloudflare.length > 0 && (
        <div className="alert warn">
          <b>{res.hostname}</b> passa pelo proxy da Cloudflare ({res.cloudflare.join(', ')}). Esses IPs não são desta máquina e não podem ser autorizados. Crie um registro <b>DNS only</b> (nuvem cinza) apontando para o IP do servidor e verifique de novo.
        </div>
      )}

      {res && !res.ok && (
        <div className="alert warn">
          Não encontrei registro A/AAAA para <b>{res.hostname}</b>. Crie o apontamento no DNS e verifique de novo, ou continue mesmo assim.
        </div>
      )}

      {res && res.ok && (
        <>
          <div className="tablewrap">
            <table>
              <tbody>
                {res.ips.map((ip) => (
                  <tr key={ip}>
                    <td style={{ width: 32 }}><input type="checkbox" checked={sel.has(ip)} disabled={res.cloudflare.includes(ip)} onChange={() => toggle(ip)} aria-label={`Autorizar ${ip}`} /></td>
                    <td className="mono">{ip}{res.cloudflare.includes(ip) ? <span className="muted small"> (Cloudflare)</span> : null}</td>
                    <td className="wrap small">
                      {res.ptr[ip] ? <>PTR: <span className="mono">{res.ptr[ip]}</span>{res.ptr[ip].toLowerCase() !== res.hostname ? <span className="muted"> (diferente do hostname)</span> : null}</> : <span className="muted">sem PTR (DNS reverso)</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="row">
            <button type="button" className="sm" onClick={authorize} disabled={busy || sel.size === 0}>Autorizar os IPs marcados</button>
            <span className="help">Marque só os IPs desta máquina.</span>
          </div>
        </>
      )}

      {msg && <div className={`alert ${msg.t === 'ok' ? 'ok' : 'err'}`}>{msg.s}</div>}
    </div>
  )
}
