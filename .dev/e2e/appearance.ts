import assert from "node:assert/strict";
import type { Browser, Page } from "playwright";

async function expectTheme(page: Page, theme: string) {
  await page.waitForFunction(
    (expected) => document.documentElement.dataset.theme === expected,
    theme,
  );
  assert.equal(
    await page.locator("html").evaluate((el) =>
      getComputedStyle(el).colorScheme
    ),
    theme,
  );
}

export async function verifyAppearance(browser: Browser, base: string) {
  const context = await browser.newContext({ colorScheme: "dark" });
  try {
    const page = await context.newPage();
    let configureRequests = 0;
    page.on("request", (request) => {
      if (request.url().includes("/api/configure")) configureRequests++;
    });
    await page.goto(base);
    await page.getByTestId("connect-overlay").waitFor({ state: "hidden" });
    await expectTheme(page, "dark");
    await page.getByTestId("status-settings").click();
    const theme = page.getByRole("combobox", { name: "Theme", exact: true });
    assert.equal(await theme.inputValue(), "system");
    await page.emulateMedia({ colorScheme: "light" });
    await expectTheme(page, "light");
    // Exercise the native select with keyboard, including overriding the OS.
    await theme.focus();
    await page.keyboard.press("End");
    await page.keyboard.press("Enter");
    assert.equal(await theme.inputValue(), "dark");
    await expectTheme(page, "dark");
    await page.getByTestId("config-cancel").click();
    await page.reload();
    await expectTheme(page, "dark");
    await page.getByTestId("connect-overlay").waitFor({ state: "hidden" });
    await page.getByTestId("status-settings").click();
    assert.equal(await theme.inputValue(), "dark");
    await theme.selectOption("light");
    await page.emulateMedia({ colorScheme: "dark" });
    await expectTheme(page, "light");
    const otherTab = await context.newPage();
    await otherTab.goto(base);
    await expectTheme(otherTab, "light");
    await theme.selectOption("dark");
    await expectTheme(otherTab, "dark");
    await theme.selectOption("system");
    await page.emulateMedia({ colorScheme: "light" });
    await expectTheme(page, "light");
    await page.reload();
    await expectTheme(page, "light");
    assert.equal(
      configureRequests,
      0,
      "appearance must not change server config",
    );
    // Mobile settings should not force horizontal scrolling.
    await page.setViewportSize({ width: 375, height: 812 });
    await page.getByTestId("connect-overlay").waitFor({ state: "hidden" });
    await page.getByTestId("status-settings").click();
    assert(await theme.isVisible());
    assert(
      await page.evaluate(() =>
        document.documentElement.scrollWidth <= window.innerWidth
      ),
    );
  } finally {
    await context.close();
  }
  // A denied storage API must still allow following the OS and local overrides.
  const denied = await browser.newContext({ colorScheme: "dark" });
  try {
    await denied.addInitScript(() => {
      const get = Storage.prototype.getItem;
      const set = Storage.prototype.setItem;
      Storage.prototype.getItem = function (key) {
        if (key === "tcTheme") {
          throw new DOMException("Storage denied", "SecurityError");
        }
        return get.call(this, key);
      };
      Storage.prototype.setItem = function (key, value) {
        if (key === "tcTheme") {
          throw new DOMException("Storage denied", "SecurityError");
        }
        return set.call(this, key, value);
      };
    });
    const page = await denied.newPage();
    await page.goto(base);
    await expectTheme(page, "dark");
    await page.getByTestId("connect-overlay").waitFor({ state: "hidden" });
    await page.getByTestId("status-settings").click();
    await page.getByRole("combobox", { name: "Theme", exact: true })
      .selectOption("light");
    await expectTheme(page, "light");
  } finally {
    await denied.close();
  }
  console.log(
    "PASS: system/light/dark appearance, persistence, tabs, keyboard and mobile",
  );
}
