package store

import (
	"database/sql"
	"time"
)

// HasAdmin reports whether an admin account has ever been created.
func (s *Store) HasAdmin() (bool, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM admins`).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// CreateAdmin stores the single admin account's password hash.
func (s *Store) CreateAdmin(passwordHash string) error {
	_, err := s.db.Exec(`INSERT INTO admins (password_hash, created_at) VALUES (?, ?)`,
		passwordHash, time.Now().UTC())
	return err
}

// AdminPasswordHash returns the single admin account's bcrypt hash.
func (s *Store) AdminPasswordHash() (string, error) {
	var hash string
	err := s.db.QueryRow(`SELECT password_hash FROM admins ORDER BY id LIMIT 1`).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return hash, err
}

// SetAdminPassword updates the admin account's password hash.
func (s *Store) SetAdminPassword(passwordHash string) error {
	_, err := s.db.Exec(`UPDATE admins SET password_hash = ? WHERE id = (SELECT id FROM admins ORDER BY id LIMIT 1)`,
		passwordHash)
	return err
}

// CreateSession persists a new session token with its expiry.
func (s *Store) CreateSession(token string, expiresAt time.Time) error {
	_, err := s.db.Exec(`INSERT INTO sessions (token, created_at, expires_at) VALUES (?, ?, ?)`,
		token, time.Now().UTC(), expiresAt)
	return err
}

// ValidSession reports whether token exists and has not expired.
func (s *Store) ValidSession(token string) (bool, error) {
	var expiresAt time.Time
	err := s.db.QueryRow(`SELECT expires_at FROM sessions WHERE token = ?`, token).Scan(&expiresAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return time.Now().UTC().Before(expiresAt), nil
}

// DeleteSession removes a session token, e.g. on logout.
func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}
