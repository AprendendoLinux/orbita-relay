# OrbitaRelay

Relay SMTP (Exim) + painel de relatórios, unificando `clubenaval/relatorios-smtp` e `AprendendoLinux/exim-relay`.

- **Backend:** Go (API, ingestão de logs, gerador de config do Exim)
- **Frontend:** React + TypeScript (Vite), temas via CSS variables
- **Banco:** PostgreSQL
- **MTA:** Exim 4.96 com configuração gerada pelo painel

## Subir

```bash
docker compose up -d --build
docker compose logs app | grep TOKEN
```

Acesse `http://servidor:8080`, informe o token e conclua o assistente (banco + administrador).
No banco embutido use host `db`, usuário/banco `orbita` e a senha de `ORBITA_DB_PASSWORD` (padrão `orbita`; defina em `.env`).
É possível apontar para um PostgreSQL externo no assistente.

Todo o resto (LDAP, domínios, DKIM, smarthost, redes autorizadas, IPv6, retenção, temas) é configurado no painel.
Segredos ficam criptografados (AES-256-GCM) com a chave `/data/master.key`. **Faça backup do volume `app-data`.**

## Logs do Exim

- Exim grava em `/var/log/exim/main.log` (volume `mta-logs`), com `+subject`, TLS e confirmação SMTP.
- `logrotate` roda no container do MTA: diário **e** por tamanho (`maxsize`), `rotate N`, compressão. Valores editáveis no painel.
- O painel lê o log incrementalmente (controla inode/offset, sobrevive a rotação) e grava no PostgreSQL.
- A retenção no banco é automática (padrão 365 dias). O arquivo bruto não cresce sem limite.

## IPv6

O container `mta` usa `network_mode: host`: escuta em `0.0.0.0` e `::` nas portas 25/587 e enxerga o IP real de origem (IPv4 e IPv6) para a lista de redes autorizadas.
A rede interna do compose também tem IPv6 habilitado. Modo `ipv4` disponível em Configurações > Relay SMTP.
Requer que a porta 25 do host esteja livre.

## Temas

Padrão **Clube Naval** (vermelho `#C8102E`/azul-marinho, logo incluído), além de *Orbita Neon* e *Aurora Verde*.
Temas padrão são imutáveis: duplique para editar cores, fonte, vidro/brilho e trocar o logotipo.

## Perfis

`admin` (tudo), `operator` e `viewer` (relatórios). Login local e/ou LDAP/AD (grupo de acesso e grupo admin opcionais).

## Estrutura

```
backend/   API Go (auth, settings, themes, domains/DKIM, eximcfg, ingest, reports)
frontend/  Painel React
mta/       Imagem do Exim + entrypoint (aplica config, recarrega, rotaciona logs)
web-static/ Logo do tema Clube Naval
```

## Roadmap

- Fila em tempo real (ver/reenviar/remover) e rejeições SMTP (reject log)
- Alertas (Telegram/webhook/e-mail), monitor de blacklists, métricas Prometheus
- Parser de relatórios DMARC, rate limiting por remetente, 2FA
