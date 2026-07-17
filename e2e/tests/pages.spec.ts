import { test, expect } from "@playwright/test";

// Static/content pages should render server-side without a client error boundary.
for (const path of ["/metodologia"]) {
  test(`${path} renders`, async ({ page }) => {
    const res = await page.goto(path);
    expect(res?.status(), `${path} should return 200`).toBe(200);
    // Next's error overlay / boundary would surface this text; it must be absent.
    await expect(page.getByText(/application error|unhandled runtime error/i)).toHaveCount(0);
    await expect(page.locator("body")).not.toBeEmpty();
  });
}
