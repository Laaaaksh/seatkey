# Security Policy

## Supported versions

Seatkey is a young project. Security fixes are made against the **latest
release** and `main` only — please confirm you can reproduce the issue on the
newest release (`seatkeyd --version`) before reporting.

| Version        | Supported |
| -------------- | --------- |
| latest release | yes       |
| older releases | no        |

## Reporting a vulnerability

Please do **not** open a public GitHub issue for anything you believe is a
security problem.

Use GitHub's private vulnerability reporting instead:

> https://github.com/Laaaaksh/seatkey/security/advisories/new

That link reaches the maintainer privately — the report, follow-up
discussion, and any fix coordination stay confidential until a patched
release ships.

When reporting, please include:

- your `seatkeyd --version` output
- how you're running it (source build, Docker, OS)
- clear steps to reproduce

## What belongs in a report

Seatkey signs and verifies license activations with Ed25519, and gates the
admin dashboard with a password-based session. Things worth reporting:

- Any way to forge, replay, or extend a signed activation/validation envelope
  (`internal/crypto`) without the server's private key — this is the single
  most consequential class of bug in this project: it either lets a cracked
  license key through, or falsely locks out a legitimate customer.
- Any way to activate a device beyond a license's device limit without
  going through the intended seat-accounting path in `internal/license`.
- Session or password handling issues that let one admin dashboard session
  be hijacked or the password check be bypassed (`internal/web/auth.go`).
- SQL injection or path traversal in `internal/store` or `internal/web`.
- A way to make the webhook sender (`internal/webhook`) deliver to, or leak
  the shared secret to, a URL other than the one configured in Settings.

## What's out of scope

- **Offline activation cannot be remotely revoked.** Once a signed offline
  activation file is issued, it verifies against the server's public key
  forever (or until the license's own expiry), because by design there is no
  server round-trip to check against. This is documented, inherent behavior
  of the offline-activation feature, not a vulnerability — see
  `internal/license/service.go`'s `offlineHorizon` comment.
- A vendor's own webhook receiver validating (or failing to validate) the
  `X-Seatkey-Signature` header is that receiver's responsibility, not
  Seatkey's.
- Denial of service against a self-hosted instance you control yourself.
- Issues that only reproduce on an already-compromised host (Seatkey trusts
  its own database file and process environment).

## Credits

Reporters who wish to be credited in a fix's release notes may say so in the
private report; otherwise reports are handled without attribution.
