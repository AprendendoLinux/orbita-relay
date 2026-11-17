package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type msgRow struct {
	ID         int64     `json:"id"`
	EximID     string    `json:"exim_id"`
	ReceivedAt time.Time `json:"received_at"`
	Sender     string    `json:"sender"`
	OriginHost string    `json:"origin_host"`
	OriginIP   string    `json:"origin_ip"`
	Subject    string    `json:"subject"`
	Size       int       `json:"size"`
	Status     string    `json:"status"`
	Recipients []string  `json:"recipients"`
}

var likeEsc = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (a *App) location() *time.Location {
	if l, err := time.LoadLocation(a.sStr("timezone")); err == nil {
		return l
	}
	return time.UTC
}

func (a *App) buildFilter(r *http.Request) (string, []any) {
	q := r.URL.Query()
	var conds []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	loc := a.location()
	if s := q.Get("from"); s != "" {
		if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
			add("m.received_at >= ?", t)
		}
	}
	if s := q.Get("to"); s != "" {
		if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
			add("m.received_at < ?", t.AddDate(0, 0, 1))
		}
	}
	if s := q.Get("status"); s == "queued" || s == "delivered" || s == "failed" || s == "deferred" {
		add("m.status = ?", s)
	}
	if s := strings.TrimSpace(q.Get("sender")); s != "" {
		add(`m.sender ILIKE ? ESCAPE '\'`, "%"+likeEsc.Replace(s)+"%")
	}
	if s := strings.TrimSpace(q.Get("host")); s != "" {
		add(`(m.origin_host ILIKE ? ESCAPE '\' OR m.origin_ip ILIKE ? ESCAPE '\')`, "%"+likeEsc.Replace(s)+"%")
	}
	if s := strings.TrimSpace(q.Get("recipient")); s != "" {
		add(`EXISTS (SELECT 1 FROM deliveries d WHERE d.message_id=m.id AND d.recipient ILIKE ? ESCAPE '\')`, "%"+likeEsc.Replace(s)+"%")
	}
	if s := strings.TrimSpace(q.Get("q")); s != "" {
		p := "%" + likeEsc.Replace(s) + "%"
		args = append(args, p)
		n := strconv.Itoa(len(args))
		conds = append(conds, `(m.subject ILIKE $`+n+` ESCAPE '\' OR m.sender ILIKE $`+n+` ESCAPE '\' OR m.exim_id ILIKE $`+n+
			` ESCAPE '\' OR EXISTS (SELECT 1 FROM deliveries d WHERE d.message_id=m.id AND d.recipient ILIKE $`+n+` ESCAPE '\'))`)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

const msgSelect = `SELECT m.id,m.exim_id,m.received_at,m.sender,m.origin_host,m.origin_ip,m.subject,m.size,m.status,
	COALESCE((SELECT array_agg(DISTINCT d.recipient) FROM deliveries d WHERE d.message_id=m.id),'{}') FROM messages m`

func (a *App) handleMessages(w http.ResponseWriter, r *http.Request) {
	where, args := a.buildFilter(r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 50
	}
	var total int
	if err := a.db.QueryRow(r.Context(), `SELECT count(*) FROM messages m`+where, args...).Scan(&total); err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	n := len(args)
	rows, err := a.db.Query(r.Context(), msgSelect+where+fmt.Sprintf(` ORDER BY m.received_at DESC LIMIT $%d OFFSET $%d`, n+1, n+2),
		append(args, size, (page-1)*size)...)
	if err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	defer rows.Close()
	out := []msgRow{}
	for rows.Next() {
		var m msgRow
		if rows.Scan(&m.ID, &m.EximID, &m.ReceivedAt, &m.Sender, &m.OriginHost, &m.OriginIP, &m.Subject, &m.Size, &m.Status, &m.Recipients) == nil {
			out = append(out, m)
		}
	}
	writeJSON(w, 200, map[string]any{"total": total, "page": page, "size": size, "items": out})
}

func (a *App) handleMessageDetail(w http.ResponseWriter, r *http.Request) {
	var m msgRow
	err := a.db.QueryRow(r.Context(), msgSelect+` WHERE m.id=$1`, r.PathValue("id")).
		Scan(&m.ID, &m.EximID, &m.ReceivedAt, &m.Sender, &m.OriginHost, &m.OriginIP, &m.Subject, &m.Size, &m.Status, &m.Recipients)
	if err != nil {
		writeErr(w, 404, "mensagem não encontrada")
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT at,recipient,status,remote_host,remote_ip,detail FROM deliveries WHERE message_id=$1 ORDER BY at,id`, m.ID)
	if err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	defer rows.Close()
	type dl struct {
		At         time.Time `json:"at"`
		Recipient  string    `json:"recipient"`
		Status     string    `json:"status"`
		RemoteHost string    `json:"remote_host"`
		RemoteIP   string    `json:"remote_ip"`
		Detail     string    `json:"detail"`
	}
	ds := []dl{}
	for rows.Next() {
		var d dl
		if rows.Scan(&d.At, &d.Recipient, &d.Status, &d.RemoteHost, &d.RemoteIP, &d.Detail) == nil {
			ds = append(ds, d)
		}
	}
	writeJSON(w, 200, map[string]any{"message": m, "deliveries": ds})
}

func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func (a *App) handleExport(w http.ResponseWriter, r *http.Request) {
	where, args := a.buildFilter(r)
	rows, err := a.db.Query(r.Context(), msgSelect+where+` ORDER BY m.received_at DESC LIMIT 200000`, args...)
	if err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="orbitarelay-`+time.Now().Format("20060102-150405")+`.csv"`)
	_, _ = w.Write([]byte("\xef\xbb\xbf"))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"Data/Hora", "ID", "Remetente", "Destinatarios", "Assunto", "Status", "Host de origem", "IP de origem", "Tamanho"})
	loc := a.location()
	for rows.Next() {
		var m msgRow
		if rows.Scan(&m.ID, &m.EximID, &m.ReceivedAt, &m.Sender, &m.OriginHost, &m.OriginIP, &m.Subject, &m.Size, &m.Status, &m.Recipients) != nil {
			continue
		}
		_ = cw.Write([]string{m.ReceivedAt.In(loc).Format("2006-01-02 15:04:05"), m.EximID, csvSafe(m.Sender),
			csvSafe(strings.Join(m.Recipients, "; ")), csvSafe(m.Subject), m.Status, csvSafe(m.OriginHost), m.OriginIP, strconv.Itoa(m.Size)})
	}
	cw.Flush()
}

type kv struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

func (a *App) topList(ctx context.Context, q string, days int) []kv {
	out := []kv{}
	rows, err := a.db.Query(ctx, q, days)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var x kv
		if rows.Scan(&x.Key, &x.Count) == nil {
			out = append(out, x)
		}
	}
	return out
}

func (a *App) handleStats(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 || days > 365 {
		days = 7
	}
	ctx := r.Context()
	tz := a.sStr("timezone")
	totals := map[string]int{"queued": 0, "delivered": 0, "failed": 0, "deferred": 0}
	type pt struct {
		Day    string `json:"day"`
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	series := []pt{}
	rows, err := a.db.Query(ctx, `SELECT to_char((received_at AT TIME ZONE $2)::date,'YYYY-MM-DD'), status, count(*)
		FROM messages WHERE received_at >= now() - make_interval(days => $1) GROUP BY 1,2 ORDER BY 1`, days, tz)
	if err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	for rows.Next() {
		var p pt
		if rows.Scan(&p.Day, &p.Status, &p.Count) == nil {
			series = append(series, p)
			totals[p.Status] += p.Count
		}
	}
	rows.Close()
	writeJSON(w, 200, map[string]any{
		"days": days, "totals": totals, "series": series,
		"top_senders": a.topList(ctx, `SELECT sender_domain, count(*) c FROM messages WHERE received_at >= now() - make_interval(days => $1) AND sender_domain<>'' GROUP BY 1 ORDER BY c DESC LIMIT 8`, days),
		"top_hosts": a.topList(ctx, `SELECT CASE WHEN origin_host<>'' THEN origin_host ELSE origin_ip END, count(*) c FROM messages WHERE received_at >= now() - make_interval(days => $1) GROUP BY 1 ORDER BY c DESC LIMIT 8`, days),
		"top_recipient_domains": a.topList(ctx, `SELECT split_part(d.recipient,'@',2), count(*) c FROM deliveries d WHERE d.at >= now() - make_interval(days => $1) GROUP BY 1 ORDER BY c DESC LIMIT 8`, days),
	})
}
