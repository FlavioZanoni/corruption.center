# Identity Matching

How the project decides that a record from an official source refers to a
specific person in the graph, and when that decision may be made without a human.

This is the single source of truth for the policy. `docs/legal_compliance.md`,
`docs/workerDetails/DJEN.md` and `docs/workerDetails/SANCTIONS.md` point here
rather than restating it. Implementation: `backend/matching`.

## The problem

Official sources are trustworthy about **what happened** and frequently silent
about **who it happened to**.

- **DJEN** (court publications) lists party names with **no document at all**.
  It is the only official source that publishes case parties, and it never
  publishes a CPF.
- **CGU** masks CPFs on the person-level registries: `***.435.151-**`. Only the
  six middle digits are visible.
- **DataJud** does not publish parties at all (Portaria CNJ 160/2020).

So linking "a person named X" to "our politician X" is an **inference we make**,
not a fact the source states. A source link does not cure a wrong inference: the
source never claimed it was the politician, we did.

The risk is concrete, not theoretical. A single DJEN search for a party named
"JOSE SILVA" returns ~10,000 publications belonging to visibly different people
(`ANTONIO JOSE SILVA VIANA`, `ANTONIO JOSE SILVA DO NASCIMENTO`, ...). Publishing
"Deputy X is a defendant in a corruption case" when it is a different X is
defamation, regardless of how impeccable the source link is.

## The rule

**A document identifies. A name only leads.**

`matching.Score()` weights the evidence into a confidence in [0,1], and
`matching.AutoLinkThreshold` (0.85) decides whether a link may be written with no
human in the loop. The weights are calibrated so that **no combination of
name-only evidence can reach the threshold**.

| Evidence | Score | Outcome |
| --- | --- | --- |
| Full CPF/CNPJ published by the source (`full_document`) | 1.00 | auto-link |
| Masked CPF middle-6 + exact name + 4+ name tokens | 0.95 | auto-link |
| Masked CPF middle-6 + exact name | 0.90 | auto-link |
| Masked CPF middle-6, name does not match | 0.60 | human review |
| Exact name, no document | 0.30 to 0.35 | human review |
| Any evidence that fits more than one politician (`ambiguous_match`) | capped at 0.50 | human review |

Signals recorded on the edge: `full_document`, `masked_cpf_middle6`,
`exact_name`, `long_name`, `ambiguous_match`.

Names are compared after normalization (uppercase, accents folded, whitespace
collapsed), so "José da Silva" equals "JOSE DA SILVA". A substring is never a
match: "JOSE SILVA" does not match "ANTONIO JOSE SILVA VIANA".

## What gets written

Every auto-created edge carries its evidence, so any link can be explained after
the fact:

- `confidence` (float) and `confidence_signals` (list of signal names), written
  on `SANCTIONED_IN` by the sanctions worker.
- `source` (`backoffice_review` for a human-approved edge, otherwise the worker
  that created it: `djen`, `sanctions`, ...).

The frontend surfaces this on every connection: either "Confirmado por revisao
humana" or "Identificacao: N%" with the signals, and `/metodologia` explains the
tiers to the public. See `frontend/components/DetailPanel.tsx` (ProvenanceBadge).

## Claims vs. records

Not everything a worker does is a claim about a person, and only claims need the
gate:

- **Registering a case for watching asserts nothing about anybody.** It creates a
  `LegalProceeding` node and a `watcher_tracking` row so the case is polled. DJEN
  name mode does this automatically, with no review (see `DJEN.md`).
- **"This politician is a defendant in this case"** is a claim about a person, and
  DJEN gives only a name, so it can never be auto-created. It always goes to
  `pending_review` (`djen_party_match`).
- **"This politician was sanctioned"** is a claim about a person. CGU sometimes
  gives a full document (auto-link) and sometimes a masked one (scored per the
  table above).

## Review types still gated on a human

| Type | Why it cannot be automated |
| --- | --- |
| `djen_party_match` | DJEN publishes names with no document, ever. |
| `possible_politician_in_qsa` | Company partner lists match on name. |
| `possible_politician_sanction` | Masked CPF that did not reach document grade, or fits several people. |
| `unknown_cnpj` | Data quality: attaching a CNPJ to a name-only Organization. |

## Rematch

A party is matched against the politician index exactly once, at discovery, and
is then snapshotted so it never reappears in a roster delta. When the politician
base grows (a TSE import adding governors, or older election years), defendants
discovered earlier would stay anonymous forever.

`djen -rematch-mode` re-tests every existing `Person` defendant against the
current index and files a `djen_party_match` review for each hit. For a pre-2023
case, whose party list can never be re-fetched from any official source, this is
the only path by which it can ever gain a politician.
