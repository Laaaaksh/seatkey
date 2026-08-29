// Package web provides Seatkey's HTTP surface: the public license-check API
// under /v1 and the session-authenticated admin dashboard everything else.
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/Laaaaksh/seatkey/internal/license"
	"github.com/Laaaaksh/seatkey/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server is Seatkey's HTTP handler: the public /v1 API plus the
// session-authenticated admin dashboard.
type Server struct {
	store        *store.Store
	licenses     *license.Service
	mux          *http.ServeMux
	tmpl         *template.Template
	cookieSecure bool
}

// Config holds NewServer's runtime options.
type Config struct {
	// CookieSecure marks the session cookie Secure, so browsers only send it
	// over HTTPS. Leave false for plain-HTTP self-hosting (e.g. behind a
	// reverse proxy on a private network); set true when serving TLS directly.
	CookieSecure bool
}

// NewServer builds a Server backed by s and licenses, applying cfg.
func NewServer(s *store.Store, licenses *license.Service, cfg Config) (*Server, error) {
	tmpl, err := template.New("").Funcs(templateFuncs).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	srv := &Server{
		store:        s,
		licenses:     licenses,
		mux:          http.NewServeMux(),
		tmpl:         tmpl,
		cookieSecure: cfg.CookieSecure,
	}
	srv.routes()
	return srv, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	// Public license-check API.
	s.mux.HandleFunc("POST /v1/activate", s.handleAPIActivate)
	s.mux.HandleFunc("POST /v1/validate", s.handleAPIValidate)
	s.mux.HandleFunc("POST /v1/deactivate", s.handleAPIDeactivate)
	s.mux.HandleFunc("GET /v1/pubkey", s.handleAPIPubkey)
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)

	// Auth.
	s.mux.HandleFunc("GET /setup", s.handleSetup)
	s.mux.HandleFunc("POST /setup", s.handleSetup)
	s.mux.HandleFunc("GET /login", s.handleLogin)
	s.mux.HandleFunc("POST /login", s.handleLogin)
	s.mux.HandleFunc("POST /logout", s.handleLogout)

	// Dashboard (session auth required).
	s.mux.HandleFunc("GET /{$}", s.requireAuth(s.handleHome))
	s.mux.HandleFunc("GET /products", s.requireAuth(s.handleProductsList))
	s.mux.HandleFunc("POST /products", s.requireAuth(s.handleProductCreate))
	s.mux.HandleFunc("GET /products/{id}", s.requireAuth(s.handleProductDetail))
	s.mux.HandleFunc("POST /products/{id}/licenses", s.requireAuth(s.handleLicenseCreate))
	s.mux.HandleFunc("GET /licenses/{id}", s.requireAuth(s.handleLicenseDetail))
	s.mux.HandleFunc("POST /licenses/{id}/revoke", s.requireAuth(s.handleLicenseRevoke))
	s.mux.HandleFunc("POST /licenses/{id}/offline-activate", s.requireAuth(s.handleLicenseOfflineActivate))
	s.mux.HandleFunc("POST /licenses/{id}/devices/{deviceID}/deactivate", s.requireAuth(s.handleDeviceDeactivate))
	s.mux.HandleFunc("GET /settings", s.requireAuth(s.handleSettings))
	s.mux.HandleFunc("POST /settings/webhook", s.requireAuth(s.handleWebhookSettings))
	s.mux.HandleFunc("POST /settings/password", s.requireAuth(s.handleChangePassword))

	s.mux.Handle("GET /static/", http.FileServerFS(staticFS))
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/products", http.StatusSeeOther)
}

// pageMeta maps each content template's name to (Title, Authed) so callers
// only need to name the page; render wraps it in the shared layout.
var pageMeta = map[string]struct {
	title  string
	authed bool
}{
	"setup":    {"Setup", false},
	"login":    {"Log in", false},
	"products": {"Products", true},
	"product":  {"Product", true},
	"license":  {"License", true},
	"settings": {"Settings", true},
}

// render executes the named content template into a buffer, then wraps it in
// the shared layout. Content templates and layout.html are parsed together
// (see NewServer) but kept in separate files with distinct {{define}} names
// so they don't collide in html/template's single flat namespace.
func (s *Server) render(w http.ResponseWriter, name string, data any) {
	meta, ok := pageMeta[name]
	if !ok {
		s.serverError(w, fmt.Errorf("render: unknown page %q", name))
		return
	}

	var body bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&body, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := s.tmpl.ExecuteTemplate(w, "layout", map[string]any{
		"Title":  meta.title,
		"Authed": meta.authed,
		"Body":   template.HTML(body.String()), //nolint:gosec // body is our own rendered template output, not raw user input
	})
	if err != nil {
		log.Printf("render layout for %s: %v", name, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	log.Printf("server error: %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
