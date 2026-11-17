import { useEffect, useState } from 'react'
import { api, fmtDate } from '../api'
import { useBranding } from '../branding'

type Row = { at: string; username: string; action: string; detail: string }

export default function Audit() {
  const { branding } = useBranding()
  const [rows, setRows] = useState<Row[]>([])
  const [err, setErr] = useState('')
  useEffect(() => { api<Row[]>('/api/admin/audit').then(setRows).catch((e) => setErr(e.message)) }, [])
  return (
    <>
      <div className="topbar"><h1>Auditoria</h1></div>
      {err && <div className="alert err">{err}</div>}
      <div className="glass card tablewrap">
        <table>
          <thead><tr><th>Data</th><th>Usuário</th><th>Ação</th><th>Detalhe</th></tr></thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={i}>
                <td className="mono">{fmtDate(r.at, branding?.timezone)}</td>
                <td>{r.username}</td>
                <td className="mono">{r.action}</td>
                <td className="wrap">{r.detail}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  )
}
