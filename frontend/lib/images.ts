// Photos are hotlinked from the official sources that publish them (Câmara,
// Senado, Wikimedia Commons, TSE). next/image refuses any host that is not
// declared in next.config, so the allow-list lives here and is consumed BOTH by
// next.config.ts (to configure remotePatterns) and by the entity pages (to check
// a URL before handing it to <Image>, so an unexpected host degrades to the
// placeholder instead of crashing the page).

export const ALLOWED_IMAGE_HOST_SUFFIXES = [
  "camara.leg.br",
  "senado.leg.br",
  "wikimedia.org",
  "wikipedia.org",
  "tse.jus.br",
  "gov.br",
] as const

export function isAllowedImageUrl(url: string | undefined): url is string {
  if (!url) return false
  let host: string
  try {
    const parsed = new URL(url)
    if (parsed.protocol !== "https:") return false
    host = parsed.hostname.toLowerCase()
  } catch {
    return false
  }
  return ALLOWED_IMAGE_HOST_SUFFIXES.some(
    (suffix) => host === suffix || host.endsWith(`.${suffix}`),
  )
}
