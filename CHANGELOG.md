# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- License key issuance per product/customer with a configurable per-license
  device (seat) limit.
- `/v1/activate`, `/v1/validate`, `/v1/deactivate`, and `/v1/pubkey` API
  endpoints, returning Ed25519-signed envelopes clients can cache and verify
  offline for a 7-day grace period.
- Offline activation flow: an air-gapped client generates a request file, an
  admin pastes it into the dashboard, and the resulting signed activation
  file verifies forever with no further server contact.
- Web dashboard: product and license management, device/seat view with the
  ability to free a seat, license revocation, webhook configuration, and
  admin password management.
- HMAC-signed webhooks fired on activation and deactivation, so a vendor can
  wire up their own billing without Seatkey integrating with it directly.
- `democli`, a reference client demonstrating activation, seat-limit
  enforcement, and offline verification against a running `seatkeyd`.
- Single-binary server (`seatkeyd`) backed by embedded SQLite - no external
  database required to self-host.

[Unreleased]: https://github.com/Laaaaksh/seatkey/compare/7ccbd9b...HEAD
