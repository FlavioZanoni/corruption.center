import { test, expect, request as playwrightRequest } from "@playwright/test";
import { API_URL } from "../playwright.config";

// Runs under the "mobile" project only (iPhone 13 viewport, touch enabled).
// The desktop suite covers behavior; this one asks: does the layout actually
// hold on a phone — no sideways scroll, and the primary flows still reachable.

// How far the page can scroll sideways beyond the viewport. 0 on a healthy
// page; a positive value means something overflows and the page pans.
async function horizontalOverflow(page: import("@playwright/test").Page) {
  return page.evaluate(
    () =>
      document.documentElement.scrollWidth -
      document.documentElement.clientWidth,
  );
}

async function firstPersonId(): Promise<string> {
  const ctx = await playwrightRequest.newContext();
  const res = await ctx.get(`${API_URL}/api/v1/search?q=silva`);
  const person = ((await res.json()).nodes ?? []).find(
    (n: { type?: string }) => n.type === "person",
  );
  await ctx.dispose();
  expect(person).toBeTruthy();
  return person.id as string;
}

test.describe("mobile layout", () => {
  for (const path of ["/", "/politicos", "/sancoes", "/processos", "/metodologia"]) {
    test(`${path} has no horizontal overflow`, async ({ page }) => {
      await page.goto(path);
      expect(await horizontalOverflow(page)).toBeLessThanOrEqual(0);
    });
  }

  test("person detail fits the viewport and shows the disclaimer", async ({
    page,
  }) => {
    const id = await firstPersonId();
    await page.goto(`/pessoa/${id}`);
    expect(await horizontalOverflow(page)).toBeLessThanOrEqual(0);
    await expect(
      page.getByText(/citação em um processo não é condenação/i),
    ).toBeVisible();
  });

  test("search works with the on-screen flow", async ({ page }) => {
    await page.goto("/");
    const search = page.getByRole("combobox", {
      name: /buscar político, escândalo ou organização/i,
    });
    await expect(search).toBeVisible();
    await search.tap();
    await search.fill("silva");
    const firstResult = page.getByRole("option").first();
    await expect(firstResult).toBeVisible();
    await firstResult.tap();
    await expect(page).toHaveURL(
      /\/(pessoa|politico|organizacao|escandalo|processo|sancao)\/[^/]+$/,
    );
    expect(await horizontalOverflow(page)).toBeLessThanOrEqual(0);
  });
});
