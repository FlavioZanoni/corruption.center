import { test, expect } from "@playwright/test";

test.describe("home", () => {
  test("loads with the expected title and a working search box", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveTitle(/corruption\.center/i);

    // The search bar is the primary entry point; it must be present and focusable.
    const search = page.getByRole("combobox", {
      name: /buscar político, escândalo ou organização/i,
    });
    await expect(search).toBeVisible();
    await expect(search).toBeEditable();
  });
});
