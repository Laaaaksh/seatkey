// Package license implements Seatkey's core licensing rules: activation,
// seat-limit enforcement, validation, and offline activation. It sits between
// the HTTP layer (internal/web) and persistence (internal/store), and is
// where the signed-envelope crypto (internal/crypto) gets applied to real
// license data.
package license

import "time"

// Token is the payload signed inside every Envelope Seatkey issues, whether
// for an online activate/validate call or an offline activation file. A
// client caches the whole Envelope and re-verifies Token.ValidUntil locally
// without contacting the server again until that deadline.
type Token struct {
	LicenseKey   string     `json:"license_key"`
	Product      string     `json:"product"`
	CustomerName string     `json:"customer_name"`
	DeviceID     string     `json:"device_id"`
	SeatsMax     int        `json:"seats_max"`
	SeatsUsed    int        `json:"seats_used"`
	Status       string     `json:"status"`
	IssuedAt     time.Time  `json:"issued_at"`
	ValidUntil   time.Time  `json:"valid_until"`
	LicenseExp   *time.Time `json:"license_expires_at,omitempty"`
	Offline      bool       `json:"offline"`
}

// OnlineGracePeriod is how long a client may keep treating a cached, signed
// online activate/validate response as valid without contacting the server
// again. The client is expected to re-validate before this deadline.
const OnlineGracePeriod = 7 * 24 * time.Hour
