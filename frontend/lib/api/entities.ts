// ─── Server-side entity fetchers ──────────────────────────────────────────────
// Used by the server-rendered, crawlable entity pages (/politico/[id],
// /escandalo/[id]). These run on the server only, so they go through
// SERVER_API_BASE (which prefers API_URL: inside Docker the frontend container
// cannot reach "localhost:8080").

import type { GraphResponse } from "@/lib/types"
import { SERVER_API_BASE } from "@/lib/seo"

// Cache entity pages for an hour (ISR): official records change slowly and this
// keeps crawler traffic off the Go API.
const REVALIDATE_SECONDS = 3600

// Returns null when the entity does not exist (HTTP 404); throws on any other
// failure so a broken API surfaces as an error, never as a bogus "not found".
async function fetchEntity<T>(path: string): Promise<T | null> {
  const res = await fetch(`${SERVER_API_BASE}${path}`, {
    next: { revalidate: REVALIDATE_SECONDS },
  })
  if (res.status === 404) return null
  if (!res.ok) {
    throw new Error(`API error ${res.status}: ${res.statusText} (${path})`)
  }
  return (await res.json()) as T
}

// ─── Politician ───────────────────────────────────────────────────────────────

export interface PoliticianRecord {
  id: string
  name: string
  cpf?: string
  name_aliases?: string[] | null
  party_current?: string
  role_current?: string
  state?: string
  birth_date?: string
  photo_url?: string
  photo_attribution?: string
  photo_source?: string
  tse_profile_urls?: string[] | null
  wikipedia_url?: string
  source_urls?: string[] | null
  active?: boolean
}

export interface PoliticianDetail {
  politician: PoliticianRecord
  connections: GraphResponse
}

export function fetchPoliticianDetail(
  id: string,
): Promise<PoliticianDetail | null> {
  return fetchEntity<PoliticianDetail>(
    `/api/v1/politician/${encodeURIComponent(id)}`,
  )
}

// ─── Scandal ──────────────────────────────────────────────────────────────────

export interface ScandalRecord {
  id: string
  name: string
  aliases?: string[] | null
  description?: string
  date_start?: string | null
  date_end?: string | null
  total_amount_brl?: number
  status?: string
  wikipedia_url?: string
  source_urls?: string[] | null
  tags?: string[] | null
}

export interface ScandalDetail {
  scandal: ScandalRecord
  connections: GraphResponse
}

export function fetchScandalDetail(id: string): Promise<ScandalDetail | null> {
  return fetchEntity<ScandalDetail>(
    `/api/v1/scandal/${encodeURIComponent(id)}`,
  )
}
