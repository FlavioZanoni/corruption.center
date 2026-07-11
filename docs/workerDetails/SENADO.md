# Senado Sync: Deep Dive

## Purpose

Fetches **all current federal senators** (`em exercício`) and upserts
`Politician` nodes.
Sets `active: true`, updates `party_current`, `role_current`, `photo_url`.
Source of truth for current mandate; overrides TSE data.

---

## Endpoint

`GET https://legis.senado.leg.br/dadosabertos/senador/lista/atual.json`
JSON only (append `.json` or `Accept: application/json`).
No auth. Returns **full current list in one call** (no pagination).

**Response structure** (root → `ListaParlamentarEmExercicio` → array of
senator objects):

Key fields:

- `IdentificacaoParlamentar.NomeParlamentar` / `NomeCompletoParlamentar` → `name`
- `IdentificacaoParlamentar.SiglaPartidoParlamentar` → `party_current`
- `IdentificacaoParlamentar.UfParlamentar` → `state`
- `IdentificacaoParlamentar.UrlFotoParlamentar` → `photo_url`
- `Mandato.DescricaoParticipacao` + `Exercicios` → confirm active ("Titular")

**No `cpf`** in list or `/historico.json`.

```json
{
  "ListaParlamentarEmExercicio": {
    "noNamespaceSchemaLocation": "https://legis.senado.leg.br/dadosabertos/dados/ListaParlamentarEmExerciciov4.xsd",
    "Metadados": {
      "Versao": "07/04/2026 11:18:00",
      "VersaoServico": "4",
      "DataVersaoServico": "2020-07-15",
      "DescricaoDataSet": "Lista dos Parlamentares que estão atualmente em Exercício. Informações atuais sobre o Parlamentar."
    },
    "Parlamentares": {
      "Parlamentar": [
        {
          "IdentificacaoParlamentar": {
            "CodigoParlamentar": "5672",
            "CodigoPublicoNaLegAtual": "800",
            "NomeParlamentar": "Alan Rick",
            "NomeCompletoParlamentar": "Alan Rick Miranda",
            "SexoParlamentar": "Masculino",
            "FormaTratamento": "Senador ",
            "UrlFotoParlamentar": "http://www.senado.leg.br/senadores/img/fotos-oficiais/senador5672.jpg",
            "UrlPaginaParlamentar": "http://www25.senado.leg.br/web/senadores/senador/-/perfil/5672",
            "EmailParlamentar": "sen.alanrick@senado.leg.br",
            "Telefones": {
              "Telefone": [
                {
                  "NumeroTelefone": "33036333",
                  "OrdemPublicacao": "1",
                  "IndicadorFax": "Não"
                }
              ]
            },
            "SiglaPartidoParlamentar": "REPUBLICANOS",
            "UfParlamentar": "AC",
            "Bloco": {
              "CodigoBloco": "346",
              "NomeBloco": "Bloco Parlamentar Aliança",
              "NomeApelido": "BLALIANÇA",
              "DataCriacao": "2023-03-20"
            },
            "MembroMesa": "Não",
            "MembroLideranca": "Sim"
          },
          "Mandato": {
            "CodigoMandato": "596",
            "UfParlamentar": "AC",
            "PrimeiraLegislaturaDoMandato": {
              "NumeroLegislatura": "57",
              "DataInicio": "2023-02-01",
              "DataFim": "2027-01-31"
            },
            "SegundaLegislaturaDoMandato": {
              "NumeroLegislatura": "58",
              "DataInicio": "2027-02-01",
              "DataFim": "2031-01-31"
            },
            "DescricaoParticipacao": "Titular",
            "Suplentes": {
              "Suplente": [
                {
                  "DescricaoParticipacao": "1º Suplente",
                  "CodigoParlamentar": "6343",
                  "NomeParlamentar": "Gemil Junior"
                },
                {
                  "DescricaoParticipacao": "2º Suplente",
                  "CodigoParlamentar": "6344",
                  "NomeParlamentar": "Coronel Casagrande"
                }
              ]
            },
            "Exercicios": {
              "Exercicio": [
                {
                  "CodigoExercicio": "3028",
                  "DataInicio": "2023-02-01"
                }
              ]
            }
          }
        }
      ]
    }
  }
}
```

---

## Processing logic

```txt
GET /senador/lista/atual.json
for each senator in response:
  upsert Politician (match by existing node / name + state + party):
    name ← NomeCompletoParlamentar or NomeParlamentar
    party_current ← SiglaPartidoParlamentar
    role_current ← "Senador"
    photo_url ← UrlFotoParlamentar
    state ← UfParlamentar
    active ← true
```

**Note**: CPF not available. Match/update existing nodes created by TSE import.
`historico.json` only adds links to filiações/mandatos (not needed for current sync).

---

## `active` field

Always `true` for senators in `/atual.json`. TSE keeps `false` for historical.

---

## Schedule

Weekly.
