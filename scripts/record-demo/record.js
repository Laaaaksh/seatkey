#!/usr/bin/env node
// Records a genuine end-to-end walkthrough of seatkeyd + democli using
// Playwright. Boots a real seatkeyd against a scratch database, drives the
// real HTML dashboard, and shells out to the real democli binary for every
// client-side step. Nothing on screen is synthetic: the terminal panes show
// actual stdout/stderr from actual processes, paced with waits so a viewer
// can read each screen.
"use strict";

const { chromium } = require("@playwright/test");
const { spawn, execFileSync } = require("child_process");
const http = require("http");
const fs = require("fs");
const path = require("path");
const os = require("os");

const REPO_ROOT = path.resolve(__dirname, "..", "..");
const BIN_DIR = path.join(REPO_ROOT, "bin");
const SEATKEYD = path.join(BIN_DIR, "seatkeyd");
const DEMOCLI = path.join(BIN_DIR, "democli");
const ASSETS_DIR = path.join(REPO_ROOT, "docs", "assets");

const APP_PORT = process.env.APP_PORT || "8080";
const APP_URL = `http://127.0.0.1:${APP_PORT}`;
const ADMIN_PASSWORD = "Sh1p-The-Seats!";

const RUN_DIR = fs.mkdtempSync(path.join(os.tmpdir(), "seatkey-demo-"));
const DB_PATH = path.join(RUN_DIR, "seatkey.db");
const VIDEO_DIR = path.join(RUN_DIR, "video");
const SERVER_LOG = path.join(RUN_DIR, "seatkeyd.log");

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function log(msg) {
  console.log(`[record-demo] ${msg}`);
}

function ensureBuilt() {
  log("building seatkeyd + democli (make build)...");
  execFileSync("make", ["build"], { cwd: REPO_ROOT, stdio: "inherit" });
}

function waitForHealth(url, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  return new Promise((resolve, reject) => {
    const attempt = () => {
      http
        .get(`${url}/healthz`, (res) => {
          res.resume();
          if (res.statusCode === 200) return resolve();
          retry();
        })
        .on("error", retry);
    };
    const retry = () => {
      if (Date.now() > deadline) return reject(new Error("seatkeyd did not become healthy in time"));
      setTimeout(attempt, 200);
    };
    attempt();
  });
}

function startServer() {
  const child = spawn(SEATKEYD, [], {
    cwd: REPO_ROOT,
    env: {
      ...process.env,
      SEATKEY_DB_PATH: DB_PATH,
      SEATKEY_ADDR: `:${APP_PORT}`,
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  const logStream = fs.createWriteStream(SERVER_LOG);
  child.stdout.pipe(logStream);
  child.stderr.pipe(logStream);
  return child;
}

// --- a tiny static server for the terminal-pane page ---

function startStageServer() {
  const html = fs.readFileSync(path.join(__dirname, "terminal.html"));
  const server = http.createServer((req, res) => {
    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(html);
  });
  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => resolve(server));
  });
}

// --- terminal pane helpers (genuine democli invocations, rendered live) ---

function escapeHtml(s) {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

async function appendTermLine(page, html) {
  await page.evaluate((h) => {
    const out = document.getElementById("output");
    out.innerHTML += h + "\n";
  }, html);
}

async function clearTerm(page) {
  await page.evaluate(() => {
    document.getElementById("output").innerHTML = "";
  });
}

// Runs the real democli binary, streams its real output into the terminal
// pane, and returns { code, stdout, stderr }.
function runDemocli(args, home) {
  return new Promise((resolve) => {
    const child = spawn(DEMOCLI, args, {
      env: { ...process.env, SEATKEY_DEMO_HOME: home },
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (d) => (stdout += d.toString()));
    child.stderr.on("data", (d) => (stderr += d.toString()));
    child.on("close", (code) => resolve({ code, stdout, stderr }));
  });
}

function shellQuote(arg) {
  return /\s/.test(arg) ? `"${arg}"` : arg;
}

async function runAndShow(page, label, args, home) {
  log(label);
  const shown = args.map(shellQuote).join(" ");
  await appendTermLine(page, `<span class="prompt">$</span> democli ${escapeHtml(shown)}`);
  await sleep(500);
  const { code, stdout, stderr } = await runDemocli(args, home);
  const text = (stdout + stderr).trim();
  const cls = code === 0 ? "ok" : "err";
  await appendTermLine(page, `<span class="${cls}">${escapeHtml(text)}</span>`);
  return { code, stdout, stderr };
}

// --- main choreography ---

async function main() {
  fs.mkdirSync(VIDEO_DIR, { recursive: true });
  fs.mkdirSync(ASSETS_DIR, { recursive: true });

  ensureBuilt();

  log(`starting seatkeyd on ${APP_URL} (db: ${DB_PATH})`);
  const server = startServer();
  server.on("exit", (code, sig) => {
    if (code !== null && code !== 0) log(`seatkeyd exited unexpectedly (code ${code}, sig ${sig})`);
  });

  const stage = await startStageServer();
  const stagePort = stage.address().port;
  const stageURL = `http://127.0.0.1:${stagePort}/`;

  const deviceHome = (name) => {
    const dir = path.join(RUN_DIR, "devices", name);
    fs.mkdirSync(dir, { recursive: true });
    return dir;
  };

  let browser;
  try {
    await waitForHealth(APP_URL, 15000);
    log("seatkeyd is up");

    browser = await chromium.launch();
    const context = await browser.newContext({
      viewport: { width: 1280, height: 800 },
      deviceScaleFactor: 2,
      recordVideo: { dir: VIDEO_DIR, size: { width: 1280, height: 800 } },
    });
    const page = await context.newPage();

    // 1. First boot: create the admin account.
    await page.goto(APP_URL + "/", { waitUntil: "networkidle" });
    await sleep(1800);
    await page.fill("#password", ADMIN_PASSWORD);
    await page.fill("#confirm", ADMIN_PASSWORD);
    await sleep(1200);
    await page.click("button[type=submit]");
    await page.waitForURL("**/products");
    await sleep(3000);

    // 2. Create a product.
    await page.fill("#name", "Acme CLI");
    await sleep(900);
    await page.click("form[action='/products'] button[type=submit]");
    await page.waitForURL("**/products/**");
    await sleep(3000);

    // 3. Issue a license key with a 2-seat limit.
    await page.fill("#customer_name", "Nimbus Robotics");
    await page.fill("#customer_email", "ops@nimbusrobotics.example");
    await page.fill("#max_devices", "2");
    await sleep(1200);
    await page.click("form[action*='/licenses'] button[type=submit]");
    await page.waitForURL("**/licenses/**");
    await sleep(3600);

    const licenseKey = (await page.locator("h1.mono").innerText()).trim();
    log(`issued license key: ${licenseKey}`);
    const licenseURL = page.url();
    await sleep(2600);

    // 4. Terminal: activate two devices, then a third gets refused.
    await page.goto(stageURL);
    await clearTerm(page);
    await sleep(700);

    const laptop = deviceHome("laptop");
    const desktop = deviceHome("desktop");
    const tablet = deviceHome("tablet");

    await runAndShow(
      page,
      "activate laptop",
      ["activate", "--server", APP_URL, "--key", licenseKey, "--device", "laptop-a1b2", "--name", "Dev laptop"],
      laptop
    );
    await sleep(3800);

    await runAndShow(
      page,
      "activate desktop",
      ["activate", "--server", APP_URL, "--key", licenseKey, "--device", "desktop-c3d4", "--name", "Studio desktop"],
      desktop
    );
    await sleep(3800);

    await runAndShow(
      page,
      "activate tablet (should be refused - seat limit reached)",
      ["activate", "--server", APP_URL, "--key", licenseKey, "--device", "tablet-e5f6", "--name", "Field tablet"],
      tablet
    );
    await sleep(5500);

    // 5. Back to the dashboard: free the laptop's seat from the server side.
    await page.goto(licenseURL, { waitUntil: "networkidle" });
    await sleep(3000);
    await page
      .locator("table tr", { hasText: "laptop-a1b2" })
      .getByRole("button", { name: "Free seat" })
      .click();
    await page.waitForLoadState("networkidle");
    await sleep(2200);

    // 6. Terminal: the freed device loses access on its next check.
    await page.goto(stageURL);
    await clearTerm(page);
    await sleep(700);

    await runAndShow(
      page,
      "validate laptop (seat was just freed by the admin)",
      ["validate", "--server", APP_URL],
      laptop
    );
    await sleep(4500);

    await page.close();
    const videoPath = await page.video().path();
    await context.close();
    await browser.close();
    browser = null;

    log(`raw capture: ${videoPath}`);
    convert(videoPath);
  } finally {
    if (browser) await browser.close().catch(() => {});
    stage.close();
    server.kill();
  }

  log("done. Verify docs/assets/demo.mp4 and docs/assets/demo.gif before committing.");
}

function convert(webmPath) {
  const mp4Path = path.join(ASSETS_DIR, "demo.mp4");
  const gifPath = path.join(ASSETS_DIR, "demo.gif");
  const palettePath = path.join(RUN_DIR, "palette.png");

  log("converting to demo.mp4 (H.264, yuv420p, 1280px wide)...");
  execFileSync("ffmpeg", [
    "-y", "-i", webmPath,
    "-vf", "scale=1280:-2",
    "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart",
    mp4Path,
  ], { stdio: "inherit" });

  log("building palette for GIF...");
  execFileSync("ffmpeg", [
    "-y", "-i", mp4Path,
    "-vf", "fps=12,scale=960:-2:flags=lanczos,palettegen",
    palettePath,
  ], { stdio: "inherit" });

  log("converting to demo.gif (960px wide, ~12fps)...");
  execFileSync("ffmpeg", [
    "-y", "-i", mp4Path, "-i", palettePath,
    "-lavfi", "fps=12,scale=960:-2:flags=lanczos[x];[x][1:v]paletteuse",
    gifPath,
  ], { stdio: "inherit" });

  const mp4Size = fs.statSync(mp4Path).size;
  const gifSize = fs.statSync(gifPath).size;
  log(`demo.mp4: ${(mp4Size / 1024 / 1024).toFixed(2)} MB`);
  log(`demo.gif: ${(gifSize / 1024 / 1024).toFixed(2)} MB`);
  if (gifSize > 10 * 1024 * 1024) {
    throw new Error(`demo.gif is ${(gifSize / 1024 / 1024).toFixed(2)} MB, over the 10 MB limit - shorten the walkthrough or drop fps`);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
