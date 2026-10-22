import assert from "node:assert/strict";
import type { Browser, Route } from "playwright";

// Hold provider/API responses at the HTTP boundary to reproduce slow requests
// deterministically, without depending on a live search provider or swarm.
export async function verifySearchCancellation(browser: Browser, base: string) {
  const context = await browser.newContext();
  const details: Route[] = [];
  const urls: Route[] = [];
  let magnets = 0;
  const magnet = "magnet:?xt=urn:btih:5705481528e4be430011e1afab78b230664c6e2e";
  try {
    const page = await context.newPage();
    await page.route("**/search/**", async (route) => {
      if (new URL(route.request().url()).pathname.endsWith("/item")) {
        details.push(route);
      } else {
        await route.fulfill({
          json: [
            { name: "Slow detail", path: "/fixture-detail" },
            {
              name: "Slow torrent URL",
              torrent: "https://fixture.invalid/test.torrent",
            },
            { name: "Ready magnet", magnet },
          ],
        });
      }
    });
    await page.route("**/api/url", (route) => {
      urls.push(route);
    });
    await page.route("**/api/magnet", async (route) => {
      magnets++;
      await route.fulfill({ body: "OK" });
    });
    await page.goto(base);
    await page.getByTestId("connect-overlay").waitFor({ state: "hidden" });
    await page.getByTestId("omni-input").fill("fixture");
    await page.getByTestId("search-submit").click();
    const buttons = page.getByTestId("search-download");
    await buttons.nth(2).waitFor();
    await page.clock.install();
    await page.clock.pauseAt(new Date(Date.now() + 1000));
    const detail = buttons.nth(0);
    const before = await detail.boundingBox();
    const started = page.waitForRequest((request) =>
      request.url().includes("/item?")
    );
    await detail.click();
    await started;
    assert(await detail.isDisabled());
    assert.equal(await detail.locator(".btn-spinner").count(), 1);
    assert.equal(
      await detail.locator("svg").count(),
      0,
      "download icon must be replaced",
    );
    const during = await detail.boundingBox();
    assert.equal(before?.width, during?.width);
    assert.equal(
      before?.height,
      during?.height,
      "spinner must not add a second line",
    );
    await page.clock.runFor(4999);
    assert(await detail.isDisabled(), "cannot cancel before five seconds");
    await page.clock.runFor(1);
    assert(await detail.isEnabled());
    assert.equal(
      await detail.getAttribute("aria-label"),
      "Cancel download of Slow detail",
    );
    const cancelled = page.waitForEvent("requestfailed", {
      predicate: (request) => request.url().includes("/item?"),
    });
    await detail.click();
    await cancelled;
    assert.equal(await detail.locator("svg").count(), 1);
    assert.equal(await detail.locator(".btn-spinner").count(), 0);
    // A cancelled provider response must never enqueue a download.
    await details[0].fulfill({ json: { magnet } }).catch(() => {});
    assert.equal(magnets, 0);
    assert.equal(await page.getByTestId("omni-error").count(), 0);

    // Retry the same row immediately, then complete it successfully.
    await detail.click();
    await page.waitForFunction(() =>
      document.querySelector('[data-testid="search-download"] .btn-spinner') !==
        null
    );
    assert.equal(details.length, 2);
    await details[1].fulfill({ json: { magnet } });
    await detail.locator("svg").waitFor();
    assert.equal(magnets, 1);

    // A pending URL submission is cancellable too, independently of other rows.
    const direct = buttons.nth(1);
    await direct.click();
    await buttons.nth(2).click();
    await buttons.nth(2).locator("svg").waitFor();
    assert.equal(magnets, 2);
    assert(await direct.isDisabled());
    await page.clock.runFor(5000);
    const urlCancelled = page.waitForEvent("requestfailed", {
      predicate: (request) => request.url().endsWith("/api/url"),
    });
    await direct.click();
    await urlCancelled;
    assert.equal(urls.length, 1);
    await direct.locator("svg").waitFor();

    // Removing rows cancels work, including before the manual cancel threshold.
    await detail.click();
    const removed = page.waitForEvent("requestfailed", {
      predicate: (request) => request.url().includes("/item?"),
    });
    await page.getByTestId("omni-input").fill("another fixture");
    await removed;
    assert.equal(await buttons.count(), 0);
    assert.equal(await page.getByTestId("omni-error").count(), 0);
    assert.equal(await page.getByTestId("error-banner").count(), 0);
    console.log(
      "PASS: search spinner replacement, five-second abort, retry and row cleanup",
    );
  } finally {
    await context.close();
  }
}
