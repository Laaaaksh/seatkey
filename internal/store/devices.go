package store

import (
	"database/sql"
	"errors"
	"time"
)

// ActiveDeviceCount returns how many devices currently hold a seat on licenseID.
func (s *Store) ActiveDeviceCount(licenseID string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM devices WHERE license_id = ? AND deactivated_at IS NULL`,
		licenseID).Scan(&n)
	return n, err
}

// GetActiveDevice returns the device row for deviceID on this license if it
// is currently active (not deactivated). ErrNotFound otherwise.
func (s *Store) GetActiveDevice(licenseID, deviceID string) (Device, error) {
	var d Device
	err := s.db.QueryRow(`
		SELECT id, license_id, device_id, device_name, activated_at, last_seen_at, deactivated_at
		FROM devices WHERE license_id = ? AND device_id = ? AND deactivated_at IS NULL
	`, licenseID, deviceID).Scan(&d.ID, &d.LicenseID, &d.DeviceID, &d.DeviceName,
		&d.ActivatedAt, &d.LastSeenAt, &d.DeactivatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return d, err
}

// ActivateDevice inserts a new active device row. Callers must first check
// ActiveDeviceCount against the license's MaxDevices themselves, inside the
// same logical operation, to enforce the seat limit.
func (s *Store) ActivateDevice(licenseID, deviceID, deviceName string) (Device, error) {
	now := time.Now().UTC()
	d := Device{
		LicenseID:   licenseID,
		DeviceID:    deviceID,
		DeviceName:  deviceName,
		ActivatedAt: now,
		LastSeenAt:  now,
	}
	res, err := s.db.Exec(`
		INSERT INTO devices (license_id, device_id, device_name, activated_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)
	`, d.LicenseID, d.DeviceID, d.DeviceName, d.ActivatedAt, d.LastSeenAt)
	if err != nil {
		return Device{}, err
	}
	d.ID, err = res.LastInsertId()
	return d, err
}

// TouchDevice updates a device's last-seen timestamp.
func (s *Store) TouchDevice(id int64) error {
	_, err := s.db.Exec(`UPDATE devices SET last_seen_at = ? WHERE id = ?`, time.Now().UTC(), id)
	return err
}

// DeactivateDevice frees the seat held by deviceID on licenseID.
func (s *Store) DeactivateDevice(licenseID, deviceID string) error {
	res, err := s.db.Exec(`
		UPDATE devices SET deactivated_at = ?
		WHERE license_id = ? AND device_id = ? AND deactivated_at IS NULL
	`, time.Now().UTC(), licenseID, deviceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListDevices returns every device ever activated on licenseID, active or not.
func (s *Store) ListDevices(licenseID string) ([]Device, error) {
	rows, err := s.db.Query(`
		SELECT id, license_id, device_id, device_name, activated_at, last_seen_at, deactivated_at
		FROM devices WHERE license_id = ? ORDER BY activated_at DESC
	`, licenseID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var devices []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.LicenseID, &d.DeviceID, &d.DeviceName,
			&d.ActivatedAt, &d.LastSeenAt, &d.DeactivatedAt); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}
