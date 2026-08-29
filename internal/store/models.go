package store

import "time"

// Product groups the licenses issued for one piece of software.
type Product struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// License is one issued key: who it belongs to, how many devices it may be
// activated on, and whether it has an expiry or has been revoked.
type License struct {
	ID            string
	ProductID     string
	ProductName   string
	Key           string
	CustomerName  string
	CustomerEmail string
	MaxDevices    int
	ExpiresAt     *time.Time
	RevokedAt     *time.Time
	CreatedAt     time.Time
}

// Status computes the license's current state; it is derived, not stored, so
// changing the clock or the license's expiry is reflected immediately.
func (l License) Status(now time.Time) string {
	if l.RevokedAt != nil {
		return "revoked"
	}
	if l.ExpiresAt != nil && now.After(*l.ExpiresAt) {
		return "expired"
	}
	return "active"
}

// Device is one activation of a License, identified by a client-supplied
// device ID (a machine fingerprint or similar stable identifier).
type Device struct {
	ID            int64
	LicenseID     string
	DeviceID      string
	DeviceName    string
	ActivatedAt   time.Time
	LastSeenAt    time.Time
	DeactivatedAt *time.Time
}

// Active reports whether this device currently holds a seat.
func (d Device) Active() bool {
	return d.DeactivatedAt == nil
}
