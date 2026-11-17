package main

import (
	"bufio"
	"context"
	"database/sql"
	"io"
	"log"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	reLine    = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) ([+-]\d{4}) ([0-9A-Za-z]{6}-[0-9A-Za-z]{6,11}-[0-9A-Za-z]{2,4}) (<=|=>|->|\*\*|==|Completed)(?: (.*))?$`)
	reHostIP  = regexp.MustCompile(`\bH=(?:([^\s(\[]\S*) )?(?:\(\S*\) )?\[([0-9A-Fa-f:.%]+)\](?::\d+)?`)
	reSize    = regexp.MustCompile(`\bS=(\d+)\b`)
	reSubject = regexp.MustCompile(`\bT="((?:[^"\\]|\\.)*)"`)
	reRemote  = regexp.MustCompile(`\bH=(?:(\S+) )?\[([0-9A-Fa-f:.%]+)\]`)
	reCode    = regexp.MustCompile(`\bC="((?:[^"\\]|\\.)*)"`)
)

type logEvent struct {
	at      time.Time
	eximID  string
	kind    string
	sender  string
	host    string
	ip      string
	subject string
	size    int
	rcpt    string
	rhost   string
	rip     string
	detail  string
}

var wordDecoder = new(mime.WordDecoder)

func decodeSubject(s string) string {
	if u, err := strconv.Unquote(`"` + s + `"`); err == nil {
		s = u
	}
	if d, err := wordDecoder.DecodeHeader(s); err == nil {
		s = d
	}
	s = strings.ToValidUTF8(s, "")
	s = strings.ReplaceAll(s, "\x00", "")
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}

func clean(s string) string {
	s = strings.ToValidUTF8(s, "")
	return strings.ReplaceAll(s, "\x00", "")
}

func parseLine(line string) (logEvent, bool) {
	m := reLine.FindStringSubmatch(line)
	if m == nil {
		return logEvent{}, false
	}
	at, err := time.Parse("2006-01-02 15:04:05 -0700", m[1]+" "+m[2])
	if err != nil {
		return logEvent{}, false
	}
	ev := logEvent{at: at.UTC(), eximID: m[3], kind: m[4]}
	rest := m[5]
	switch ev.kind {
	case "<=":
		tok, _, _ := strings.Cut(rest, " ")
		ev.sender = strings.ToLower(strings.Trim(tok, "<>"))
		if h := reHostIP.FindStringSubmatch(rest); h != nil {
			ev.host, ev.ip = h[1], h[2]
		} else {
			ev.host, ev.ip = "local", "local"
		}
		if s := reSize.FindStringSubmatch(rest); s != nil {
			ev.size, _ = strconv.Atoi(s[1])
		}
		if s := reSubject.FindStringSubmatch(rest); s != nil {
			ev.subject = decodeSubject(s[1])
		}
	case "=>", "->", "**", "==":
		tok, tail, _ := strings.Cut(rest, " ")
		ev.rcpt = strings.ToLower(strings.Trim(tok, "<>:"))
		if ev.rcpt == "" {
			return ev, false
		}
		if h := reRemote.FindStringSubmatch(tail); h != nil {
			ev.rhost, ev.rip = h[1], h[2]
		}
		if c := reCode.FindStringSubmatch(tail); c != nil {
			ev.detail = decodeSubject(c[1])
		} else if ev.kind == "**" || ev.kind == "==" {
			ev.detail = clean(tail)
		}
		if len(ev.detail) > 1000 {
			ev.detail = ev.detail[:1000]
		}
	}
	return ev, true
}

func domainOf(addr string) string {
	if i := strings.LastIndexByte(addr, '@'); i >= 0 {
		return addr[i+1:]
	}
	return ""
}

func execEvent(ctx context.Context, tx *sql.Tx, node int64, ev logEvent) error {
	run := func(q string, args ...any) error { _, err := tx.ExecContext(ctx, q, args...); return err }
	const stub = `INSERT IGNORE INTO messages(node_id,exim_id,received_at) VALUES(?,?,?)`
	switch ev.kind {
	case "<=":
		return run(`INSERT INTO messages(node_id,exim_id,received_at,sender,sender_domain,origin_host,origin_ip,subject,size)
			VALUES(?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE received_at=VALUES(received_at), sender=VALUES(sender),
			sender_domain=VALUES(sender_domain), origin_host=VALUES(origin_host), origin_ip=VALUES(origin_ip),
			subject=VALUES(subject), size=VALUES(size)`,
			node, ev.eximID, ev.at, trunc(clean(ev.sender), 320), trunc(clean(domainOf(ev.sender)), 255),
			trunc(clean(ev.host), 255), trunc(clean(ev.ip), 64), trunc(ev.subject, 500), ev.size)
	case "=>", "->", "**", "==":
		status := map[string]string{"=>": "delivered", "->": "delivered", "**": "failed", "==": "deferred"}[ev.kind]
		if err := run(stub, node, ev.eximID, ev.at); err != nil {
			return err
		}
		rcpt, rhost := trunc(clean(ev.rcpt), 320), trunc(clean(ev.rhost), 255)
		if err := run(`INSERT IGNORE INTO deliveries(message_id,at,recipient,status,remote_host,remote_ip,detail,dk)
			SELECT id,?,?,?,?,?,?,UNHEX(LEFT(SHA2(CONCAT_WS('|',id,?,?,?,?),256),32)) FROM messages WHERE node_id=? AND exim_id=?`,
			ev.at, rcpt, status, rhost, trunc(clean(ev.rip), 64), ev.detail,
			ev.at.Format("2006-01-02 15:04:05"), rcpt, status, rhost, node, ev.eximID); err != nil {
			return err
		}
		switch status {
		case "failed":
			return run(`UPDATE messages SET status='failed' WHERE node_id=? AND exim_id=?`, node, ev.eximID)
		case "deferred":
			return run(`UPDATE messages SET status='deferred' WHERE node_id=? AND exim_id=? AND status='queued'`, node, ev.eximID)
		}
	case "Completed":
		if err := run(stub, node, ev.eximID, ev.at); err != nil {
			return err
		}
		return run(`UPDATE messages SET completed_at=?, status=CASE WHEN EXISTS
			(SELECT 1 FROM deliveries d WHERE d.message_id=messages.id AND d.status='failed') THEN 'failed' ELSE 'delivered' END
			WHERE node_id=? AND exim_id=?`, ev.at, node, ev.eximID)
	}
	return nil
}

type sink func(ctx context.Context, evs []logEvent) error

func (a *App) localSink(ctx context.Context, evs []logEvent) error { return a.flush(ctx, 0, evs) }

func (a *App) flush(ctx context.Context, node int64, evs []logEvent) error {
	if len(evs) == 0 {
		return nil
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, ev := range evs {
		if err := execEvent(ctx, tx, node, ev); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *App) readFrom(ctx context.Context, path string, offset int64, emit sink) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return offset, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}
	rd := bufio.NewReaderSize(f, 256*1024)
	pos := offset
	var evs []logEvent
	pending := pos
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			break
		}
		pos += int64(len(line))
		if ev, ok := parseLine(strings.TrimRight(line, "\r\n")); ok {
			evs = append(evs, ev)
		}
		if len(evs) >= 1000 {
			if err := emit(ctx, evs); err != nil {
				return pending, err
			}
			evs, pending = evs[:0], pos
		}
	}
	if err := emit(ctx, evs); err != nil {
		return pending, err
	}
	return pos, nil
}

func inodeOf(fi os.FileInfo) int64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return int64(st.Ino)
	}
	return 0
}

func (a *App) ingestOnce(ctx context.Context, key string, emit sink) error {
	path := filepath.Join(a.logDir, "main.log")
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	var ino, off int64
	err = a.row(ctx, `SELECT inode,pos FROM ingest_state WHERE file=?`, key).Scan(&ino, &off)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	cur := inodeOf(fi)
	if ino != 0 && ino != cur {
		if rfi, err := os.Stat(path + ".1"); err == nil && inodeOf(rfi) == ino {
			if _, err := a.readFrom(ctx, path+".1", off, emit); err != nil {
				return err
			}
		}
		off = 0
	}
	if fi.Size() < off {
		off = 0
	}
	newOff, err := a.readFrom(ctx, path, off, emit)
	_, e2 := a.exec(ctx, `INSERT INTO ingest_state(file,inode,pos) VALUES(?,?,?)
		ON DUPLICATE KEY UPDATE inode=VALUES(inode), pos=VALUES(pos)`, key, cur, newOff)
	if err != nil {
		return err
	}
	return e2
}

func (a *App) runIngest(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !a.ready() {
				continue
			}
			if err := a.ingestOnce(ctx, "main", a.localSink); err != nil && ctx.Err() == nil {
				log.Printf("ingestão de logs: %v", err)
			}
		}
	}
}

var reEximID = regexp.MustCompile(`^[0-9A-Za-z]{6}-[0-9A-Za-z]{6,11}-[0-9A-Za-z]{2,4}$`)
