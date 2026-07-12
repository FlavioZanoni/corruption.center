import type { MetadataRoute } from "next"
import { fetchAllIds, siteUrl } from "@/lib/seo"

// Regenerate the sitemap every 24h. `sitemap.ts` is a cached Route Handler, so
// without this it would be frozen at build time and never pick up new entities.
export const revalidate = 86_400

/**
 * Sitemaps are capped at 50.000 URLs by the sitemap protocol (and by Google).
 * We are well under it today (~3.3k politicians, a handful of scandals), so a
 * single /sitemap.xml is correct and keeps robots.txt pointing at a real file.
 *
 * If we ever cross this line, split with `generateSitemaps()`: it emits
 * /sitemap/[id].xml and does NOT create a /sitemap.xml index for you, so the
 * robots.ts `sitemap` field has to be updated to list each shard at the same
 * time. Until then, truncating with a loud warning beats silently shipping an
 * oversized sitemap that crawlers reject wholesale.
 */
const MAX_SITEMAP_URLS = 50_000

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const lastModified = new Date()

  const staticRoutes: MetadataRoute.Sitemap = [
    {
      url: siteUrl("/"),
      lastModified,
      changeFrequency: "daily",
      priority: 1,
    },
    {
      url: siteUrl("/politicos"),
      lastModified,
      changeFrequency: "daily",
      priority: 0.9,
    },
    {
      url: siteUrl("/processos"),
      lastModified,
      changeFrequency: "daily",
      priority: 0.9,
    },
    {
      url: siteUrl("/sancoes"),
      lastModified,
      changeFrequency: "daily",
      priority: 0.9,
    },
    {
      url: siteUrl("/metodologia"),
      lastModified,
      changeFrequency: "monthly",
      priority: 0.5,
    },
  ]

  // Both endpoints are enumerated concurrently. Each one degrades to an empty
  // list on failure, so a missing /scandals endpoint (it is still being added)
  // costs us those URLs instead of breaking the build.
  // The browse pages paginate client-side, so a crawler cannot walk them: the
  // sitemap is the ONLY way a case or a sanction page is ever discovered.
  //
  // /pessoa is deliberately absent. Those are private citizens whose name a court
  // publication printed; they are noindex and robots-disallowed (see robots.ts).
  // Listing them here would be asking a crawler to index the one thing we have
  // decided it must not.
  const [politicianIds, scandalIds, proceedingIds] = await Promise.all([
    fetchAllIds("/api/v1/politicians"),
    fetchAllIds("/api/v1/scandals"),
    fetchAllIds("/api/v1/proceedings"),
  ])

  const politicianRoutes: MetadataRoute.Sitemap = politicianIds.map((id) => ({
    url: siteUrl(`/politico/${id}`),
    lastModified,
    changeFrequency: "weekly",
    priority: 0.7,
  }))

  // Scandals are the editorial heart of the site: rank them above profiles.
  const scandalRoutes: MetadataRoute.Sitemap = scandalIds.map((id) => ({
    url: siteUrl(`/escandalo/${id}`),
    lastModified,
    changeFrequency: "weekly",
    priority: 0.8,
  }))

  // A court case IS the public record: the case number, the court and the class are
  // all published by the CNJ. These are safe to index and are what a reader looking
  // for a case will actually search for.
  const proceedingRoutes: MetadataRoute.Sitemap = proceedingIds.map((id) => ({
    url: siteUrl(`/processo/${id}`),
    lastModified,
    changeFrequency: "weekly",
    priority: 0.6,
  }))

  const routes = [
    ...staticRoutes,
    ...scandalRoutes,
    ...politicianRoutes,
    ...proceedingRoutes,
  ]

  console.log(
    `[sitemap] ${routes.length} URLs (${staticRoutes.length} static, ${scandalRoutes.length} scandals, ${politicianRoutes.length} politicians, ${proceedingRoutes.length} proceedings)`,
  )

  if (routes.length > MAX_SITEMAP_URLS) {
    console.warn(
      `[sitemap] ${routes.length} URLs exceeds the ${MAX_SITEMAP_URLS} limit, truncating. Split with generateSitemaps().`,
    )
    return routes.slice(0, MAX_SITEMAP_URLS)
  }

  return routes
}
