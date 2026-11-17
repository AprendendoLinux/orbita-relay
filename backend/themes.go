package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var themeColorKeys = []string{"bg", "bg2", "surface", "border", "text", "muted", "primary", "secondary", "accent", "success", "warning", "danger", "glow"}

var (
	reColor = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|rgba?\(\s*\d{1,3}\s*,\s*\d{1,3}\s*,\s*\d{1,3}\s*(,\s*(0|1|0?\.\d+)\s*)?\))$`)
	reSlug  = regexp.MustCompile(`^[a-z0-9-]{2,40}$`)
	fonts   = map[string]bool{"space-grotesk": true, "inter": true, "jetbrains-mono": true}
)

type Theme struct {
	ID      int64             `json:"id"`
	Slug    string            `json:"slug"`
	Name    string            `json:"name"`
	Builtin bool              `json:"builtin"`
	Tokens  map[string]string `json:"tokens"`
	Logo    string            `json:"logo"`
}

func validateTokens(t map[string]string) error {
	for _, k := range themeColorKeys {
		v, ok := t[k]
		if !ok || !reColor.MatchString(v) {
			return &httpErr{400, "cor inválida: " + k}
		}
	}
	if !fonts[t["font"]] {
		return &httpErr{400, "fonte inválida"}
	}
	for _, k := range []string{"radius", "blur"} {
		n, ok := atoiSafe(t[k])
		if !ok || n < 0 || n > 40 {
			return &httpErr{400, k + " inválido (0-40)"}
		}
	}
	for k := range t {
		known := k == "font" || k == "radius" || k == "blur"
		for _, ck := range themeColorKeys {
			if ck == k {
				known = true
			}
		}
		if !known {
			return &httpErr{400, "token desconhecido: " + k}
		}
	}
	return nil
}

type httpErr struct {
	code int
	msg  string
}

func (e *httpErr) Error() string { return e.msg }

func atoiSafe(s string) (int, bool) {
	n := 0
	if s == "" || len(s) > 3 {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

var builtinThemes = []Theme{
	{Slug: "clube-naval", Name: "Clube Naval", Builtin: true, Logo: "/static/clube-naval-logo.png", Tokens: map[string]string{
		"bg": "#070f1c", "bg2": "#12294a", "surface": "rgba(26,60,94,0.38)", "border": "rgba(255,255,255,0.14)",
		"text": "#f2f6fb", "muted": "#9db2cc", "primary": "#e0162f", "secondary": "#2f6db5", "accent": "#ffffff",
		"success": "#2ee6a6", "warning": "#ffc24d", "danger": "#ff4d6a", "glow": "rgba(224,22,47,0.55)",
		"radius": "16", "blur": "18", "font": "space-grotesk"}},
	{Slug: "orbita-neon", Name: "Orbita Neon", Builtin: true, Logo: "", Tokens: map[string]string{
		"bg": "#05060f", "bg2": "#150a33", "surface": "rgba(255,255,255,0.06)", "border": "rgba(120,240,255,0.22)",
		"text": "#eaf6ff", "muted": "#8aa0bd", "primary": "#22e5ff", "secondary": "#9b5cff", "accent": "#39ffb0",
		"success": "#39ffb0", "warning": "#ffd24d", "danger": "#ff4d8d", "glow": "rgba(34,229,255,0.5)",
		"radius": "18", "blur": "20", "font": "space-grotesk"}},
	{Slug: "aurora-verde", Name: "Aurora Verde", Builtin: true, Logo: "", Tokens: map[string]string{
		"bg": "#040d0b", "bg2": "#0a2a22", "surface": "rgba(255,255,255,0.055)", "border": "rgba(57,255,176,0.2)",
		"text": "#e9fff7", "muted": "#86a89d", "primary": "#39ffb0", "secondary": "#22e5ff", "accent": "#b6ff4d",
		"success": "#39ffb0", "warning": "#ffd24d", "danger": "#ff5c7a", "glow": "rgba(57,255,176,0.45)",
		"radius": "14", "blur": "16", "font": "jetbrains-mono"}},
}

func (a *App) seedThemes(ctx context.Context) error {
	for _, t := range builtinThemes {
		b, _ := json.Marshal(t.Tokens)
		_, err := a.db.Exec(ctx, `INSERT INTO themes(slug,name,builtin,tokens,logo) VALUES($1,$2,true,$3,$4)
			ON CONFLICT (slug) DO UPDATE SET name=EXCLUDED.name, tokens=EXCLUDED.tokens, logo=EXCLUDED.logo WHERE themes.builtin`,
			t.Slug, t.Name, b, t.Logo)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *App) activeThemeSlug() string {
	if v, ok := a.raw("active_theme"); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return "clube-naval"
}

func scanTheme(row interface{ Scan(...any) error }) (Theme, error) {
	var t Theme
	var raw []byte
	if err := row.Scan(&t.ID, &t.Slug, &t.Name, &t.Builtin, &raw, &t.Logo); err != nil {
		return t, err
	}
	_ = json.Unmarshal(raw, &t.Tokens)
	return t, nil
}

const themeCols = `id,slug,name,builtin,tokens,logo`

func (a *App) getTheme(ctx context.Context, slug string) (Theme, error) {
	return scanTheme(a.db.QueryRow(ctx, `SELECT `+themeCols+` FROM themes WHERE slug=$1`, slug))
}

func (a *App) handleBranding(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"version": version, "setup_required": !a.setupDone(), "db_ready": a.ready(), "app_name": "OrbitaRelay"}
	if a.ready() {
		out["app_name"] = a.sStr("app_name")
		out["timezone"] = a.sStr("timezone")
		t, err := a.getTheme(r.Context(), a.activeThemeSlug())
		if err != nil {
			t, _ = a.getTheme(r.Context(), "clube-naval")
		}
		out["theme"] = t
	} else {
		out["theme"] = builtinThemes[0]
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, out)
}

func (a *App) handleListThemes(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT `+themeCols+` FROM themes ORDER BY builtin DESC, name`)
	if err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	defer rows.Close()
	out := []Theme{}
	for rows.Next() {
		if t, err := scanTheme(rows); err == nil {
			out = append(out, t)
		}
	}
	writeJSON(w, 200, map[string]any{"themes": out, "active": a.activeThemeSlug()})
}

func (a *App) handleCreateTheme(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name   string            `json:"name"`
		Tokens map[string]string `json:"tokens"`
		From   string            `json:"from"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 40 || !reNoCtrl.MatchString(in.Name) {
		writeErr(w, 400, "nome inválido")
		return
	}
	logo := ""
	if in.Tokens == nil && in.From != "" {
		src, err := a.getTheme(r.Context(), in.From)
		if err != nil {
			writeErr(w, 404, "tema base não encontrado")
			return
		}
		in.Tokens, logo = src.Tokens, src.Logo
	}
	if err := validateTokens(in.Tokens); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	slug := strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(in.Name), "-"), "-") + "-" + randomToken(2)
	if !reSlug.MatchString(slug) {
		slug = "tema-" + randomToken(3)
	}
	b, _ := json.Marshal(in.Tokens)
	if _, err := a.db.Exec(r.Context(), `INSERT INTO themes(slug,name,tokens,logo) VALUES($1,$2,$3,$4)`, slug, in.Name, b, logo); err != nil {
		writeErr(w, 500, "falha ao criar tema")
		return
	}
	a.audit(r.Context(), userFrom(r).Username, "theme.create", slug)
	writeJSON(w, 201, map[string]any{"slug": slug})
}

func (a *App) handleUpdateTheme(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	t, err := a.getTheme(r.Context(), slug)
	if err != nil {
		writeErr(w, 404, "tema não encontrado")
		return
	}
	if t.Builtin {
		writeErr(w, 400, "temas padrão não podem ser editados; duplique-o")
		return
	}
	var in struct {
		Name   string            `json:"name"`
		Tokens map[string]string `json:"tokens"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 40 || !reNoCtrl.MatchString(in.Name) {
		writeErr(w, 400, "nome inválido")
		return
	}
	if err := validateTokens(in.Tokens); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	b, _ := json.Marshal(in.Tokens)
	_, _ = a.db.Exec(r.Context(), `UPDATE themes SET name=$2,tokens=$3 WHERE slug=$1`, slug, in.Name, b)
	a.audit(r.Context(), userFrom(r).Username, "theme.update", slug)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleDeleteTheme(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	t, err := a.getTheme(r.Context(), slug)
	if err != nil {
		writeErr(w, 404, "tema não encontrado")
		return
	}
	if t.Builtin || slug == a.activeThemeSlug() {
		writeErr(w, 400, "não é possível excluir tema padrão ou ativo")
		return
	}
	_, _ = a.db.Exec(r.Context(), `DELETE FROM themes WHERE slug=$1`, slug)
	a.removeLogoFile(t.Logo)
	a.audit(r.Context(), userFrom(r).Username, "theme.delete", slug)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleActivateTheme(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if _, err := a.getTheme(r.Context(), slug); err != nil {
		writeErr(w, 404, "tema não encontrado")
		return
	}
	_ = a.setRaw(r.Context(), "active_theme", slug)
	a.audit(r.Context(), userFrom(r).Username, "theme.activate", slug)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) uploadsDir() string { return filepath.Join(a.dataDir, "uploads") }

func (a *App) removeLogoFile(logo string) {
	if strings.HasPrefix(logo, "/uploads/") {
		_ = os.Remove(filepath.Join(a.uploadsDir(), filepath.Base(logo)))
	}
}

func (a *App) handleThemeLogo(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	t, err := a.getTheme(r.Context(), slug)
	if err != nil {
		writeErr(w, 404, "tema não encontrado")
		return
	}
	if t.Builtin {
		writeErr(w, 400, "duplique o tema padrão para trocar o logotipo")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	file, _, err := r.FormFile("logo")
	if err != nil {
		writeErr(w, 400, "arquivo inválido (máx. 2 MB)")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil || len(data) == 0 {
		writeErr(w, 400, "arquivo inválido")
		return
	}
	var ext string
	switch ct := http.DetectContentType(data); {
	case ct == "image/png":
		ext = ".png"
	case ct == "image/jpeg":
		ext = ".jpg"
	case ct == "image/webp":
		ext = ".webp"
	case strings.Contains(strings.ToLower(string(data[:min(len(data), 512)])), "<svg") || strings.HasPrefix(ct, "text/xml"):
		ext = ".svg"
	default:
		writeErr(w, 400, "formato não suportado (PNG, JPG, WEBP ou SVG)")
		return
	}
	if err := os.MkdirAll(a.uploadsDir(), 0o755); err != nil {
		writeErr(w, 500, "falha ao salvar")
		return
	}
	name := randomToken(8) + ext
	if err := os.WriteFile(filepath.Join(a.uploadsDir(), name), data, 0o644); err != nil {
		writeErr(w, 500, "falha ao salvar")
		return
	}
	a.removeLogoFile(t.Logo)
	_, _ = a.db.Exec(r.Context(), `UPDATE themes SET logo=$2 WHERE slug=$1`, slug, "/uploads/"+name)
	a.audit(r.Context(), userFrom(r).Username, "theme.logo", slug)
	writeJSON(w, 200, map[string]any{"logo": "/uploads/" + name})
}

func (a *App) handleDeleteThemeLogo(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	t, err := a.getTheme(r.Context(), slug)
	if err != nil || t.Builtin {
		writeErr(w, 400, "operação inválida")
		return
	}
	a.removeLogoFile(t.Logo)
	_, _ = a.db.Exec(r.Context(), `UPDATE themes SET logo='' WHERE slug=$1`, slug)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleUploads(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("name"))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, filepath.Join(a.uploadsDir(), name))
}
