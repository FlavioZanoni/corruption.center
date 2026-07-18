// Server-rendered, crawlable page for a single person.
// This is typically a private citizen whose name appeared in a court publication.
//
// No 'use client', no graph canvas: everything a reader (or a crawler with no
// JavaScript) needs is real HTML, including the provenance of every link.

import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft } from "lucide-react"

import { fetchPersonDetail, type PersonRecord } from "@/lib/api/entities"
import { buildConnections, ofType, prop, type Connection } from "@/lib/entity-graph"
import { plural } from "@/lib/entity-labels"
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

const PERSON_COLOR = "#6dbf8f"
const SITE_NAME = "corruption.center"

type Props = { params: Promise<{ id: string }> }

function summarize(p: PersonRecord, connections: Connection[]): string {
  const proceedings = ofType(connections, "legal_proceeding").length
  const sanctions = ofType(connections, "sanction").length

  const counts = [
    plural(proceedings, "processo judicial", "processos judiciais"),
    plural(sanctions, "sanção registrada", "sanções registradas"),
  ].join(", ")

  return `${p.name}. Registros públicos oficiais: ${counts}. Dados do CNJ, CGU e TCU.`
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id } = await params
  const detail = await fetchPersonDetail(id)
  if (!detail) return { title: "Pessoa não encontrada" }

  const p = detail.person
  const connections = buildConnections(detail.connections, p.id)
  const title = `${p.name}: processos e sanções`
  const description = summarize(p, connections)
  const url = siteUrl(`/pessoa/${p.id}`)

  return {
    title,
    description,
    // NOINDEX, deliberately. A Person here is, overwhelmingly, a private citizen
    // whose name DJEN printed in a court publication — the DEFENDANT_IN edge says
    // "cited", not accused, not charged, not convicted. Letting a search engine
    // surface "corruption.center — <their name>" is exactly the harm the page's own
    // disclaimer exists to prevent, except permanent, and reaching people who never
    // read the disclaimer. The page stays reachable and linkable so a reader who
    // follows a case can see who was named; it just does not become the first
    // result for a stranger's name. Politicians (public figures), companies, cases
    // and sanctions are all indexed — this exception is only for private people.
    robots: { index: false, follow: true },
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

function personJsonLd(
  p: PersonRecord,
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
    "@type": "Person",
    name: p.name,
    identifier: p.id,
    url: siteUrl(`/pessoa/${p.id}`),
    // Only assert subjectOf (this person is the subject of these records) when the
    // node is a specific, document-identified person. A name-only node may merge
    // records of several people who share the name; emitting them here as one
    // schema.org Person is machine-readable defamation of whichever one is innocent.
    ...(!p.ambiguous && subjectOf.length ? { subjectOf } : {}),
  }
}

export default async function PessoaPage({ params }: Props) {
  const { id } = await params
  const detail = await fetchPersonDetail(id)
  if (!detail) notFound()

  const p = detail.person
  const connections = buildConnections(detail.connections, p.id)

  const proceedings = ofType(connections, "legal_proceeding")
  const sanctions = ofType(connections, "sanction")
  const others = ofType(connections, "organization", "person", "politician")

  return (
    <main className="h-screen overflow-y-auto bg-bg text-text">
      <JsonLd data={personJsonLd(p, connections)} />

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
            style={{ color: PERSON_COLOR }}
          >
            Pessoa
          </p>
          <h1 className="mt-1 font-serif text-3xl font-bold leading-tight text-text">
            {p.name}
          </h1>
          <p className="mt-3 font-mono text-[10px] text-text-dim">{p.id}</p>
        </header>

        {/* ── Factual summary ── */}
        <p className="mt-6 text-sm leading-relaxed text-text-muted">
          Registros públicos oficiais vinculados a este nome:{" "}
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

        {/* ── Homonym warning ── */}
        {/* A node with no CPF is keyed by name alone, so records below may belong
            to different people who share it. Say so before the reader reads the
            list as one person's history. */}
        {p.ambiguous && (
          <p className="mt-4 rounded-sm border border-sky-600/40 bg-sky-500/5 px-4 py-3 text-xs leading-relaxed text-text-muted">
            <strong className="block text-text">
              Nome comum — pode corresponder a mais de uma pessoa.
            </strong>
            Este registro é identificado apenas pelo nome (sem CPF), então os
            processos e sanções listados abaixo{" "}
            <strong className="text-text">
              podem se referir a pessoas diferentes
            </strong>{" "}
            com o mesmo nome. Cada item deve ser lido individualmente, a partir da
            sua fonte oficial, e não como o histórico de uma única pessoa.
          </p>
        )}

        {/* ── LEGAL SENSITIVITY DISCLAIMER ── */}
        <p className="mt-4 rounded-sm border border-yellow-600/30 bg-yellow-500/5 px-4 py-3 text-xs leading-relaxed text-text-muted">
          <strong className="block text-text">⚠ Aviso importante:</strong> Esta
          pessoa é usualmente um cidadão privado cujo nome apareceu em um
          processo judicial publicado. A{" "}
          <strong className="text-text">citação em um processo não é condenação</strong> e não implica culpa, investigação ou qualquer irregularidade. A presença do
          nome pode significar apenas que a pessoa foi mencionada ou citada nos
          autos, sem ser acusada. Situações processuais mudam continuamente. Em
          caso de divergência, prevalecem sempre os registros oficiais. Veja a{" "}
          <Link
            href="/metodologia"
            className="text-text underline decoration-text-dim underline-offset-4 hover:decoration-text"
          >
            metodologia, as fontes e o direito de remoção
          </Link>
          .
        </p>

        <LegalDisclaimer />

        {/* ── Processos ── */}
        <Section title="Processos judiciais" count={proceedings.length} color="#8b7ec8">
          {proceedings.length === 0 ? (
            <EmptySection>
              Nenhum processo judicial vinculado a esta pessoa nos registros
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
              Nenhuma sanção registrada para esta pessoa nos cadastros oficiais
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

        {p.provenance_link && (
          <SourceLinks urls={[p.provenance_link]} title="Origem do registro" />
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
