import { expect, test } from "@playwright/test";

test("renders the ROOM marketing page", async ({ page }) => {
  await page.goto("/");

  await expect(
    page.getByRole("heading", {
      name: /recursive repo improvement with a live control surface/i,
    }),
  ).toBeVisible();

  await expect(
    page.getByRole("link", { name: /inspect the github repo/i }),
  ).toHaveAttribute("href", "https://github.com/jcpsimmons/room");

  await expect(
    page.locator("#signal").getByText("cold starts only", { exact: true }),
  ).toBeVisible();
  await expect(
    page.locator("#signal").getByText("forced pivots", { exact: true }),
  ).toBeVisible();
});
