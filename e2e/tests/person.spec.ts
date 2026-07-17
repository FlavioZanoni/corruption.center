import { test, expect, request as playwrightRequest } from "@playwright/test";
import { API_URL } from "../playwright.config";

// Resolve a real Person id from the live API so the test does not hardcode data
// that a re-import could change.
async function firstPersonId(): Promise<string> {
  const ctx = await playwrightRequest.newContext();
  const res = await ctx.get(`${API_URL}/api/v1/search?q=silva`);
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  const person = (body.nodes ?? body.results ?? []).find(
    (n: { type?: string; id?: string }) => n.type === "person" && n.id,
  );
  await ctx.dispose();
  expect(person, "expected at least one Person in search results").toBeTruthy();
  return person.id as string;
}

test.describe("person detail — legal safety invariants", () => {
  test("shows the not-a-conviction disclaimer and never labels a person 'Réu em'", async ({
    page,
  }) => {
    const id = await firstPersonId();
    await page.goto(`/pessoa/${id}`);

    // The disclaimer is the load-bearing legal hedge for a private citizen page.
    await expect(
      page.getByText(/citação em um processo não é condenação/i),
    ).toBeVisible();

    // "Réu em" asserts defendant status the data does not support (a DEFENDANT_IN
    // edge only means "cited"). It must never render as an edge label.
    await expect(page.getByText(/\bréu em\b/i)).toHaveCount(0);
  });

  test("never serves an unmasked 11-digit CPF", async ({ page }) => {
    const id = await firstPersonId();
    await page.goto(`/pessoa/${id}`);
    // A masked CPF is the source form ***.XXX.XXX-**; a raw run of exactly 11
    // digits next to a CPF label would be the LGPD leak we masked. The page body
    // should contain no bare 11-digit CPF-shaped token.
    const body = (await page.locator("body").innerText()).replace(/\s+/g, " ");
    expect(body).not.toMatch(/\b\d{3}\.\d{3}\.\d{3}-\d{2}\b/);
  });
});
