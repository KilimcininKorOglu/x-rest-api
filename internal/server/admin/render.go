package admin

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"strings"
	"time"
)

//go:embed templates/layout.html templates/pages/*.html static/*
var assets embed.FS

var funcMap = template.FuncMap{
	"ts":          fmtTime,
	"tsp":         fmtTimePtr,
	"statusClass": statusClass,
	"did":         derefID,
	"deref":       derefInt,
}

func fmtTime(t time.Time) string { return t.Local().Format("2006-01-02 15:04:05") }

func fmtTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return fmtTime(*t)
}

func statusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	default:
		return "2xx"
	}
}

func derefID(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// render executes the layout with one page template.
func (h *Handler) render(w http.ResponseWriter, r *http.Request, page string, data map[string]any) {
	base := h.baseData(w, r, data)
	t, err := template.New("").Funcs(funcMap).ParseFS(assets, "templates/layout.html", "templates/pages/"+page+".html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", base); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = buf.WriteTo(w)
}

// renderPartial executes a single named template (for htmx swaps).
func (h *Handler) renderPartial(w http.ResponseWriter, page, define string, data map[string]any) {
	t, err := template.New("").Funcs(funcMap).ParseFS(assets, "templates/pages/"+page+".html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, define, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// baseData merges page data with layout defaults and a one-shot flash message.
func (h *Handler) baseData(w http.ResponseWriter, r *http.Request, data map[string]any) map[string]any {
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["Nav"]; !ok {
		data["Nav"] = true
	}
	if _, ok := data["Title"]; !ok {
		data["Title"] = "Admin"
	}
	if f := readFlash(w, r); f != nil {
		data["Flash"] = f
	}
	return data
}

// secureCookie reports whether admin cookies should carry the Secure flag: true
// when the request arrived over TLS directly or through a TLS-terminating proxy
// (X-Forwarded-Proto: https), so the panel still works on plain HTTP in dev.
func secureCookie(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// setFlash stores a one-shot message in a cookie (type is "ok" or "err").
func setFlash(w http.ResponseWriter, r *http.Request, typ, msg string) {
	// #nosec G124 -- Secure is set at runtime via secureCookie(r); HttpOnly and SameSite are always on.
	http.SetCookie(w, &http.Cookie{
		Name: "flash", Value: typ + "|" + msg, Path: "/admin",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureCookie(r),
	})
}

// readFlash reads and clears the flash cookie.
func readFlash(w http.ResponseWriter, r *http.Request) map[string]string {
	c, err := r.Cookie("flash")
	if err != nil || c.Value == "" {
		return nil
	}
	// #nosec G124 -- Secure is set at runtime via secureCookie(r); HttpOnly and SameSite are always on.
	http.SetCookie(w, &http.Cookie{Name: "flash", Value: "", Path: "/admin", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureCookie(r)})
	typ, msg, ok := splitFlash(c.Value)
	if !ok {
		return nil
	}
	return map[string]string{"Type": typ, "Msg": msg}
}

func splitFlash(v string) (typ, msg string, ok bool) {
	for i := 0; i < len(v); i++ {
		if v[i] == '|' {
			return v[:i], v[i+1:], true
		}
	}
	return "", "", false
}
