package crypto

import (
	"encoding/base64"
	"strings"
	"testing"
)

type testPayload struct {
	LicenseKey string `json:"license_key"`
	SeatsMax   int    `json:"seats_max"`
}

func TestSignVerifyRoundTrip(t *testing.T) {
	seed, err := GenerateSeed()
	if err != nil {
		t.Fatalf("GenerateSeed: %v", err)
	}
	priv, pub, err := KeypairFromSeed(seed)
	if err != nil {
		t.Fatalf("KeypairFromSeed: %v", err)
	}

	want := testPayload{LicenseKey: "SEAT-AAAA-BBBB-CCCC-DDDD", SeatsMax: 2}
	env, err := Sign(priv, want)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	var got testPayload
	if err := Verify(pub, env, &got); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got != want {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, want)
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	seed, _ := GenerateSeed()
	priv, pub, _ := KeypairFromSeed(seed)

	env, err := Sign(priv, testPayload{LicenseKey: "SEAT-AAAA-BBBB-CCCC-DDDD", SeatsMax: 1})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Flip the payload to claim a higher seat limit than was actually signed.
	raw, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	tampered := strings.Replace(string(raw), `"seats_max":1`, `"seats_max":99`, 1)
	if tampered == string(raw) {
		t.Fatal("test setup: tamper substring not found in payload")
	}
	env.Payload = base64.StdEncoding.EncodeToString([]byte(tampered))

	var got testPayload
	if err := Verify(pub, env, &got); err != ErrInvalidSignature {
		t.Fatalf("Verify on tampered payload: got err=%v, want ErrInvalidSignature", err)
	}
}

func TestVerifyRejectsWrongKey(t *testing.T) {
	seedA, _ := GenerateSeed()
	privA, _, _ := KeypairFromSeed(seedA)
	seedB, _ := GenerateSeed()
	_, pubB, _ := KeypairFromSeed(seedB)

	env, err := Sign(privA, testPayload{LicenseKey: "SEAT-AAAA-BBBB-CCCC-DDDD", SeatsMax: 1})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	var got testPayload
	if err := Verify(pubB, env, &got); err != ErrInvalidSignature {
		t.Fatalf("Verify with wrong public key: got err=%v, want ErrInvalidSignature", err)
	}
}

func TestVerifyRejectsTamperedSignature(t *testing.T) {
	seed, _ := GenerateSeed()
	priv, pub, _ := KeypairFromSeed(seed)

	env, err := Sign(priv, testPayload{LicenseKey: "SEAT-AAAA-BBBB-CCCC-DDDD", SeatsMax: 1})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	sigBytes[0] ^= 0xFF
	env.Signature = base64.StdEncoding.EncodeToString(sigBytes)

	var got testPayload
	if err := Verify(pub, env, &got); err != ErrInvalidSignature {
		t.Fatalf("Verify with tampered signature: got err=%v, want ErrInvalidSignature", err)
	}
}

func TestKeypairFromSeedRejectsBadLength(t *testing.T) {
	if _, _, err := KeypairFromSeed(base64.StdEncoding.EncodeToString([]byte("too-short"))); err == nil {
		t.Fatal("expected error for short seed, got nil")
	}
}

func TestDecodePublicKeyRejectsBadLength(t *testing.T) {
	if _, err := DecodePublicKey(base64.StdEncoding.EncodeToString([]byte("too-short"))); err == nil {
		t.Fatal("expected error for short public key, got nil")
	}
}
