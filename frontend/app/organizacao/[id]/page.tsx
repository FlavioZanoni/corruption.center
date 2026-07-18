// Server-rendered, crawlable page for a single organization.
//
// No 'use client', no graph canvas: everything a reader (or a crawler with no
// JavaScript) needs is real HTML, including the provenance of every link.

import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft } from "lucide-react"

import { fetchOrganizationDetail, type OrganizationRecord } from "@/lib/api/entities"
import { buildConnections, ofType, prop, type Connection } from "@/lib/entity-graph"
import { plural } from "@/lib/entity-labels"
import { formatCNPJ } from "@/lib/format"
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
  entityHref,
} from "@/components/EntityUI"

export const revalidate = 3600

const ORGANIZATION_COLOR = "#5b9bd5"
const SITE_NAME = "corruption.center"

type Props = { params: Promise<{ id: string }> }

// Display name: prefer name, fall back to CNPJ formatted
function displayName(org: OrganizationRecord): string {
  if (org.name && org.name.trim()) return org.name
  if (org.razao_social && org.razao_social.trim()) return org.razao_social
  if (org.cnpj) return formatCNPJ(org.cnpj)
  return org.id
}

function summarize(org: OrganizationRecord, connections: Connection[]): string {
  const proceedings = ofType(connections, "legal_proceeding").length
  const sanctions = ofType(connections, "sanction").length

  const counts = [
    plural(proceedings, "processo judicial", "processos judiciais"),
    plural(sanctions, "sanção registrada", "sanções registradas"),
  ].join(", ")

  return `${displayName(org)}. Registros públicos oficiais: ${counts}. Dados do CNJ, CGU e TCU.`
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id } = await params
  const detail = await fetchOrganizationDetail(id)
  if (!detail) return { title: "Organização não encontrada" }

  const org = detail.organization
  const connections = buildConnections(detail.connections, org.id)
  const name = displayName(org)
  const title = `${name}: processos, sanções e conexões`
  const description = summarize(org, connections)
  const url = siteUrl(`/organizacao/${org.id}`)

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
      // Page-level openGraph replaces the root block (shallow merge), which
      // silently dropped the file-convention og:image — re-point at the card.
      images: [siteUrl("/opengraph-image.png")],
    },
    twitter: {
      card: "summary",
      title: `${title} | ${SITE_NAME}`,
      description,
    },
  }
}

function organizationJsonLd(
  org: OrganizationRecord,
  connections: Connection[],
): Record<string, unknown> {
  const proceedings = ofType(connections, "legal_proceeding")
  const sanctions = ofType(connections, "sanction")

  const subjectOf: Record<string, unknown>[] = [
    ...proceedings.map(({ node }) => ({
      "@type": "CreativeWork",
      name: `Processo ${prop(node, "case_number") ?? node.label}`,
      ...(prop(node, "court")
        ? { publisher: { "@type": "Organization", name: prop(node, "court") } }
        : {}),
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

  return {
    "@context": "https://schema.org",
    "@type": "Organization",
    name: displayName(org),
    identifier: org.id,
    url: siteUrl(`/organizacao/${org.id}`),
    ...(org.cnpj ? { taxID: org.cnpj } : {}),
    ...(subjectOf.length ? { subjectOf } : {}),
  }
}

export default async function OrganizacaoPage({ params }: Props) {
  const { id } = await params
  const detail = await fetchOrganizationDetail(id)
  if (!detail) notFound()

  const org = detail.organization
  const connections = buildConnections(detail.connections, org.id)

  const proceedings = ofType(connections, "legal_proceeding")
  const sanctions = ofType(connections, "sanction")
  const others = ofType(connections, "organization", "person", "politician")

  const displayedName = displayName(org)

  return (
    <main className="h-screen overflow-y-auto bg-bg text-text">
      <JsonLd data={organizationJsonLd(org, connections)} />

      <div className="mx-auto max-w-3xl px-6 py-12">
        <nav className="mb-10 flex items-center justify-between text-sm">
          <Link
            href="/"
            className="inline-flex items-center gap-1.5 font-mono text-text-muted transition-colors hover:text-text"
          >
            <ArrowLeft size={13} strokeWidth={1.5} />
            Voltar
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
            style={{ color: ORGANIZATION_COLOR }}
          >
            Organização
          </p>
          <h1 className="mt-1 font-serif text-3xl font-bold leading-tight text-text">
            {displayedName}
          </h1>
          {org.razao_social && org.razao_social.trim() && org.razao_social !== displayedName && (
            <p className="mt-1 font-mono text-xs text-text-muted">
              {org.razao_social}
            </p>
          )}
          {org.cnpj && (
            <p className="mt-2 font-mono text-xs text-text-muted">
              CNPJ: {formatCNPJ(org.cnpj)}
            </p>
          )}
          <p className="mt-3 font-mono text-[10px] text-text-dim">{org.id}</p>
        </header>

        {/* ── Factual summary ── */}
        <p className="mt-6 text-sm leading-relaxed text-text-muted">
          Registros públicos oficiais vinculados a esta organização:{" "}
          <strong className="text-text">
            {plural(proceedings.length, "processo judicial", "processos judiciais")}
          </strong>
          {" "}
          e{" "}
          <strong className="text-text">
            {plural(sanctions.length, "sanção", "sanções")}
          </strong>
          .
        </p>

        <LegalDisclaimer />

        {/* ── Processos ── */}
        <Section title="Processos judiciais" count={proceedings.length} color="#8b7ec8">
          {proceedings.length === 0 ? (
            <EmptySection>
              Nenhum processo judicial vinculado a esta organização nos registros
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
              Nenhuma sanção registrada para esta organização nos cadastros
              oficiais consultados.
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
              {others.map((c) => {
                const href = entityHref(c.node.type, c.node.id)
                return (
                  <ConnectionCard
                    key={c.node.id}
                    connection={c}
                    href={href}
                  />
                )
              })}
            </div>
          </Section>
        )}

        {org.provenance_link && (
          <SourceLinks urls={[org.provenance_link]} title="Origem do registro" />
        )}

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
