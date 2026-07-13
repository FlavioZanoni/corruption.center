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

// ─── Person ───────────────────────────────────────────────────────────────────

export interface PersonRecord {
  id: string
  name: string
  cpf?: string
  provenance_source?: string
  provenance_link?: string
  provenance_tribunal?: string
  // A name-only node (no full CPF): its records may belong to more than one real
  // person of the same name. The profile must not present them as one history.
  ambiguous?: boolean
}

export interface PersonDetail {
  person: PersonRecord
  connections: GraphResponse
}

export function fetchPersonDetail(id: string): Promise<PersonDetail | null> {
  return fetchEntity<PersonDetail>(`/api/v1/person/${encodeURIComponent(id)}`)
}

// ─── Organization ─────────────────────────────────────────────────────────────

export interface OrganizationRecord {
  id: string
  name?: string
  cnpj?: string
  razao_social?: string
  provenance_source?: string
  provenance_link?: string
}

export interface OrganizationDetail {
  organization: OrganizationRecord
  connections: GraphResponse
}

export function fetchOrganizationDetail(
  id: string,
): Promise<OrganizationDetail | null> {
  return fetchEntity<OrganizationDetail>(
    `/api/v1/organization/${encodeURIComponent(id)}`,
  )
}

// ─── Legal Proceeding ─────────────────────────────────────────────────────────

export interface DefendantInfo {
  id: string
  name: string
  type: "Politician" | "Person" | "Organization"
  outcome: string
  source: string
  properties?: {
    outcome?: string
    outcome_source?: string
    outcome_evidence_url?: string
    outcome_recorded_at?: string
    source?: string
  }
}

export interface LegalProceedingRecord {
  id: string
  case_number: string
  court?: string
  status?: string
  phase?: string
  has_conviction: boolean
  polled: boolean
  type?: string
  assuntos?: string[]
  url?: string
  scandal?: {
    id: string
    name: string
  } | null
  defendants: DefendantInfo[]
}

export function fetchLegalProceedingDetail(
  id: string,
): Promise<LegalProceedingRecord | null> {
  return fetchEntity<LegalProceedingRecord>(
    `/api/v1/proceeding/${encodeURIComponent(id)}`,
  )
}

// ─── Sanction ─────────────────────────────────────────────────────────────────
// Sanctions are the largest thing in the graph (64,779 nodes) and had no page at
// all, so selecting one in search navigated nowhere — the click just closed the
// dropdown.

export interface SanctionRecord {
  id: string
  registry: string
  sanction_type: string
  organ: string
  date_start: string | null
  date_end: string | null
  process_ref: string
  source_url: string
}

export interface SanctionedPartyRecord {
  id: string
  name?: string
  cnpj?: string
  cpf?: string
  type: string // "Person" | "Organization" | "Politician"
}

export interface SanctionDetail {
  sanction: SanctionRecord
  parties: SanctionedPartyRecord[]
}

export function fetchSanctionDetail(id: string): Promise<SanctionDetail | null> {
  return fetchEntity<SanctionDetail>(`/api/v1/sanction/${encodeURIComponent(id)}`)
}
