// Server-rendered page for a single sanction.
//
// Sanctions are the largest body of data we hold (64,779 nodes, 52,058 links to a
// party) and had no page at all: selecting one in search navigated nowhere, and
// every list that mentioned one dead-ended. A sanction is also the one record here
// that needs no hedging — unlike a court case, it IS the finding: a public body
// published it, and we link straight back to that publication.

import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft, ExternalLink } from "lucide-react"

import { fetchSanctionDetail, type SanctionedPartyRecord } from "@/lib/api/entities"
import { formatDate } from "@/lib/entity-labels"
import { formatDocument } from "@/lib/format"
import { siteUrl } from "@/lib/seo"
import { entityHref, EmptySection, LegalDisclaimer, Section } from "@/components/EntityUI"

export const revalidate = 3600

type Props = { params: Promise<{ id: string }> }

// A sanctioned company frequently reaches us as a bare CNPJ: the registries publish
// the document, not the razão social. Naming it "(sem nome)" would be a lie of
// omission — we know exactly who it is, we just know them by their document.
function partyLabel(p: SanctionedPartyRecord): string {
  if (p.name && p.name.trim() !== "") return p.name
  const doc = p.cnpj ?? p.cpf
  return doc ? formatDocument(doc) : p.id
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id } = await params
  const data = await fetchSanctionDetail(id)
  if (!data) return { title: "Sanção não encontrada" }

  const s = data.sanction
  const who = data.parties.length > 0 ? partyLabel(data.parties[0]) : null
  const title = who
    ? `${s.sanction_type} — ${who} (${s.registry})`
    : `${s.sanction_type} — ${s.registry}`

  return {
    title,
    description: `Sanção ${s.registry} aplicada por ${s.organ}.`,
    alternates: { canonical: `${siteUrl}/sancao/${encodeURIComponent(s.id)}` },
  }
}

export default async function SanctionPage({ params }: Props) {
  const { id } = await params
  const data = await fetchSanctionDetail(id)
  if (!data) notFound()

  const s = data.sanction
  const period = [formatDate(s.date_start ?? ""), formatDate(s.date_end ?? "")]
    .filter(Boolean)
    .join(" — ")

  return (
    <main className="min-h-screen bg-bg text-text">
      <div className="max-w-3xl mx-auto px-6 py-8">
        <nav className="flex items-center gap-4 text-xs font-mono mb-6">
          <Link
            href="/sancoes"
            className="inline-flex items-center gap-1 text-text-muted hover:text-white transition-colors"
          >
            <ArrowLeft size={12} strokeWidth={1.5} />
            Sanções
          </Link>
        </nav>

        <header className="mb-8">
          <span className="text-[10px] font-mono uppercase tracking-widest text-[#d98a4b]">
            {s.registry}
          </span>
          <h1 className="mt-2 text-2xl font-serif text-white">{s.sanction_type}</h1>
          <p className="mt-2 text-sm text-text-muted">
            Aplicada por <span className="text-text">{s.organ}</span>
          </p>
          {period && (
            <p className="mt-1 text-xs font-mono text-text-dim">{period}</p>
          )}
          {s.process_ref && (
            <p className="mt-1 text-xs font-mono text-text-dim">
              Processo administrativo: {s.process_ref}
            </p>
          )}
        </header>

        <Section title="Quem foi sancionado">
          {data.parties.length === 0 ? (
            <EmptySection>
              O cadastro oficial não vincula esta sanção a uma pessoa ou empresa
              identificável em nossa base.
            </EmptySection>
          ) : (
            <ul className="space-y-2">
              {data.parties.map((p) => {
                const href = entityHref(p.type.toLowerCase(), p.id)
                const label = partyLabel(p)
                const doc = p.cnpj ?? p.cpf
                const inner = (
                  <>
                    <span className="text-sm text-text group-hover:text-white transition-colors">
                      {label}
                    </span>
                    {doc && p.name && (
                      <span className="block text-[10px] font-mono text-text-dim mt-0.5">
                        {formatDocument(doc)}
                      </span>
                    )}
                  </>
                )
                return (
                  <li
                    key={p.id}
                    className="border border-border rounded-sm px-4 py-3 bg-surface"
                  >
                    {href ? (
                      <Link href={href} className="group block">
                        {inner}
                      </Link>
                    ) : (
                      inner
                    )}
                  </li>
                )
              })}
            </ul>
          )}
        </Section>

        {s.source_url && (
          <Section title="Registro oficial">
            <a
              href={s.source_url}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1.5 text-sm text-[#c8a96e] hover:underline break-all"
            >
              {s.source_url}
              <ExternalLink size={12} strokeWidth={1.5} className="shrink-0" />
            </a>
          </Section>
        )}

        <LegalDisclaimer />
      </div>
    </main>
  )
}
