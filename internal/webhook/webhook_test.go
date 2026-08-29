package webhook

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	body := []byte(`{"event":"activation"}`)
	sig := Sign("s3cr3t", body)
	if !Verify("s3cr3t", body, sig) {
		t.Fatal("Verify: expected true for matching secret")
	}
	if Verify("wrong-secret", body, sig) {
		t.Fatal("Verify: expected false for wrong secret")
	}
	if Verify("s3cr3t", []byte(`{"event":"tampered"}`), sig) {
		t.Fatal("Verify: expected false for tampered body")
	}
}

func TestNotifyDeliversSignedPayload(t *testing.T) {
	var (
		mu       sync.Mutex
		gotBody  []byte
		gotSig   string
		received = make(chan struct{})
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotBody = body
		gotSig = r.Header.Get(SignatureHeader)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		close(received)
	}))
	defer srv.Close()

	sender := NewSender(func() (string, string) { return srv.URL, "s3cr3t" })
	sender.Notify("activation", map[string]any{"event": "activation", "license_key": "SEAT-TEST"})

	select {
	case <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("webhook was not delivered within timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	if !Verify("s3cr3t", gotBody, gotSig) {
		t.Fatalf("delivered payload failed signature verification: body=%s sig=%s", gotBody, gotSig)
	}
}

func TestNotifyNoopsWithoutConfiguredURL(t *testing.T) {
	called := false
	sender := NewSender(func() (string, string) {
		called = true
		return "", ""
	})
	sender.Notify("activation", map[string]any{"event": "activation"})
	if !called {
		t.Fatal("expected lookup to be called")
	}
	// No assertion beyond "does not panic / hang" - Notify returns immediately
	// when no URL is configured, verified implicitly by the test completing.
}
