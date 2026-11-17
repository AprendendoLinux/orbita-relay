# OrbitaRelay

Relay SMTP (Exim) + painel de relatórios, unificando `clubenaval/relatorios-smtp` e `AprendendoLinux/exim-relay`.

- **Backend:** Go (API, ingestão de logs, gerador de config do Exim)
- **Frontend:** React + TypeScript (Vite), visual futurista com modo claro/escuro
- **Banco:** MySQL (`mysql:latest`)
- **MTA:** Exim 4.96 com configuração gerada pelo painel

## Subir

```bash
docker compose up -d --build
docker compose logs app | grep TOKEN
```

A conexão com o MySQL (host, porta, banco, usuário, senha, SSL) fica no `docker-compose.yml` (variáveis `ORBITA_DB_*`, com padrões que podem ser sobrescritos em `.env`). **Troque a senha padrão `orbita` antes de usar em produção.**
Acesse `http://servidor:8080`, informe o token e crie o administrador; em seguida o assistente de configuração abre.

No primeiro login um **assistente** guia a configuração (nome, fuso, modo Relay SMTP ou Smarthost, autenticação, logs); ele pode ser reaberto em Configurações.
No modo Smarthost o menu Domínios e DKIM fica oculto; no modo Relay SMTP o painel mostra os registros SPF, DKIM e DMARC de cada domínio.

Todo o resto (LDAP, domínios, DKIM, smarthost, redes autorizadas, IPv6, retenção) é configurado no painel.
Segredos ficam criptografados (AES-256-GCM) com a chave `/data/master.key`. **Faça backup do volume `app-data`.**

## Logs do Exim

- Exim grava em `/var/log/exim/main.log` (volume `mta-logs`), com `+subject`, TLS e confirmação SMTP.
- `logrotate` roda no container do MTA: diário **e** por tamanho (`maxsize`), `rotate N`, compressão. Valores editáveis no painel.
- O painel lê o log incrementalmente (controla inode/offset, sobrevive a rotação) e grava no MySQL.
- A retenção no banco é automática (padrão 365 dias). O arquivo bruto não cresce sem limite.

## IPv6

O container `mta` usa `network_mode: host`: escuta em `0.0.0.0` e `::` nas portas 25/587 e enxerga o IP real de origem (IPv4 e IPv6) para a lista de redes autorizadas.
A rede interna do compose também tem IPv6 habilitado. Modo `ipv4` disponível em Configurações > Relay SMTP.
Requer que a porta 25 do host esteja livre.

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
