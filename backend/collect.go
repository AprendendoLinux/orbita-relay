package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxCollectEvents = 5000
	maxCollectBody   = 8 << 20
)

type wireEvent struct {
	At      time.Time `json:"at"`
	ID      string    `json:"id"`
	Kind    string    `json:"k"`
	Sender  string    `json:"sender,omitempty"`
	Host    string    `json:"host,omitempty"`
	IP      string    `json:"ip,omitempty"`
	Subject string    `json:"subject,omitempty"`
	Size    int       `json:"size,omitempty"`
	Rcpt    string    `json:"rcpt,omitempty"`
	RHost   string    `json:"rhost,omitempty"`
	RIP     string    `json:"rip,omitempty"`
	Detail  string    `json:"detail,omitempty"`
}

type pushStatus struct {
	mu      sync.Mutex
	lastOK  time.Time
	lastErr string
	sent    int64
}

func (p *pushStatus) ok(n int) {
	p.mu.Lock()
	p.lastOK, p.lastErr = time.Now(), ""
	p.sent += int64(n)
	p.mu.Unlock()
}

func (p *pushStatus) fail(err error) (changed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	msg := err.Error()
	changed = p.lastErr != msg
	p.lastErr = msg
	return
}

func (p *pushStatus) idleFor() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return time.Since(p.lastOK)
}

func (a *App) collectURL(path string) (string, error) {
	host := a.sStr("collect_host")
	port := a.sInt("collect_port")
	if host == "" || port == 0 {
		return "", errors.New("servidor central não configurado")
	}
	scheme := "https"
	if a.sStr("collect_scheme") == "http" {
		scheme = "http"
	}
	return scheme + "://" + net.JoinHostPort(host, strconv.Itoa(port)) + a.sStr("collect_path") + path, nil
}

func (a *App) collectDo(ctx context.Context, method, path string, body []byte, gz bool) (*http.Response, func(), error) {
	u, err := a.collectURL(path)
	if err != nil {
		return nil, nil, err
	}
	token := a.sStr("collect_token")
	if token == "" {
		return nil, nil, errors.New("token do servidor não informado")
	}
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: a.sBool("collect_skip_verify"), MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	}
	cl := &http.Client{
		Transport:     tr,
		Timeout:       45 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "OrbitaRelay/"+version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		if gz {
			req.Header.Set("Content-Encoding", "gzip")
		}
	}
	resp, err := cl.Do(req)
	if err != nil {
		tr.CloseIdleConnections()
		return nil, nil, err
	}
	return resp, tr.CloseIdleConnections, nil
}

func respError(resp *http.Response) error {
	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return fmt.Errorf("o servidor respondeu com redirecionamento (%d); ajuste protocolo, porta e caminho para a URL final", resp.StatusCode)
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return errors.New("token recusado pelo servidor central")
	case resp.StatusCode == 404:
		return errors.New("endpoint não encontrado (o destino está em modo Servidor? o caminho está correto?)")
	case resp.StatusCode == 429:
		return errors.New("servidor central limitou as requisições (muitas tentativas)")
	}
	return fmt.Errorf("resposta inesperada do servidor central: HTTP %d", resp.StatusCode)
}

func (a *App) pushBatch(ctx context.Context, evs []logEvent) error {
	if len(evs) == 0 {
		return nil
	}
	wire := make([]wireEvent, len(evs))
	for i, e := range evs {
		wire[i] = wireEvent{At: e.at, ID: e.eximID, Kind: e.kind, Sender: e.sender, Host: e.host, IP: e.ip,
			Subject: e.subject, Size: e.size, Rcpt: e.rcpt, RHost: e.rhost, RIP: e.rip, Detail: e.detail}
	}
	raw, err := json.Marshal(map[string]any{"events": wire})
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(raw)
	_ = zw.Close()
	resp, done, err := a.collectDo(ctx, http.MethodPost, "/api/collect", buf.Bytes(), true)
	if err != nil {
		return err
	}
	defer done()
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != 200 {
		return respError(resp)
	}
	a.push.ok(len(evs))
	return nil
}

func (a *App) runPush(ctx context.Context) {
	for {
		wait := 10 * time.Second
		if a.ready() {
			if n := a.sInt("collect_interval"); n >= 2 {
				wait = time.Duration(n) * time.Second
			}
			if a.sStr("deploy_mode") == "client" {
				err := a.ingestOnce(ctx, "push", a.pushBatch)
				if err == nil && a.push.idleFor() > time.Minute {
					_, err = a.pingCentral(ctx)
					if err == nil {
						a.push.ok(0)
					}
				}
				if err != nil && ctx.Err() == nil {
					if a.push.fail(err) {
						log.Printf("envio ao servidor central: %v", err)
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

type pingInfo struct {
	Node    string `json:"node"`
	Version string `json:"version"`
}

func (a *App) pingCentral(ctx context.Context) (pingInfo, error) {
	var out pingInfo
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, done, err := a.collectDo(ctx, http.MethodGet, "/api/collect/ping", nil, false)
	if err != nil {
		return out, fmt.Errorf("falha na conexão: %w", err)
	}
	defer done()
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return out, respError(resp)
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out)
	return out, nil
}

func (a *App) handleCollectTest(w http.ResponseWriter, r *http.Request) {
	out, err := a.pingCentral(r.Context())
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "node": out.Node, "version": out.Version})
}

func (a *App) handleCollectStatus(w http.ResponseWriter, r *http.Request) {
	a.push.mu.Lock()
	out := map[string]any{"last_error": a.push.lastErr, "sent": a.push.sent}
	if !a.push.lastOK.IsZero() {
		out["last_ok"] = a.push.lastOK
	}
	a.push.mu.Unlock()
	pending := int64(0)
	if fi, err := os.Stat(filepath.Join(a.logDir, "main.log")); err == nil {
		var ino, pos int64
		if a.row(r.Context(), `SELECT inode,pos FROM ingest_state WHERE file='push'`).Scan(&ino, &pos) != nil || ino == inodeOf(fi) {
			pending = max(fi.Size()-pos, 0)
		} else {
			pending = fi.Size()
		}
	}
	out["pending_bytes"] = pending
	writeJSON(w, 200, out)
}

type nodeCtx struct {
	ID   int64
	Name string
}

func (a *App) nodeAuth(w http.ResponseWriter, r *http.Request) (nodeCtx, bool) {
	var n nodeCtx
	if a.sStr("deploy_mode") != "server" {
		writeErr(w, 404, "não encontrado")
		return n, false
	}
	ip := clientIP(r)
	if a.collectRL.over("collect:" + ip) {
		w.Header().Set("Retry-After", "300")
		writeErr(w, 429, "muitas tentativas")
		return n, false
	}
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if ok && tok != "" {
		err := a.row(r.Context(), `SELECT id,name FROM nodes WHERE token_hash=? AND disabled=0 AND kind='relay'`, hashToken(strings.TrimSpace(tok))).Scan(&n.ID, &n.Name)
		if err == nil {
			_, _ = a.exec(r.Context(), `UPDATE nodes SET last_seen=UTC_TIMESTAMP(6), last_ip=?, version=? WHERE id=?`,
				trunc(ip, 64), trunc(r.Header.Get("User-Agent"), 32), n.ID)
			return n, true
		}
	}
	a.collectRL.allow("collect:" + ip)
	writeErr(w, 401, "token inválido")
	return n, false
}

func (a *App) handleCollectPing(w http.ResponseWriter, r *http.Request) {
	n, ok := a.nodeAuth(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "node": n.Name, "version": version})
}

func (a *App) handleCollect(w http.ResponseWriter, r *http.Request) {
	n, ok := a.nodeAuth(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxCollectBody)
	var rd io.Reader = r.Body
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			writeErr(w, 400, "gzip inválido")
			return
		}
		defer gz.Close()
		rd = io.LimitReader(gz, 4*maxCollectBody)
	}
	var in struct {
		Events []wireEvent `json:"events"`
	}
	if err := json.NewDecoder(rd).Decode(&in); err != nil {
		writeErr(w, 400, "corpo inválido")
		return
	}
	if len(in.Events) > maxCollectEvents {
		writeErr(w, 413, "lote grande demais")
		return
	}
	evs := make([]logEvent, 0, len(in.Events))
	limit := time.Now().Add(48 * time.Hour)
	for _, e := range in.Events {
		if !reEximID.MatchString(e.ID) || e.At.IsZero() || e.At.After(limit) || e.At.Year() < 2000 {
			continue
		}
		ev := logEvent{at: e.At.UTC(), eximID: e.ID, kind: e.Kind, size: max(e.Size, 0)}
		switch e.Kind {
		case "<=":
			ev.sender, ev.host, ev.ip = strings.ToLower(e.Sender), e.Host, e.IP
			ev.subject = trunc(clean(e.Subject), 500)
		case "=>", "->", "**", "==":
			if e.Rcpt == "" {
				continue
			}
			ev.rcpt, ev.rhost, ev.rip = strings.ToLower(e.Rcpt), e.RHost, e.RIP
			ev.detail = trunc(clean(e.Detail), 1000)
		case "Completed":
		default:
			continue
		}
		evs = append(evs, ev)
	}
	if err := a.flush(r.Context(), n.ID, evs); err != nil {
		log.Printf("coleta de %s: %v", n.Name, err)
		writeErr(w, 500, "erro de banco")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "accepted": len(evs), "rejected": len(in.Events) - len(evs)})
}
