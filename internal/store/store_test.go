package store

import (
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestProductCRUD(t *testing.T) {
	s := newTestStore(t)

	p, err := s.CreateProduct("Acme CLI")
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if p.ID == "" || p.Name != "Acme CLI" {
		t.Fatalf("unexpected product: %+v", p)
	}

	got, err := s.GetProduct(p.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if got != p {
		t.Fatalf("GetProduct mismatch: got %+v, want %+v", got, p)
	}

	if _, err := s.GetProduct("does-not-exist"); err != ErrNotFound {
		t.Fatalf("GetProduct missing: got %v, want ErrNotFound", err)
	}

	list, err := s.ListProducts()
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(list) != 1 || list[0].ID != p.ID {
		t.Fatalf("ListProducts: got %+v", list)
	}
}

func TestLicenseKeyFormat(t *testing.T) {
	key, err := GenerateLicenseKey()
	if err != nil {
		t.Fatalf("GenerateLicenseKey: %v", err)
	}
	// SEAT-XXXX-XXXX-XXXX-XXXX
	if len(key) != 24 {
		t.Fatalf("key length = %d, want 24 (%q)", len(key), key)
	}
	if key[:5] != "SEAT-" {
		t.Fatalf("key %q missing SEAT- prefix", key)
	}
	for _, forbidden := range []byte{'I', 'L', 'O', 'U'} {
		for _, c := range []byte(key) {
			if c == forbidden {
				t.Fatalf("key %q contains ambiguous character %q", key, forbidden)
			}
		}
	}
}

func TestCreateLicenseRejectsDuplicateKeyCollision(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProduct("Acme CLI")

	l, err := s.CreateLicense(CreateLicenseParams{ProductID: p.ID, CustomerName: "Ada", MaxDevices: 2})
	if err != nil {
		t.Fatalf("CreateLicense: %v", err)
	}
	if l.ProductName != "" {
		// ProductName is only populated by the join in Get/List, not Create.
		t.Fatalf("unexpected ProductName on create: %q", l.ProductName)
	}

	got, err := s.GetLicenseByKey(l.Key)
	if err != nil {
		t.Fatalf("GetLicenseByKey: %v", err)
	}
	if got.ID != l.ID || got.ProductName != "Acme CLI" {
		t.Fatalf("GetLicenseByKey: got %+v", got)
	}
}

func TestLicenseStatus(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name string
		l    License
		want string
	}{
		{"active, no expiry", License{}, "active"},
		{"active, not yet expired", License{ExpiresAt: &future}, "active"},
		{"expired", License{ExpiresAt: &past}, "expired"},
		{"revoked wins over expiry", License{ExpiresAt: &future, RevokedAt: &past}, "revoked"},
		{"revoked, no expiry", License{RevokedAt: &past}, "revoked"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.l.Status(now); got != tc.want {
				t.Fatalf("Status() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRevokeLicense(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProduct("Acme CLI")
	l, _ := s.CreateLicense(CreateLicenseParams{ProductID: p.ID, CustomerName: "Ada", MaxDevices: 1})

	if err := s.RevokeLicense(l.ID); err != nil {
		t.Fatalf("RevokeLicense: %v", err)
	}
	got, _ := s.GetLicense(l.ID)
	if got.RevokedAt == nil {
		t.Fatal("expected RevokedAt to be set")
	}

	if err := s.RevokeLicense(l.ID); err != ErrNotFound {
		t.Fatalf("re-revoking: got %v, want ErrNotFound", err)
	}
	if err := s.RevokeLicense("missing"); err != ErrNotFound {
		t.Fatalf("revoking missing license: got %v, want ErrNotFound", err)
	}
}

func TestDeviceActivationAndSeatCounting(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProduct("Acme CLI")
	l, _ := s.CreateLicense(CreateLicenseParams{ProductID: p.ID, CustomerName: "Ada", MaxDevices: 2})

	if n, _ := s.ActiveDeviceCount(l.ID); n != 0 {
		t.Fatalf("initial active count = %d, want 0", n)
	}

	d1, err := s.ActivateDevice(l.ID, "device-1", "Ada's Laptop")
	if err != nil {
		t.Fatalf("ActivateDevice: %v", err)
	}
	if n, _ := s.ActiveDeviceCount(l.ID); n != 1 {
		t.Fatalf("active count after 1 activation = %d, want 1", n)
	}

	if _, err := s.ActivateDevice(l.ID, "device-2", "Ada's Desktop"); err != nil {
		t.Fatalf("ActivateDevice 2: %v", err)
	}
	if n, _ := s.ActiveDeviceCount(l.ID); n != 2 {
		t.Fatalf("active count after 2 activations = %d, want 2", n)
	}

	if _, err := s.GetActiveDevice(l.ID, "device-1"); err != nil {
		t.Fatalf("GetActiveDevice: %v", err)
	}
	if _, err := s.GetActiveDevice(l.ID, "device-99"); err != ErrNotFound {
		t.Fatalf("GetActiveDevice missing: got %v, want ErrNotFound", err)
	}

	if err := s.DeactivateDevice(l.ID, d1.DeviceID); err != nil {
		t.Fatalf("DeactivateDevice: %v", err)
	}
	if n, _ := s.ActiveDeviceCount(l.ID); n != 1 {
		t.Fatalf("active count after deactivation = %d, want 1", n)
	}
	if _, err := s.GetActiveDevice(l.ID, d1.DeviceID); err != ErrNotFound {
		t.Fatalf("GetActiveDevice after deactivation: got %v, want ErrNotFound", err)
	}

	if err := s.DeactivateDevice(l.ID, d1.DeviceID); err != ErrNotFound {
		t.Fatalf("re-deactivating: got %v, want ErrNotFound", err)
	}

	devices, err := s.ListDevices(l.ID)
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("ListDevices: got %d devices, want 2", len(devices))
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	if _, ok, err := s.GetSetting("missing"); err != nil || ok {
		t.Fatalf("GetSetting missing: ok=%v err=%v", ok, err)
	}
	if err := s.SetSetting("webhook_url", "https://example.com/hook"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	v, ok, err := s.GetSetting("webhook_url")
	if err != nil || !ok || v != "https://example.com/hook" {
		t.Fatalf("GetSetting: v=%q ok=%v err=%v", v, ok, err)
	}
	if err := s.SetSetting("webhook_url", "https://example.com/hook2"); err != nil {
		t.Fatalf("SetSetting overwrite: %v", err)
	}
	v, _, _ = s.GetSetting("webhook_url")
	if v != "https://example.com/hook2" {
		t.Fatalf("overwrite did not take effect, got %q", v)
	}
}

func TestAdminAndSessionLifecycle(t *testing.T) {
	s := newTestStore(t)

	if has, err := s.HasAdmin(); err != nil || has {
		t.Fatalf("HasAdmin before creation: has=%v err=%v", has, err)
	}
	if err := s.CreateAdmin("hashed-password"); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if has, err := s.HasAdmin(); err != nil || !has {
		t.Fatalf("HasAdmin after creation: has=%v err=%v", has, err)
	}
	hash, err := s.AdminPasswordHash()
	if err != nil || hash != "hashed-password" {
		t.Fatalf("AdminPasswordHash: hash=%q err=%v", hash, err)
	}

	if err := s.SetAdminPassword("new-hash"); err != nil {
		t.Fatalf("SetAdminPassword: %v", err)
	}
	hash, _ = s.AdminPasswordHash()
	if hash != "new-hash" {
		t.Fatalf("AdminPasswordHash after update: %q", hash)
	}

	if err := s.CreateSession("tok1", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if ok, err := s.ValidSession("tok1"); err != nil || !ok {
		t.Fatalf("ValidSession: ok=%v err=%v", ok, err)
	}
	if ok, err := s.ValidSession("missing"); err != nil || ok {
		t.Fatalf("ValidSession missing: ok=%v err=%v", ok, err)
	}

	if err := s.CreateSession("expired", time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("CreateSession expired: %v", err)
	}
	if ok, err := s.ValidSession("expired"); err != nil || ok {
		t.Fatalf("ValidSession expired: ok=%v err=%v", ok, err)
	}

	if err := s.DeleteSession("tok1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if ok, _ := s.ValidSession("tok1"); ok {
		t.Fatal("session still valid after DeleteSession")
	}
}
