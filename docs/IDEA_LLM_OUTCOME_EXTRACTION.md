# Idea: per-defendant outcomes via LLM extraction from DJEN texts

Status: **idea, not built** (2026-07-13). Parked for cost reasons, not design ones.
Revisit when: the backoffice outcome queue becomes the bottleneck, or the site
needs per-person convictions at scale.

## The problem this solves

The database's central claim — *this person was convicted* — cannot be automated
from structured data, and we have verified that empirically twice:

- **DataJud público** carries no party names anywhere. Audited ~2,900 raw
  movements across 6 cases and 3 tribunals: no `partes[]`, every `complementos`
  empty, every `complementosTabelados` a tabulated enum (Portaria CNJ 160/2020).
  A movement cannot attribute anything to a person.
- **DJEN publication texts** do name people, but regex-grade extraction fails.
  On a 5-case sample of confirmed convictions: 2 cases had no DJEN text at all
  (pre-2023 sentenças are simply absent), 1 case abbreviated every name to
  initials in the operative paragraph, and 1 case (Youssef, 5083376-05.2014)
  contained the roster format `NOME : condenado` — and then annulled two of
  those convictions three paragraphs later (STF, PET 12.648/DF). A ±N-chars
  window matcher would also have convicted an acquitted man whose name sits 50
  chars before someone else's "condenado".

So: names exist only in prose, and the prose reverses itself. Reading prose that
reverses itself is exactly what an LLM is for and exactly what a regex is not.

## The design

A batch job over **convicted criminal cases only** (case-level
`has_conviction = true`, derived by the DataJud watcher):

1. **Fetch** each case's DJEN comunicações (free, keyless, already implemented
   in `workers/djen/client.go`).
2. **Pre-filter mechanically** to texts containing dispositive language
   (folded contains: `condena`, `absolvi`, `improceden`, `extincao da
   punibilidade`, `transito em julgado`). Roughly half of a case's publications
   pass. Free.
3. **LLM reads** the filtered texts plus the case's defendant roster, and
   returns per defendant: `status` (condenado / absolvido / anulado /
   extinta a punibilidade / indeterminado), a **verbatim quote** carrying the
   determination, and the publication date of the quoted text.
4. **Mechanical validation** (free, non-negotiable):
   - the quoted span must appear **byte-for-byte** in the official text;
   - the defendant name must exactly match a roster entry (folded);
   - the quote must come from the **latest** dispositive publication mentioning
     that person (the Youssef lesson: later text wins);
   - anything failing any check is discarded before a human ever sees it.
5. **Output**: pre-filled rows in the backoffice outcome queue
   (`/backoffice/outcomes`), quote and link attached — one click to confirm,
   which writes through the existing `SetDefendantOutcome` path
   (`outcome_source='human'`, evidence URL mandatory for convictions).

### Publish modes considered

| Mode | Human effort | Risk |
|---|---|---|
| Pre-filled one-click queue (**recommended**) | ~5s/defendant | none published unreviewed |
| Auto-publish as quoted record ("a publicação oficial de DD/MM diz: '…'") | zero | an LLM misread goes public with our name on it |
| Stay fully manual (status quo) | full reading of each decision | none |

The auto-publish mode is legally *defensible* (verbatim quote of an official
publication, attributed to the document, never a naked "Condenado" badge) but
the Youssef annulment pattern shows the residual risk is real: the model must
get "which statement is operative" right.

## Cost (measured token estimates, 2026-07 prices)

~800 convicted cases × ~8k input / ~500 output tokens ≈ 6.4M in / 0.4M out.

| Model | Standard | Batch API (−50%) |
|---|---|---|
| Haiku 4.5 ($1/$5 per MTok) | ~$9 | ~$4.50 |
| Sonnet ($3/$15; intro $2/$10) | ~$17 | ~$8.50 |
| **Opus 4.8 ($5/$25)** | ~$42 | **~$21** |

Recommendation if built: **Opus 4.8 via the Batch API (~$21 one-time)** — the
failure mode is defamation, so this is not the place to save $15. Incremental
re-runs (only new publications on changed cases) cost pennies.

## Constraints to remember

- DJEN coverage starts ~2023; older convictions will mostly yield
  `indeterminado` — that is correct behavior, not a bug.
- 434 DJEN records are unservable at any page size (skipped, counted).
- Appeals: per-person status must be recomputed whenever the case gains new
  publications; never latch.
- Whatever the mode, the per-defendant write goes through the ONE existing
  path (`SetDefendantOutcome`) so audit, provenance and sanitization keep
  holding.

## Appendix: why no official "who is convicted" API exists — and the one registry that does

**The gap is deliberate, not incompetence.** A criminal conviction is personal
data under the LGPD, and Brazilian doctrine adds the right to resocialization
(the "direito ao esquecimento" line of cases): criminal records are issued as
per-person, per-purpose certidões, never as a browsable database. Portaria CNJ
160/2020 strips party names from DataJud *on purpose*. A live public registry
of convicted individuals would also be operationally defamatory: appeals
reverse, prescrição extinguishes, segredo de justiça hides — any snapshot is
wrong for someone within weeks (we watched one document convict and un-convict
the same man).

**The exceptions are exactly the corruption-shaped ones**, where public
interest overrides privacy — and we already ingest most of them:

| Registry | Per-person? | Machine-readable? | Status here |
|---|---|---|---|
| CEIS / CNEP / CEAF / leniência (CGU) | yes | yes (API, keyed) | **ingested** (64k sanctions) |
| TCU inabilitados / inidôneos / contas irregulares | yes | yes | **ingested** |
| TSE inelegibilidade (Ficha Limpa) | yes | partially | only covers people who run for office |
| **CNJ CNCIAI — Cadastro Nacional de Condenações Cíveis por Ato de Improbidade Administrativa e Inelegibilidade** | **yes — named individuals, the condemning court, the article of Lei 8.429/92** | **no API found** (consultation portal alive at cnj.jus.br/improbidade_adm; CNJ open-data endpoints unresponsive) | **not ingested — the biggest untapped source** |

The CNCIAI is the closest thing Brazil has to the API being asked about:
improbidade administrativa is corruption's civil twin (same conduct, civil
liability), and the registry exists precisely so the public can check names.

**There is an official API, and we are exactly who it is for.** Portaria 94
(CNJ/STF) allows public bodies *and interested institutions, including news
media*, to connect to the registry via API so it can be "associated with other
services and products offered to the public, without the need for individual
consultations". So the action is **request API access under Portaria 94** — not
a LAI request, and not scraping. Scraping would be slower, more fragile, and
would trade a credential CNJ granted us for something we took.

What it yields: named individuals and companies convicted under Lei 8.429/92,
plus those made ineligible under the Ficha Limpa (LC 135), searchable by
**CPF/CNPJ**, name, or case number, with the condemning court. The CPF/CNPJ is
the prize — it is a DOCUMENT, so these records fuse straight onto the sanctions
island instead of stranding in the name-only DJEN island. "A document
identifies. A name only leads."

Publishing it (lawyer should still sign off, but the footing is strong): the
registry is public by design, its purpose is public consultation, and Portaria
94 explicitly contemplates redistribution by media. Two conditions treat as
non-negotiable: **stay current** (an improbidade conviction can be overturned;
a stale "condenado" badge is precisely the defamation this project exists to
avoid — re-sync, and let removals remove), and **show source + date on every
record**. The provenance model and backoffice already do both.

Court front-ends remain off-limits: that is where LGPD-protected criminal data
lives. An official anti-corruption registry that publishes an API for
redistribution is a different animal entirely.

## Appendix 2: nobody else has this API either

No country publishes a browsable "who is a convicted criminal" database. The
reason is identical everywhere: criminal records are issued as per-person
certificates so convictions can expire, be sealed, and be reversed.

| | Criminal convictions by name | Corruption / integrity registry |
|---|---|---|
| **Brazil** | none (LGPD + resocialização; DataJud strips names on purpose) | **excellent** — CGU, TCU, TSE, CNJ improbidade; all APIs |
| **US** | federal dockets carry names (PACER; free via CourtListener/RECAP) but per-docket, no "convicted persons" endpoint; states are a patchwork | good — SAM.gov exclusions, OFAC, FARA |
| **UK** | none; DBS checks are consent-based, per-person | good — Companies House disqualified directors, free API |
| **Canada** | none; CPIC is police-only; court records per-province | weaker — federal Ineligibility & Suspension list |

(Sketch from knowledge, not researched — verify before relying on it.)

The US is the only one that yields named criminal defendants at scale, and only
as a byproduct of open dockets rather than by design. Everywhere else the shape
is Brazil's: **convictions are private, corruption sanctions are public.**

Which validates the architecture: the spine of a corruption graph is the
sanctions and integrity registries, not criminal convictions. On that axis
Brazil is ahead of the UK and Canada. The thing that felt like a Brazilian
failure is a thing nobody has.
