package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// GenerateSeed returns a new base64-encoded Ed25519 private key seed, meant
// to be persisted once (in Store settings) and reused across restarts so a
// server's public key — and therefore every offline activation file it has
// ever issued — stays valid.
func GenerateSeed() (string, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(priv.Seed()), nil
}

// KeypairFromSeed reconstructs the Ed25519 keypair from a base64 seed
// produced by GenerateSeed.
func KeypairFromSeed(seed string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(seed)
	if err != nil {
		return nil, nil, fmt.Errorf("decode seed: %w", err)
	}
	if len(raw) != ed25519.SeedSize {
		return nil, nil, fmt.Errorf("seed must be %d bytes, got %d", ed25519.SeedSize, len(raw))
	}
	priv := ed25519.NewKeyFromSeed(raw)
	pub := priv.Public().(ed25519.PublicKey)
	return priv, pub, nil
}

// EncodePublicKey renders a public key as base64 for embedding in clients or
// serving from /v1/pubkey.
func EncodePublicKey(pub ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(pub)
}

// DecodePublicKey parses a base64 public key produced by EncodePublicKey.
func DecodePublicKey(s string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key must be %d bytes, got %d", ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}
