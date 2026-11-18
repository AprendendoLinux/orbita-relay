package main

import (
	"context"
	"database/sql"
	"log"
	"mime"
	"path/filepath"
	"time"

	"orbitarelay/internal/eximlog"
)

type logEvent = eximlog.Event

func execEvent(ctx context.Context, tx *sql.Tx, node int64, ev logEvent) error {
	run := func(q string, args ...any) error { _, err := tx.ExecContext(ctx, q, args...); return err }
	const stub = `INSERT IGNORE INTO messages(node_id,exim_id,received_at) VALUES(?,?,?)`
	switch ev.Kind {
	case "<=":
		return run(`INSERT INTO messages(node_id,exim_id,received_at,sender,sender_domain,origin_host,origin_ip,subject,size)
			VALUES(?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE received_at=VALUES(received_at), sender=VALUES(sender),
			sender_domain=VALUES(sender_domain), origin_host=VALUES(origin_host), origin_ip=VALUES(origin_ip),
			subject=VALUES(subject), size=VALUES(size)`,
			node, ev.ID, ev.At, trunc(clean(ev.Sender), 320), trunc(clean(domainOf(ev.Sender)), 255),
			trunc(clean(ev.Host), 255), trunc(clean(ev.IP), 64), trunc(ev.Subject, 500), ev.Size)
	case "=>", "->", "**", "==":
		status := map[string]string{"=>": "delivered", "->": "delivered", "**": "failed", "==": "deferred"}[ev.Kind]
		if err := run(stub, node, ev.ID, ev.At); err != nil {
			return err
		}
		rcpt, rhost := trunc(clean(ev.Rcpt), 320), trunc(clean(ev.RHost), 255)
		if err := run(`INSERT IGNORE INTO deliveries(message_id,at,recipient,status,remote_host,remote_ip,detail,dk)
			SELECT id,?,?,?,?,?,?,UNHEX(LEFT(SHA2(CONCAT_WS('|',id,?,?,?,?),256),32)) FROM messages WHERE node_id=? AND exim_id=?`,
			ev.At, rcpt, status, rhost, trunc(clean(ev.RIP), 64), ev.Detail,
			ev.At.Format("2006-01-02 15:04:05"), rcpt, status, rhost, node, ev.ID); err != nil {
			return err
		}
		switch status {
		case "failed":
			return run(`UPDATE messages SET status='failed' WHERE node_id=? AND exim_id=?`, node, ev.ID)
		case "deferred":
			return run(`UPDATE messages SET status='deferred' WHERE node_id=? AND exim_id=? AND status='queued'`, node, ev.ID)
		}
	case "Completed":
		if err := run(stub, node, ev.ID, ev.At); err != nil {
			return err
		}
		return run(`UPDATE messages SET completed_at=?, status=CASE WHEN EXISTS
			(SELECT 1 FROM deliveries d WHERE d.message_id=messages.id AND d.status='failed') THEN 'failed' ELSE 'delivered' END
			WHERE node_id=? AND exim_id=?`, ev.At, node, ev.ID)
	}
	return nil
}

type sink = eximlog.Sink

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

type dbStore struct {
	a   *App
	key string
}

func (d dbStore) Load(ctx context.Context) (eximlog.State, error) {
	var st eximlog.State
	err := d.a.row(ctx, `SELECT inode,pos FROM ingest_state WHERE file=?`, d.key).Scan(&st.Inode, &st.Pos)
	if err != nil && err != sql.ErrNoRows {
		return st, err
	}
	return st, nil
}

func (d dbStore) Save(ctx context.Context, st eximlog.State) error {
	_, err := d.a.exec(ctx, `INSERT INTO ingest_state(file,inode,pos) VALUES(?,?,?)
		ON DUPLICATE KEY UPDATE inode=VALUES(inode), pos=VALUES(pos)`, d.key, st.Inode, st.Pos)
	return err
}

func (a *App) ingestOnce(ctx context.Context, key string, emit sink) error {
	return eximlog.Once(ctx, filepath.Join(a.logDir, "main.log"), dbStore{a, key}, emit)
}

func (a *App) runIngest(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	var lastErr string
	var lastLog time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !a.ready() {
				continue
			}
			err := a.ingestOnce(ctx, "main", a.localSink)
			switch {
			case err == nil:
				lastErr = ""
			case ctx.Err() == nil && (err.Error() != lastErr || time.Since(lastLog) > 10*time.Minute):
				log.Printf("ingestão de logs: %v", err)
				lastErr, lastLog = err.Error(), time.Now()
			}
		}
	}
}

var wordDecoder = new(mime.WordDecoder)

func clean(s string) string       { return eximlog.Clean(s) }
func domainOf(addr string) string { return eximlog.DomainOf(addr) }
