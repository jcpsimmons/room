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
