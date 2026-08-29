package store

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// crockford omits I, L, O, U to avoid transcription mistakes when a customer
// reads a key aloud or copies it by hand.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// GenerateLicenseKey returns a key shaped SEAT-XXXX-XXXX-XXXX-XXXX.
func GenerateLicenseKey() (string, error) {
	const groups, groupLen = 4, 4
	buf := make([]byte, groups*groupLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("SEAT")
	for g := 0; g < groups; g++ {
		b.WriteByte('-')
		for i := 0; i < groupLen; i++ {
			b.WriteByte(crockford[int(buf[g*groupLen+i])%len(crockford)])
		}
	}
	return b.String(), nil
}

// CreateLicenseParams are the fields an admin chooses when issuing a new license.
type CreateLicenseParams struct {
	ProductID     string
	CustomerName  string
	CustomerEmail string
	MaxDevices    int
	ExpiresAt     *time.Time
}

// CreateLicense generates a fresh key and issues a new license for p.ProductID.
func (s *Store) CreateLicense(p CreateLicenseParams) (License, error) {
	key, err := GenerateLicenseKey()
	if err != nil {
		return License{}, fmt.Errorf("generate key: %w", err)
	}

	l := License{
		ID:            uuid.NewString(),
		ProductID:     p.ProductID,
		Key:           key,
		CustomerName:  p.CustomerName,
		CustomerEmail: p.CustomerEmail,
		MaxDevices:    p.MaxDevices,
		ExpiresAt:     p.ExpiresAt,
		CreatedAt:     time.Now().UTC(),
	}
	_, err = s.db.Exec(`
		INSERT INTO licenses (id, product_id, key, customer_name, customer_email, max_devices, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, l.ID, l.ProductID, l.Key, l.CustomerName, l.CustomerEmail, l.MaxDevices, l.ExpiresAt, l.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: licenses.key") {
			return License{}, ErrKeyExists
		}
		return License{}, err
	}
	return l, nil
}

const licenseSelect = `
	SELECT l.id, l.product_id, p.name, l.key, l.customer_name, l.customer_email,
	       l.max_devices, l.expires_at, l.revoked_at, l.created_at
	FROM licenses l JOIN products p ON p.id = l.product_id
`

func scanLicense(row interface{ Scan(...any) error }) (License, error) {
	var l License
	err := row.Scan(&l.ID, &l.ProductID, &l.ProductName, &l.Key, &l.CustomerName, &l.CustomerEmail,
		&l.MaxDevices, &l.ExpiresAt, &l.RevokedAt, &l.CreatedAt)
	return l, err
}

// GetLicense looks up a license by its internal ID.
func (s *Store) GetLicense(id string) (License, error) {
	l, err := scanLicense(s.db.QueryRow(licenseSelect+` WHERE l.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return License{}, ErrNotFound
	}
	return l, err
}

// GetLicenseByKey looks up a license by the key a client presents.
func (s *Store) GetLicenseByKey(key string) (License, error) {
	l, err := scanLicense(s.db.QueryRow(licenseSelect+` WHERE l.key = ?`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return License{}, ErrNotFound
	}
	return l, err
}

// ListLicenses returns every license for productID, or every license across
// all products when productID is empty.
func (s *Store) ListLicenses(productID string) ([]License, error) {
	query := licenseSelect
	args := []any{}
	if productID != "" {
		query += ` WHERE l.product_id = ?`
		args = append(args, productID)
	}
	query += ` ORDER BY l.created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var licenses []License
	for rows.Next() {
		l, err := scanLicense(rows)
		if err != nil {
			return nil, err
		}
		licenses = append(licenses, l)
	}
	return licenses, rows.Err()
}

// RevokeLicense marks a license revoked; it stays revoked once set.
func (s *Store) RevokeLicense(id string) error {
	res, err := s.db.Exec(`UPDATE licenses SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
