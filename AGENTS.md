# Project agent memory

This file is the project's committed home for project-intrinsic agent knowledge: build, test, release, architecture, and sharp-edge notes that should travel with the code.

## What this is

Seatkey is a self-hosted license-key server for indie software vendors: issue keys, enforce
per-license device limits, and support fully offline activation. MIT-licensed alternative to
Keygen's source-available (Fair Core License) core. See README.md for the pitch and CONTRIBUTING.md
for the build/release workflow — don't duplicate those here.

## Architecture

- `internal/store` — SQLite persistence (pure-Go `modernc.org/sqlite`, no cgo). Owns schema and CRUD only, no business rules.
- `internal/crypto` — generic Ed25519 sign/verify of a `{payload, signature}` envelope. Signs raw JSON bytes rather than re-marshaling on verify, so there's no canonicalization step to get wrong.
- `internal/license` — the actual licensing rules (seat-limit enforcement, activate/validate/deactivate, offline activation) on top of `store` + `crypto`. Put new business rules here, not in HTTP handlers.
- `internal/webhook` — outbound HMAC-signed event delivery, fire-and-forget with one retry.
- `internal/web` — HTTP: public `/v1/*` API (no auth) + session-cookie admin dashboard (server-rendered `html/template`, no JS framework). Templates use a two-step render (content template → wrapped in `layout`) because Go's `html/template` shares one flat namespace across all parsed files — see `server.go`'s `render`/`pageMeta`.
- `cmd/seatkeyd` — server binary, wires everything together, generates/persists the Ed25519 signing key in `store` settings on first boot.
- `cmd/democli` — reference client demonstrating the full flow (activate/validate/deactivate/run/offline-request/offline-activate) against a running `seatkeyd`. Also the thing the demo GIF is built from.

## Design decisions worth knowing before changing

- **SQLite, not Postgres**, despite the original spec suggesting an embedded/external Postgres. Chosen because a pure-Go SQLite driver gets closer to "one binary, zero infra" than Postgres does, which is the whole point of the tech pick. Revisit only if a genuine multi-writer/scale need shows up.
- **Offline activation tokens cannot be revoked.** Once issued, they verify forever (or until the license's own expiry) against the server's public key, since by definition there's no server round-trip. This is documented in SECURITY.md and in the `offlineHorizon` comment in `internal/license/service.go` — don't "fix" this without re-reading why it's inherent to the feature.
- **Online tokens carry a 7-day grace period** (`license.OnlineGracePeriod`) so a client can cache a signed response and skip the network on every startup.
- Signed envelopes sign raw payload bytes, not a re-marshaled struct — see `internal/crypto/envelope.go`. Any change to `license.Token`'s JSON shape is backward-compatible as long as old fields aren't removed; verification never re-derives the payload.

## Build/test/lint

Standard: `make build` / `make test` / `make lint`. Full details (release tagging, changelog process, code style) are in CONTRIBUTING.md — read that, not this file, before cutting a release.

- `golangci-lint`'s `revive` `exported` rule requires a doc comment on every exported symbol; `.golangci.yml` has the exact linter set.
- Crypto/license tests must cover tampering, wrong-key, and expiry cases, not just the happy path — see `internal/crypto/envelope_test.go` and `internal/license/service_test.go` for the pattern to follow.

## Demo assets

`docs/assets/seatkey-demo.gif` and the two dashboard PNGs were built from real captured output/screenshots of the actual running server (headless Chrome + ffmpeg), not mockups. If the CLI output format or dashboard UI changes meaningfully, regenerate them the same way rather than hand-editing stale assets.

## Maintaining this file

Keep this file for knowledge useful to almost every future agent session in this project.
Do not repeat what the codebase already shows; point to the authoritative file or command instead.
Prefer rewriting or pruning existing entries over appending new ones.
When updating this file, preserve this bar for all agents and keep entries concise.
