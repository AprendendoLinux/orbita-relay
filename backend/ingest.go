package main

import (
	"bufio"
	"context"
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

	"github.com/jackc/pgx/v5"
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

const upsertStub = `INSERT INTO messages(exim_id,received_at) VALUES($1,$2) ON CONFLICT (exim_id) DO NOTHING`

func queueEvent(b *pgx.Batch, ev logEvent) {
	switch ev.kind {
	case "<=":
		b.Queue(`INSERT INTO messages(exim_id,received_at,sender,sender_domain,origin_host,origin_ip,subject,size)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (exim_id) DO UPDATE SET received_at=EXCLUDED.received_at, sender=EXCLUDED.sender,
			sender_domain=EXCLUDED.sender_domain, origin_host=EXCLUDED.origin_host, origin_ip=EXCLUDED.origin_ip,
			subject=EXCLUDED.subject, size=EXCLUDED.size`,
			ev.eximID, ev.at, clean(ev.sender), clean(domainOf(ev.sender)), clean(ev.host), clean(ev.ip), ev.subject, ev.size)
	case "=>", "->", "**", "==":
		status := map[string]string{"=>": "delivered", "->": "delivered", "**": "failed", "==": "deferred"}[ev.kind]
		b.Queue(upsertStub, ev.eximID, ev.at)
		b.Queue(`INSERT INTO deliveries(message_id,at,recipient,status,remote_host,remote_ip,detail)
			SELECT id,$2,$3,$4,$5,$6,$7 FROM messages WHERE exim_id=$1`,
			ev.eximID, ev.at, clean(ev.rcpt), status, clean(ev.rhost), clean(ev.rip), ev.detail)
		switch status {
		case "failed":
			b.Queue(`UPDATE messages SET status='failed' WHERE exim_id=$1`, ev.eximID)
		case "deferred":
			b.Queue(`UPDATE messages SET status='deferred' WHERE exim_id=$1 AND status='queued'`, ev.eximID)
		}
	case "Completed":
		b.Queue(upsertStub, ev.eximID, ev.at)
		b.Queue(`UPDATE messages SET completed_at=$2, status=CASE WHEN EXISTS
			(SELECT 1 FROM deliveries d WHERE d.message_id=messages.id AND d.status='failed') THEN 'failed' ELSE 'delivered' END
			WHERE exim_id=$1`, ev.eximID, ev.at)
	}
}

func (a *App) flush(ctx context.Context, evs []logEvent) error {
	if len(evs) == 0 {
		return nil
	}
	b := &pgx.Batch{}
	for _, ev := range evs {
		queueEvent(b, ev)
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	res := tx.SendBatch(ctx, b)
	for i := 0; i < b.Len(); i++ {
		if _, err := res.Exec(); err != nil {
			res.Close()
			return err
		}
	}
	if err := res.Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *App) readFrom(ctx context.Context, path string, offset int64) (int64, error) {
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
			if err := a.flush(ctx, evs); err != nil {
				return pending, err
			}
			evs, pending = evs[:0], pos
		}
	}
	if err := a.flush(ctx, evs); err != nil {
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

func (a *App) ingestOnce(ctx context.Context) error {
	path := filepath.Join(a.logDir, "main.log")
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	var ino, off int64
	err = a.db.QueryRow(ctx, `SELECT inode,off FROM ingest_state WHERE file='main'`).Scan(&ino, &off)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	cur := inodeOf(fi)
	if ino != 0 && ino != cur {
		if rfi, err := os.Stat(path + ".1"); err == nil && inodeOf(rfi) == ino {
			if _, err := a.readFrom(ctx, path+".1", off); err != nil {
				return err
			}
		}
		off = 0
	}
	if fi.Size() < off {
		off = 0
	}
	newOff, err := a.readFrom(ctx, path, off)
	_, e2 := a.db.Exec(ctx, `INSERT INTO ingest_state(file,inode,off) VALUES('main',$1,$2)
		ON CONFLICT (file) DO UPDATE SET inode=EXCLUDED.inode, off=EXCLUDED.off`, cur, newOff)
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
			if err := a.ingestOnce(ctx); err != nil && ctx.Err() == nil {
				log.Printf("ingestão de logs: %v", err)
			}
		}
	}
}
