// Server-rendered, crawlable page for a single politician.
//
// No 'use client', no graph canvas: everything a reader (or a crawler with no
// JavaScript) needs is real HTML, including the provenance of every link.

import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft } from "lucide-react"

import { fetchPoliticianDetail, type PoliticianRecord } from "@/lib/api/entities"
import { buildConnections, ofType, prop, type Connection } from "@/lib/entity-graph"
import { formatDate, isoDate, plural } from "@/lib/entity-labels"
import { isAllowedImageUrl } from "@/lib/images"
import { siteUrl } from "@/lib/seo"
import {
  ConnectionCard,
  EmptySection,
  EntityPhoto,
  JsonLd,
  LegalDisclaimer,
  ProceedingCard,
  SanctionCard,
  Section,
  SourceLinks,
} from "@/components/EntityUI"

export const revalidate = 3600

const POLITICIAN_COLOR = "#c8a96e"
// app/layout.tsx appends "| corruption.center" to every page title via its
// metadata title template, so `title` below must NOT repeat the suffix. The
// OpenGraph title is standalone (no template applies), so it spells it out.
const SITE_NAME = "corruption.center"

type Props = { params: Promise<{ id: string }> }

// party / state as "PSD-BA", tolerating either being absent.
function partyState(p: PoliticianRecord): string {
  return [p.party_current, p.state].filter(Boolean).join("-")
}

function officialLinks(p: PoliticianRecord): string[] {
  return [
    ...(p.tse_profile_urls ?? []),
    ...(p.wikipedia_url ? [p.wikipedia_url] : []),
    ...(p.source_urls ?? []),
  ].filter(Boolean)
}

// Factual, data-derived summary. Nothing here is asserted beyond the counts of
// records the official sources link to this person.
function summarize(p: PoliticianRecord, connections: Connection[]): string {
  const scandals = ofType(connections, "scandal").length
  const proceedings = ofType(connections, "legal_proceeding").length
  const sanctions = ofType(connections, "sanction").length

  const who = [p.role_current, partyState(p)].filter(Boolean).join(", ")
  const counts = [
    plural(proceedings, "processo judicial", "processos judiciais"),
    plural(sanctions, "sanção registrada", "sanções registradas"),
    plural(scandals, "escândalo vinculado", "escândalos vinculados"),
  ].join(", ")

  return `${p.name}${who ? ` (${who})` : ""}. Registros públicos oficiais: ${counts}. Dados do TSE, Câmara, Senado, CNJ, CGU e TCU, com a origem de cada vínculo indicada.`
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id } = await params
  const detail = await fetchPoliticianDetail(id)
  if (!detail) return { title: "Político não encontrado" }

  const p = detail.politician
  const connections = buildConnections(detail.connections, p.id)
  const ps = partyState(p)
  const title = `${p.name}${ps ? ` (${ps})` : ""}: processos, sanções e escândalos`
  const description = summarize(p, connections)
  const url = siteUrl(`/politico/${p.id}`)
  const image = isAllowedImageUrl(p.photo_url) ? p.photo_url : undefined

  return {
    title,
    description,
    alternates: { canonical: url },
    openGraph: {
      type: "profile",
      title: `${title} | ${SITE_NAME}`,
      description,
      url,
      siteName: SITE_NAME,
      locale: "pt_BR",
      ...(image ? { images: [{ url: image, alt: p.name }] } : {}),
    },
    twitter: {
      card: image ? "summary_large_image" : "summary",
      title: `${title} | ${SITE_NAME}`,
      description,
      ...(image ? { images: [image] } : {}),
    },
  }
}

function personJsonLd(
  p: PoliticianRecord,
  connections: Connection[],
): Record<string, unknown> {
  const proceedings = ofType(connections, "legal_proceeding")
  const scandals = ofType(connections, "scandal")
  const sanctions = ofType(connections, "sanction")

  const subjectOf: Record<string, unknown>[] = [
    ...proceedings.map(({ node }) => ({
      "@type": "CreativeWork",
      name: `Processo ${prop(node, "case_number") ?? node.label}`,
      ...(prop(node, "court")
        ? { publisher: { "@type": "Organization", name: prop(node, "court") } }
        : {}),
    })),
    ...scandals.map(({ node }) => ({
      "@type": "Event",
      name: node.label,
      url: siteUrl(`/escandalo/${node.id}`),
    })),
    ...sanctions.map(({ node }) => ({
      "@type": "CreativeWork",
      name: `${prop(node, "registry") ?? node.label}: ${prop(node, "sanction_type") ?? "sanção"}`,
      ...(prop(node, "source_url") ? { url: prop(node, "source_url") } : {}),
      ...(prop(node, "organ")
        ? { publisher: { "@type": "Organization", name: prop(node, "organ") } }
        : {}),
    })),
  ]

  const sameAs = officialLinks(p)

  return {
    "@context": "https://schema.org",
    "@type": "Person",
    name: p.name,
    identifier: p.id,
    url: siteUrl(`/politico/${p.id}`),
    ...(p.name_aliases?.length ? { alternateName: p.name_aliases } : {}),
    ...(isAllowedImageUrl(p.photo_url) ? { image: p.photo_url } : {}),
    ...(p.role_current ? { jobTitle: p.role_current } : {}),
    ...(p.party_current
      ? { affiliation: { "@type": "Organization", name: p.party_current } }
      : {}),
    ...(p.state
      ? { workLocation: { "@type": "AdministrativeArea", name: p.state } }
      : {}),
    ...(p.birth_date && isoDate(p.birth_date)
      ? { birthDate: isoDate(p.birth_date) }
      : {}),
    ...(sameAs.length ? { sameAs } : {}),
    ...(subjectOf.length ? { subjectOf } : {}),
  }
}

export default async function PoliticoPage({ params }: Props) {
  const { id } = await params
  const detail = await fetchPoliticianDetail(id)
  if (!detail) notFound()

  const p = detail.politician
  const connections = buildConnections(detail.connections, p.id)

  const scandals = ofType(connections, "scandal")
  const proceedings = ofType(connections, "legal_proceeding")
  const sanctions = ofType(connections, "sanction")
  const others = ofType(connections, "organization", "person", "politician")

  const ps = partyState(p)

  return (
    <main className="h-screen overflow-y-auto bg-bg text-text">
      <JsonLd data={personJsonLd(p, connections)} />

      <div className="mx-auto max-w-3xl px-6 py-12">
        <nav className="mb-10 flex items-center justify-between text-sm">
          <Link
            href="/politicos"
            className="inline-flex items-center gap-1.5 font-mono text-text-muted transition-colors hover:text-text"
          >
            <ArrowLeft size={13} strokeWidth={1.5} />
            Políticos
          </Link>
          <Link
            href="/"
            className="font-mono text-xs uppercase tracking-widest text-text-dim transition-colors hover:text-text"
          >
            Ver no grafo
          </Link>
        </nav>

        {/* ── Header ── */}
        <header className="flex flex-wrap items-start gap-6">
          <EntityPhoto
            url={p.photo_url}
            alt={p.name}
            attribution={p.photo_attribution}
            ringColor={POLITICIAN_COLOR}
          />
          <div className="min-w-0 flex-1">
            <p
              className="font-mono text-[9px] uppercase tracking-widest"
              style={{ color: POLITICIAN_COLOR }}
            >
              Político
            </p>
            <h1 className="mt-1 font-serif text-3xl font-bold leading-tight text-text">
              {p.name}
            </h1>
            {(ps || p.role_current) && (
              <p className="mt-2 font-mono text-xs uppercase tracking-wider text-text-muted">
                {[ps, p.role_current].filter(Boolean).join(" · ")}
              </p>
            )}
            {p.name_aliases && p.name_aliases.length > 0 && (
              <p className="mt-1 font-mono text-[11px] text-text-dim">
                Também conhecido como: {p.name_aliases.join(", ")}
              </p>
            )}
            {p.birth_date && (
              <p className="mt-1 font-mono text-[11px] text-text-dim">
                Nascimento: {formatDate(p.birth_date)}
              </p>
            )}
            <p className="mt-3 font-mono text-[10px] text-text-dim">{p.id}</p>
          </div>
        </header>

        {/* ── Factual summary (also the meta description) ── */}
        <p className="mt-6 text-sm leading-relaxed text-text-muted">
          Registros públicos oficiais vinculados a este nome:{" "}
          <strong className="text-text">
            {plural(proceedings.length, "processo judicial", "processos judiciais")}
          </strong>
          ,{" "}
          <strong className="text-text">
            {plural(sanctions.length, "sanção", "sanções")}
          </strong>{" "}
          e{" "}
          <strong className="text-text">
            {plural(scandals.length, "escândalo", "escândalos")}
          </strong>
          . A origem de cada vínculo é exibida abaixo.
        </p>

        <LegalDisclaimer />

        {/* ── Escândalos ── */}
        <Section title="Escândalos" count={scandals.length} color="#cc2222">
          {scandals.length === 0 ? (
            <EmptySection>
              Nenhum escândalo vinculado a este político nos registros consultados.
            </EmptySection>
          ) : (
            <div className="space-y-2">
              {scandals.map((c) => (
                <ConnectionCard
                  key={c.node.id}
                  connection={c}
                  href={`/escandalo/${c.node.id}`}
                  description={prop(c.node, "description")}
                />
              ))}
            </div>
          )}
        </Section>

        {/* ── Processos ── */}
        <Section title="Processos judiciais" count={proceedings.length} color="#8b7ec8">
          {proceedings.length === 0 ? (
            <EmptySection>
              Nenhum processo judicial vinculado a este político nos registros
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

        {/* ── Sanções ── */}
        <Section title="Sanções" count={sanctions.length} color="#d98a4b">
          {sanctions.length === 0 ? (
            <EmptySection>
              Nenhuma sanção registrada para este político nos cadastros oficiais
              consultados.
            </EmptySection>
          ) : (
            <div className="space-y-2">
              {sanctions.map((c) => (
                <SanctionCard key={c.node.id} node={c.node} />
              ))}
            </div>
          )}
        </Section>

        {/* ── Outras conexões ── */}
        {others.length > 0 && (
          <Section title="Outras conexões" count={others.length}>
            <div className="space-y-2">
              {others.map((c) => (
                <ConnectionCard
                  key={c.node.id}
                  connection={c}
                  href={
                    c.node.type === "politician"
                      ? `/politico/${c.node.id}`
                      : undefined
                  }
                />
              ))}
            </div>
          </Section>
        )}

        <SourceLinks urls={officialLinks(p)} />

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
