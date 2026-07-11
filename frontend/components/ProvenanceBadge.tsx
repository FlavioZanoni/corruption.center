// Provenance rendering for every link (edge) shown to a reader.
//
// This is a legal requirement of the project, not decoration: an official source
// tells us *what* happened, but often not *with whom*. Tying a record to a named
// politician is an inference of ours, and the reader must always be able to see
// how it was established. See /metodologia, section 03.
//
// No hooks, no browser APIs: safe to render from a Server Component (the entity
// pages) and from the client DetailPanel alike.

import { AlertTriangle } from "lucide-react"
import { CONFIDENCE_SIGNAL_LABELS, sourceLabel } from "@/lib/entity-labels"

// Legally required label for any node/edge not yet confirmed by a human
// (see docs/legal_compliance.md - "unconfirmed - pending review" discipline).
export function ReviewBadge() {
  return (
    <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-sm border border-[#ccaa22]/40 bg-[#ccaa22]/10">
      <AlertTriangle size={9} strokeWidth={1.5} className="text-[#ccaa22] shrink-0" />
      <span className="text-[8px] font-mono uppercase tracking-wider text-[#ccaa22] leading-none">
        não confirmado - pendente de revisão
      </span>
    </span>
  )
}

export function ProvenanceBadge({
  properties,
}: {
  properties: Record<string, unknown>
}) {
  const source = properties.source as string | undefined

  if (source === "backoffice_review") {
    return (
      <span
        className="mt-1.5 inline-block text-[9px] font-mono uppercase tracking-wider text-[#7aa87a]"
        title="Vínculo criado por revisão humana a partir de registro oficial."
      >
        ✓ Confirmado por revisão humana
      </span>
    )
  }

  const confidence = properties.confidence as number | undefined
  const signals = Array.isArray(properties.confidence_signals)
    ? (properties.confidence_signals as string[])
    : []
  const reasons = signals
    .map((s) => CONFIDENCE_SIGNAL_LABELS[s] ?? s)
    .join(" · ")

  // No confidence score recorded: the most we can honestly state is which
  // official dataset the link came from. A zero is treated as "unscored", never
  // as "0% sure": a DJEN defendant edge carries no score because DJEN names the
  // party outright, and nothing in the scoring policy can legitimately produce a
  // stored zero (an auto-linked edge is >= 0.90 by definition).
  if (typeof confidence !== "number" || confidence <= 0) {
    if (!source) return null
    return (
      <span
        className="mt-1.5 inline-block text-[9px] font-mono uppercase tracking-wider text-text-muted"
        title="Vínculo importado de um registro oficial."
      >
        Origem: {sourceLabel(source)}
      </span>
    )
  }

  return (
    <span
      className="mt-1.5 inline-block text-[9px] font-mono uppercase tracking-wider text-text-muted"
      title={
        reasons
          ? `Identificação automática. Evidência: ${reasons}.`
          : "Identificação automática a partir de registro oficial."
      }
    >
      Identificação: {Math.round(confidence * 100)}%
      {reasons ? ` · ${reasons}` : ""}
      {source ? ` · ${sourceLabel(source)}` : ""}
    </span>
  )
}
