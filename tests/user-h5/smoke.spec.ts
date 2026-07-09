import { expect, test } from "@playwright/test";

test("user H5 shell renders usable content", async ({ page }) => {
  const pageErrors: string[] = [];
  page.on("pageerror", error => pageErrors.push(error.message));

  await page.goto("/");
  const app = page.locator("#app");
  await expect(app).toBeVisible();
  await expect.poll(async () => (await app.innerText()).trim().length).toBeGreaterThan(5);
  expect(pageErrors).toEqual([]);
});
