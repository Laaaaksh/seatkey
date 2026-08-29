// Command democli is a stand-in for a real licensed application. It exists
// to demonstrate Seatkey's activation, seat-limit enforcement, and offline
// activation flows end to end, and to serve as a minimal reference client
// for the /v1 API and the signed-envelope verification it relies on.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Laaaaksh/seatkey/internal/crypto"
	"github.com/Laaaaksh/seatkey/internal/license"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	home := stateDir()
	if err := os.MkdirAll(home, 0o700); err != nil {
		fatalf("create state dir %s: %v", home, err)
	}

	var err error
	switch os.Args[1] {
	case "activate":
		err = cmdActivate(home, os.Args[2:])
	case "validate":
		err = cmdValidate(home, os.Args[2:])
	case "deactivate":
		err = cmdDeactivate(home, os.Args[2:])
	case "run":
		err = cmdRun(home, os.Args[2:])
	case "offline-request":
		err = cmdOfflineRequest(os.Args[2:])
	case "offline-activate":
		err = cmdOfflineActivate(home, os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fatalf("%v", err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `democli - a demo app that licenses itself against a running seatkeyd

Usage:
  democli activate         --server URL --key KEY --device ID [--name NAME]
  democli validate         --server URL
  democli deactivate       --server URL
  democli run
  democli offline-request  --key KEY --device ID [--name NAME]
  democli offline-activate --file PATH --pubkey BASE64

State (cached license + device id) is stored under $SEATKEY_DEMO_HOME
(default ~/.seatkey-demo).`)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "democli: "+format+"\n", args...)
	os.Exit(1)
}

func stateDir() string {
	if dir := os.Getenv("SEATKEY_DEMO_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".seatkey-demo")
}

func licensePath(home string) string { return filepath.Join(home, "license.json") }
func pubkeyPath(home string) string  { return filepath.Join(home, "pubkey.txt") }

type cachedState struct {
	Envelope crypto.Envelope `json:"envelope"`
	DeviceID string          `json:"device_id"`
	Server   string          `json:"server,omitempty"`
}

func saveState(home string, st cachedState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(licensePath(home), data, 0o600)
}

func loadState(home string) (cachedState, error) {
	var st cachedState
	data, err := os.ReadFile(licensePath(home))
	if err != nil {
		return st, err
	}
	err = json.Unmarshal(data, &st)
	return st, err
}

func savePubkey(home, pub string) error {
	return os.WriteFile(pubkeyPath(home), []byte(pub), 0o600)
}

func loadPubkey(home string) (string, error) {
	data, err := os.ReadFile(pubkeyPath(home))
	return string(data), err
}

// --- server calls ---

type apiErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func postJSON(server, path string, body any) (crypto.Envelope, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return crypto.Envelope{}, err
	}
	resp, err := http.Post(server+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return crypto.Envelope{}, fmt.Errorf("contact %s: %w", server, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		var apiErr apiErrorResponse
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Message != "" {
			return crypto.Envelope{}, fmt.Errorf("server rejected request: %s", apiErr.Message)
		}
		return crypto.Envelope{}, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var env crypto.Envelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		return crypto.Envelope{}, fmt.Errorf("decode response: %w", err)
	}
	return env, nil
}

// postJSONOK is used for endpoints that return {"ok":true} rather than a
// signed envelope, such as /v1/deactivate.
func postJSONOK(server, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := http.Post(server+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("contact %s: %w", server, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		var apiErr apiErrorResponse
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Message != "" {
			return fmt.Errorf("server rejected request: %s", apiErr.Message)
		}
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}
	return nil
}

func fetchPubkey(server string) (string, error) {
	resp, err := http.Get(server + "/v1/pubkey")
	if err != nil {
		return "", fmt.Errorf("contact %s: %w", server, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.PublicKey, nil
}

// --- commands ---

func cmdActivate(home string, args []string) error {
	fs := flag.NewFlagSet("activate", flag.ExitOnError)
	server := fs.String("server", "http://localhost:8080", "seatkeyd base URL")
	key := fs.String("key", "", "license key")
	device := fs.String("device", "", "device id (a stable fingerprint for this machine)")
	name := fs.String("name", "", "human-readable device name")
	_ = fs.Parse(args)

	if *key == "" || *device == "" {
		return fmt.Errorf("--key and --device are required")
	}

	env, err := postJSON(*server, "/v1/activate", map[string]string{
		"license_key": *key, "device_id": *device, "device_name": *name,
	})
	if err != nil {
		return err
	}

	pub, err := fetchPubkey(*server)
	if err != nil {
		return err
	}
	if err := savePubkey(home, pub); err != nil {
		return err
	}
	if err := saveState(home, cachedState{Envelope: env, DeviceID: *device, Server: *server}); err != nil {
		return err
	}

	return printToken(pub, env)
}

func cmdValidate(home string, args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	server := fs.String("server", "http://localhost:8080", "seatkeyd base URL")
	_ = fs.Parse(args)

	st, err := loadState(home)
	if err != nil {
		return fmt.Errorf("no cached activation found, run `democli activate` first: %w", err)
	}
	var tok license.Token
	if err := json.Unmarshal(mustDecodePayload(st.Envelope), &tok); err != nil {
		return err
	}

	env, err := postJSON(*server, "/v1/validate", map[string]string{
		"license_key": tok.LicenseKey, "device_id": st.DeviceID,
	})
	if err != nil {
		return err
	}
	st.Envelope = env
	if err := saveState(home, st); err != nil {
		return err
	}

	pub, err := loadPubkey(home)
	if err != nil {
		return err
	}
	return printToken(pub, env)
}

func cmdDeactivate(home string, args []string) error {
	fs := flag.NewFlagSet("deactivate", flag.ExitOnError)
	server := fs.String("server", "http://localhost:8080", "seatkeyd base URL")
	_ = fs.Parse(args)

	st, err := loadState(home)
	if err != nil {
		return fmt.Errorf("no cached activation found: %w", err)
	}
	var tok license.Token
	if err := json.Unmarshal(mustDecodePayload(st.Envelope), &tok); err != nil {
		return err
	}

	if err := postJSONOK(*server, "/v1/deactivate", map[string]string{
		"license_key": tok.LicenseKey, "device_id": st.DeviceID,
	}); err != nil {
		return err
	}
	_ = os.Remove(licensePath(home))
	fmt.Println("deactivated - seat freed")
	return nil
}

// cmdRun is what a real licensed app calls on every startup: verify the
// cached, signed token entirely offline, with no server contact.
func cmdRun(home string, args []string) error {
	st, err := loadState(home)
	if err != nil {
		return fmt.Errorf("not activated - run `democli activate` first: %w", err)
	}
	pub, err := loadPubkey(home)
	if err != nil {
		return fmt.Errorf("no cached public key - run `democli activate` or `offline-activate` first: %w", err)
	}

	pubKey, err := crypto.DecodePublicKey(pub)
	if err != nil {
		return err
	}
	var tok license.Token
	if err := crypto.Verify(pubKey, st.Envelope, &tok); err != nil {
		return fmt.Errorf("license signature invalid, refusing to run: %w", err)
	}
	if time.Now().After(tok.ValidUntil) {
		kind := "cached grace period"
		if tok.Offline {
			kind = "offline activation"
		}
		return fmt.Errorf("license check expired (%s ended %s) - reconnect and run `democli validate`", kind, tok.ValidUntil.Format(time.RFC3339))
	}

	fmt.Printf("✓ licensed to %s (%s) — seat %s, %d/%d seats in use, valid until %s\n",
		tok.CustomerName, tok.Product, tok.DeviceID, tok.SeatsUsed, tok.SeatsMax,
		tok.ValidUntil.Format("2006-01-02"))
	return nil
}

func cmdOfflineRequest(args []string) error {
	fs := flag.NewFlagSet("offline-request", flag.ExitOnError)
	key := fs.String("key", "", "license key (for your own reference; not otherwise used)")
	device := fs.String("device", "", "device id for this air-gapped machine")
	name := fs.String("name", "", "human-readable device name")
	_ = fs.Parse(args)

	if *device == "" {
		return fmt.Errorf("--device is required")
	}

	req := map[string]string{"device_id": *device, "device_name": *name}
	if *key != "" {
		req["license_key_hint"] = *key
	}
	out, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	fmt.Fprintln(os.Stderr, "\nSend the JSON above to your vendor. They will paste it into the Seatkey")
	fmt.Fprintln(os.Stderr, "dashboard and send back a signed activation file for `democli offline-activate`.")
	return nil
}

func cmdOfflineActivate(home string, args []string) error {
	fs := flag.NewFlagSet("offline-activate", flag.ExitOnError)
	file := fs.String("file", "", "path to the signed activation file from your vendor")
	pubkey := fs.String("pubkey", "", "base64 server public key (skip if already cached from a prior `activate`)")
	_ = fs.Parse(args)

	if *file == "" {
		return fmt.Errorf("--file is required")
	}

	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	var env crypto.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("%s is not a valid activation file: %w", *file, err)
	}

	pub := *pubkey
	if pub == "" {
		pub, err = loadPubkey(home)
		if err != nil {
			return fmt.Errorf("no public key available - pass --pubkey (get it once from a vendor or /v1/pubkey while online): %w", err)
		}
	}

	pubKey, err := crypto.DecodePublicKey(pub)
	if err != nil {
		return err
	}
	var tok license.Token
	if err := crypto.Verify(pubKey, env, &tok); err != nil {
		return fmt.Errorf("activation file signature invalid: %w", err)
	}

	if err := savePubkey(home, pub); err != nil {
		return err
	}
	if err := saveState(home, cachedState{Envelope: env, DeviceID: tok.DeviceID}); err != nil {
		return err
	}
	fmt.Println("offline activation verified and cached - `democli run` now works with no network.")
	return printToken(pub, env)
}

func printToken(pub string, env crypto.Envelope) error {
	pubKey, err := crypto.DecodePublicKey(pub)
	if err != nil {
		return err
	}
	var tok license.Token
	if err := crypto.Verify(pubKey, env, &tok); err != nil {
		return err
	}
	mode := "online"
	if tok.Offline {
		mode = "offline"
	}
	fmt.Printf("✓ activated (%s) — %s, seat %s, %d/%d seats, valid until %s\n",
		mode, tok.CustomerName, tok.DeviceID, tok.SeatsUsed, tok.SeatsMax,
		tok.ValidUntil.Format("2006-01-02"))
	return nil
}

func mustDecodePayload(env crypto.Envelope) []byte {
	// The payload is base64 of JSON we produced ourselves via crypto.Sign, so
	// a decode failure here means the cached state file was corrupted.
	data, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		fatalf("corrupted cached license state: %v", err)
	}
	return data
}
