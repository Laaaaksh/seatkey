# record-demo

Records the README's `docs/assets/demo.mp4` / `demo.gif` by driving a real, locally
running `seatkeyd` through Playwright and shelling out to the real `democli` binary.
Nothing on screen is staged: every screen and every terminal line is produced by the
actual server and client responding to actual requests.

This is dev-only tooling. It has its own `package.json` and is never imported by the
Go build.

## What it records

1. First-boot admin setup and login.
2. Creating a product and issuing a 2-seat license key.
3. `democli activate` on two devices (succeeds), then a third (refused — the seat
   limit is genuinely enforced by `internal/license`).
4. Freeing a seat from the admin dashboard, then `democli validate` on that device
   failing on its next check because the server has actually revoked it.

## Running it

From the repo root:

```bash
make demo
```

Or directly:

```bash
cd scripts/record-demo
npm install
npx playwright install chromium   # once
npm run record
```

By default it boots seatkeyd on `:8080`. If that port is taken, override it without
touching any committed config:

```bash
APP_PORT=8090 npm run record
```

Output lands in `docs/assets/demo.mp4` and `docs/assets/demo.gif`. Verify the GIF
actually plays the loop (`ffprobe docs/assets/demo.gif`) and is under 10 MB before
committing.

## Notes

- Each run boots seatkeyd against a fresh scratch SQLite database in a temp directory
  (never the repo's `seatkey.db`), so re-running is always a clean walkthrough.
- The "terminal" pane is a small local HTML page that Playwright records like any other
  page; the text in it is the real stdout/stderr of real `democli` invocations, appended
  live, not typed or scripted text.
- Re-running produces the same sequence of steps and the same enforcement outcomes
  every time; the license key string itself is randomly generated per run, since that's
  how the server issues keys.
