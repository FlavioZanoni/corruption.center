# Convictions, sanctions, and the two islands

## The question this database exists to answer

*Who was convicted, and who was merely cited?*

Today the graph cannot answer it, and the reason is not a missing feature. It is
structural, and worth writing down before someone "fixes" it the fast way.

## The graph is two disconnected islands

**Island A — the scandal side.** 3 scandals → 4 proceedings → 10 defendants.
People arrive here through DJEN, which publishes **party names and never a
document**. No CPF, ever.

**Island B — the sanction side.** 52,058 `SANCTIONED_IN` edges over 31,454
entities (16,938 Person, 14,235 Organization, 281 Politician), from CEIS, CNEP,
CEAF, leniência and TCU. Everyone here arrived **with a CPF or a CNPJ**.

The islands do not touch. Not one of the 10 defendants carries a single sanction,
while 52k sanctions sit on entities that are party to no case.

They cannot touch automatically, and that is deliberate. From `backend/matching`:

> A document identifies. A name only leads.

A name alone scores at most 0.35, below the 0.85 auto-link threshold, so a DJEN
defendant is **never** fused with a sanctioned person by name. Which is correct:
"JOSE SILVA" the defendant and "JOSE SILVA" the sanctioned contractor are not the
same human just because a string matched.

So the punished entities are not the ones in the scandals — and the graph looks
empty while holding 64,779 sanctions.

## The bridge is the Politician, and it is one human decision wide

A `Politician` is the only node that exists on **both** islands:

- TSE gives them a **CPF** → the sanctions worker matches them by document
  (281 politicians already carry sanctions).
- DJEN gives them a **name** → an exact name match files a `djen_party_match`
  review rather than an edge, because a name is only a lead.

Approve one of those reviews and `Politician -[DEFENDANT_IN]-> LegalProceeding`
appears. At that instant the two islands fuse at that node: the same politician
now carries their prosecution *and* their punishments, and the canvas can draw a
path from a scandal to a CGU sanction.

**There are currently zero `Politician -[DEFENDANT_IN]->` edges.** That single
number is why the product looks unfinished.

The queue that produces them is small — cases attached to scandals you actually
publish, not the 457 DJEN leads.

## Why per-person conviction cannot be automated

`LegalProceeding.has_conviction` is real, official, and well-built: it comes from
DataJud's TPU movement codes (60 Condenação / 61 Absolvição / 848 Sentença), the
**last** disposition wins, and an acquittal on appeal **clears** the flag rather
than latching it. The code comment calls latching it "defamation-grade", and it is
right.

But it is **case-level**. A case with ten defendants that ends in a conviction
says nothing about which of the ten. And no official source we use closes that
gap:

| source | gives us | per-person outcome? |
|---|---|---|
| DataJud (CNJ) | case movements | **no** — the public API exposes no parties at all |
| DJEN | party names | **no** — names only, no outcome, no document |
| CGU / TCU | sanctions per CPF/CNPJ | yes, but these are *administrative*, not criminal |

### The inference we rejected, and the real name it would have libelled

The tempting rule:

```
if lp.has_conviction && len(known_defendants) == 1:
    mark that defendant convicted
```

It is wrong, and the proof is already in the data.

Case `05095035720164025101` is **Operação Calicute**. Our roster knows exactly one
defendant: **ADRIANA DE LOURDES ANCELMO**. Sérgio Cabral — the principal defendant
— **is not in our roster at all**, because DJEN only handed us the publications
that happened to name her.

The moment DataJud polls that case and flips `has_conviction`, that rule publishes
a criminal conviction against a named private individual, alone, because our data
was missing the co-defendant.

The flaw is structural, not a bug to patch: **"exactly one defendant" is a fact
about our roster, never about the case.** DJEN only names parties in publications
we fetched, its coverage starts in 2021, we cap at 300 items per name, and we skip
records it will not serve. A thin roster is our blind spot, and that rule reads our
blind spot as certainty.

480 of 484 cases have `has_conviction = null` — DataJud has not polled them. The
457 cases DJEN just discovered were found *by searching a politician's name*, so
their roster will very often be exactly that one politician. The rule would
auto-convict them en masse.

## What we do instead

1. **Conviction is shown on the case, never on the person.** The canvas draws a red
   ring on a proceeding whose `has_conviction` is true. A reader sees *"Fulano is a
   defendant in a case that ended in a conviction"* — true, official, defensible —
   and draws their own line. We never assert *"Fulano was convicted"*, because we
   cannot source it.

2. **Sanctions carry the volume.** 52,058 per-person, per-company, official,
   structured outcomes with a `portaldatransparencia.gov.br` URL on every one, and
   **zero human review required**. This is the scalable answer to "who was
   punished". The timeline now returns them; they render the moment their entity
   becomes a party to a case.

3. **Per-person criminal conviction stays human-only.** An operator records it with
   the decision attached. Workers may never write it. The queue is dozens of
   headline cases, not thousands — and that judgment *is* the product, not overhead.

## If you want to automate per-person conviction later

The only honest route is the decision text itself — the sentença/acórdão names who
was convicted and who was acquitted, and DJEN already carries it in `Item.Texto`.
That is inference from unstructured legal prose, and a parsing error becomes a
false public accusation of a crime. If it is ever built it needs: a confidence
score, the quoted passage stored as evidence, and a human in the loop above some
threshold. It is not a weekend job.
