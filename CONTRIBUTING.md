# Contributing to Seatkey

Thank you for your interest in contributing. Seatkey is a self-hosted
license-key server for indie software vendors, open source under the MIT
license.

## Getting started

```bash
git clone https://github.com/<your-username>/seatkey.git   # your fork, see below
cd seatkey
go mod download
make build
make test
```

## Requirements

- Go 1.26+
- No external services are needed to build or test - the test suite runs
  against an in-memory SQLite database and an in-process HTTP test server.

## Contribution workflow

The `main` branch is protected: every change lands through a pull request,
required status checks must pass, and protection is enforced for everyone -
including the maintainer. There are no direct pushes to `main`.

1. Fork the repo on GitHub, then clone your fork (command above).
2. Create a descriptively named feature branch from `main`.
3. Make your changes as small, focused commits, each leaving the tree
   buildable.
4. Run `make lint` and `make test` - both must pass.
5. If your change is user-facing (a feature, fix, or behavior change), add
   one bullet under the `Unreleased` heading in [CHANGELOG.md](CHANGELOG.md).
6. Push the branch to your fork.
7. Open a pull request against `main` here.

A PR can merge only when every required check passes (`Test` and `Lint`) and
all conversation threads are resolved.

### Manual testing

Run the real server and demo CLI against each other to see a change working
end to end:

```bash
make build
./bin/seatkeyd &                      # visit http://localhost:8080 to finish setup
./bin/democli activate --key <key-from-dashboard> --device my-machine
./bin/democli run
```

## Releases

Releases are cut by pushing a tag; GitHub Actions does the rest
(`.github/workflows/release.yml`):

1. Make sure every user-facing change since the last release has a bullet
   under `Unreleased` in [CHANGELOG.md](CHANGELOG.md) (step 5 of the workflow
   above).
2. Give the release its own changelog section: insert `## [x.y.z] - YYYY-MM-DD`
   above the (now empty) `## [Unreleased]` heading, following the format of
   the existing sections, and update the compare links at the bottom of the
   file - add `[x.y.z]: https://github.com/Laaaaksh/seatkey/compare/v<prev>...vx.y.z`
   and repoint `[Unreleased]` at `compare/vx.y.z...HEAD`.
3. Land those changelog edits on `main` through a pull request (see the
   contribution workflow above), then tag and push:

   ```bash
   git tag vx.y.z && git push origin vx.y.z
   ```

The workflow extracts the tagged version's CHANGELOG section as the GitHub
release notes (a tag with no changelog entry fails the release rather than
publishing empty notes), builds `seatkeyd` and `democli` for Linux/macOS
(amd64 and arm64) with GoReleaser - pinned to an exact version, see the
workflow - and publishes them alongside a multi-arch Docker image. The tag
itself becomes the binaries' self-reported version (`seatkeyd --version`).

## Code style

- Standard `gofmt` / `goimports` formatting (enforced by CI).
- Package layout: `internal/store` (persistence), `internal/crypto` (signed
  envelopes), `internal/license` (activation/seat-limit business rules),
  `internal/webhook` (outbound event delivery), `internal/web` (HTTP +
  dashboard). Keep business rules in `internal/license`, not in HTTP
  handlers or SQL.
- Every exported symbol needs a doc comment (enforced by `golangci-lint`'s
  `revive` `exported` rule) - keep it to one line unless there's a real
  non-obvious constraint to explain.
- Signed-envelope changes (`internal/crypto`, `internal/license/token.go`)
  need tests that cover tampering and wrong-key verification, not just the
  happy path - this is the part of the codebase where a bug has the worst
  consequences.

## Reporting issues

Please open a GitHub issue before starting large changes or proposing new
features, so scope and approach can be settled before code is written. Bug
reports should include:

- `seatkeyd --version` output
- how you're running it (source build, Docker, OS)
- steps to reproduce
- what you expected vs. what happened

For anything you believe is a security vulnerability, see
[SECURITY.md](SECURITY.md) instead of opening a public issue.
