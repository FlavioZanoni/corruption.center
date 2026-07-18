// Server-rendered, crawlable page for a single legal proceeding.
//
// No 'use client', no graph canvas: everything a reader (or a crawler with no
// JavaScript) needs is real HTML, including the provenance of every link.

import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft, ExternalLink } from "lucide-react"

import {
  fetchLegalProceedingDetail,
  type LegalProceedingRecord,
  type DefendantInfo,
} from "@/lib/api/entities"
import { plural, valueLabel } from "@/lib/entity-labels"
import { siteUrl } from "@/lib/seo"
import {
  JsonLd,
  LegalDisclaimer,
  Section,
  EmptySection,
  NodeIcon,
  entityHref,
} from "@/components/EntityUI"

export const revalidate = 3600

const PROCEEDING_COLOR = "#8b7ec8"
const SITE_NAME = "corruption.center"

type Props = { params: Promise<{ id: string }> }

function convictionStatus(record: LegalProceedingRecord): string {
  if (!record.polled) {
    return "Não verificado"
  }
  return record.has_conviction ? "Com condenação" : "Sem condenação"
}

function convictionLabel(record: LegalProceedingRecord): string | undefined {
  if (!record.polled) {
    return "não verificado (DataJud nunca consultou)"
  }
  return record.has_conviction ? "com condenação registrada" : "sem condenação"
}

function summarize(p: LegalProceedingRecord): string {
  const defendants = plural(
    p.defendants.length,
    "acusado/citado",
    "acusados/citados",
  )
  const status = convictionLabel(p)
  return `Processo ${p.case_number}${status ? ` - ${status}` : ""}. ${defendants}. Tribunal: ${p.court ?? "não especificado"}.`
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id } = await params
  const detail = await fetchLegalProceedingDetail(id)
  if (!detail) return { title: "Processo não encontrado" }

  const p = detail
  const title = `Processo ${p.case_number}: ${convictionStatus(p)}`
  const description = summarize(p)
  const url = siteUrl(`/processo/${p.id}`)

  return {
    title,
    description,
    alternates: { canonical: url },
    openGraph: {
      type: "website",
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

function proceedingJsonLd(p: LegalProceedingRecord): Record<string, unknown> {
  // Deliberately NO defendant list here. The page's HTML shows the names — a case
  // is public record and omitting its parties would be a lie — but structured data
  // is different in kind: a schema.org Person entity is a machine-readable
  // invitation to index a private citizen by name, which is exactly what
  // /pessoa's noindex was built to prevent. The public record stays readable;
  // it does not get amplified.
  return {
    "@context": "https://schema.org",
    "@type": "LegalCase",
    identifier: p.case_number,
    url: siteUrl(`/processo/${p.id}`),
    ...(p.court ? { location: { "@type": "Place", name: p.court } } : {}),
    ...(p.status ? { status: p.status } : {}),
    ...(p.phase ? { phase: p.phase } : {}),
    ...(p.scandal ? { about: { "@type": "Event", name: p.scandal.name } } : {}),
  }
}

export default async function ProcessoPage({ params }: Props) {
  const { id } = await params
  const detail = await fetchLegalProceedingDetail(id)
  if (!detail) notFound()

  const p = detail

  const convictionMsg = convictionLabel(p)

  return (
    <main className="h-screen overflow-y-auto bg-bg text-text">
      <JsonLd data={proceedingJsonLd(p)} />

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
            style={{ color: PROCEEDING_COLOR }}
          >
            Processo Judicial
          </p>
          <h1 className="mt-1 font-serif text-3xl font-bold leading-tight text-text break-all">
            {p.case_number}
          </h1>
          <p className="mt-3 font-mono text-[10px] text-text-dim">{p.id}</p>
        </header>

        {/* ── Key facts ── */}
        <div className="mt-6 space-y-2 font-mono text-xs">
          {p.court && (
            <div>
              <span className="text-text-muted">Tribunal / Vara:</span>{" "}
              <span className="text-text">{p.court}</span>
            </div>
          )}
          {p.status && (
            <div>
              <span className="text-text-muted">Situação:</span>{" "}
              <span className="text-text">{valueLabel(p.status)}</span>
            </div>
          )}
          {p.phase && (
            <div>
              <span className="text-text-muted">Fase:</span>{" "}
              <span className="text-text">{valueLabel(p.phase)}</span>
            </div>
          )}
          {p.type && (
            <div>
              <span className="text-text-muted">Classe:</span>{" "}
              <span className="text-text">{p.type}</span>
            </div>
          )}
        </div>

        {/* ── Conviction status (three-valued!) ── */}
        <div className="mt-6 rounded-sm border border-border bg-surface px-4 py-3">
          <p className="font-mono text-[10px] uppercase tracking-wider text-text-muted">
            Situação de condenação
          </p>
          <p className="mt-2 text-sm text-text">
            <strong>{convictionStatus(p)}</strong>
          </p>
          {!p.polled && (
            <p className="mt-2 text-xs leading-relaxed text-text-muted">
              O DataJud nunca consultou este processo. A situação de condenação
              não foi verificada. Dos {plural(6264, "processo", "processos")} vinculados à{" "}
              <em>Operação Lava Jato</em>, apenas {plural(13, "processo", "processos")}{" "}
              foi verificado como tendo condenação, {plural(1, "tem", "têm")} confirmada
              sem condenação, e os demais permanecem não verificados.
            </p>
          )}
        </div>

        {/* ── Escândalo vinculado ── */}
        {p.scandal && (
          <Section title="Escândalo vinculado">
            <div className="rounded-sm border border-border bg-surface px-4 py-3">
              <Link
                href={`/escandalo/${p.scandal.id}`}
                className="block font-serif text-sm leading-tight text-text hover:underline"
              >
                {p.scandal.name}
              </Link>
            </div>
          </Section>
        )}

        {/* ── Defendants ── */}
        <Section
          title="Partes citadas no processo"
          count={p.defendants.length}
          color={PROCEEDING_COLOR}
        >
          {p.defendants.length === 0 ? (
            <EmptySection>Nenhum acusado/citado registrado.</EmptySection>
          ) : (
            <div className="space-y-2">
              {p.defendants.map((defendant) => (
                <DefendantCard key={defendant.id} defendant={defendant} />
              ))}
            </div>
          )}
        </Section>

        <LegalDisclaimer />

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

function DefendantCard({ defendant }: { defendant: DefendantInfo }) {
  const href = entityHref(defendant.type, defendant.id)
  const outcomeLabel = getOutcomeLabel(defendant.outcome)
  const isHumanVerified =
    defendant.properties?.outcome_source === "human"
  const evidenceUrl = defendant.properties?.outcome_evidence_url

  // A plain <div> with SIBLING links. Wrapping the card in <Link> nested the
  // evidence <a> inside it — the exact hydration bug fixed on /sancoes the same
  // day, reintroduced here. It also swallowed the click on the one link that
  // matters most: the decision itself.
  return (
    <div className="rounded-sm border border-border bg-surface px-4 py-3">
      <div className="mb-1 flex items-center gap-2">
        <NodeIcon type={defendant.type.toLowerCase() as any} />
        <span className="font-mono text-[10px] uppercase tracking-wider text-text-muted">
          {defendant.type}
        </span>
      </div>
      <h3 className="font-serif text-sm leading-tight text-text break-all">
        {href ? (
          <Link href={href} className="hover:text-white transition-colors">
            {defendant.name}
          </Link>
        ) : (
          defendant.name
        )}
      </h3>

      {/* Outcome badge */}
      <div className="mt-2 inline-block rounded bg-surface px-2 py-1 font-mono text-[9px] uppercase tracking-wider text-text-muted">
        {isHumanVerified && (
          <span className="mr-1 text-yellow-600">✓ Verificado:</span>
        )}
        {outcomeLabel}
      </div>

      {/* Citation disclaimer */}
      {defendant.outcome === "cited" && (
        <p className="mt-2 text-xs leading-relaxed text-text-muted">
          Esta pessoa foi apenas{" "}
          <strong className="text-text">citada no processo</strong> — seu nome
          apareceu nos autos publicados pelo DJEN, mas isto não implica acusação,
          investigação ou culpa.
        </p>
      )}

      {/* Human-verified link */}
      {isHumanVerified && evidenceUrl && (
        <div className="mt-2">
          <a
            href={evidenceUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 font-mono text-[11px] text-text-muted transition-colors hover:text-[#cc2222]"
          >
            <ExternalLink size={10} strokeWidth={1.5} className="shrink-0" />
            <span>Sentença ou decisão</span>
          </a>
        </div>
      )}
    </div>
  )
}

function getOutcomeLabel(outcome: string): string {
  switch (outcome.toLowerCase()) {
    case "convicted":
      return "Condenado"
    case "acquitted":
      return "Absolvido"
    case "dismissed":
      return "Arquivado"
    case "indicted":
      return "Denunciado"
    case "cited":
      return "Apenas citado"
    default:
      return valueLabel(outcome)
  }
}
