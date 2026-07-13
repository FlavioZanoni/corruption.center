// Shared server-rendered building blocks for the crawlable entity pages
// (/politico/[id], /escandalo/[id], /pessoa/[id], /organizacao/[id], /processo/[id]).
//
// Everything here is plain HTML: no hooks, no client bundle, no graph canvas.
// A crawler with JavaScript disabled must be able to read every fact and every
// provenance label on the page.

import Link from "next/link"
import Image from "next/image"
import { ExternalLink, Ban, Scale, AlertTriangle, User, Users, Building2, FileText } from "lucide-react"
import type { GraphNode } from "@/lib/types"
import type { Connection } from "@/lib/entity-graph"
import { prop } from "@/lib/entity-graph"
import { NODE_COLORS } from "@/lib/constants"
import { isAllowedImageUrl } from "@/lib/images"
import { ProvenanceBadge, ReviewBadge } from "@/components/ProvenanceBadge"
import {
  EDGE_TYPE_LABELS,
  NODE_TYPE_LABELS,
  formatDate,
  valueLabel,
} from "@/lib/entity-labels"

// ─── Structured data ──────────────────────────────────────────────────────────

export function JsonLd({ data }: { data: Record<string, unknown> }) {
  return (
    <script
      type="application/ld+json"
      // Escapes "<" so a hostile string in the data cannot close the script tag.
      dangerouslySetInnerHTML={{
        __html: JSON.stringify(data).replace(/</g, "\\u003c"),
      }}
    />
  )
}

// ─── Entity routing ───────────────────────────────────────────────────────────

/**
 * Maps entity type to the appropriate page route.
 * Returns undefined for entity types that don't have a dedicated page.
 */
export function entityHref(type: string, id: string): string | undefined {
  const encoded = encodeURIComponent(id)
  switch (type.toLowerCase()) {
    case "politician":
      return `/politico/${encoded}`
    case "person":
      return `/pessoa/${encoded}`
    case "organization":
      return `/organizacao/${encoded}`
    case "legal_proceeding":
      return `/processo/${encoded}`
    case "scandal":
      return `/escandalo/${encoded}`
    case "sanction":
      return `/sancao/${encoded}`
    default:
      return undefined
  }
}

// ─── Chrome ───────────────────────────────────────────────────────────────────

export function NodeIcon({ type, size = 15 }: { type: GraphNode["type"]; size?: number }) {
  const props = { size, strokeWidth: 1.5 }
  switch (type) {
    case "politician":
      return <User {...props} className="text-[#c8a96e]" />
    case "person":
      return <Users {...props} className="text-[#6dbf8f]" />
    case "scandal":
      return <AlertTriangle {...props} className="text-[#cc2222]" />
    case "organization":
      return <Building2 {...props} className="text-[#5b9bd5]" />
    case "legal_proceeding":
      return <Scale {...props} className="text-[#8b7ec8]" />
    case "sanction":
      return <Ban {...props} className="text-[#d98a4b]" />
    default:
      return <FileText {...props} className="text-[#7a7a7a]" />
  }
}

export function Section({
  title,
  count,
  color = "#999999",
  children,
}: {
  title: string
  count?: number
  color?: string
  children: React.ReactNode
}) {
  return (
    <section className="mt-10">
      <h2
        className="mb-3 font-mono text-[10px] uppercase tracking-widest"
        style={{ color }}
      >
        {title}
        {typeof count === "number" && (
          <span className="ml-1.5 text-text-dim">({count})</span>
        )}
      </h2>
      {children}
    </section>
  )
}

export function EmptySection({ children }: { children: React.ReactNode }) {
  return (
    <p className="rounded-sm border border-border bg-surface px-4 py-3 font-mono text-xs text-text-muted">
      {children}
    </p>
  )
}

// ─── Photo ────────────────────────────────────────────────────────────────────

export function EntityPhoto({
  url,
  alt,
  attribution,
  ringColor,
  size = 112,
}: {
  url?: string
  alt: string
  attribution?: string
  ringColor: string
  size?: number
}) {
  const usable = isAllowedImageUrl(url)
  return (
    <div className="flex shrink-0 flex-col items-center gap-1.5" style={{ width: size }}>
      <div
        className="overflow-hidden rounded-full"
        style={{
          width: size,
          height: size,
          boxShadow: `0 0 0 3px ${ringColor}, 0 0 0 5px #0a0a0a`,
        }}
      >
        {usable ? (
          <Image
            src={url}
            alt={alt}
            width={size}
            height={size}
            className="h-full w-full object-cover object-center"
            // Hotlinked from the official source: never re-hosted, and some
            // public hosts reject requests carrying a referrer.
            referrerPolicy="no-referrer"
            unoptimized
          />
        ) : (
          <div
            className="flex h-full w-full items-center justify-center font-mono text-[10px] text-text-dim"
            style={{ background: `${ringColor}22` }}
          >
            s/ foto
          </div>
        )}
      </div>
      {usable && attribution && (
        <span className="line-clamp-2 text-center font-mono text-[8px] leading-tight text-text-dim">
          {attribution}
        </span>
      )}
    </div>
  )
}

// ─── Legal disclaimer (required on every entity page) ─────────────────────────

export function LegalDisclaimer() {
  return (
    <p className="mt-10 rounded-sm border border-border bg-surface px-4 py-3 text-xs leading-relaxed text-text-muted">
      Todos os dados desta página vêm de{" "}
      <strong className="text-text">registros públicos oficiais brasileiros</strong>{" "}
      (TSE, Câmara, Senado, CNJ, CGU, TCU). A citação de uma pessoa em um
      processo, sanção ou escândalo{" "}
      <strong className="text-text">não é condenação</strong> e não implica culpa
      ou irregularidade. Situações processuais mudam, e em caso de divergência
      prevalece sempre a fonte oficial. Veja a{" "}
      <Link
        href="/metodologia"
        className="text-text underline decoration-text-dim underline-offset-4 hover:decoration-text"
      >
        metodologia, as fontes e o direito de remoção
      </Link>
      .
    </p>
  )
}

// ─── Link lists ───────────────────────────────────────────────────────────────

export function SourceLinks({ urls, title = "Fontes oficiais" }: { urls: string[]; title?: string }) {
  if (urls.length === 0) return null
  return (
    <Section title={title}>
      <ul className="space-y-1">
        {urls.map((url) => (
          <li key={url}>
            <a
              href={url}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex max-w-full items-center gap-1.5 font-mono text-xs text-text-muted transition-colors hover:text-[#cc2222]"
            >
              <ExternalLink size={10} strokeWidth={1.5} className="shrink-0" />
              <span className="truncate">{url}</span>
            </a>
          </li>
        ))}
      </ul>
    </Section>
  )
}

// ─── Cards ────────────────────────────────────────────────────────────────────

function CardShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="rounded-sm border border-border bg-surface px-4 py-3">
      {children}
    </div>
  )
}

function Field({ label, value }: { label: string; value?: string }) {
  if (!value) return null
  return (
    <div className="flex gap-2 py-0.5">
      <span className="w-32 shrink-0 font-mono text-[10px] uppercase tracking-wider text-text-muted">
        {label}
      </span>
      <span className="break-words font-mono text-[11px] text-text">{value}</span>
    </div>
  )
}

// Registry entry (CEIS, CNEP, TCU inelegibility...). source_url is mandatory:
// every sanction deep-links its authoritative record.
export function SanctionCard({ node }: { node: GraphNode }) {
  const registry = prop(node, "registry") ?? node.label
  const sanctionType = prop(node, "sanction_type")
  const organ = prop(node, "organ")
  const dateStart = prop(node, "date_start")
  const dateEnd = prop(node, "date_end")
  const processRef = prop(node, "process_ref")
  const sourceUrl = prop(node, "source_url")
  const period = [dateStart, dateEnd]
    .filter((d): d is string => Boolean(d))
    .map(formatDate)
    .join(" até ")

  return (
    <CardShell>
      <div className="mb-1.5 flex flex-wrap items-center gap-2">
        <span className="font-mono text-[10px] uppercase tracking-wider text-[#d98a4b]">
          {registry}
        </span>
        {sanctionType && (
          <h3 className="font-serif text-sm leading-tight text-text">
            {valueLabel(sanctionType)}
          </h3>
        )}
      </div>
      <Field label="Órgão sancionador" value={organ} />
      <Field label="Vigência" value={period || undefined} />
      <Field label="Processo de referência" value={processRef} />
      <div className="mt-2 flex flex-wrap items-center gap-2">
        {/* The sanction's own page. Without this link the 64,779 sanction pages have
            no inbound link from any indexed page — the browse paginates client-side,
            so a crawler cannot walk it — and they are too many to list in the
            sitemap (the protocol caps it at 50,000). A page nothing links to and no
            sitemap names does not exist. */}
        <Link
          href={`/sancao/${encodeURIComponent(node.id)}`}
          className="font-mono text-[11px] text-text-muted transition-colors hover:text-white"
        >
          Detalhes
        </Link>
        {sourceUrl && (
          <a
            href={sourceUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 font-mono text-[11px] text-text-muted transition-colors hover:text-[#cc2222]"
          >
            <ExternalLink size={10} strokeWidth={1.5} className="shrink-0" />
            <span>Registro oficial</span>
          </a>
        )}
        <ReviewBadge />
      </div>
    </CardShell>
  )
}

// convictionState reads the three-valued conviction off raw node props. Graph
// endpoints carry has_conviction only when DataJud actually derived one, so an
// absent key IS the "never checked" state — and it must never render like an
// acquittal. "Nao verificado" and "sem condenacao" are different claims.
function convictionState(node: GraphNode): { label: string; cls: string } {
  const hc = node.properties?.has_conviction
  if (hc === true)
    return { label: "Com condenação", cls: "bg-rose-950/60 text-rose-300" }
  if (hc === false)
    return { label: "Sem condenação", cls: "bg-emerald-950/60 text-emerald-300" }
  return { label: "Não verificado", cls: "bg-surface text-text-dim" }
}

export function ProceedingCard({ connection }: { connection: Connection }) {
  const { node, edge, via } = connection
  const caseNumber = prop(node, "case_number") ?? node.label
  const court = prop(node, "court")
  const status = prop(node, "status")
  const phase = prop(node, "phase")
  const outcome = prop(node, "outcome") ?? (edge && (edge.properties.outcome as string | undefined))
  const sourceUrls = (node.properties.source_urls as string[] | undefined) ?? []
  const conviction = convictionState(node)
  const href = entityHref("legal_proceeding", node.id)

  return (
    <CardShell>
      <div className="mb-1.5 flex flex-wrap items-center gap-2">
        <Scale size={13} strokeWidth={1.5} className="shrink-0 text-[#8b7ec8]" />
        <h3 className="font-mono text-sm leading-tight break-all text-text">
          {href ? (
            <Link href={href} className="hover:text-white transition-colors">
              {caseNumber}
            </Link>
          ) : (
            caseNumber
          )}
        </h3>
        <span className={`rounded px-2 py-0.5 font-mono text-[9px] uppercase tracking-wider ${conviction.cls}`}>
          {conviction.label}
        </span>
      </div>
      <Field label="Tribunal / vara" value={court} />
      <Field label="Situação" value={status ? valueLabel(status) : undefined} />
      <Field label="Fase" value={phase ? valueLabel(phase) : undefined} />
      <Field label="Resultado" value={outcome ? valueLabel(outcome) : undefined} />
      {via && (
        <Field
          label={EDGE_TYPE_LABELS[edge?.type ?? ""] ?? "Vínculo"}
          value={via.label}
        />
      )}
      {sourceUrls.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-3">
          {sourceUrls.map((url) => (
            <a
              key={url}
              href={url}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1.5 font-mono text-[11px] text-text-muted transition-colors hover:text-[#cc2222]"
            >
              <ExternalLink size={10} strokeWidth={1.5} className="shrink-0" />
              <span>Registro oficial</span>
            </a>
          ))}
        </div>
      )}
      {edge && <ProvenanceBadge properties={edge.properties} />}
    </CardShell>
  )
}

// A generic connected entity (scandal, politician, person, organization). When
// `href` is given the card is a real anchor, so crawlers can walk the graph.
export function ConnectionCard({
  connection,
  href,
  description,
}: {
  connection: Connection
  href?: string
  description?: string
}) {
  const { node, edge, via } = connection
  const edgeLabel = edge
    ? (EDGE_TYPE_LABELS[edge.type] ?? edge.type.replace(/_/g, " ").toLowerCase())
    : NODE_TYPE_LABELS[node.type]
  const color = NODE_COLORS[node.type] ?? "#999999"
  const provenanceLink = prop(node, "provenance_link")

  const body = (
    <>
      <div className="mb-1 flex items-center gap-2">
        <NodeIcon type={node.type} />
        <span
          className="font-mono text-[10px] uppercase tracking-wider"
          style={{ color }}
        >
          {edgeLabel}
          {via ? `: ${via.label}` : ""}
        </span>
      </div>
      <h3 className="font-serif text-sm leading-tight text-text">{node.label}</h3>
      {description && (
        <p className="mt-1 line-clamp-3 text-xs leading-relaxed text-text-muted">
          {description}
        </p>
      )}
      {edge && <ProvenanceBadge properties={edge.properties} />}
    </>
  )

  return (
    <div className="rounded-sm border border-border bg-surface px-4 py-3 transition-colors hover:border-[#c8a96e]/60">
      {href ? (
        <Link href={href} className="block">
          {body}
        </Link>
      ) : (
        body
      )}
      {!href && provenanceLink && (
        <a
          href={provenanceLink}
          target="_blank"
          rel="noopener noreferrer"
          className="mt-2 inline-flex items-center gap-1.5 font-mono text-[11px] text-text-muted transition-colors hover:text-[#cc2222]"
        >
          <ExternalLink size={10} strokeWidth={1.5} className="shrink-0" />
          <span>Registro oficial</span>
        </a>
      )}
    </div>
  )
}
