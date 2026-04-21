# Câmara Sync — Deep Dive

## Purpose

Fetches **all currently active federal deputies** and upserts them
as `Politician` nodes.
Sets `active: true`, updates `party_current`, `role_current`, `photo_url`.
Source of truth for current mandate — overrides TSE data.

By default, `GET /deputados` (no time params) returns **only deputies in
exercise at request time**.

---

## Endpoints

Base: `https://dadosabertos.camara.leg.br/api/v2/`
JSON, no auth, paginated (`pagina`, `itens` ≤ 100).

### 1. `GET /deputados` — Current list (default = active only)

No `idLegislatura` or date params needed for current.
Optional: `siglaUf`, `siglaPartido`, `itens=100`.

Response fields (from `dados`):

- `id` → `camara_id`
- `nome` → `name`
- `siglaPartido` → `party_current`
- `siglaUf` → `state`
- `urlFoto` → `photo_url`
- `cpf` (in some responses / via detail)

### 2. `GET /deputados/{id}` — Full details

Required for: `cpf`, `nomeCivil`, `ultimoStatus` (confirm "Em exercício"),
`sexo`, birth data, etc.

Key: `ultimoStatus.siglaPartido`, `urlFoto`, `descricaoStatus`.

---

## Processing logic

```txt
for pagina = 1 to totalPaginas:
  GET /deputados?itens=100&pagina={pagina}
  for each deputy in dados:
    GET /deputados/{deputy.id}
    upsert Politician by cpf:
      name ← nome (or nomeCivil)
      party_current ← siglaPartido
      role_current ← "Deputado Federal"
      photo_url ← urlFoto
      state ← siglaUf
      active ← true
```

---

## Politician node mapping

- `cpf`: dedup key (from detail)
- `name`: from list or nomeCivil
- `party_current`, `role_current`, `state`, `photo_url`, `active: true`

TSE remains historical (`active: false`). Câmara overrides for current deputies.

Corrected version. Use list endpoint without time filters for current mandate.
