# Sanctions & Convictions Workers - Deep Dive

## Purpose

The authoritative "who was actually punished" layer, from official registries
that publish **CPF/CNPJ-keyed** records. Unlike DataJud/DJEN these are
deterministic: no name guessing, no scraping of court front-ends.

Two workers, one shared graph shape.

## Graph shape

New node + edge:

```txt
Sanction (id unique = registry + registry_entry_id)
  · registry        -- CEIS | CNEP | CEAF | LENIENCIA | TCU_IRREGULAR | TCU_INABILITADO | TCU_INIDONEO
  · sanction_type   -- registry-specific description
  · organ           -- sanctioning body
  · date_start, date_end (nullable)
  · process_ref     -- official process/acórdão reference
  · source_url      -- deep link to the official record

SANCTIONED_IN → Politician/Person/Organization → Sanction
```

Matching policy:

- Full CPF/CNPJ in source → deterministic upsert + edge. No review needed.
- Masked CPF (`***XXXXXX**`) → partial match against Politician CPFs; any hit
  → `pending_review` (type: `possible_politician_sanction`), never auto-link.
- Name-only rows → `pending_review` only.

## Worker 1: Portal da Transparência (CGU)

Registries: CEIS (inidôneas/suspensas), CNEP (Lei Anticorrupção), CEAF
(servidores expulsos), Acordos de Leniência.

**Endpoint**

```txt
GET https://api.portaldatransparencia.gov.br/api-de-dados/{ceis|cnep|ceaf|acordos-leniencia}
    ?pagina=N
Header: chave-api-dados: $TRANSPARENCIA_API_KEY
```

Free key, instant self-service signup at
`https://api.portaldatransparencia.gov.br/api-de-dados/cadastrar-email`.
Docs: `https://api.portaldatransparencia.gov.br/swagger-ui/index.html`.

Full-base bulk CSVs also exist under "Dados do Portal > Dados Abertos" — use
the API for incremental sync; fall back to CSV for the initial load if
pagination proves slow.

**Logic**

```txt
for each registry:
  paginate until empty page
  for each record: upsert Sanction, apply matching policy above
  CNPJ hit with no existing Organization → create node, trigger CNPJ Enricher
```

**Schedule** — weekly. Rate limit: 90 req/min (400/min 00:00–06:00); respect
with backoff.

## Worker 2: TCU lists

Lists: contas julgadas irregulares (Cadirreg), inabilitados para função
pública, licitantes inidôneos.

**Endpoints**

```txt
CSV bulk:  https://sites.tcu.gov.br/dados-abertos/inidoneos-irregulares/
           e.g. arquivos/resp-contas-julgadas-irregulares.csv
REST:      documented under https://sites.tcu.gov.br/dados-abertos/webservices-tcu/
           (contas irregulares queryable by nome/CPF/CNPJ/UF)
```

No auth. **Logic**: download CSVs, parse, same matching policy. Include
`process_ref` (acórdão) and deliberation link in every Sanction node.

**Schedule** — weekly.

## CNCIAI (CNJ improbidade registry) — deferred

The Cadastro Nacional de Condenações Cíveis por Ato de Improbidade
Administrativa e Inelegibilidade is the single best "convicted" source
(final improbidade convictions, searchable by CPF/CNPJ at
`https://www.cnj.jus.br/improbidade_adm/consultar_requerido.php`), but its
API requires an institutional request under Portaria CNJ 94. Action:
**send the request to CNJ** (pairs with the CNJ notification in
`legal_compliance.md`). Until granted, do not scrape the consultation form —
keep the project on documented public APIs only. Individual lookups during
backoffice review (a human checking one CPF manually) are fine and should be
linked as evidence.

## Why these sources are legally safe

All are official government open-data services published for exactly this
purpose (CC-alike terms, Lei de Acesso à Informação). Every displayed
sanction links back to the official record via `source_url` — this is the
"sourced from official records" defense described in
`docs/legal_compliance.md`.
