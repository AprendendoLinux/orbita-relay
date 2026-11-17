import Tip from './Tip'

export default function HostSendTip() {
  const h = window.location.hostname
  const local = ['localhost', '127.0.0.1', '::1', '[::1]'].includes(h)
  return (
    <Tip title="Enviando a partir do próprio servidor">
      Na máquina que roda o Docker, configure o servidor SMTP com o <b>endereço IP da máquina</b>
      {!local && <>{' '}(por exemplo <span className="mono">{h}</span>)</>}
      , nunca com <span className="mono">localhost</span> ou <span className="mono">127.0.0.1</span>.
      {' '}O Docker mascara essa origem e o relay recusa a conexão com <span className="mono">550 Connection through Docker gateway refused</span>.
      {' '}Exemplo no mutt: <span className="mono">set smtp_url="smtp://IP_DA_MAQUINA:25"</span>. O IP precisa estar em <b>Redes autorizadas</b>.
    </Tip>
  )
}
