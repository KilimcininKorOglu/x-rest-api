// Package admin is the htmx dark-mode admin panel served at /admin. It manages
// every runtime setting: X.com accounts (cookies), API keys, request logs, and
// global settings. Auth is a first-run setup wizard plus session cookies.
package admin

import (
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"x-rest-api/internal/store"
)

const (
	sessionCookie = "admin_session"
	sessionTTL    = 12 * time.Hour
)

// Handler is the admin panel.
type Handler struct {
	st      *store.Store
	refresh func() (int, error) // refresh queryIds from the x.com bundle; returns count found
}

// New builds the admin handler. refresh triggers a live queryId refresh.
func New(st *store.Store, refresh func() (int, error)) *Handler {
	return &Handler{st: st, refresh: refresh}
}

// Router returns the /admin subtree (mounted under /admin by the parent).
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()

	sub, _ := fs.Sub(assets, "static")
	r.Handle("/static/*", http.StripPrefix("/admin/static/", http.FileServer(http.FS(sub))))

	r.Get("/setup", h.setupForm)
	r.Post("/setup", h.setupSubmit)
	r.Get("/login", h.loginForm)
	r.Post("/login", h.loginSubmit)
	r.Get("/logout", h.logout)

	r.Group(func(pr chi.Router) {
		pr.Use(h.requireAuth)
		pr.Get("/", h.dashboard)
		pr.Get("/accounts", h.accountsPage)
		pr.Post("/accounts", h.accountCreate)
		pr.Post("/accounts/{id}/toggle", h.accountToggle)
		pr.Post("/accounts/{id}/delete", h.accountDelete)
		pr.Get("/keys", h.keysPage)
		pr.Post("/keys", h.keyCreate)
		pr.Post("/keys/{id}/toggle", h.keyToggle)
		pr.Post("/keys/{id}/delete", h.keyDelete)
		pr.Get("/logs", h.logsPage)
		pr.Get("/logs/table", h.logsTable)
		pr.Get("/query-ids", h.queryIDsPage)
		pr.Post("/query-ids/refresh", h.queryIDsRefresh)
		pr.Get("/settings", h.settingsPage)
		pr.Post("/settings", h.settingsSave)
	})
	return r
}

// requireAuth gates the panel: no admin -> setup; not signed in -> login.
func (h *Handler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := h.st.CountAdmins()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if n == 0 {
			http.Redirect(w, r, "/admin/setup", http.StatusFound)
			return
		}
		if !h.signedIn(r) {
			http.Redirect(w, r, "/admin/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) signedIn(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	_, ok := h.st.SessionAdminID(c.Value)
	return ok
}

// ---- setup ------------------------------------------------------------------ //

func (h *Handler) setupForm(w http.ResponseWriter, r *http.Request) {
	if n, _ := h.st.CountAdmins(); n > 0 {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}
	h.render(w, r, "setup", map[string]any{"Title": "Setup", "Nav": false})
}

func (h *Handler) setupSubmit(w http.ResponseWriter, r *http.Request) {
	if n, _ := h.st.CountAdmins(); n > 0 {
		http.Error(w, "admin already exists", http.StatusForbidden)
		return
	}
	user := r.FormValue("username")
	pass := r.FormValue("password")
	if user == "" || pass == "" || pass != r.FormValue("password2") {
		setFlash(w, r, "err", "check the username and matching passwords")
		http.Redirect(w, r, "/admin/setup", http.StatusFound)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id, err := h.st.CreateAdmin(user, string(hash))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.startSession(w, r, id)
	http.Redirect(w, r, "/admin", http.StatusFound)
}

// ---- login / logout --------------------------------------------------------- //

func (h *Handler) loginForm(w http.ResponseWriter, r *http.Request) {
	if n, _ := h.st.CountAdmins(); n == 0 {
		http.Redirect(w, r, "/admin/setup", http.StatusFound)
		return
	}
	h.render(w, r, "login", map[string]any{"Title": "Sign in", "Nav": false})
}

func (h *Handler) loginSubmit(w http.ResponseWriter, r *http.Request) {
	u, err := h.st.GetAdminByUsername(r.FormValue("username"))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(r.FormValue("password"))) != nil {
		setFlash(w, r, "err", "invalid credentials")
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}
	h.startSession(w, r, u.ID)
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = h.st.DeleteSession(c.Value)
	}
	// #nosec G124 -- Secure is set at runtime via secureCookie(r); HttpOnly and SameSite are always on.
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureCookie(r)})
	http.Redirect(w, r, "/admin/login", http.StatusFound)
}

// startSession creates a server-side session and sets the cookie.
func (h *Handler) startSession(w http.ResponseWriter, r *http.Request, adminID int64) {
	id := newToken()
	if err := h.st.CreateSession(id, adminID, time.Now().Add(sessionTTL)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// #nosec G124 -- Secure is set at runtime via secureCookie(r); HttpOnly and SameSite are always on.
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: id, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(sessionTTL), Secure: secureCookie(r),
	})
}

// newToken returns a random 32-byte hex token (session id / API key).
func newToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
