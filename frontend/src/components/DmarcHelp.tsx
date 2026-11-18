const opts: { id: string; nome: string; txt: string }[] = [
  { id: 'none', nome: 'Nenhuma (none)', txt: 'Só monitora. Mensagens que falham em SPF e DKIM continuam sendo entregues normalmente e você passa a receber relatórios. É a opção certa para começar.' },
  { id: 'quarantine', nome: 'Quarentena (quarantine)', txt: 'Pede ao destinatário que trate as mensagens que falham como suspeitas, normalmente enviando-as para a pasta de spam.' },
  { id: 'reject', nome: 'Rejeitar (reject)', txt: 'Pede ao destinatário que recuse as mensagens que falham. É a opção mais rígida: use só depois de confirmar que SPF e DKIM estão corretos.' },
]

export default function DmarcHelp({ value }: { value: string }) {
  return (
    <div className="glass card stack optdoc" style={{ background: 'transparent', gap: 8 }}>
      <b>O que cada política DMARC faz</b>
      {opts.map((o) => (
        <div key={o.id} className={`opt-row ${value === o.id ? 'on' : ''}`}>
          <b>{o.nome}</b>
          <span>{o.txt}</span>
        </div>
      ))}
      <span className="help">O e-mail de relatórios (rua) recebe resumos diários de quem enviou em nome do seu domínio e do que passou ou falhou.</span>
    </div>
  )
}
