import { expect, test } from "@playwright/test";

test("renders the ROOM marketing page", async ({ page }) => {
  await page.goto("/");

  await expect(
    page.getByRole("heading", {
      name: /repetitively obsessively optimize me/i,
    }),
  ).toBeVisible();

  await expect(
    page.getByRole("link", { name: /inspect the github repo/i }),
  ).toHaveAttribute("href", "https://github.com/jcpsimmons/room");

  await expect(
    page.getByRole("link", {
      name: /example build\s+room-signal-garden\.vercel\.app\s+built from one prompt/i,
    }),
  ).toHaveAttribute("href", "https://room-signal-garden.vercel.app/");

  await expect(
    page.locator("#signal").getByText("cold starts only", { exact: true }),
  ).toBeVisible();
  await expect(
    page.locator("#signal").getByText("forced pivots", { exact: true }),
  ).toBeVisible();

  await expect(page.locator("#contract")).toContainText(
    "ROOM stays local, keeps the tape, and refuses to hallucinate momentum.",
  );
  await expect(page.locator("#contract")).toContainText("Inputs");
  await expect(page.locator("#contract")).toContainText(
    "Prompt, stdout, stderr, result, diff, and commit choice.",
  );

  await expect(page.getByTestId("embed-iframe")).toHaveAttribute(
    "src",
    "https://open.spotify.com/embed/album/1dedNPacu6iCzgAblljBCr?utm_source=generator",
  );
});

test("honors reduced motion and exposes a skip link", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/");

  await page.keyboard.press("Tab");
  await expect(
    page.getByRole("link", { name: /skip to signal path/i }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: /skip to signal path/i }),
  ).toHaveAttribute("href", "#signal");

  await expect(page.locator(".ticker__rail")).toHaveCSS(
    "animation-name",
    "none",
  );
  await expect(page.locator(".scope-display__orbit--outer")).toHaveCSS(
    "animation-name",
    "none",
  );
  await expect(page.locator(".meter-bank__bar").first()).toHaveCSS(
    "animation-name",
    "none",
  );

  const counter = page.locator(".scope-display__core strong");
  await expect(counter).toHaveText("0000");
  await page.waitForTimeout(750);
  await expect(counter).toHaveText("0000");
});
