import { useState, type FormEvent } from 'react'
import { statusLabel } from '../api'
import NodeSelect, { useServerMode } from './NodeSelect'
import HideFilters, { hideParam } from './HideFilters'

export type Filters = { q: string; from: string; to: string; status: string; sender: string; recipient: string; host: string; node: string; direction: string; hide: string }

export default function ExportDialog({ initial, kind, onClose }: { initial: Filters; kind: 'pdf' | 'csv'; onClose: () => void }) {
  const pdf = kind === 'pdf'
  const server = useServerMode()
  const [f, setF] = useState<Filters>(initial)
  const [title, setTitle] = useState('Relatório de mensagens')
  const [cols, setCols] = useState({ node: server, id: false, origin: true, size: false })
  const [summary, setSummary] = useState(true)
  const [order, setOrder] = useState('desc')
  const [max, setMax] = useState('2000')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const set = (k: keyof Filters) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value })

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setErr('')
    try {
      if (!pdf) {
        const q = new URLSearchParams()
        Object.entries(f).forEach(([k, v]) => k !== 'hide' && v && q.set(k, v))
        q.set('hide', hideParam(f.hide))
        q.set('order', order)
        const a = document.createElement('a')
        a.href = `/api/messages/export?${q}`
        document.body.appendChild(a)
        a.click()
        a.remove()
        onClose()
        return
      }
      const p = new URLSearchParams()
      Object.entries(f).forEach(([k, v]) => k !== 'hide' && v && p.set(k, v))
      p.set('hide', hideParam(f.hide))
      p.set('title', title)
      p.set('cols', Object.entries(cols).filter(([, v]) => v).map(([k]) => k).join(','))
      p.set('summary', summary ? '1' : '0')
      p.set('order', order)
      p.set('max', max)
      const res = await fetch(`/api/messages/report.pdf?${p}`, { credentials: 'same-origin' })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error((d as { error?: string }).error ?? `Erro ${res.status}`)
      }
      const blob = await res.blob()
      const cd = res.headers.get('Content-Disposition') ?? ''
      const name = /filename="([^"]+)"/.exec(cd)?.[1] ?? 'relatorio.pdf'
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = name
      document.body.appendChild(a)
      a.click()
      a.remove()
      setTimeout(() => URL.revokeObjectURL(url), 10000)
      onClose()
    } catch (ex) {
      setErr((ex as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-bg" onClick={onClose}>
      <form className="modal glass stack" role="dialog" aria-modal="true" aria-label={pdf ? 'Exportar PDF' : 'Exportar CSV'} onClick={(e) => e.stopPropagation()} onSubmit={submit}>
        <div className="row spread">
          <h2>{pdf ? 'Exportar PDF' : 'Exportar CSV'}</h2>
          <button type="button" className="sm" onClick={onClose}>Fechar</button>
        </div>
        <p className="muted small" style={{ marginTop: -6 }}>{pdf ? 'Escolha o que deve aparecer no relatório. O arquivo é gerado em A4 vertical, com tema claro.' : 'Escolha quais mensagens entram no arquivo. Sem filtros, todas as mensagens são exportadas.'}</p>

        {pdf && <label className="f">Título do relatório<input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={80} required /></label>}

        <h3>{pdf ? 'O que incluir' : 'Filtros'}</h3>
        <div className="form-grid">
          <label className="f">De<input type="date" value={f.from} onChange={set('from')} /></label>
          <label className="f">Até<input type="date" value={f.to} onChange={set('to')} /></label>
          <label className="f">Status
            <select value={f.status} onChange={set('status')}>
              <option value="">Todos</option>
              {Object.entries(statusLabel).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
            </select>
          </label>
          <label className="f">Sentido
            <select value={f.direction} onChange={set('direction')}>
              <option value="">Todos</option>
              <option value="in">Recebidas</option>
              <option value="out">Enviadas</option>
            </select>
          </label>
          {server && <NodeSelect value={f.node} onChange={(v) => setF({ ...f, node: v })} />}
          <label className="f">Remetente<input value={f.sender} onChange={set('sender')} /></label>
          <label className="f">Destinatário<input value={f.recipient} onChange={set('recipient')} /></label>
          <label className="f">Origem (host/IP)<input value={f.host} onChange={set('host')} /></label>
          <label className="f">Busca geral<input value={f.q} onChange={set('q')} placeholder="assunto, remetente, ID" /></label>
        </div>

        <HideFilters value={f.hide} onChange={(v) => setF({ ...f, hide: v })} />

        {pdf ? (
          <>
          <h3>Conteúdo do relatório</h3>
          <div className="checks">
            <label className="opt"><input type="checkbox" checked={summary} onChange={(e) => setSummary(e.target.checked)} /><span>Resumo e gráfico<small>Totais por status, sentido e mensagens por dia</small></span></label>
            {server && <label className="opt"><input type="checkbox" checked={cols.node} onChange={(e) => setCols({ ...cols, node: e.target.checked })} /><span>Servidor<small>Nome do servidor de origem</small></span></label>}
            <label className="opt"><input type="checkbox" checked={cols.origin} onChange={(e) => setCols({ ...cols, origin: e.target.checked })} /><span>Origem<small>Host e IP de origem</small></span></label>
            <label className="opt"><input type="checkbox" checked={cols.size} onChange={(e) => setCols({ ...cols, size: e.target.checked })} /><span>Tamanho<small>Tamanho da mensagem</small></span></label>
            <label className="opt"><input type="checkbox" checked={cols.id} onChange={(e) => setCols({ ...cols, id: e.target.checked })} /><span>ID<small>Identificador da fila</small></span></label>
          </div>
          <div className="form-grid">
            <label className="f">Ordem
              <select value={order} onChange={(e) => setOrder(e.target.value)}>
                <option value="desc">Mais recentes primeiro</option>
                <option value="asc">Mais antigas primeiro</option>
              </select>
            </label>
            <label className="f">Limite de mensagens
              <select value={max} onChange={(e) => setMax(e.target.value)}>
                {['200', '500', '1000', '2000', '5000', '10000'].map((n) => <option key={n} value={n}>até {n}</option>)}
              </select>
            </label>
          </div>
          </>
        ) : (
          <div className="form-grid">
            <label className="f">Ordem
              <select value={order} onChange={(e) => setOrder(e.target.value)}>
                <option value="desc">Mais recentes primeiro</option>
                <option value="asc">Mais antigas primeiro</option>
              </select>
            </label>
          </div>
        )}
        {err && <div className="alert err" role="alert">{err}</div>}
        <div className="row">
          <button className="primary" disabled={busy}>{busy ? 'Gerando PDF…' : pdf ? '⇩ Gerar PDF' : '⇩ Exportar CSV'}</button>
          <button type="button" onClick={onClose}>Cancelar</button>
        </div>
      </form>
    </div>
  )
}
