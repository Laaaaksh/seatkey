package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	skcrypto "github.com/Laaaaksh/seatkey/internal/crypto"
	"github.com/Laaaaksh/seatkey/internal/license"
	"github.com/Laaaaksh/seatkey/internal/store"
)

func newTestServer(t *testing.T) *httptest.Server {
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
	svc := license.NewService(s, priv, pub)

	srv, err := NewServer(s, svc, Config{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return ts
}

// newAuthedClient runs first-boot setup against ts and returns a client that
// carries the resulting session cookie on every subsequent request.
func newAuthedClient(t *testing.T, ts *httptest.Server) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	resp, err := client.PostForm(ts.URL+"/setup", url.Values{
		"password": {"correct-horse-battery"},
		"confirm":  {"correct-horse-battery"},
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	resp.Body.Close()
	return client
}

func TestSetupThenLoginFlow(t *testing.T) {
	ts := newTestServer(t)

	// Before setup, / redirects to /setup.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != "/setup" {
		t.Fatalf("redirect before setup = %q, want /setup", loc)
	}

	authed := newAuthedClient(t, ts)

	// After setup, /setup itself redirects to /login (admin already exists).
	resp, err = authed.Get(ts.URL + "/setup")
	if err != nil {
		t.Fatalf("GET /setup after admin exists: %v", err)
	}
	resp.Body.Close()

	// The authed client's session cookie should now reach the dashboard.
	resp, err = authed.Get(ts.URL + "/products")
	if err != nil {
		t.Fatalf("GET /products: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /products status = %d, want 200", resp.StatusCode)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	ts := newTestServer(t)
	_ = newAuthedClient(t, ts) // creates the admin account

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	resp, err := client.PostForm(ts.URL+"/login", url.Values{"password": {"wrong"}})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	body, _ := readAll(resp)
	if !strings.Contains(body, "Incorrect password") {
		t.Fatalf("expected incorrect-password message in body, got: %s", body)
	}

	// No session cookie should have been set.
	resp2, err := client.Get(ts.URL + "/products")
	if err != nil {
		t.Fatalf("GET /products: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.Request.URL.Path != "/login" {
		t.Fatalf("expected redirect to /login without a session, ended at %s", resp2.Request.URL.Path)
	}
}

func TestUnauthenticatedDashboardRedirectsToLogin(t *testing.T) {
	ts := newTestServer(t)
	_ = newAuthedClient(t, ts) // ensures an admin exists so /products doesn't redirect to /setup instead

	client := &http.Client{}
	resp, err := client.Get(ts.URL + "/products")
	if err != nil {
		t.Fatalf("GET /products: %v", err)
	}
	defer resp.Body.Close()
	if resp.Request.URL.Path != "/login" {
		t.Fatalf("expected redirect to /login, ended at %s", resp.Request.URL.Path)
	}
}

// productAndLicense drives the dashboard through creating one product and
// one 2-seat license, returning the issued key.
func productAndLicense(t *testing.T, ts *httptest.Server, client *http.Client, maxDevices string) string {
	t.Helper()
	resp, err := client.PostForm(ts.URL+"/products", url.Values{"name": {"Acme CLI"}})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	resp.Body.Close()
	productURL := resp.Request.URL.String()

	resp, err = client.PostForm(productURL+"/licenses", url.Values{
		"customer_name": {"Ada Lovelace"},
		"max_devices":   {maxDevices},
	})
	if err != nil {
		t.Fatalf("create license: %v", err)
	}
	defer resp.Body.Close()
	body, _ := readAll(resp)

	idx := strings.Index(body, "SEAT-")
	if idx == -1 {
		t.Fatalf("license key not found in license detail page: %s", body)
	}
	return body[idx : idx+24]
}

func TestDashboardIssuesLicenseAndAPIEnforcesSeatLimit(t *testing.T) {
	ts := newTestServer(t)
	client := newAuthedClient(t, ts)

	key := productAndLicense(t, ts, client, "2")

	activate := func(deviceID string) (int, map[string]any) {
		resp, err := http.Post(ts.URL+"/v1/activate", "application/json", jsonBody(map[string]string{
			"license_key": key, "device_id": deviceID,
		}))
		if err != nil {
			t.Fatalf("activate %s: %v", deviceID, err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	if status, _ := activate("device-1"); status != http.StatusOK {
		t.Fatalf("activate device-1: status %d", status)
	}
	if status, _ := activate("device-2"); status != http.StatusOK {
		t.Fatalf("activate device-2: status %d", status)
	}
	status, out := activate("device-3")
	if status != http.StatusConflict {
		t.Fatalf("activate device-3 (should hit seat limit): status=%d body=%v", status, out)
	}
	if out["error"] != "device_limit_reached" {
		t.Fatalf("expected device_limit_reached error, got %v", out)
	}
}

func TestAPIActivateUnknownKeyReturns404(t *testing.T) {
	ts := newTestServer(t)
	resp, err := http.Post(ts.URL+"/v1/activate", "application/json", jsonBody(map[string]string{
		"license_key": "SEAT-0000-0000-0000-0000", "device_id": "d1",
	}))
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestAPIActivateMissingFieldsReturns400(t *testing.T) {
	ts := newTestServer(t)
	resp, err := http.Post(ts.URL+"/v1/activate", "application/json", jsonBody(map[string]string{}))
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPubkeyEndpointMatchesActivationSignature(t *testing.T) {
	ts := newTestServer(t)
	client := newAuthedClient(t, ts)
	key := productAndLicense(t, ts, client, "1")

	resp, err := http.Post(ts.URL+"/v1/activate", "application/json", jsonBody(map[string]string{
		"license_key": key, "device_id": "device-1",
	}))
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	defer resp.Body.Close()
	var env skcrypto.Envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}

	pkResp, err := http.Get(ts.URL + "/v1/pubkey")
	if err != nil {
		t.Fatalf("pubkey: %v", err)
	}
	defer pkResp.Body.Close()
	var pk struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(pkResp.Body).Decode(&pk); err != nil {
		t.Fatalf("decode pubkey: %v", err)
	}

	pub, err := skcrypto.DecodePublicKey(pk.PublicKey)
	if err != nil {
		t.Fatalf("DecodePublicKey: %v", err)
	}
	var tok license.Token
	if err := skcrypto.Verify(pub, env, &tok); err != nil {
		t.Fatalf("Verify with /v1/pubkey key: %v", err)
	}
	if tok.LicenseKey != key {
		t.Fatalf("tok.LicenseKey = %q, want %q", tok.LicenseKey, key)
	}
}

func TestDeviceDeactivateFreesSeatThroughDashboard(t *testing.T) {
	ts := newTestServer(t)
	client := newAuthedClient(t, ts)
	key := productAndLicense(t, ts, client, "1")

	resp, err := http.Post(ts.URL+"/v1/activate", "application/json", jsonBody(map[string]string{
		"license_key": key, "device_id": "device-1",
	}))
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	resp.Body.Close()

	// A second device should be rejected while device-1 holds the only seat.
	resp, err = http.Post(ts.URL+"/v1/activate", "application/json", jsonBody(map[string]string{
		"license_key": key, "device_id": "device-2",
	}))
	if err != nil {
		t.Fatalf("activate device-2: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("activate device-2 before freeing a seat: status = %d, want 409", resp.StatusCode)
	}

	licenseID := licenseIDFromKey(t, ts, client, key)
	resp, err = client.PostForm(ts.URL+"/licenses/"+licenseID+"/devices/device-1/deactivate", nil)
	if err != nil {
		t.Fatalf("deactivate via dashboard: %v", err)
	}
	resp.Body.Close()

	resp, err = http.Post(ts.URL+"/v1/activate", "application/json", jsonBody(map[string]string{
		"license_key": key, "device_id": "device-2",
	}))
	if err != nil {
		t.Fatalf("activate device-2 after freeing seat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("activate device-2 after freeing seat: status = %d, want 200", resp.StatusCode)
	}
}

func licenseIDFromKey(t *testing.T, ts *httptest.Server, client *http.Client, key string) string {
	t.Helper()
	resp, err := client.Get(ts.URL + "/products")
	if err != nil {
		t.Fatalf("GET /products: %v", err)
	}
	defer resp.Body.Close()
	body, _ := readAll(resp)
	idx := strings.Index(body, "/products/")
	if idx == -1 {
		t.Fatalf("no product link found")
	}
	end := strings.Index(body[idx:], "\"")
	productURL := body[idx : idx+end]

	resp2, err := client.Get(ts.URL + productURL)
	if err != nil {
		t.Fatalf("GET product page: %v", err)
	}
	defer resp2.Body.Close()
	body2, _ := readAll(resp2)
	linkIdx := strings.Index(body2, "/licenses/")
	if linkIdx == -1 {
		t.Fatalf("no license link found in product page")
	}
	rest := body2[linkIdx+len("/licenses/"):]
	end2 := strings.IndexAny(rest, "\"?")
	return rest[:end2]
}

func readAll(resp *http.Response) (string, error) {
	data, err := io.ReadAll(resp.Body)
	return string(data), err
}

func jsonBody(v any) *strings.Reader {
	data, _ := json.Marshal(v)
	return strings.NewReader(string(data))
}
