# OrbitaRelay

Relay SMTP (Exim) + painel de relatórios, unificando `clubenaval/relatorios-smtp` e `AprendendoLinux/exim-relay`.

- **Backend:** Go (API, ingestão de logs, gerador de config do Exim, coleta de servidores Mailcow, geração de PDF)
- **Frontend:** React + TypeScript (Vite), visual futurista com modo claro/escuro
- **Banco:** MySQL (`mysql:latest`)
- **MTA:** Exim 4.96 com configuração gerada pelo painel

**Recursos principais:** relay SMTP com DKIM/SPF/DMARC, relatórios com filtros e exportação (CSV e PDF), vários servidores (central e clientes), servidores Mailcow, 2FA (aplicativo ou e-mail), recuperação de senha por e-mail, login local e LDAP/AD, diagnóstico de IPv6 no Docker e detecção automática das redes autorizadas.

## Subir

```bash
docker compose up -d
docker compose logs orbitarelay-app | grep TOKEN
```

## Atualizando

As imagens públicas ficam em `ghcr.io/aprendendolinux/orbita-relay-app` e `ghcr.io/aprendendolinux/orbita-relay-mta` (tags por versão e `latest`). Para atualizar, troque a tag no `docker-compose.yml` (ou use `latest`) e rode:

```bash
docker compose pull && docker compose up -d
```

Atualize **as duas imagens** (app e mta) juntas. O banco é migrado automaticamente na inicialização, sem perda de dados.

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
Acesse `http://servidor:8080`. O primeiro acesso é feito em etapas: **Acesso** (token, nome do sistema, usuário e e-mail, obrigatório) e **Senha** (com confirmação, medidor de força e gerador de senha). Se o MySQL não estiver configurado pelo compose, há antes uma etapa de **Banco de dados**. Em seguida o assistente de configuração abre.

No primeiro login um **assistente** guia a configuração (nome, fuso, modo Relay SMTP ou Smarthost, autenticação, logs); ele pode ser reaberto em Configurações.
No modo Smarthost o menu Domínios e DKIM fica oculto; no modo Relay SMTP o painel mostra os registros SPF, DKIM e DMARC de cada domínio.

Todo o resto (LDAP, domínios, DKIM, smarthost, redes autorizadas, IPv6, SMTP do sistema, retenção, tema padrão) é configurado no painel.
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

## Variáveis de ambiente (serviço `orbitarelay-app`)

| Variável | Função |
|---|---|
| `ORBITA_DB_HOST`, `ORBITA_DB_PORT`, `ORBITA_DB_NAME`, `ORBITA_DB_USER`, `ORBITA_DB_PASSWORD`, `ORBITA_DB_SSL` | Conexão com o MySQL |
| `ORBITA_DOCKER_API` | Endereço do `orbitarelay-proxy`, usado no diagnóstico de IPv6 |
| `ORBITA_HOST_IPS` | (opcional) IPs ou CIDRs da máquina, separados por vírgula, incluídos nas redes autorizadas |
| `ORBITA_DOCKER_GATEWAYS` | (opcional) gateways do Docker a recusar, se a detecção automática não bastar |

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

## Configurações do servidor de e-mail

As configurações do MTA ficam em abas separadas por finalidade (e o assistente segue a mesma divisão, com um passo próprio de **Redes**):

| Aba | Conteúdo | Aparece |
|---|---|---|
| **Servidor de e-mail** | Modo de operação (Relay SMTP ou Smarthost), hostname (HELO), tamanho máximo, restringir remetentes | Sempre |
| **Smarthost** | Servidor, porta, criptografia, usuário e senha | Só no modo Smarthost |
| **Redes e acesso** | Redes autorizadas a enviar, detecção automática, modo IP e diagnóstico de IPv6 | Sempre |
| **Entrega e DNS** | IPs públicos de saída (SPF), seletor DKIM e política DMARC | Só no modo Relay SMTP |

Em **Entrega e DNS** também ficam os domínios: cadastro, geração da chave DKIM e os registros SPF, DKIM e DMARC prontos para copiar, com verificação de DNS (a página **Domínios e DKIM** continua disponível no menu). Cada registro de DNS mostra o nome e o valor, com um botão pequeno para copiar o **nome completo** (por exemplo `_dmarc.seudominio.com.br`) e o botão **Copiar valor**. O registro **SPF** é montado automaticamente: usa os IPs públicos de saída configurados; sem eles, usa `a:<hostname do relay>`; e, sem hostname, usa o próprio domínio adicionado (`a:<domínio>`), mostrando os IPs detectados pelo DNS e avisando se o domínio não tem registro A/AAAA. Ao adicionar um domínio, a chave DKIM (RSA 2048) já é gerada e ativada, e o nome do registro DKIM acompanha o **seletor** configurado (padrão `dkim`). A tela explica o que cada política DMARC faz (none, quarantine, reject). Em **Redes e acesso**, as redes detectadas e as sugestões aparecem numa única lista recolhível, com **Adicionar todos**.

No modo Relay SMTP o assistente tem o passo **Entrega e DNS** (seletor DKIM, IPs públicos, DMARC, domínios com a chave DKIM e os registros de DNS), que não aparece no Smarthost. A explicação das políticas DMARC é uma linha discreta, com o link "Ver todas".

O **hostname do relay (HELO)** é obrigatório e já vem preenchido com o endereço pelo qual o painel foi acessado. No passo **Redes** do assistente, todas as redes encontradas já são adicionadas à lista (remova as que não quiser).

As abas seguem o modo escolhido na hora, mesmo antes de salvar, e os valores das abas ocultas não são apagados.

## Servidores Mailcow

O OrbitaRelay pode registrar as mensagens recebidas e enviadas por servidores [Mailcow](https://mailcow.email), lendo a API deles. Funciona em qualquer modo de implantação e nada é instalado nem alterado no Mailcow.

1. No Mailcow, abra **Sistema > Configuração > Acesso > API** (versões antigas: Configuração > Acesso).
2. Ative o acesso à API e, em **Permitir acesso à API a partir destes IPs/redes**, informe o IP público do OrbitaRelay.
3. Gere a **chave de API somente leitura** (nunca use a de leitura e escrita).
4. No OrbitaRelay, abra **Servidores Mailcow**, informe nome, endereço (`https://mail.exemplo.com.br`) e a chave, use **Testar conexão** e salve.

Como funciona:

- O OrbitaRelay consulta a cada 30 segundos os logs `postfix` e `rspamd-history` (`GET /api/v1/get/logs/...`, cabeçalho `X-API-Key`) e correlaciona as duas fontes pelo ID de fila e pelo `Message-ID`.
- Cada mensagem aparece em **Mensagens** com o nome do servidor, o sentido (↓ recebida, ↑ enviada), os destinatários e o resultado da entrega (entregue, adiada ou falha). Rejeições do Rspamd aparecem como falha.
- Sentido: mensagens autenticadas por um usuário do Mailcow contam como enviadas; as demais, como recebidas.
- Mensagens de que o Mailcow já não guarda o log de entrega do Postfix aparecem como **Aceita** (o Rspamd aceitou, sem confirmação de entrega). Avisos gerados pelo próprio Postfix (atraso ou devolução, sem remetente) aparecem com o assunto "Notificação gerada pelo Postfix". Campos vazios mostram "Sem remetente", "Sem assunto", "Sem destinatário" e "Sem origem".
- A chave de API fica criptografada (AES-256-GCM) no banco. O histórico inicial é limitado ao que o Mailcow mantém nos logs.
- Se o servidor ficar indisponível, o erro aparece na lista e a coleta continua de onde parou assim que ele voltar, desde que os logs do Mailcow ainda cubram o período.

## Relatório em PDF e atualização automática

- **Exportar CSV** (em Mensagens) também abre uma janela com os mesmos filtros, para não exportar tudo.
- **Exportar PDF** (em Mensagens): abre uma janela para escolher período, status, sentido, servidor, remetente, destinatário, origem e busca, além do conteúdo (resumo com gráfico, servidor, origem, tamanho, ID), a ordem e o limite de mensagens. O arquivo sai em A4 vertical, tema claro, com o logo do sistema; textos longos quebram linha em vez de serem cortados.
- **Atualização automática**: Mensagens e Painel se atualizam sozinhos (a cada 15, 30, 60 ou 120 s) enquanto a aba está visível. Há uma chave para ligar e desligar e o botão **Atualizar** para buscar na hora. A preferência fica salva no navegador.

## Autenticação em dois fatores (2FA)

Cada usuário, local ou do LDAP/AD, ativa o 2FA em **Segurança (2FA)**:

- **Aplicativo autenticador** (recomendado): QR Code ou chave manual, compatível com Google Authenticator, Microsoft Authenticator, Authy, 1Password e Bitwarden (TOTP, 6 dígitos, 30 s).
- **Código por e-mail**: exige configurar o SMTP em **Configurações > E-mail do sistema (2FA)**. O código vai sempre para o **e-mail cadastrado na conta** (o usuário não escolhe outro endereço). A configuração do SMTP fica salva no banco, aparece como resumo e é alterada pelo botão **Editar**.
- Ao ativar, são gerados **10 códigos de recuperação** de uso único, exibidos uma só vez.
- O login passa a pedir o código depois da senha. Há limite de tentativas e o mesmo código do aplicativo não pode ser reutilizado.
- Ao informar o código no login, dá para marcar **Não pedir o código neste dispositivo por 30 dias**. O dispositivo confiável é revogado ao desativar o 2FA, ao trocar ou redefinir a senha e pelo botão "Remover todos" em Segurança (2FA).
- **Desativar o 2FA** não pede o código: abre uma confirmação avisando do risco (e pede a senha, para usuários locais).
- Um administrador pode **redefinir o 2FA** de um usuário em Usuários (por exemplo, se ele perdeu o celular e os códigos).
- Dados sensíveis (segredo TOTP e senha do SMTP) ficam criptografados no banco (AES-256-GCM).

## E-mails do sistema e recuperação de senha

- Em **Configurações > E-mail do sistema (2FA)** informe o SMTP de saída (pode ser o seu próprio relay ou qualquer outro). Há um campo para enviar um e-mail de teste ao lado do botão.
- Todos os e-mails do sistema (teste, códigos de 2FA, recuperação de senha, aviso de senha alterada) usam um modelo HTML com o logo e a identidade visual do sistema, mais uma versão em texto simples.
- **Esqueci minha senha**: aparece no login quando o SMTP e o **Endereço público do painel** estão configurados (o endereço é preenchido sozinho na primeira vez que um administrador abre as Configurações pelo endereço real). Vale para usuários locais com e-mail cadastrado (usuários do LDAP/AD trocam a senha no diretório). O sistema envia um e-mail com um **botão/link para redefinir a senha**, válido por 30 minutos e de uso único; a resposta é sempre a mesma, exista ou não a conta.
- **Alterar o próprio e-mail** (usuários locais, em **Minha conta**): o usuário informa o novo endereço e a senha atual, e recebe no **novo** e-mail um botão de confirmação (link de 1 hora). Só depois do clique o e-mail da conta muda, e um aviso vai para o e-mail antigo. Cada e-mail só pode estar em uma conta.
- Ao redefinir a senha, todas as sessões do usuário são encerradas e, se a conta usa 2FA, ele continua sendo exigido no login.

## Redes autorizadas a enviar (detecção automática)

Em **Configurações > Servidor de e-mail**, a opção **Incluir automaticamente as redes detectadas** (ligada por padrão) soma à lista manual: o loopback, a rede Docker em que o contêiner está (IPv4 e IPv6, lida das interfaces dele) e os IPs da máquina conhecidos. A lista detectada aparece na própria tela e é recalculada ao iniciar e ao salvar.

- Dentro do contêiner só é possível ver as interfaces dele. O IP da máquina é aprendido quando você acessa o painel por ele (ex.: `http://192.168.4.129:8080`) ou informado na variável `ORBITA_HOST_IPS` do serviço `orbitarelay-app` (separe por vírgula; aceita IP ou CIDR).
- O campo **Redes autorizadas a enviar** já abre (em instalações novas) com as redes detectadas, e o botão **Adicionar as redes detectadas ao campo** as lista explicitamente nas instalações existentes. Há também sugestões (outras redes Docker, IP do host em cada rede e IPs do nome DNS usado no painel) e um campo para colar os IPs da máquina (`hostname -I`).
- **Redes autorizadas a enviar** e **IPs públicos de saída** são editados como lista: cada item tem o botão **Remover** e há um campo para adicionar manualmente (aceita vários valores separados por espaço ou vírgula, com validação de IPv4/IPv6 e CIDR).
- Nas sugestões, o botão **Adicionar todos** inclui tudo de uma vez, e os itens adicionados saem da lista de sugestões.
- Conexões que chegam pelo gateway do Docker (IP mascarado) continuam sendo recusadas, mesmo dentro de uma rede autorizada.

## Ocultar falhas (caixa inexistente e rejeições do Rspamd)

Em **Mensagens**, no **Painel** e na janela de **Exportar PDF** há duas chaves:

- **Ocultar falhas por caixa inexistente**: esconde mensagens que falharam só porque o destinatário não existe (respostas como `550 5.1.1`, "user unknown", "mailbox unavailable").
- **Ocultar rejeições do Rspamd** (aparece com servidores Mailcow): esconde as mensagens barradas como spam.

Só são ocultadas mensagens em que **todas** as falhas são desse tipo e que não foram entregues a nenhum outro destinatário. Os dados não são apagados: basta desligar a chave para ver tudo. A lista mostra quantas mensagens estão ocultas pelo filtro, e o PDF registra o filtro aplicado. Para deixar ligado por padrão, use **Configurações > Logs e retenção**.

## Usuários e perfis

Há dois perfis:

- **Administrador:** acesso total (Configurações, Usuários, Domínios, Servidores, Mailcow, Auditoria e assistente).
- **Leitura:** vê o Painel e as Mensagens, exporta CSV e PDF e cuida da própria conta (senha, e-mail e 2FA).

Login local e/ou LDAP/AD (grupo de acesso e grupo administrador opcionais). Em **Usuários**, ao criar um usuário local o **e-mail é obrigatório** (e único por conta) e o botão **Gerar senha aleatória** cria uma senha de 16 caracteres só com letras maiúsculas, minúsculas e números, já preenchendo a senha e a confirmação, com botão para copiá-la.

## Aparência

Visual futurista único, com modo escuro e claro. O **modo padrão** do sistema é escolhido em **Configurações > Geral** (escuro por padrão) e vale para quem ainda não escolheu o próprio modo no botão do canto da tela; a escolha pessoal fica salva no navegador.

O sistema não usa pop-ups do navegador (`confirm`/`prompt`): confirmações e pedidos de dados abrem sempre uma janela própria.

## Estrutura

```
backend/   API Go (auth, 2FA, e-mails, settings, domains/DKIM, eximcfg, ingest, Mailcow, reports/PDF)
frontend/  Painel React
mta/       Imagem do Exim + entrypoint (aplica config, recarrega, rotaciona logs)
```

## Roadmap

- Fila em tempo real (ver/reenviar/remover) e rejeições SMTP (reject log)
- Alertas (Telegram/webhook/e-mail), monitor de blacklists, métricas Prometheus
- Parser de relatórios DMARC, rate limiting por remetente
- Envio das mensagens de um Mailcow do cliente para o servidor central

## Licenças de terceiros

Os relatórios em PDF usam a fonte Liberation Sans (SIL Open Font License 1.1), embutida em `backend/fonts/`.
