// Package eximlog lê o main.log do Exim e o transforma em eventos, com cursor persistente.
package eximlog

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"os"
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

// Event é uma linha relevante do log do Exim.
type Event struct {
	At      time.Time
	ID      string
	Kind    string
	Sender  string
	Host    string
	IP      string
	Subject string
	Size    int
	Rcpt    string
	RHost   string
	RIP     string
	Detail  string
}

// ReID valida o identificador de mensagem do Exim.
var ReID = regexp.MustCompile(`^[0-9A-Za-z]{6}-[0-9A-Za-z]{6,11}-[0-9A-Za-z]{2,4}$`)

var wordDecoder = new(mime.WordDecoder)

// DecodeSubject decodifica e limita o assunto.
func DecodeSubject(s string) string {
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

// Clean remove bytes inválidos.
func Clean(s string) string {
	s = strings.ToValidUTF8(s, "")
	return strings.ReplaceAll(s, "\x00", "")
}

// ParseLine interpreta uma linha do main.log; ok é falso para linhas irrelevantes.
func ParseLine(line string) (Event, bool) {
	m := reLine.FindStringSubmatch(line)
	if m == nil {
		return Event{}, false
	}
	at, err := time.Parse("2006-01-02 15:04:05 -0700", m[1]+" "+m[2])
	if err != nil {
		return Event{}, false
	}
	ev := Event{At: at.UTC(), ID: m[3], Kind: m[4]}
	rest := m[5]
	switch ev.Kind {
	case "<=":
		tok, _, _ := strings.Cut(rest, " ")
		ev.Sender = strings.ToLower(strings.Trim(tok, "<>"))
		if h := reHostIP.FindStringSubmatch(rest); h != nil {
			ev.Host, ev.IP = h[1], h[2]
		} else {
			ev.Host, ev.IP = "local", "local"
		}
		if s := reSize.FindStringSubmatch(rest); s != nil {
			ev.Size, _ = strconv.Atoi(s[1])
		}
		if s := reSubject.FindStringSubmatch(rest); s != nil {
			ev.Subject = DecodeSubject(s[1])
		}
	case "=>", "->", "**", "==":
		tok, tail, _ := strings.Cut(rest, " ")
		ev.Rcpt = strings.ToLower(strings.Trim(tok, "<>:"))
		if ev.Rcpt == "" {
			return ev, false
		}
		if h := reRemote.FindStringSubmatch(tail); h != nil {
			ev.RHost, ev.RIP = h[1], h[2]
		}
		if c := reCode.FindStringSubmatch(tail); c != nil {
			ev.Detail = DecodeSubject(c[1])
		} else if ev.Kind == "**" || ev.Kind == "==" {
			ev.Detail = Clean(tail)
		}
		if len(ev.Detail) > 1000 {
			ev.Detail = ev.Detail[:1000]
		}
	}
	return ev, true
}

// DomainOf devolve o domínio de um endereço.
func DomainOf(addr string) string {
	if i := strings.LastIndexByte(addr, '@'); i >= 0 {
		return addr[i+1:]
	}
	return ""
}

// ReadFrom lê o arquivo a partir de offset e entrega os eventos em lotes; devolve a posição segura para salvar.
func ReadFrom(ctx context.Context, path string, offset int64, emit Sink) (int64, error) {
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
	var evs []Event
	pending := pos
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			break
		}
		pos += int64(len(line))
		if ev, ok := ParseLine(strings.TrimRight(line, "\r\n")); ok {
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

// InodeOf devolve o inode do arquivo.
func InodeOf(fi os.FileInfo) int64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return int64(st.Ino)
	}
	return 0
}

// Sink recebe lotes de eventos.
type Sink func(ctx context.Context, evs []Event) error

// State é a posição de leitura: inode e deslocamento.
type State struct{ Inode, Pos int64 }

// Store guarda o estado entre execuções.
type Store interface {
	Load(ctx context.Context) (State, error)
	Save(ctx context.Context, s State) error
}

// Once lê o que foi acrescentado ao log desde a última chamada, tratando rotação e truncamento.
func Once(ctx context.Context, path string, st Store, emit Sink) error {
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	cur, err := st.Load(ctx)
	if err != nil {
		return err
	}
	ino, off := cur.Inode, cur.Pos
	now := InodeOf(fi)
	if ino != 0 && ino != now {
		if rfi, err := os.Stat(path + ".1"); err == nil && InodeOf(rfi) == ino {
			if _, err := ReadFrom(ctx, path+".1", off, emit); err != nil {
				return err
			}
		}
		off = 0
	}
	if fi.Size() < off {
		off = 0
	}
	newOff, err := ReadFrom(ctx, path, off, emit)
	e2 := st.Save(ctx, State{Inode: now, Pos: newOff})
	if err != nil {
		return err
	}
	return e2
}

// FileStore guarda o estado em um arquivo JSON.
type FileStore struct{ Path string }

// Load lê o estado; arquivo ausente ou inválido significa começar do zero.
func (f FileStore) Load(context.Context) (State, error) {
	var s State
	b, err := os.ReadFile(f.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return s, err
	}
	var raw struct {
		Inode int64 `json:"inode"`
		Pos   int64 `json:"pos"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return s, nil
	}
	return State{Inode: raw.Inode, Pos: raw.Pos}, nil
}

// Save grava o estado de forma atômica.
func (f FileStore) Save(_ context.Context, s State) error {
	b, _ := json.Marshal(map[string]int64{"inode": s.Inode, "pos": s.Pos})
	tmp := f.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.Path)
}
