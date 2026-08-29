package store

const schema = `
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS admins (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	password_hash TEXT NOT NULL,
	created_at    DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	token      TEXT PRIMARY KEY,
	created_at DATETIME NOT NULL,
	expires_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS products (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS licenses (
	id             TEXT PRIMARY KEY,
	product_id     TEXT NOT NULL REFERENCES products(id),
	key            TEXT NOT NULL UNIQUE,
	customer_name  TEXT NOT NULL,
	customer_email TEXT NOT NULL DEFAULT '',
	max_devices    INTEGER NOT NULL,
	expires_at     DATETIME,
	revoked_at     DATETIME,
	created_at     DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_licenses_product_id ON licenses(product_id);

CREATE TABLE IF NOT EXISTS devices (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	license_id     TEXT NOT NULL REFERENCES licenses(id),
	device_id      TEXT NOT NULL,
	device_name    TEXT NOT NULL DEFAULT '',
	activated_at   DATETIME NOT NULL,
	last_seen_at   DATETIME NOT NULL,
	deactivated_at DATETIME,
	UNIQUE(license_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_devices_license_id ON devices(license_id);
`
