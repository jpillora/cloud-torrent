import assert from "node:assert/strict";
import type { Browser } from "playwright";

export async function verifySettings(browser: Browser, base: string) {
  const context = await browser.newContext();
  try {
    const page = await context.newPage();
    await page.goto(base);
    await page.getByTestId("connect-overlay").waitFor({ state: "hidden" });
    await page.getByTestId("main-page").waitFor();
    assert.equal(await page.getByTestId("config-editor").count(), 0);
    await page.getByTestId("status-settings").click();
    await page.waitForURL("**/#/settings");
    await page.getByRole("heading", { name: "Settings", exact: true }).waitFor();
    assert.equal(await page.getByTestId("main-page").count(), 0, "settings must replace the main page");
    const upload = page.getByTestId("config-UploadLimitKiB");
    await upload.waitFor();
    const help = page.getByRole("button", { name: "About Upload limit (KiB/s)", exact: true });
    const explanation = page.locator("#help-UploadLimitKiB");
    assert.equal(await explanation.isVisible(), false);
    const layout = (element: HTMLElement) => {
      const box = element.getBoundingClientRect();
      return { x: box.x + scrollX, y: box.y + scrollY, width: box.width, height: box.height };
    };
    const before = await upload.evaluate(layout);
    await help.hover();
    await explanation.waitFor();
    assert.equal(await explanation.getAttribute("role"), "tooltip");
    assert.deepEqual(await upload.evaluate(layout), before, "opening a tooltip must not move the setting");
    assert.match(await explanation.innerText(), /Shared upload payload limit/);
    await explanation.hover();
    assert(await explanation.isVisible(), "tooltip stays open while its text is hovered");
    await page.getByRole("heading", { name: "Settings", exact: true }).hover();
    await explanation.waitFor({ state: "hidden" });
    await help.click();
    await page.getByRole("heading", { name: "Settings", exact: true }).hover();
    assert.equal(await help.getAttribute("aria-expanded"), "true", "click must keep help open after hover ends");
    await help.click();
    await explanation.waitFor({ state: "hidden" });
    await page.getByRole("heading", { name: "Settings", exact: true }).hover();
    await upload.focus();
    await help.focus();
    await explanation.waitFor();
    await page.keyboard.press("Escape");
    await explanation.waitFor({ state: "hidden" });
    await page.keyboard.press("Enter");
    await explanation.waitFor();
    await page.keyboard.press("Enter");
    await explanation.waitFor({ state: "hidden" });
    // Every actual setting input has a labelled info control and description.
    for (const input of await page.locator(".config-form input, .config-form select").all()) {
      const id = await input.getAttribute("aria-describedby");
      assert(id);
      assert.equal(await page.locator(`[aria-controls="${id}"]`).count(), 1);
      assert((await page.locator(`#${id}`).textContent() ?? "").length > 20);
    }
    // Preserve a draft across sync activity, display save failures, then success.
    await upload.fill("123");
    let fail = true;
    let submissions = 0;
    await page.route("**/api/configure", async route => {
      assert.equal(route.request().postDataJSON().UploadLimitKiB, 123);
      submissions++;
      await route.fulfill({ status: fail ? 400 : 200, body: fail ? "Cannot save test settings" : "OK" });
    });
    await page.getByTestId("config-save").click();
    await page.getByTestId("config-error").waitFor();
    assert.equal(await upload.inputValue(), "123");
    fail = false;
    await page.getByTestId("config-save").click();
    await page.getByTestId("config-saved").waitFor();
    assert.equal(submissions, 2);
    await page.getByTestId("settings-back").click();
    await page.getByTestId("main-page").waitFor();
    await page.goBack();
    await page.getByTestId("config-editor").waitFor();
    await page.reload();
    await page.getByTestId("connect-overlay").waitFor({ state: "hidden" });
    await upload.waitFor();
    assert.notEqual(await upload.inputValue(), "123", "leaving settings must discard the unpersisted draft");
    await page.goBack();
    await page.getByTestId("main-page").waitFor();
  } finally {
    await context.close();
  }
  const mobile = await browser.newContext({ viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true });
  try {
    const page = await mobile.newPage();
    await page.goto(base + "/#/settings");
    await page.getByTestId("connect-overlay").waitFor({ state: "hidden" });
    const help = page.getByRole("button", { name: "About Download directory", exact: true });
    const input = page.getByTestId("config-DownloadDirectory");
    await input.waitFor();
    const value = await input.inputValue();
    await help.tap();
    await page.locator("#help-DownloadDirectory").waitFor();
    assert.equal(await input.inputValue(), value, "tapping info must not change the setting");
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth));
    const popup = await page.locator("#help-DownloadDirectory").boundingBox();
    assert(popup && popup.x >= 0 && popup.x + popup.width <= 375 && popup.y >= 0 && popup.y + popup.height <= 812, "tooltip must fit the mobile viewport");
    await help.tap();
    await page.locator("#help-DownloadDirectory").waitFor({ state: "hidden" });
    const checkbox = page.getByTestId("config-AutoStart");
    const checked = await checkbox.isChecked();
    await page.getByRole("button", { name: "About Auto start", exact: true }).tap();
    await page.locator("#help-AutoStart").waitFor();
    assert.equal(await checkbox.isChecked(), checked, "info must not toggle an adjacent checkbox");
    await page.getByRole("heading", { name: "Settings", exact: true }).tap();
    await page.locator("#help-AutoStart").waitFor({ state: "hidden" });
  } finally {
    await mobile.close();
  }
  console.log("PASS: dedicated settings page, history/reload, per-setting hover/click/keyboard/touch help and save feedback");
}
