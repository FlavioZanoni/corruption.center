// ─── SEO helpers ──────────────────────────────────────────────────────────────
// Shared by app/sitemap.ts, app/robots.ts and app/layout.tsx.
//
// Two different API base URLs exist on purpose:
//
//   NEXT_PUBLIC_API_URL  browser-reachable base (baked into the client bundle)
//   API_URL              server-reachable base, used by server components,
//                        route handlers and the sitemap. Inside Docker the
//                        frontend container cannot reach "localhost:8080", it
//                        has to go through the compose service name (api:8080).
//
// Anything that runs on the server should prefer API_URL and fall back to the
// public one, which is what SERVER_API_BASE does.

/** Public origin of the site, used for canonical URLs and sitemap entries. */
export const SITE_URL = normalizeBase(
  process.env.NEXT_PUBLIC_SITE_URL || "http://localhost:3000",
)

/** API base for server-side fetches (sitemap, server components). */
export const SERVER_API_BASE = normalizeBase(
  process.env.API_URL ||
    process.env.NEXT_PUBLIC_API_URL ||
    "http://localhost:8080",
)

function normalizeBase(value: string): string {
  return value.trim().replace(/\/+$/, "") // tolerate a configured trailing slash
}

/** Absolute URL on the public site, for sitemap / canonical use. */
export function siteUrl(path: string): string {
  return `${SITE_URL}${path.startsWith("/") ? path : `/${path}`}`
}

// ─── Paginated API enumeration ────────────────────────────────────────────────

/** Shape returned by the paginated list endpoints (politicians, scandals). */
export interface PagedResponse<T> {
  items: T[]
  page: number
  page_size: number
  total: number
}

/** We only ever need the id to build a URL. */
interface HasId {
  id?: string | null
}

/** Per-request budget. A dead API must never hang the build. */
const FETCH_TIMEOUT_MS = 10_000
const PAGE_SIZE = 100
/** Belt and braces: stops a misbehaving `total` from looping forever. */
const MAX_PAGES = 1_000

/**
 * Fetch one page. Returns null on any failure (timeout, 404, non-JSON, network
 * error) so callers can degrade instead of throwing: the scandals endpoint in
 * particular may not exist yet in a given deployment.
 */
async function fetchPage(
  path: string,
  page: number,
): Promise<PagedResponse<HasId> | null> {
  const url = `${SERVER_API_BASE}${path}?page=${page}&page_size=${PAGE_SIZE}`
  try {
    const res = await fetch(url, {
      headers: { accept: "application/json" },
      signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
    })
    if (!res.ok) {
      console.warn(`[sitemap] ${path} page ${page}: HTTP ${res.status}`)
      return null
    }
    return (await res.json()) as PagedResponse<HasId>
  } catch (err) {
    console.warn(
      `[sitemap] ${path} page ${page} failed: ${err instanceof Error ? err.message : String(err)}`,
    )
    return null
  }
}

/**
 * Page through a list endpoint and collect every id.
 *
 * Degrades gracefully by design: if the endpoint is missing or the API is down
 * we return whatever we managed to collect (possibly nothing) rather than
 * throwing, so a partial sitemap still builds. Losing URLs from one sitemap
 * regeneration is recoverable; failing the production build is not.
 */
export async function fetchAllIds(path: string): Promise<string[]> {
  // A Set, not an array: the sync workers insert rows while we are paging, which
  // shifts offsets under us and can hand back the same row on two pages. A
  // sitemap with duplicate <loc> entries is invalid, so dedupe as we go.
  const ids = new Set<string>()

  for (let page = 1; page <= MAX_PAGES; page++) {
    const data = await fetchPage(path, page)
    if (!data || !Array.isArray(data.items) || data.items.length === 0) break

    for (const item of data.items) {
      if (item?.id) ids.add(item.id)
    }

    // Stop as soon as we have everything the API says exists. Compare against
    // the page number rather than ids.size, since dedupe can leave us short of
    // `total` forever and spin until MAX_PAGES.
    if (typeof data.total === "number" && data.total > 0) {
      if (page >= Math.ceil(data.total / PAGE_SIZE)) break
    }
    if (data.items.length < PAGE_SIZE) break
  }

  return [...ids]
}
