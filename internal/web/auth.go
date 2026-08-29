package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "seatkey_session"
	sessionTTL        = 30 * 24 * time.Hour
)

func newSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

type ctxKey int

const authedCtxKey ctxKey = 0

// requireAuth redirects to /setup or /login when there is no valid session,
// and otherwise marks the request context as authenticated.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hasAdmin, err := s.store.HasAdmin()
		if err != nil {
			s.serverError(w, err)
			return
		}
		if !hasAdmin {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}

		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		valid, err := s.store.ValidSession(cookie.Value)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if !valid {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), authedCtxKey, true)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) startSession(w http.ResponseWriter) error {
	token, err := newSessionToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(sessionTTL)
	if err := s.store.CreateSession(token, expiresAt); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
	return nil
}

func (s *Server) endSession(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.store.DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

var errWeakPassword = errors.New("password must be at least 8 characters")

func validatePassword(password string) error {
	if len(password) < 8 {
		return errWeakPassword
	}
	return nil
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	hasAdmin, err := s.store.HasAdmin()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if hasAdmin {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet {
		s.render(w, "setup", map[string]any{})
		return
	}

	password := r.FormValue("password")
	confirm := r.FormValue("confirm")
	if password != confirm {
		s.render(w, "setup", map[string]any{"Error": "Passwords do not match"})
		return
	}
	if err := validatePassword(password); err != nil {
		s.render(w, "setup", map[string]any{"Error": err.Error()})
		return
	}

	hash, err := hashPassword(password)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.store.CreateAdmin(hash); err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.startSession(w); err != nil {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	hasAdmin, err := s.store.HasAdmin()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !hasAdmin {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet {
		s.render(w, "login", map[string]any{})
		return
	}

	hash, err := s.store.AdminPasswordHash()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !checkPassword(hash, r.FormValue("password")) {
		s.render(w, "login", map[string]any{"Error": "Incorrect password"})
		return
	}
	if err := s.startSession(w); err != nil {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.endSession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	hash, err := s.store.AdminPasswordHash()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !checkPassword(hash, r.FormValue("current_password")) {
		s.renderSettings(w, r, "Current password is incorrect")
		return
	}
	newPassword := r.FormValue("new_password")
	if newPassword != r.FormValue("confirm_password") {
		s.renderSettings(w, r, "New passwords do not match")
		return
	}
	if err := validatePassword(newPassword); err != nil {
		s.renderSettings(w, r, err.Error())
		return
	}
	newHash, err := hashPassword(newPassword)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.store.SetAdminPassword(newHash); err != nil {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}
