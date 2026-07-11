// ─── Core graph types ────────────────────────────────────────────────────────

export type NodeType =
  | "politician"
  | "person"
  | "scandal"
  | "organization"
  | "legal_proceeding"
  | "source"
  | "sanction"

export type EdgeType =
  | "INVOLVED_IN"
  | "DEFENDANT_IN"
  | "MEMBER_OF"
  | "CONTROLS"
  | "OWNED_BY"
  | "IMPLICATED_IN"
  | "INVESTIGATES"
  | "RELATED_TO"
  | "SUPPORTS"
  | "SANCTIONED_IN"

export type EdgeStatus = "convicted" | "indicted" | "cited" | "membership"

export interface GraphNode {
  id: string
  type: NodeType
  label: string
  properties: Record<string, unknown>
}

export interface GraphEdge {
  id: string
  from: string
  to: string
  type: string
  properties: Record<string, unknown>
}

export interface GraphResponse {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

// ─── Entity-specific property shapes ─────────────────────────────────────────

export interface PoliticianProperties {
  name: string
  cpf_hash?: string
  party?: string
  state?: string
  role?: string
  photo_url?: string
  photo_source?: string
  photo_attribution?: string
  birth_date?: string
  name_aliases?: string[]
  source_urls?: string[]
  confidence?: number
  start_date?: string
  end_date?: string
}

export interface ScandalProperties {
  name: string
  description?: string
  start_date?: string
  end_date?: string
  status?: string
  source_urls?: string[]
  tags?: string[]
  total_amount?: number
  currency?: string
}

export interface OrganizationProperties {
  name: string
  cnpj?: string
  type?: string
  state?: string
  sector?: string
  logo_url?: string
  photo_url?: string
  photo_source?: string
  photo_attribution?: string
  source_urls?: string[]
}

// Sanction node: an official registry entry (CEIS, CNEP, CEPIM, TCU
// inelegibility, etc.) that a Politician/Person/Organization is SANCTIONED_IN.
// source_url is mandatory: every sanction deep-links its authoritative record.
export interface SanctionProperties {
  registry?: string
  sanction_type?: string
  organ?: string
  date_start?: string
  date_end?: string
  process_ref?: string
  source_url: string
  // provenance flags surfaced by ingest workers
  provenance_source?: string
}

export interface LegalProceedingProperties {
  case_number: string
  court?: string
  status?: string
  start_date?: string
  judgment_date?: string
  outcome?: string
  source_urls?: string[]
}

// ─── Search ──────────────────────────────────────────────────────────────────

export interface SearchResult {
  id: string
  type: NodeType
  label: string
  snippet?: string
  properties: Record<string, unknown>
}

export interface SearchResponse {
  results: SearchResult[]
  total: number
}

// ─── Politician browse (paginated list) ──────────────────────────────────────

export interface PoliticianListItem {
  id: string
  name: string
  party_current: string
  role_current: string
  state: string
  photo_url?: string
  photo_attribution?: string
  sanction_count: number
  connection_count: number
  proceeding_count: number
}

export interface PoliticianListResponse {
  items: PoliticianListItem[]
  page: number
  page_size: number
  total: number
}

// ─── Timeline ────────────────────────────────────────────────────────────────

export interface TimelineResponse extends GraphResponse {
  from: string
  to: string
}

// ─── Filter state types (used in store) ──────────────────────────────────────

export interface NodeTypeFilters {
  politician: boolean
  person: boolean
  scandal: boolean
  organization: boolean
  legal_proceeding: boolean
  source: boolean
  sanction: boolean
}

export interface EdgeStatusFilters {
  convicted: boolean
  indicted: boolean
  cited: boolean
  membership: boolean
}

export type ReliabilityLevel = "high" | "medium" | "low"

export interface ReliabilityFilters {
  high: boolean
  medium: boolean
  low: boolean
}

export interface ActiveFilters {
  nodeTypes: NodeTypeFilters
  edgeStatus: EdgeStatusFilters
  reliability: ReliabilityFilters
}

export interface TimelineRange {
  from: number // year
  to: number   // year
}
