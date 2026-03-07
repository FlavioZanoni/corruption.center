// ─── Core graph types ────────────────────────────────────────────────────────

export type NodeType = "politician" | "scandal" | "organization" | "legal_proceeding"

export type EdgeType =
  | "INVOLVED_IN"
  | "DEFENDANT_IN"
  | "MEMBER_OF"
  | "IMPLICATED_IN"
  | "INVESTIGATES"
  | "RELATED_TO"
  | "SUPPORTS"

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
  source_urls?: string[]
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

// ─── Timeline ────────────────────────────────────────────────────────────────

export interface TimelineResponse extends GraphResponse {
  from: string
  to: string
}

// ─── Filter state types (used in store) ──────────────────────────────────────

export interface NodeTypeFilters {
  politician: boolean
  scandal: boolean
  organization: boolean
  legal_proceeding: boolean
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
