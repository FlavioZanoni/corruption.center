// Server-rendered, crawlable page for a single scandal.
//
// No 'use client', no graph canvas: everything a reader (or a crawler with no
// JavaScript) needs is real HTML, including the provenance of every link.

import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft } from "lucide-react"

import { fetchScandalDetail, type ScandalRecord } from "@/lib/api/entities"
import { buildConnections, ofType, prop, type Connection } from "@/lib/entity-graph"
import { formatBRL, formatDate, isoDate, plural, valueLabel } from "@/lib/entity-labels"
import { siteUrl } from "@/lib/seo"
import {
  ConnectionCard,
  EmptySection,
  JsonLd,
  LegalDisclaimer,
  ProceedingCard,
  SanctionCard,
  Section,
  SourceLinks,
} from "@/components/EntityUI"

export const revalidate = 3600

const SCANDAL_COLOR = "#cc2222"
// app/layout.tsx appends "| corruption.center" to every page title via its
// metadata title template, so `title` below must NOT repeat the suffix. The
// OpenGraph title is standalone (no template applies), so it spells it out.
const SITE_NAME = "corruption.center"

type Props = { params: Promise<{ id: string }> }

function officialLinks(s: ScandalRecord): string[] {
  return [
    ...(s.wikipedia_url ? [s.wikipedia_url] : []),
    ...(s.source_urls ?? []),
  ].filter(Boolean)
}

function period(s: ScandalRecord): string {
  const start = s.date_start ? formatDate(s.date_start) : undefined
  const end = s.date_end ? formatDate(s.date_end) : undefined
  if (start && end) return `${start} até ${end}`
  if (start) return `desde ${start}`
  if (end) return `até ${end}`
  return ""
}

// Factual, data-derived summary: only counts of records the official sources
// link to this scandal, never a characterisation of it.
function summarize(s: ScandalRecord, connections: Connection[]): string {
  const proceedings = ofType(connections, "legal_proceeding").length
  const politicians = ofType(connections, "politician").length
  const people = ofType(connections, "person").length
  const orgs = ofType(connections, "organization").length
  const sanctions = ofType(connections, "sanction").length

  const when = period(s)
  const counts = [
    plural(proceedings, "processo judicial", "processos judiciais"),
    plural(politicians, "político vinculado", "políticos vinculados"),
    plural(people, "pessoa citada", "pessoas citadas"),
    plural(orgs, "organização vinculada", "organizações vinculadas"),
    plural(sanctions, "sanção", "sanções"),
  ].join(", ")

  const status = s.status ? valueLabel(s.status).toLowerCase() : ""
  const head = [
    s.name,
    [status, when].filter(Boolean).join(", "),
  ]
    .filter(Boolean)
    .join(": ")

  return `${head}. Registros públicos oficiais: ${counts}. Dados do CNJ, CGU, TCU e demais fontes oficiais, com a origem de cada vínculo indicada.`
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id } = await params
  const detail = await fetchScandalDetail(id)
  if (!detail) return { title: "Escândalo não encontrado" }

  const s = detail.scandal
  const connections = buildConnections(detail.connections, s.id)
  const title = `${s.name}: políticos, processos e sanções`
  const description = summarize(s, connections)
  const url = siteUrl(`/escandalo/${s.id}`)

  return {
    title,
    description,
    alternates: { canonical: url },
    openGraph: {
      type: "article",
      title: `${title} | ${SITE_NAME}`,
      description,
      url,
      siteName: SITE_NAME,
      locale: "pt_BR",
    },
    twitter: {
      card: "summary",
      title: `${title} | ${SITE_NAME}`,
      description,
    },
  }
}

function eventJsonLd(
  s: ScandalRecord,
  connections: Connection[],
): Record<string, unknown> {
  const proceedings = ofType(connections, "legal_proceeding")
  const orgs = ofType(connections, "organization")
  const politicians = ofType(connections, "politician")
  const people = ofType(connections, "person")

  const about = orgs.map(({ node }) => ({
    "@type": "Organization",
    name: node.label,
    ...(prop(node, "cnpj") ? { taxID: prop(node, "cnpj") } : {}),
  }))

  const mentions: Record<string, unknown>[] = [
    ...politicians.map(({ node }) => ({
      "@type": "Person",
      name: node.label,
      url: siteUrl(`/politico/${node.id}`),
    })),
    ...people.map(({ node }) => ({ "@type": "Person", name: node.label })),
    ...proceedings.map(({ node }) => ({
      "@type": "CreativeWork",
      name: `Processo ${prop(node, "case_number") ?? node.label}`,
      ...(prop(node, "court")
        ? { publisher: { "@type": "Organization", name: prop(node, "court") } }
        : {}),
    })),
  ]

  const sameAs = officialLinks(s)
  const start = s.date_start ? isoDate(s.date_start) : undefined
  const end = s.date_end ? isoDate(s.date_end) : undefined

  return {
    "@context": "https://schema.org",
    "@type": "Event",
    name: s.name,
    identifier: s.id,
    url: siteUrl(`/escandalo/${s.id}`),
    ...(s.description ? { description: s.description } : {}),
    ...(s.aliases?.length ? { alternateName: s.aliases } : {}),
    ...(start ? { startDate: start } : {}),
    ...(end ? { endDate: end } : {}),
    location: { "@type": "Place", name: "Brasil" },
    ...(sameAs.length ? { sameAs } : {}),
    ...(about.length ? { about } : {}),
    ...(mentions.length ? { mentions } : {}),
  }
}

export default async function EscandaloPage({ params }: Props) {
  const { id } = await params
  const detail = await fetchScandalDetail(id)
  if (!detail) notFound()

  const s = detail.scandal
  const connections = buildConnections(detail.connections, s.id)

  const politicians = ofType(connections, "politician")
  const people = ofType(connections, "person")
  const proceedings = ofType(connections, "legal_proceeding")
  const sanctions = ofType(connections, "sanction")
  const orgs = ofType(connections, "organization")

  const when = period(s)

  return (
    <main className="h-screen overflow-y-auto bg-bg text-text">
      <JsonLd data={eventJsonLd(s, connections)} />

      <div className="mx-auto max-w-3xl px-6 py-12">
        <nav className="mb-10 flex items-center justify-between text-sm">
          <Link
            href="/"
            className="inline-flex items-center gap-1.5 font-mono text-text-muted transition-colors hover:text-text"
          >
            <ArrowLeft size={13} strokeWidth={1.5} />
            corruption.center
          </Link>
          <Link
            href="/"
            className="font-mono text-xs uppercase tracking-widest text-text-dim transition-colors hover:text-text"
          >
            Ver no grafo
          </Link>
        </nav>

        {/* ── Header ── */}
        <header>
          <p
            className="font-mono text-[9px] uppercase tracking-widest"
            style={{ color: SCANDAL_COLOR }}
          >
            Escândalo
          </p>
          <h1 className="mt-1 font-serif text-4xl font-bold leading-tight text-text">
            {s.name}
          </h1>
          <p className="mt-2 font-mono text-xs uppercase tracking-wider text-text-muted">
            {[s.status ? valueLabel(s.status) : undefined, when]
              .filter(Boolean)
              .join(" · ")}
          </p>
          {s.aliases && s.aliases.length > 0 && (
            <p className="mt-1 font-mono text-[11px] text-text-dim">
              Também conhecido como: {s.aliases.join(", ")}
            </p>
          )}
          {typeof s.total_amount_brl === "number" && s.total_amount_brl > 0 && (
            <p className="mt-1 font-mono text-[11px] text-[#e8c97a]">
              Valor apurado nos registros: {formatBRL(s.total_amount_brl)}
            </p>
          )}
          <p className="mt-3 font-mono text-[10px] text-text-dim">{s.id}</p>
        </header>

        {s.description && (
          <p className="mt-6 text-base leading-relaxed text-text-muted">
            {s.description}
          </p>
        )}

        <LegalDisclaimer />

        {/* ── Políticos ── */}
        <Section title="Políticos vinculados" count={politicians.length} color="#c8a96e">
          {politicians.length === 0 ? (
            <EmptySection>
              Nenhum político vinculado a este escândalo nos registros consultados.
            </EmptySection>
          ) : (
            <div className="space-y-2">
              {politicians.map((c) => (
                <ConnectionCard
                  key={c.node.id}
                  connection={c}
                  href={`/politico/${c.node.id}`}
                />
              ))}
            </div>
          )}
        </Section>

        {/* ── Processos ── */}
        <Section title="Processos judiciais" count={proceedings.length} color="#8b7ec8">
          {proceedings.length === 0 ? (
            <EmptySection>
              Nenhum processo judicial vinculado a este escândalo nos registros
              consultados.
            </EmptySection>
          ) : (
            <div className="space-y-2">
              {proceedings.map((c) => (
                <ProceedingCard key={c.node.id} connection={c} />
              ))}
            </div>
          )}
        </Section>

        {/* ── Pessoas citadas ── */}
        {people.length > 0 && (
          <Section title="Pessoas citadas nos processos" count={people.length} color="#6dbf8f">
            <div className="space-y-2">
              {people.map((c) => (
                <ConnectionCard key={c.node.id} connection={c} />
              ))}
            </div>
          </Section>
        )}

        {/* ── Organizações ── */}
        {orgs.length > 0 && (
          <Section title="Organizações" count={orgs.length} color="#5b9bd5">
            <div className="space-y-2">
              {orgs.map((c) => (
                <ConnectionCard key={c.node.id} connection={c} />
              ))}
            </div>
          </Section>
        )}

        {/* ── Sanções ── */}
        {sanctions.length > 0 && (
          <Section title="Sanções" count={sanctions.length} color="#d98a4b">
            <div className="space-y-2">
              {sanctions.map((c) => (
                <SanctionCard key={c.node.id} node={c.node} />
              ))}
            </div>
          </Section>
        )}

        <SourceLinks urls={officialLinks(s)} />

        <footer className="mt-16 border-t border-border pt-6">
          <p className="font-mono text-xs text-text-dim">
            corruption.center - ferramenta de transparência não comercial,
            construída sobre dados públicos oficiais.
          </p>
        </footer>
      </div>
    </main>
  )
}
