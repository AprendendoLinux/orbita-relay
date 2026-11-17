# OrbitaRelay

Relay SMTP (Exim) + painel de relatórios, unificando `clubenaval/relatorios-smtp` e `AprendendoLinux/exim-relay`.

- **Backend:** Go (API, ingestão de logs, gerador de config do Exim)
- **Frontend:** React + TypeScript (Vite), visual futurista com modo claro/escuro
- **Banco:** MySQL (`mysql:latest`)
- **MTA:** Exim 4.96 com configuração gerada pelo painel

## Subir

```bash
docker compose up -d
docker compose logs orbitarelay-app | grep TOKEN
```

## Containers

| Serviço, container e hostname | Função |
|---|---|
| `orbitarelay-db` | MySQL |
| `orbitarelay-app` | Painel e API (porta 8080) |
| `orbitarelay-mta` | Exim (portas 25 e 587) |
| `orbitarelay-proxy` | Proxy somente leitura da API do Docker (diagnóstico de IPv6) |

O nome do serviço, do container e do hostname é o mesmo, então os comandos são equivalentes: `docker compose logs orbitarelay-mta` ou `docker logs -f orbitarelay-mta`, `docker exec -it orbitarelay-app sh`.
Com o campo "Hostname do relay (HELO)" vazio, o Exim se apresenta como `orbitarelay-mta`. Em produção, preencha com o nome DNS real do servidor (o mesmo do PTR e do SPF).

A conexão com o MySQL (host, porta, banco, usuário, senha, SSL) fica no `docker-compose.yml` (variáveis `ORBITA_DB_*`). **Troque a senha padrão `orbita` antes de usar em produção.**
Acesse `http://servidor:8080`, informe o token e crie o administrador; em seguida o assistente de configuração abre.

No primeiro login um **assistente** guia a configuração (nome, fuso, modo Relay SMTP ou Smarthost, autenticação, logs); ele pode ser reaberto em Configurações.
No modo Smarthost o menu Domínios e DKIM fica oculto; no modo Relay SMTP o painel mostra os registros SPF, DKIM e DMARC de cada domínio.

Todo o resto (LDAP, domínios, DKIM, smarthost, redes autorizadas, IPv6, retenção) é configurado no painel.
Segredos ficam criptografados (AES-256-GCM) com a chave `/data/master.key`. **Faça backup do volume `app-data`.**

## Diretórios mapeados (bind mounts)

Em vez de volumes nomeados, você pode mapear diretórios do host, por exemplo:

```yaml
orbitarelay-app:
  volumes:
    - /srv/orbitarelay:/data
    - /srv/orbitarelay/mta/config:/config
    - /srv/orbitarelay/mta/logs:/var/log/exim:ro
```

O Docker cria os diretórios que não existem como `root`. Para isso não causar `open /data/master.key: permission denied`, a imagem do app inicia como root, ajusta as permissões e então passa a rodar como o usuário `orbita` (uid 10001):

- `/data` e `/config` passam a pertencer ao usuário `orbita` (em `/config`, de forma recursiva).
- Em `/data`, apenas os arquivos da raiz (como `master.key`) são ajustados, **sem recursão**. Assim, subdiretórios de outros serviços dentro do mesmo caminho (como `dbdata` do MySQL e `mta/spool` do Exim) não são alterados.
- O MySQL e o Exim ajustam os próprios diretórios na inicialização.

Não é necessário rodar `chown` manualmente. Se você iniciar o container com `--user`, o ajuste é ignorado e o diretório precisa já estar com a permissão correta.

## Logs do Exim

- Exim grava em `/var/log/exim/main.log` (volume `mta-logs`), com `+subject`, TLS e confirmação SMTP.
- `logrotate` roda no container do MTA: diário **e** por tamanho (`maxsize`), `rotate N`, compressão. Valores editáveis no painel.
- O painel lê o log incrementalmente (controla inode/offset, sobrevive a rotação) e grava no MySQL.
- A retenção no banco é automática (padrão 365 dias). O arquivo bruto não cresce sem limite.

## IPv6

O container `mta` publica as portas `25` e `587` (IPv4 e IPv6) em uma rede do compose com IPv6 habilitado. Para o IPv6 funcionar com o IP de origem real, o daemon do Docker precisa estar preparado.

- Em **Configurações > Servidor de e-mail** (modo IP "IPv4 + IPv6") e no assistente, o painel **IPv6 no Docker** detecta a versão do Docker, o IPv6 no daemon e no container, a saída IPv6 e se o IP de origem está sendo mascarado. Quando algo falta, mostra o passo a passo para a sua versão.
- A versão e o estado do daemon vêm do serviço `orbitarelay-proxy` (proxy somente leitura da API do Docker, com acesso apenas a versão, info e redes). Para não usá-lo, remova o serviço e a variável `ORBITA_DOCKER_API` do app; o painel passa a pedir a versão manualmente.
- Resumo: Docker 27 ou mais novo, `{"ipv6": true, "ip6tables": true}` em `/etc/docker/daemon.json`; Docker 26 ou mais antigo, o mesmo mais `"experimental": true`. Depois, `sudo systemctl restart docker` e `docker compose up -d --force-recreate`.
- **Segurança:** conexões que chegam com o IP do gateway do Docker (origem mascarada) são sempre recusadas, para que uma falha de IPv6 nunca vire open relay. Isso inclui envios feitos a partir do próprio host via `127.0.0.1`; use o IP da máquina.
- Sem IPv6 no Docker, o relay continua funcionando normalmente em IPv4 (modo IP "Somente IPv4" em Configurações).

## Enviando a partir do próprio servidor (Docker)

**Não use `localhost` nem `127.0.0.1` como servidor SMTP na máquina que roda o Docker.** Use o **endereço IP da própria máquina** (ex.: `192.168.0.10`) ou o nome DNS dela.

Motivo: ao acessar a porta publicada por `localhost`, o Docker encaminha a conexão por um intermediário (`docker-proxy`) e o Exim passa a ver a origem como o gateway da rede do Docker (ex.: `172.28.0.1`), e não `127.0.0.1`. Essa aparência é idêntica à de uma conexão externa com o IP mascarado, então o OrbitaRelay **recusa de propósito** (para nunca virar open relay):

```
550 Connection through Docker gateway refused (source IP masked); connect using the server IP address instead of localhost
```

Como configurar (troque `192.168.0.10` pelo IP da máquina; ele precisa estar em **Redes autorizadas**, e `192.168.0.0/16`, `10.0.0.0/8` e `fd00::/8` já vêm incluídas):

| Cliente | Configuração |
|---|---|
| mutt (`~/.muttrc`) | `set smtp_url="smtp://192.168.0.10:25"` |
| msmtp (`~/.msmtprc`) | `host 192.168.0.10` e `port 25` |
| swaks | `swaks --server 192.168.0.10 --to destino@exemplo.com` |
| PHP (`php.ini`) | `SMTP = 192.168.0.10` e `smtp_port = 25` |
| Aplicações/scripts | servidor `192.168.0.10`, porta `25` (ou `587`) |

Dicas:

- Fixe o IP da máquina (DHCP com reserva ou IP estático); se ele mudar, os envios deixam de funcionar.
- Outros servidores da rede e containers de outras stacks usam o mesmo IP e porta, sem restrição.
- O `localhost` não é suportado neste modelo de rede; use sempre o IP ou o nome DNS da máquina.

## Vários servidores (servidor central e clientes)

Em Configurações > Geral (ou no assistente) escolha a **implantação**:

- **Standalone:** instalação única.
- **Servidor central:** recebe os eventos de outros relays. Cadastre cada um em **Servidores** para gerar um token (exibido uma única vez). Dashboard, mensagens e CSV ganham o filtro por servidor.
- **Cliente:** envia os eventos deste relay ao servidor central (modo push; só precisa de conexão de saída). Informe protocolo (HTTP ou HTTPS), endereço, porta, caminho base opcional e o token, e use **Testar conexão**.

Detalhes:

- Atrás de proxy reverso com HTTPS: o central continua em HTTP interno; no cliente use HTTPS/443 (e o caminho base, se houver). No central ative "Estou atrás de um proxy reverso" para registrar o IP real.
- Certificado autoassinado: opção "Ignorar validação do certificado" no cliente.
- Se o central ficar fora do ar, o cliente retoma do ponto em que parou (cursor no log), sem perdas e sem duplicar eventos.
- Cada instalação mantém banco e painel próprios. Com HTTP o token trafega sem criptografia.

## Aparência

Visual futurista único, com alternância entre modo escuro (padrão) e claro, lembrada no navegador.

## Perfis

`admin` (tudo), `operator` e `viewer` (relatórios). Login local e/ou LDAP/AD (grupo de acesso e grupo admin opcionais).

## Estrutura

```
backend/   API Go (auth, settings, domains/DKIM, eximcfg, ingest, reports)
frontend/  Painel React
mta/       Imagem do Exim + entrypoint (aplica config, recarrega, rotaciona logs)
```

## Roadmap

- Fila em tempo real (ver/reenviar/remover) e rejeições SMTP (reject log)
- Alertas (Telegram/webhook/e-mail), monitor de blacklists, métricas Prometheus
- Parser de relatórios DMARC, rate limiting por remetente, 2FA
