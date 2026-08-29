// Package crypto signs and verifies the JSON envelopes Seatkey hands to
// clients so a license check can be cached and verified without a server
// round-trip, both for the short online grace period and for a fully offline
// activation file.
package crypto

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
)

// ErrInvalidSignature is returned by Verify when the signature does not
// match the payload under the given public key.
var ErrInvalidSignature = errors.New("invalid signature")

// Envelope pairs a JSON payload with an Ed25519 signature over the exact
// payload bytes. Signing raw bytes (rather than re-marshaling a struct on the
// verifying side) means there is no canonicalization step that verifier and
// signer could disagree on.
type Envelope struct {
	Payload   string `json:"payload"`   // base64 standard encoding of the payload's JSON bytes
	Signature string `json:"signature"` // base64 standard encoding of the Ed25519 signature
}

// Sign marshals payload to JSON and signs the resulting bytes.
func Sign(priv ed25519.PrivateKey, payload any) (Envelope, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	sig := ed25519.Sign(priv, data)
	return Envelope{
		Payload:   base64.StdEncoding.EncodeToString(data),
		Signature: base64.StdEncoding.EncodeToString(sig),
	}, nil
}

// Verify checks the envelope's signature against pub and, if valid, unmarshals
// the payload into v.
func Verify(pub ed25519.PublicKey, env Envelope, v any) error {
	data, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, data, sig) {
		return ErrInvalidSignature
	}
	return json.Unmarshal(data, v)
}
