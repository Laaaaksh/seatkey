package license

import (
	"errors"
	"testing"
	"time"

	skcrypto "github.com/Laaaaksh/seatkey/internal/crypto"
	"github.com/Laaaaksh/seatkey/internal/store"
)

type recordingNotifier struct {
	events []string
}

func (r *recordingNotifier) Notify(event string, data map[string]any) {
	r.events = append(r.events, event)
}

func newTestService(t *testing.T) (*Service, *store.Store, store.License) {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	seed, err := skcrypto.GenerateSeed()
	if err != nil {
		t.Fatalf("GenerateSeed: %v", err)
	}
	priv, pub, err := skcrypto.KeypairFromSeed(seed)
	if err != nil {
		t.Fatalf("KeypairFromSeed: %v", err)
	}

	svc := NewService(s, priv, pub)

	p, err := s.CreateProduct("Acme CLI")
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	lic, err := s.CreateLicense(store.CreateLicenseParams{
		ProductID:    p.ID,
		CustomerName: "Ada Lovelace",
		MaxDevices:   2,
	})
	if err != nil {
		t.Fatalf("CreateLicense: %v", err)
	}
	return svc, s, lic
}

func verifyToken(t *testing.T, svc *Service, env skcrypto.Envelope) Token {
	t.Helper()
	var tok Token
	if err := skcrypto.Verify(svc.PublicKey(), env, &tok); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	return tok
}

func TestActivateWithinSeatLimitSucceeds(t *testing.T) {
	svc, _, lic := newTestService(t)

	env, err := svc.Activate(lic.Key, "device-1", "Laptop")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	tok := verifyToken(t, svc, env)
	if tok.SeatsUsed != 1 || tok.SeatsMax != 2 || tok.Status != "active" {
		t.Fatalf("unexpected token: %+v", tok)
	}
	if tok.Offline {
		t.Fatal("online activation should not be marked offline")
	}
	if !tok.ValidUntil.After(time.Now()) {
		t.Fatalf("ValidUntil %v should be in the future", tok.ValidUntil)
	}
}

func TestActivateReusesExistingDeviceIdempotently(t *testing.T) {
	svc, _, lic := newTestService(t)

	if _, err := svc.Activate(lic.Key, "device-1", "Laptop"); err != nil {
		t.Fatalf("first Activate: %v", err)
	}
	env, err := svc.Activate(lic.Key, "device-1", "Laptop")
	if err != nil {
		t.Fatalf("second Activate: %v", err)
	}
	tok := verifyToken(t, svc, env)
	if tok.SeatsUsed != 1 {
		t.Fatalf("re-activating the same device should not consume a second seat, got SeatsUsed=%d", tok.SeatsUsed)
	}
}

func TestActivateRejectsBeyondSeatLimit(t *testing.T) {
	svc, _, lic := newTestService(t)

	if _, err := svc.Activate(lic.Key, "device-1", "Laptop"); err != nil {
		t.Fatalf("Activate device-1: %v", err)
	}
	if _, err := svc.Activate(lic.Key, "device-2", "Desktop"); err != nil {
		t.Fatalf("Activate device-2: %v", err)
	}

	// Third distinct device on a 2-seat license must be rejected.
	if _, err := svc.Activate(lic.Key, "device-3", "Third Machine"); !errors.Is(err, ErrDeviceLimit) {
		t.Fatalf("Activate device-3: got %v, want ErrDeviceLimit", err)
	}
}

func TestActivateUnknownKeyFails(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.Activate("SEAT-0000-0000-0000-0000", "device-1", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Activate unknown key: got %v, want ErrNotFound", err)
	}
}

func TestActivateRevokedLicenseFails(t *testing.T) {
	svc, s, lic := newTestService(t)
	if err := s.RevokeLicense(lic.ID); err != nil {
		t.Fatalf("RevokeLicense: %v", err)
	}
	if _, err := svc.Activate(lic.Key, "device-1", ""); !errors.Is(err, ErrLicenseStatus) {
		t.Fatalf("Activate revoked license: got %v, want ErrLicenseStatus", err)
	}
}

func TestActivateExpiredLicenseFails(t *testing.T) {
	svc, s, _ := newTestService(t)
	p, _ := s.CreateProduct("Acme CLI 2")
	past := time.Now().Add(-time.Hour)
	expired, err := s.CreateLicense(store.CreateLicenseParams{
		ProductID: p.ID, CustomerName: "Bob", MaxDevices: 1, ExpiresAt: &past,
	})
	if err != nil {
		t.Fatalf("CreateLicense: %v", err)
	}

	if _, err := svc.Activate(expired.Key, "device-1", ""); !errors.Is(err, ErrLicenseStatus) {
		t.Fatalf("Activate expired license: got %v, want ErrLicenseStatus", err)
	}
}

func TestDeactivateFreesASeat(t *testing.T) {
	svc, _, lic := newTestService(t)

	if _, err := svc.Activate(lic.Key, "device-1", ""); err != nil {
		t.Fatalf("Activate device-1: %v", err)
	}
	if _, err := svc.Activate(lic.Key, "device-2", ""); err != nil {
		t.Fatalf("Activate device-2: %v", err)
	}
	if _, err := svc.Activate(lic.Key, "device-3", ""); !errors.Is(err, ErrDeviceLimit) {
		t.Fatalf("expected ErrDeviceLimit before deactivation, got %v", err)
	}

	if err := svc.Deactivate(lic.Key, "device-1"); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	if _, err := svc.Activate(lic.Key, "device-3", ""); err != nil {
		t.Fatalf("Activate device-3 after freeing a seat: %v", err)
	}
}

func TestValidateRequiresPriorActivation(t *testing.T) {
	svc, _, lic := newTestService(t)

	if _, err := svc.Validate(lic.Key, "device-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Validate without activation: got %v, want ErrNotFound", err)
	}

	if _, err := svc.Activate(lic.Key, "device-1", ""); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	env, err := svc.Validate(lic.Key, "device-1")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	tok := verifyToken(t, svc, env)
	if tok.DeviceID != "device-1" {
		t.Fatalf("unexpected token device: %+v", tok)
	}
}

func TestValidateAfterDeactivationFails(t *testing.T) {
	svc, _, lic := newTestService(t)
	if _, err := svc.Activate(lic.Key, "device-1", ""); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if err := svc.Deactivate(lic.Key, "device-1"); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if _, err := svc.Validate(lic.Key, "device-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Validate after deactivation: got %v, want ErrNotFound", err)
	}
}

func TestOfflineActivateProducesLongLivedVerifiableToken(t *testing.T) {
	svc, _, lic := newTestService(t)

	env, err := svc.OfflineActivate(lic.Key, "airgapped-device", "Airgapped Box")
	if err != nil {
		t.Fatalf("OfflineActivate: %v", err)
	}
	tok := verifyToken(t, svc, env)
	if !tok.Offline {
		t.Fatal("expected Offline=true on an offline activation token")
	}
	if !tok.ValidUntil.After(time.Now().AddDate(50, 0, 0)) {
		t.Fatalf("offline token should be valid far into the future, got ValidUntil=%v", tok.ValidUntil)
	}

	// Simulate an air-gapped client: verify using only the public key, no store access.
	pub := svc.PublicKey()
	var clientTok Token
	if err := skcrypto.Verify(pub, env, &clientTok); err != nil {
		t.Fatalf("client-side Verify: %v", err)
	}
	if clientTok.LicenseKey != lic.Key {
		t.Fatalf("clientTok.LicenseKey = %q, want %q", clientTok.LicenseKey, lic.Key)
	}
}

func TestOfflineActivateRespectsSeatLimit(t *testing.T) {
	svc, _, lic := newTestService(t)

	if _, err := svc.OfflineActivate(lic.Key, "device-1", ""); err != nil {
		t.Fatalf("OfflineActivate device-1: %v", err)
	}
	if _, err := svc.OfflineActivate(lic.Key, "device-2", ""); err != nil {
		t.Fatalf("OfflineActivate device-2: %v", err)
	}
	if _, err := svc.OfflineActivate(lic.Key, "device-3", ""); !errors.Is(err, ErrDeviceLimit) {
		t.Fatalf("OfflineActivate device-3: got %v, want ErrDeviceLimit", err)
	}
}

func TestOfflineTokenCappedByLicenseExpiry(t *testing.T) {
	svc, s, _ := newTestService(t)
	p, _ := s.CreateProduct("Acme CLI 3")
	exp := time.Now().Add(48 * time.Hour)
	lic, err := s.CreateLicense(store.CreateLicenseParams{
		ProductID: p.ID, CustomerName: "Carl", MaxDevices: 1, ExpiresAt: &exp,
	})
	if err != nil {
		t.Fatalf("CreateLicense: %v", err)
	}

	env, err := svc.OfflineActivate(lic.Key, "device-1", "")
	if err != nil {
		t.Fatalf("OfflineActivate: %v", err)
	}
	tok := verifyToken(t, svc, env)
	if tok.ValidUntil.After(exp.Add(time.Second)) {
		t.Fatalf("offline token ValidUntil %v should be capped at license expiry %v", tok.ValidUntil, exp)
	}
}

func TestWebhookFiresOnActivationAndDeactivation(t *testing.T) {
	svc, _, lic := newTestService(t)
	rec := &recordingNotifier{}
	svc.SetNotifier(rec)

	if _, err := svc.Activate(lic.Key, "device-1", ""); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	// Re-activating the same device should NOT fire a second activation event.
	if _, err := svc.Activate(lic.Key, "device-1", ""); err != nil {
		t.Fatalf("re-Activate: %v", err)
	}
	if err := svc.Deactivate(lic.Key, "device-1"); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	want := []string{"activation", "deactivation"}
	if len(rec.events) != len(want) {
		t.Fatalf("events = %v, want %v", rec.events, want)
	}
	for i, e := range want {
		if rec.events[i] != e {
			t.Fatalf("events = %v, want %v", rec.events, want)
		}
	}
}
