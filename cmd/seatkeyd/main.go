// Command seatkeyd is the Seatkey license server: it serves the admin
// dashboard and the public license-check API from a single binary and a
// single SQLite database file.
package main

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	skcrypto "github.com/Laaaaksh/seatkey/internal/crypto"
	"github.com/Laaaaksh/seatkey/internal/license"
	"github.com/Laaaaksh/seatkey/internal/store"
	"github.com/Laaaaksh/seatkey/internal/web"
	"github.com/Laaaaksh/seatkey/internal/webhook"
)

// version is set at build/release time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println("seatkeyd " + version)
		return
	}

	dbPath := envOr("SEATKEY_DB_PATH", "seatkey.db")
	addr := envOr("SEATKEY_ADDR", ":8080")
	cookieSecure := envOr("SEATKEY_COOKIE_SECURE", "false") == "true"

	s, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open database %s: %v", dbPath, err)
	}
	defer func() { _ = s.Close() }()

	priv, pub, err := loadOrCreateKeypair(s)
	if err != nil {
		log.Fatalf("load signing key: %v", err)
	}

	licenses := license.NewService(s, priv, pub)
	licenses.SetNotifier(webhook.NewSender(func() (string, string) {
		url, _, _ := s.GetSetting("webhook_url")
		secret, _, _ := s.GetSetting("webhook_secret")
		return url, secret
	}))

	srv, err := web.NewServer(s, licenses, web.Config{CookieSecure: cookieSecure})
	if err != nil {
		log.Fatalf("build server: %v", err)
	}

	log.Printf("seatkeyd %s listening on %s (db: %s)", version, addr, dbPath)
	log.Printf("public key (embed in clients for offline verification): %s", skcrypto.EncodePublicKey(pub))

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}

// loadOrCreateKeypair persists the server's Ed25519 seed in Store settings so
// it survives restarts - every offline activation file ever issued stays
// verifiable against the same public key for the life of the database.
func loadOrCreateKeypair(s *store.Store) (priv ed25519.PrivateKey, pub ed25519.PublicKey, err error) {
	seed, ok, err := s.GetSetting("signing_key_seed")
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		seed, err = skcrypto.GenerateSeed()
		if err != nil {
			return nil, nil, err
		}
		if err := s.SetSetting("signing_key_seed", seed); err != nil {
			return nil, nil, err
		}
	}
	return skcrypto.KeypairFromSeed(seed)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
