// Shared, user-facing pt-BR labels and formatters for entity rendering.
// Kept in one place so the server-rendered entity pages (/politico, /escandalo)
// and the client DetailPanel cannot drift apart on legally sensitive wording.

export const NODE_TYPE_LABELS: Record<string, string> = {
  politician: "Político",
  person: "Pessoa",
  scandal: "Escândalo",
  organization: "Organização",
  legal_proceeding: "Processo",
  source: "Fonte",
  sanction: "Sanção",
}

export const EDGE_TYPE_LABELS: Record<string, string> = {
  INVOLVED_IN: "Envolvido em",
  DEFENDANT_IN: "Parte citada em",
  MEMBER_OF: "Membro de",
  CONTROLS: "Controla",
  OWNED_BY: "Pertence a",
  IMPLICATED_IN: "Implicado em",
  INVESTIGATES: "Investigado por",
  RELATED_TO: "Relacionado a",
  SUPPORTS: "Apoia",
  SANCTIONED_IN: "Sancionado em",
}

// Status / enum values as published by the official sources.
export const VALUE_LABELS: Record<string, string> = {
  concluded: "Concluído",
  ongoing: "Em andamento",
  prescribed: "Prescrito",
  convicted: "Condenado",
  acquitted: "Absolvido",
  under_investigation: "Sob investigação",
  cited: "Citado",
  indicted: "Indiciado",
  pending: "Pendente",
  sentenced: "Sentenciado",
  appeal: "Em recurso",
  filed: "Autuado",
  criminal: "Criminal",
  administrative: "Administrativo",
  cpi: "CPI",
  membership: "Membro",
}

export function valueLabel(raw: string): string {
  return VALUE_LABELS[raw] ?? raw
}

// Map edge outcome values to pt-BR relationship labels.
// Used for DEFENDANT_IN edges to render the correct legal status based on outcome.
export const OUTCOME_LABELS: Record<string, string> = {
  cited: "Citado em",
  convicted: "Condenado em",
  acquitted: "Absolvido em",
  dismissed: "Processo extinto",
  indicted: "Denunciado em",
}

export function outcomeLabel(outcome: string | undefined): string {
  if (!outcome) return "Parte citada em"
  return OUTCOME_LABELS[outcome] ?? "Parte citada em"
}

// How a link between a person and a case/sanction came to exist. Official
// sources publish names, and often mask documents, so tying a record to a
// specific politician is an inference: the reader is told which one it was and
// how strong the evidence behind it is (backend: package matching).
export const CONFIDENCE_SIGNAL_LABELS: Record<string, string> = {
  full_document: "CPF/CNPJ completo na fonte",
  masked_cpf_middle6: "CPF parcial da fonte confere",
  exact_name: "nome idêntico",
  long_name: "nome completo (4+ partes)",
  ambiguous_match: "evidência serve a mais de uma pessoa",
}

// The official dataset a link was ingested from.
export const SOURCE_LABELS: Record<string, string> = {
  djen: "CNJ / DJEN",
  datajud: "CNJ / DataJud",
  camara: "Câmara dos Deputados",
  senado: "Senado Federal",
  tse: "TSE",
  tcu: "TCU",
  cgu: "CGU / Portal da Transparência",
  portal_transparencia: "CGU / Portal da Transparência",
  baseline_seed: "Registro público de referência",
  backoffice_review: "Revisão humana",
}

export function sourceLabel(raw: string): string {
  return SOURCE_LABELS[raw] ?? raw
}

// ─── Formatters ───────────────────────────────────────────────────────────────

const DATE_FORMATTER = new Intl.DateTimeFormat("pt-BR", {
  day: "numeric",
  month: "long",
  year: "numeric",
  timeZone: "UTC",
})

// Accepts YYYY, YYYY-MM, YYYY-MM-DD or a full RFC3339 timestamp.
export function formatDate(raw: string): string {
  if (!raw) return raw
  if (/^\d{4}$/.test(raw)) return raw
  const normalized =
    raw.length === 7 ? `${raw}-01` : raw.length === 10 ? raw : raw
  const d = new Date(normalized)
  if (isNaN(d.getTime())) return raw
  return DATE_FORMATTER.format(d)
}

// ISO date (YYYY-MM-DD) for machine-readable output (JSON-LD, <time>).
export function isoDate(raw: string): string | undefined {
  if (!raw) return undefined
  const d = new Date(/^\d{4}$/.test(raw) ? `${raw}-01-01` : raw)
  if (isNaN(d.getTime())) return undefined
  return d.toISOString().slice(0, 10)
}

const BRL_FORMATTER = new Intl.NumberFormat("pt-BR", {
  style: "currency",
  currency: "BRL",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
})

export function formatBRL(value: number): string {
  return BRL_FORMATTER.format(value)
}

// "1 processo" / "2 processos"
export function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`
}
