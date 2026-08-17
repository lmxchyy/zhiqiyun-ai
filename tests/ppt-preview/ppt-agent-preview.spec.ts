import { expect, test } from "@playwright/test";
import { golden2PreviewFixture } from "./golden-2-preview-fixture.mjs";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(fixture => { (window as unknown as { __PPT_AGENT_GOLDEN_2__: unknown }).__PPT_AGENT_GOLDEN_2__ = fixture; }, golden2PreviewFixture());
  await page.goto("/admin/tests/fixtures/ppt-agent-preview.html");
  await expect(page.locator("[data-preview-workspace]")).toBeVisible();
});

test("Golden 2 preview preserves authoritative geometry and navigation", async ({ page }) => {
  const canvas = page.locator(".ppt-agent-main-canvas .ppt-agent-slide-canvas");
  const title = canvas.locator('[data-preview-element-id="element_slide_golden_2_1_title"]');
  await expect(title).toHaveCSS("left", "72px");
  await expect(title).toHaveCSS("top", "132px");
  await expect(title).toHaveCSS("width", "816px");
  await expect(page.locator("[data-preview-thumbnail]")).toHaveCount(8);
  await expect(page.getByText("1 / 8", { exact: true })).toBeVisible();

  await page.locator('[data-preview-thumbnail="slide_golden_2_4"]').click();
  await expect(page.getByText("4 / 8", { exact: true })).toBeVisible();
  await expect(page.locator('.ppt-agent-main-canvas img[alt="Electric vehicle market"]')).toBeVisible();
  await expect(page.getByText("EV Market Report", { exact: true })).toBeVisible();
  await expect(page.getByText("SOURCE_SUPPORTED", { exact: true })).toBeVisible();

  await page.locator("[data-preview-workspace]").press("ArrowRight");
  await expect(page.getByText("5 / 8", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "上一页" }).click();
  await expect(page.getByText("4 / 8", { exact: true })).toBeVisible();
});

for (const [slide, name] of [[1, "cover"], [3, "bullets"], [4, "image"], [5, "two-column"], [6, "highlight"], [8, "closing"]] as const) {
  test(`Golden 2 ${name} preview visual regression`, async ({ page }) => {
    await page.locator(`[data-preview-thumbnail="slide_golden_2_${slide}"]`).click();
    const preview = page.locator(".ppt-agent-main-canvas .ppt-agent-slide-viewport");
    await expect(preview).toHaveScreenshot(`golden-2-${name}.png`, { animations: "disabled" });
  });
}
