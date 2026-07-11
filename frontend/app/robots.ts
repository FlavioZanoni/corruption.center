import type { MetadataRoute } from "next"
import { siteUrl } from "@/lib/seo"

export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: "*",
      allow: "/",
      // /backoffice is the authenticated editor surface and /api is machine
      // output: neither belongs in a search index.
      disallow: ["/backoffice", "/api"],
    },
    sitemap: siteUrl("/sitemap.xml"),
    host: siteUrl("/").replace(/\/$/, ""),
  }
}
