import { test, expect, request as playwrightRequest } from "@playwright/test";
import { API_URL } from "../playwright.config";

// Resolve a live id per type from the API so nothing is hardcoded.
async function firstIdOfType(q: string, type: string): Promise<string> {
  const ctx = await playwrightRequest.newContext();
  const res = await ctx.get(
    `${API_URL}/api/v1/search?q=${encodeURIComponent(q)}&type=${type}`,
  );
  expect(res.ok()).toBeTruthy();
  const nodes: { id: string }[] = (await res.json()).nodes ?? [];
  await ctx.dispose();
  expect(nodes.length, `expected a ${type} for query ${q}`).toBeGreaterThan(0);
  return nodes[0].id;
}

test.describe("entity detail pages", () => {
  test("politician page renders and never labels anyone 'Réu em'", async ({ page }) => {
    const id = await firstIdOfType("lula", "politician");
    const res = await page.goto(`/politico/${id}`);
    expect(res?.status()).toBe(200);
    await expect(page.locator("h1")).not.toBeEmpty();
    // The edge type must render from outcome ("Parte citada em"/"Citado em"),
    // never the bare defendant assertion.
    await expect(page.getByText(/\bréu em\b/i)).toHaveCount(0);
  });

  test("organization page renders", async ({ page }) => {
    const id = await firstIdOfType("odebrecht", "organization");
    const res = await page.goto(`/organizacao/${id}`);
    expect(res?.status()).toBe(200);
    await expect(page.locator("h1")).not.toBeEmpty();
  });

  test("sanction detail page renders with a source link", async ({ page }) => {
    const id = await firstIdOfType("ceis", "sanction");
    const res = await page.goto(`/sancao/${encodeURIComponent(id)}`);
    expect(res?.status()).toBe(200);
    // Every sanction node requires a source_url (legal compliance) — the page
    // must surface the official deep-link.
    await expect(page.locator('a[href^="http"]').first()).toBeVisible();
  });
});

test.describe("browse pages", () => {
  for (const { path, heading } of [
    { path: "/politicos", heading: /políticos/i },
    { path: "/sancoes", heading: /sanções/i },
    { path: "/processos", heading: /processos/i },
  ]) {
    test(`${path} renders its list`, async ({ page }) => {
      const res = await page.goto(path);
      expect(res?.status()).toBe(200);
      await expect(page.getByRole("heading", { name: heading }).first()).toBeVisible();
      await expect(
        page.getByText(/application error|unhandled runtime error/i),
      ).toHaveCount(0);
    });
  }
});
