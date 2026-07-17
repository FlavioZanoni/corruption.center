import { test, expect } from "@playwright/test";

const DETAIL_ROUTE =
  /\/(pessoa|politico|organizacao|escandalo|processo|sancao)\/[^/]+$/;

test.describe("search", () => {
  test("typing a common name shows results and selecting one opens its page", async ({
    page,
  }) => {
    await page.goto("/");
    const search = page.getByRole("combobox", {
      name: /buscar político, escândalo ou organização/i,
    });

    // "silva" is the most common Brazilian surname — there is always a hit.
    await search.fill("silva");

    // Results render as role=option inside the listbox; wait for the first one.
    const firstResult = page.getByRole("option").first();
    await expect(firstResult).toBeVisible();
    expect(await page.getByRole("option").count()).toBeGreaterThan(0);

    await firstResult.click();

    // Selecting a result routes to that entity's detail page.
    await expect(page).toHaveURL(DETAIL_ROUTE);
  });

  test("a query with no plausible match shows the empty state, not an error", async ({
    page,
  }) => {
    await page.goto("/");
    const search = page.getByRole("combobox", {
      name: /buscar político, escândalo ou organização/i,
    });
    await search.fill("zzzzxqjunkstring");

    const listbox = page.getByRole("listbox");
    await expect(listbox).toContainText(/nenhum resultado encontrado/i);
    // The error branch ("Erro ao buscar") must NOT appear for a normal miss.
    await expect(listbox).not.toContainText(/erro ao buscar/i);
  });
});
