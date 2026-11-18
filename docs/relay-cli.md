# Relay CLI (leve)

O **OrbitaRelay CLI** é um relay SMTP (Exim) **sem painel e sem banco de dados**. Toda a configuração fica no `docker-compose.yml`. Ele pode funcionar sozinho ou enviar os eventos a um [servidor central](multi-servidor.md).

Imagem: `ghcr.io/aprendendolinux/orbita-relay-cli` (amd64 e arm64).

## Exemplo mínimo

```yaml
services:
  orbitarelay-cli:
    image: ghcr.io/aprendendolinux/orbita-relay-cli:1.6.0
    container_name: orbitarelay-cli
    restart: unless-stopped
    ports: ["25:25", "587:587"]
    environment:
      ORBITA_HOSTNAME: relay.exemplo.com.br
      ORBITA_DOMAINS: exemplo.com.br
      ORBITA_RELAY_NETS: 10.0.0.0/8,192.168.0.0/16
      ORBITA_PUBLIC_IPS: 203.0.113.10
    volumes:
      - cli-data:/data
      - cli-spool:/var/spool/exim4
volumes:
  cli-data:
  cli-spool:
```

O arquivo [`docker-compose.cli.yml`](../docker-compose.cli.yml) traz um modelo completo.

> Mantenha o volume **`/data`**: ele guarda as chaves DKIM e o cursor de envio. Sem ele, as chaves mudam a cada recriação e o DNS fica errado (o contêiner avisa no log).

## Variáveis

| Variável | Padrão | Descrição |
|---|---|---|
| `ORBITA_MODE` | `relay` | `relay` (entrega direta, com DKIM) ou `smarthost` |
| `ORBITA_HOSTNAME` | — (**obrigatória**) | Nome DNS do relay (usado no EHLO e no certificado) |
| `ORBITA_IP_MODE` | `dual` | `dual` (IPv4+IPv6) ou `ipv4` |
| `ORBITA_MAX_MESSAGE_MB` | `25` | Tamanho máximo da mensagem |
| `ORBITA_RELAY_NETS` | `10.0.0.0/8,192.168.0.0/16,fd00::/8` | IPs/redes autorizados a enviar |
| `ORBITA_RELAY_NETS_AUTO` | `true` | Soma o loopback e a rede Docker do contêiner |
| `ORBITA_HOST_IPS` | — | IPs da máquina a autorizar |
| `ORBITA_DOMAINS` | — | Domínios de envio: recebem chave DKIM e restringem o remetente |
| `ORBITA_DKIM_SELECTOR` | `dkim` | Seletor DKIM |
| `ORBITA_ENFORCE_SENDER_DOMAINS` | `true` | Recusa remetentes fora de `ORBITA_DOMAINS` |
| `ORBITA_PUBLIC_IPS` | — | IPs públicos de saída (para o SPF). Sem isso, usa `a:<hostname>` |
| `ORBITA_DMARC_POLICY` / `ORBITA_DMARC_RUA` | `none` / — | Texto do registro DMARC |
| `ORBITA_SMARTHOST_HOST` / `_PORT` | — / `587` | Servidor de saída (modo `smarthost`) |
| `ORBITA_SMARTHOST_TLS` | `starttls` | `starttls`, `tls` ou `none` |
| `ORBITA_SMARTHOST_USER` / `_PASS` | — | Credenciais (informe as duas) |
| `ORBITA_CENTRAL_URL` | — | Ex.: `https://orbita.exemplo.com.br`. Sem ela, o relay roda sozinho |
| `ORBITA_CENTRAL_TOKEN` | — | Token gerado em **Relays remotos** (obrigatório com a URL) |
| `ORBITA_CENTRAL_SKIP_VERIFY` | `false` | Aceita certificado autoassinado |
| `ORBITA_CENTRAL_INTERVAL` | `10` | Intervalo de envio (s) |
| `ORBITA_LOG_ROTATE_MB` / `ORBITA_LOG_KEEP` | `50` / `7` | Rotação dos logs |

Valores inválidos fazem o contêiner **parar na subida** com a mensagem do erro. Para mudar a configuração, edite o compose e rode `docker compose up -d`.

## DKIM e DNS

No modo `relay`, ao subir, o contêiner gera uma chave DKIM (RSA 2048) para cada domínio de `ORBITA_DOMAINS` e imprime os registros SPF, DKIM e DMARC prontos para publicar. Para vê-los de novo:

```bash
docker exec orbitarelay-cli orbita dns
```

No modo `smarthost` a assinatura e o SPF ficam com o provedor do smarthost.

## Logs

```bash
docker logs -f orbitarelay-cli
```

Mostram o `main.log` do Exim em tempo real, linhas do `reject.log` (`[reject]`) e do `panic.log` (`[panic]`) e os eventos do próprio contêiner (`[cli]`). Os arquivos continuam em `/var/log/exim`, com rotação.

## Comandos úteis

| Comando | O que faz |
|---|---|
| `docker exec orbitarelay-cli orbita status` | SMTP, fila, chaves DKIM e conexão com o central |
| `docker exec orbitarelay-cli orbita dns` | Registros SPF, DKIM e DMARC |
| `docker exec orbitarelay-cli orbita config` | Configuração efetiva (`--exim` mostra o `exim.conf`, sem senhas) |
| `docker exec orbitarelay-cli orbita queue` | Fila de mensagens |
| `docker exec orbitarelay-cli orbita test-mail destino@exemplo.com [remetente]` | Envia uma mensagem de teste |

## Enviando dados ao servidor central

1. No central, em **Relays remotos**, adicione um servidor e copie o token.
2. No compose, informe `ORBITA_CENTRAL_URL` e `ORBITA_CENTRAL_TOKEN`.
3. Confira com `orbita status`.

O envio usa o mesmo protocolo dos clientes completos, retoma de onde parou se o central ficar fora do ar e não duplica eventos. O central mostra o **tipo** (CLI ou App completo) e a **versão** de cada relay, com aviso quando estiver desatualizado.

## Segurança

- Conexões pelo gateway do Docker (origem mascarada) são sempre recusadas; use o IP da máquina. Veja [Docker, rede e IPv6](docker-rede-ipv6.md).
- A senha do smarthost e o token ficam em texto no compose: proteja o arquivo.
