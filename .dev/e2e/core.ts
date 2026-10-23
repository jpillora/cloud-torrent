import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { chromium } from "playwright";
import { unzipSync } from "fflate";
import { files, infohash, name, startFixture } from "./fixture.ts";
import { verifyAppearance } from "./appearance.ts";
import { verifySearchCancellation, verifySearchLoading } from "./search.ts";

const publicFixture = process.argv.includes("--public-fixture");
const publicURL = "https://webtorrent.io/torrents/big-buck-bunny.torrent";
const posterSHA256 =
  "d082c4ae86f4f8d323c4a03665cdec24c20ad0dcaddcfef1bcab073457a101ff";
const directory = publicFixture ? "Big Buck Bunny" : name;
const scratch = await mkdtemp(join(tmpdir(), "cloud-torrent-e2e-"));
let fixture: Awaited<ReturnType<typeof startFixture>> | null = null;
const errors: string[] = [];
let browser: Awaited<ReturnType<typeof chromium.launch>> | undefined;
let launching: ReturnType<typeof chromium.launch> | undefined;
let interrupted = false;
let app: ReturnType<typeof Bun.spawn> | undefined;
let cleaning: Promise<void> | undefined;

async function freePort() {
  const server = createServer();
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  assert(address && typeof address !== "string");
  await new Promise<void>((resolve, reject) =>
    server.close((error) => error ? reject(error) : resolve())
  );
  return address.port;
}
async function cleanup() {
  return cleaning ??= (async () => {
    try {
      const activeBrowser = browser ?? await launching?.catch(() => undefined);
      await activeBrowser?.close();
    } finally {
      if (app) {
        app.kill();
        const timer = setTimeout(() => app?.kill("SIGKILL"), 3000);
        await app.exited;
        clearTimeout(timer);
      }
      try {
        await fixture?.close();
      } finally {
        await rm(scratch, { recursive: true, force: true });
      }
    }
  })();
}
for (const signal of ["SIGINT", "SIGTERM"] as const) {
  process.once(signal, () => {
    interrupted = true;
    void cleanup().finally(() => process.exit(1));
  });
}

try {
  assert.equal(
    infohash,
    "5705481528e4be430011e1afab78b230664c6e2e",
    "local fixture infohash drifted",
  );
  fixture = publicFixture ? null : await startFixture();
  const port = await freePort();
  let incoming = await freePort();
  while (incoming === port) incoming = await freePort();
  const base = `http://127.0.0.1:${port}`;
  const config = join(scratch, "config.json");
  await writeFile(
    config,
    JSON.stringify({
      DownloadDirectory: join(scratch, "downloads"),
      IncomingPort: incoming,
      AutoStart: !publicFixture,
      EnableUpload: false,
      EnableSeeding: false,
    }),
  );
  app = Bun.spawn([
    resolve("dist/cloud-torrent"),
    "--host",
    "127.0.0.1",
    "--port",
    String(port),
    "--config-path",
    config,
  ], {
    cwd: scratch,
    stdout: "ignore",
    stderr: Bun.file(join(scratch, "server.log")),
  });
  let ready = false;
  for (let i = 0; i < 100; i++) {
    if (app.exitCode !== null) {
      throw new Error(
        `server exited: ${await readFile(join(scratch, "server.log"), "utf8")}`,
      );
    }
    try {
      ready = (await fetch(base)).ok;
    } catch { /* listener is starting */ }
    if (ready) break;
    await delay(100);
  }
  assert(ready, "isolated server did not start");
  assert(!interrupted, "test interrupted");
  launching = chromium.launch({
    executablePath: process.env.CHROMIUM_EXECUTABLE_PATH || undefined,
    headless: true,
  });
  browser = await launching;
  assert(!interrupted, "test interrupted");
  await verifyAppearance(browser, base);
  await verifySearchLoading(browser, base);
  await verifySearchCancellation(browser, base);
  const context = await browser.newContext({
    acceptDownloads: true,
    colorScheme: "dark",
  });
  const page = await context.newPage();
  page.setDefaultTimeout(publicFixture ? 150_000 : 30_000);
  page.on("pageerror", (error) => errors.push(error.message));
  let syncConnections = 0;
  page.on("response", (response) => {
    if (
      new URL(response.url()).pathname === "/sync" && response.ok() &&
      response.headers()["content-type"]?.startsWith("text/event-stream")
    ) syncConnections++;
  });
  await page.goto(base);
  await page.getByTestId("connect-overlay").waitFor({ state: "hidden" });
  assert.equal(await page.getByTestId("torrent").count(), 0);
  await page.getByTestId("omni-input").fill(
    publicFixture ? publicURL : fixture!.magnet,
  );
  await page.getByTestId(publicFixture ? "omni-start" : "omni-load").click();
  const card = page.getByTestId("torrent");
  await card.getByTestId("torrent-name").filter({ hasText: directory })
    .waitFor();
  await card.locator(".dimmer").waitFor({ state: "hidden" });
  if (!publicFixture) {
    await card.getByText(`#${infohash}`, { exact: true }).waitFor();
  }
  if (publicFixture) {
    await card.getByRole("button", { name: "Files", exact: true }).click();
    await card.getByRole("row").filter({ hasText: "poster.jpg" }).getByTestId(
      "file-start",
    ).click();
  }
  // No direct engine polling, sync mocks, or state injection: progress must reach Vue via /sync.
  await page.waitForFunction(
    () => {
      const bar = document.querySelector<HTMLElement>(
        "[data-testid=torrent] .progress .bar",
      );
      const value = parseFloat(bar?.style.width ?? "0");
      return value > 0 && value < 100;
    },
    undefined,
    { timeout: publicFixture ? 150_000 : 30_000 },
  );
  assert(syncConnections > 0, "no successful realtime SSE connection observed");
  const expected = publicFixture ? ["poster.jpg"] : [...files.keys()];
  for (const path of expected) {
    const link = page.getByTestId("fs-file-link").filter({
      hasText: path.split("/").at(-1)!,
    });
    await link.waitFor();
    const href = await link.getAttribute("href");
    assert(href);
    const response = await context.request.get(new URL(href, base).href);
    assert.equal(response.status(), 200);
    const content = await response.body();
    if (publicFixture) {
      assert.equal(content.length, 310380);
      assert.equal(
        createHash("sha256").update(content).digest("hex"),
        posterSHA256,
      );
    } else assert.deepEqual(content, files.get(path));
  }
  if (!publicFixture) {
    await page.waitForFunction(() =>
      document.querySelector<HTMLElement>(
        "[data-testid=torrent] .progress .bar",
      )?.style.width === "100%"
    );
  }
  await card.getByTestId("torrent-stop").click();
  await card.getByTestId("torrent-remove").click();
  await card.waitFor({ state: "detached" });
  if (publicFixture) {
    // The poster shares a piece with the movie. Remove its sparse incomplete
    // neighbor through the UI rather than archiving a mostly empty movie file.
    const partial = page.getByTestId("fs-file-link").filter({
      hasText: /\.part$/,
    });
    await partial.waitFor();
    assert.equal(await partial.innerText(), "Big Buck Bunny.mp4.part");
    const row = partial.locator("..");
    await row.getByTestId("fs-actions-toggle").click();
    await row.getByTestId("fs-delete").click();
    await row.getByTestId("fs-delete-confirm").click();
    await partial.waitFor({ state: "detached" });
  }
  // Folder ZIP must work after the torrent card is removed, independently of expansion.
  const zip = page.getByRole("link", {
    name: `Download ${directory} as ZIP`,
    exact: true,
  });
  const folder = page.getByRole("button", { name: directory, exact: true });
  await folder.focus();
  await page.keyboard.press("Enter");
  assert.equal(await folder.getAttribute("aria-expanded"), "false");
  await page.keyboard.press("Space");
  assert.equal(await folder.getAttribute("aria-expanded"), "true");
  const folderRow = folder.locator("..");
  await folderRow.getByTestId("fs-actions-toggle").click();
  await zip.waitFor();
  assert.equal(await folderRow.getByTestId("fs-preview-toggle").count(), 0);
  const downloadPending = page.waitForEvent("download");
  await zip.click();
  const download = await downloadPending;
  const archive = join(scratch, "folder.zip");
  await download.saveAs(archive);
  const entries = unzipSync(await readFile(archive));
  assert.deepEqual(Object.keys(entries).sort(), [...expected].sort());
  for (const path of expected) {
    const bytes = Buffer.from(entries[path]);
    if (publicFixture) {
      assert.equal(
        createHash("sha256").update(bytes).digest("hex"),
        posterSHA256,
      );
    } else assert.deepEqual(bytes, files.get(path));
  }
  // Exercise media actions and confirmation on a separate file, preserving the torrent fixture.
  await writeFile(
    join(scratch, "downloads", "preview.png"),
    Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=", "base64"),
  );
  const previewLink = page.getByTestId("fs-file-link").filter({ hasText: /^preview\.png$/ });
  await previewLink.waitFor();
  const previewNode = previewLink.locator("../..");
  const actions = previewNode.getByTestId("fs-actions-toggle");
  assert.equal(await previewNode.getByTestId("fs-delete").count(), 0);
  await actions.focus();
  await page.keyboard.press("Enter");
  await previewNode.getByTestId("fs-preview-toggle").waitFor();
  assert.equal(await previewNode.getByTestId("fs-folder-download").count(), 0);
  await page.keyboard.press("Escape");
  assert.equal(await actions.getAttribute("aria-expanded"), "false");
  assert(await actions.evaluate((element) => element === document.activeElement));
  await actions.click();
  await previewNode.getByTestId("fs-preview-toggle").click();
  await previewNode.getByTestId("fs-preview").waitFor();
  assert.equal(await actions.getAttribute("aria-expanded"), "false");
  await actions.click();
  await previewNode.getByRole("button", { name: "Hide preview", exact: true }).click();
  await previewNode.getByTestId("fs-preview").waitFor({ state: "hidden" });
  await actions.click();
  await page.getByRole("heading", { name: "Downloads", exact: true }).click();
  assert.equal(await actions.getAttribute("aria-expanded"), "false");
  await page.setViewportSize({ width: 375, height: 812 });
  await actions.click();
  const menuBox = await previewNode.getByTestId("fs-actions-menu").boundingBox();
  assert(menuBox && menuBox.x >= 0 && menuBox.x + menuBox.width <= 375);
  await previewNode.getByTestId("fs-delete").click();
  await previewNode.getByTestId("fs-delete-confirm").waitFor();
  assert.equal(await context.request.get(new URL((await previewLink.getAttribute("href"))!, base).href).then((r) => r.status()), 200);
  await page.keyboard.press("Escape");
  await actions.click();
  await previewNode.getByTestId("fs-delete").waitFor();
  await previewNode.getByTestId("fs-delete").click();
  await previewNode.getByTestId("fs-delete-confirm").click();
  await previewLink.waitFor({ state: "detached" });
  console.log("PASS: download actions menu, keyboard/outside dismissal, media preview, mobile and confirmed deletion");
  assert.deepEqual(errors, [], "browser runtime errors");
  console.log(
    `PASS: ${
      publicFixture
        ? "public Big Buck Bunny poster"
        : "pinned local CC0 torrent"
    }: UI → API → SSE → file bytes → folder ZIP (${expected.length} entries)`,
  );
} catch (error) {
  console.error("Browser errors:", errors);
  console.error(
    "Isolated app log:\n",
    await readFile(join(scratch, "server.log"), "utf8").catch(() =>
      "unavailable"
    ),
  );
  throw error;
} finally {
  await cleanup();
}
