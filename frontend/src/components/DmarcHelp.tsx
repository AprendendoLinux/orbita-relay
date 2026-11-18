const opts: { id: string; nome: string; curto: string; txt: string }[] = [
  { id: 'none', nome: 'Nenhuma (none)', curto: 'só monitora e gera relatórios, sem afetar a entrega (ideal para começar)', txt: 'só monitora; as mensagens que falham continuam sendo entregues.' },
  { id: 'quarantine', nome: 'Quarentena (quarantine)', curto: 'mensagens que falham vão para o spam', txt: 'pede ao destinatário que trate as que falham como suspeitas (spam).' },
  { id: 'reject', nome: 'Rejeitar (reject)', curto: 'mensagens que falham são recusadas (o mais rígido)', txt: 'pede ao destinatário que recuse as que falham; use só com SPF e DKIM confirmados.' },
]

export default function DmarcHelp({ value }: { value: string }) {
  const cur = opts.find((o) => o.id === value) ?? opts[0]
  return (
    <details className="dmarc-help">
      <summary><b>{cur.nome.split(' (')[0]}:</b> {cur.curto}. <span className="more">Ver todas</span></summary>
      <ul>
        {opts.map((o) => <li key={o.id} className={o.id === cur.id ? 'on' : ''}><b>{o.nome}</b>: {o.txt}</li>)}
        <li>O e-mail de relatórios (rua) recebe resumos de quem enviou em nome do seu domínio.</li>
      </ul>
    </details>
  )
}
