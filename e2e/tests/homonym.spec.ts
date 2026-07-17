import { test, expect, request as playwrightRequest } from "@playwright/test";
import { API_URL } from "../playwright.config";

type PersonRow = { id: string; ambiguous?: boolean };

// Resolve one name-only (ambiguous) and one CPF-keyed person from the live API,
// so the test never hardcodes ids a re-import could change. The two kinds
// surface on different search paths — the type-filtered person search returns
// the (older) name-only nodes first, the unfiltered search the CPF-keyed ones —
// so scan both result sets.
async function findPeople(): Promise<{ ambiguous: PersonRow; documented: PersonRow }> {
  const ctx = await playwrightRequest.newContext();
  const urls = [
    `${API_URL}/api/v1/search?q=silva&type=person`,
    `${API_URL}/api/v1/search?q=silva`,
  ];

  let ambiguous: PersonRow | undefined;
  let documented: PersonRow | undefined;
  for (const url of urls) {
    if (ambiguous && documented) break;
    const res = await ctx.get(url);
    expect(res.ok()).toBeTruthy();
    const nodes: { id: string; type: string }[] = (await res.json()).nodes ?? [];
    for (const n of nodes.filter((n) => n.type === "person")) {
      if (ambiguous && documented) break;
      const detail = await ctx.get(`${API_URL}/api/v1/person/${n.id}`);
      if (!detail.ok()) continue;
      const p = (await detail.json()).person as PersonRow;
      if (p.ambiguous && !ambiguous) ambiguous = p;
      if (!p.ambiguous && !documented) documented = p;
    }
  }
  await ctx.dispose();
  expect(ambiguous, "need a name-only person in the graph").toBeTruthy();
  expect(documented, "need a CPF-keyed person in the graph").toBeTruthy();
  return { ambiguous: ambiguous!, documented: documented! };
}

test.describe("homonym safety (name-only people)", () => {
  test("a name-only person shows the 'nome comum' banner; a CPF-keyed one does not", async ({
    page,
  }) => {
    const { ambiguous, documented } = await findPeople();
    const banner = /nome comum — pode corresponder a mais de uma pessoa/i;

    await page.goto(`/pessoa/${ambiguous.id}`);
    await expect(page.getByText(banner)).toBeVisible();

    await page.goto(`/pessoa/${documented.id}`);
    await expect(page.getByText(banner)).toHaveCount(0);
  });

  test("an ambiguous person's JSON-LD does not assert the merged records as one person's", async ({
    page,
  }) => {
    const { ambiguous } = await findPeople();
    await page.goto(`/pessoa/${ambiguous.id}`);

    const raw = await page
      .locator('script[type="application/ld+json"]')
      .first()
      .textContent();
    expect(raw).toBeTruthy();
    const ld = JSON.parse(raw!);
    // subjectOf is the machine-readable claim "this person is the subject of
    // these court records" — it must be absent when the node may merge several
    // real people who share a name.
    expect(ld.subjectOf).toBeUndefined();
    expect(ld["@type"]).toBe("Person");
  });
});
