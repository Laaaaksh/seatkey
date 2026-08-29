// Package webhook delivers Seatkey's activation/deactivation events to a
// vendor's own URL, HMAC-signed so the receiver can verify it actually came
// from this server. Seatkey does not integrate with any billing provider
// directly - wiring the webhook up to Stripe, Paddle, or anything else is
// left to the receiving end.
package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// SignatureHeader carries the hex-encoded HMAC-SHA256 of the raw request
// body, keyed by the configured webhook secret.
const SignatureHeader = "X-Seatkey-Signature"

type urlSecretFunc func() (url, secret string)

// Sender delivers webhook events. It reads the target URL and secret lazily
// on every send (via lookup) so changing them in the dashboard takes effect
// immediately, with no restart.
type Sender struct {
	lookup urlSecretFunc
	client *http.Client
}

// NewSender builds a Sender that reads the webhook URL and secret via lookup
// on every delivery.
func NewSender(lookup func() (url, secret string)) *Sender {
	return &Sender{
		lookup: lookup,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Notify sends data as a JSON webhook body in the background. It never
// blocks the caller and never returns an error - delivery failures are
// logged, since a vendor's endpoint being briefly down must not affect
// license activation itself.
func (s *Sender) Notify(event string, data map[string]any) {
	url, secret := s.lookup()
	if url == "" {
		return
	}
	go s.deliver(url, secret, data)
}

func (s *Sender) deliver(url, secret string, data map[string]any) {
	body, err := json.Marshal(data)
	if err != nil {
		log.Printf("webhook: marshal event: %v", err)
		return
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		if lastErr = s.attempt(url, secret, body); lastErr == nil {
			return
		}
	}
	log.Printf("webhook: delivery to %s failed after retry: %v", url, lastErr)
}

func (s *Sender) attempt(url, secret string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set(SignatureHeader, Sign(secret, body))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook endpoint returned status %d", resp.StatusCode)
	}
	return nil
}

// Sign returns the hex-encoded HMAC-SHA256 of body keyed by secret, matching
// what SECURITY.md and the dashboard's webhook page tell a vendor to verify.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether signature (as sent in SignatureHeader) matches body
// under secret. Provided as a reference implementation for receivers written
// in Go; other languages can compute HMAC-SHA256(secret, body) equivalently.
func Verify(secret string, body []byte, signature string) bool {
	expected := Sign(secret, body)
	return hmac.Equal([]byte(expected), []byte(signature))
}
