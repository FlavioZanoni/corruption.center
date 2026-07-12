import type { MetadataRoute } from "next"
import { siteUrl } from "@/lib/seo"

export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: "*",
      allow: "/",
      // /backoffice is the authenticated editor surface and /api is machine
      // output: neither belongs in a search index.
      //
      // /pessoa is the private-citizen surface: 19,416 people whose only link to
      // this database is that a court publication printed their name. The pages
      // carry robots: noindex too; this is the belt to that page's braces, and it
      // keeps a crawler from spending its budget on them at all.
      disallow: ["/backoffice", "/api", "/pessoa"],
    },
    sitemap: siteUrl("/sitemap.xml"),
    host: siteUrl("/").replace(/\/$/, ""),
  }
}
