# Sources & Workers

Each source maps to one Python worker. Workers are independent processes —
they run on their own schedule, write directly to Memgraph and Postgres,
and fail in isolation without affecting others.

> [!NOTE]
> Document maps some sources, not sure if all will be used
> Need a better study for each one of them

---

## Government Agencies

### TSE — Tribunal Superior Eleitoral

- **URL:** `dadosabertos.tse.jus.br`
- **Access:** REST API + CSV bulk downloads
- **Data:** Candidate profiles, party affiliations, electoral history, criminal
records (`certidões criminais`), asset declarations (`bens de candidatos`),
cassations (`motivo da cassação`)
- **Worker type:** API poller + bulk CSV importer
- **Schedule:** Full re-import each election cycle, incremental monthly
- **Notes:** This is your primary `Politician` node source. Has candidate data
going back to 2004. Criminal records available since ~2018 election.

### Portal da Transparência — CGU

- **URL:** `portaldatransparencia.gov.br/api-de-dados`
- **Access:** REST API (requires free API key via email registration)
- **Rate limit:** 90 req/min (06:00–23:59), 300 req/min (00:00–05:59)
- **Data:** Public contracts, government spending, supplier blacklists
(`CEIS`, `CNEP`, `CEPIM`)
- **Worker type:** API poller
- **Schedule:** Weekly
- **Notes:** CEIS (Cadastro de Empresas Inidôneas) and CNEP
(Cadastro Nacional de Empresas Punidas) are gold for `Organization`
nodes and their `IMPLICATED_IN` edges.

### TCU — Tribunal de Contas da União

- **URL:** `portal.tcu.gov.br`
- **Access:** Partial API + HTML scraping
- **Data:** Audit rulings, sanctioned officials, irregular contracts
- **Worker type:** Playwright scraper (JS-heavy pages)
- **Schedule:** Weekly
- **Notes:** TCU rulings naming politicians are high-reliability source for
`LegalProceeding` nodes.

### dados.gov.br — Portal Brasileiro de Dados Abertos

- **URL:** `dados.gov.br`
- **Access:** REST API (token-based)
- **Data:** Aggregator of datasets from all federal agencies
- **Worker type:** API poller
- **Schedule:** Monthly catalog scan to detect new relevant datasets
- **Notes:** Use as a discovery layer — when a new relevant dataset appears
(e.g. new CGU audit data), it triggers the appropriate worker.

---

## Judiciary

### DataJud — CNJ (Conselho Nacional de Justiça)

- **URL:** `datajud.cnj.jus.br`
- **Access:** Public REST API (public key, no registration required)
- **Data:** Process metadata from all Brazilian courts — STF, STJ, TRF1–6,
TJs estaduais, Justiça Eleitoral. Includes case number, tribunal,
jurisdiction level, procedural movements.
- **Worker type:** API poller
- **Schedule:** Weekly for known cases, daily for active proceedings
- **Notes:** This is the single most important judiciary source. Covers all
courts in one API. Does not expose classified (`segredo de justiça`) cases.

### STF — Supremo Tribunal Federal

- **URL:** `portal.stf.jus.br`
- **Access:** API available for decisions and ementas
- **Data:** Rulings, full decisions, súmulas
- **Worker type:** API poller
- **Schedule:** Weekly
- **Notes:** Supplement DataJud with full decision text for NLP extraction.

### STJ — Superior Tribunal de Justiça

- **URL:** `dadosabertos.web.stj.jus.br`
- **Access:** REST API + DJe XML feed
- **Data:** Rulings, acórdãos, Diário da Justiça Eletrônico, precedents
- **Worker type:** API poller + RSS/XML feed listener
- **Schedule:** Daily for DJe feed, weekly for bulk data
- **Notes:** STJ has jurisdiction over crimes committed by governors and
desembargadores — very relevant for your use case.

---

## Parliamentary

### Câmara dos Deputados

- **URL:** `dadosabertos.camara.leg.br`
- **Access:** REST API (well-documented, open)
- **Data:** Deputies, party affiliations, voting records, CPI proceedings,
expenses (`CEAP`)
- **Worker type:** API poller
- **Schedule:** Weekly for profiles, daily during active CPIs
- **Notes:** One of the best-maintained open data APIs in Brazilian government.

### Senado Federal

- **URL:** `legis.senado.leg.br/dadosabertos`
- **Access:** REST API
- **Data:** Senators, party affiliations, voting records, legislative proceedings
- **Worker type:** API poller
- **Schedule:** Weekly

---

## Official Gazettes

### Diário Oficial da União — Imprensa Nacional

- **URL:** `www.in.gov.br`
- **Access:** REST API + search
- **Data:** Official government acts — appointments, contracts, sanctions, dismissals
- **Worker type:** API poller with NER pipeline
- **Schedule:** Daily (DOU publishes daily)
- **Notes:** Dense text requiring heavy NLP. High value for catching new
sanctions and appointments. Run spaCy NER on every relevant act.

---

## NGOs & Watchdogs

### Brasil.IO

- **URL:** `brasil.io/api`
- **Access:** REST API (open)
- **Data:** Cleaned and structured TSE data, CNPJ data, other curated government
datasets
- **Worker type:** API poller
- **Schedule:** Monthly
- **Notes:** Maintained by Álvaro Justen (turicas). Often cleaner than the
raw government sources. Good as a cross-reference to validate politician data.

### Base dos Dados

- **URL:** `basedosdados.org`
- **Access:** BigQuery public datasets + API
- **Data:** Cleaned TSE electoral data going back to 1945, CNPJ, public spending
- **Worker type:** BigQuery importer
- **Schedule:** Monthly
- **Notes:** Best source for historical electoral data. Requires a Google Cloud
account for BigQuery access but data is free.

### Transparência Internacional Brasil

- **URL:** `transparenciainternacional.org.br`
- **Access:** HTML scraping
- **Data:** Corruption Perceptions Index, reports, named cases
- **Worker type:** Playwright scraper
- **Schedule:** Monthly

### Agência Pública

- **URL:** `agenciapublica.org.br`
- **Access:** HTML scraping + RSS
- **Data:** Investigative journalism, named politicians, documented scandals
- **Worker type:** RSS listener + Playwright scraper
- **Schedule:** Daily RSS, weekly deep scrape
- **Reliability:** `medium` (journalism, but investigative and well-sourced)

---

## News Outlets

All news workers follow the same pattern: RSS feed listener for new articles,
Playwright for full content, spaCy NER for name and entity extraction.
Reliability defaults to `medium`.

| Worker | URL | RSS Available | Notes |
| --- | --- | --- | --- |
| Folha de S.Paulo | `folha.uol.com.br` | Yes | Paywalled content needs Playwright |
| O Globo | `oglobo.globo.com` | Yes | |
| Estadão | `estadao.com.br` | Yes | |
| UOL Notícias | `noticias.uol.com.br` | Yes | |
| G1 | `g1.globo.com` | Yes | Broad reach |
| Metrópoles | `metropoles.com` | Yes | Good political coverage |
| Correio Braziliense | `correiobraziliense.com.br` | Yes | Strong Brasília coverage |
| Oeste | `revistaoeste.com.br` | Yes | |
| Agência Brasil | `agenciabrasil.ebc.com.br` | Yes | State agency, free content |

---

## Worker Architecture Notes

```txt
each worker is:
  - an independent Python process
  - scheduled via cron or a simple job queue (e.g. Redis + RQ)
  - idempotent: re-running should not create duplicate nodes
  - writes source record to Postgres first, then upserts nodes/edges in Memgraph
  - logs errors to Postgres (scraper_jobs table)
  - rate-limit aware with exponential backoff
```

### scraper_jobs table (Postgres)

```sql
CREATE TABLE scraper_jobs (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  worker       TEXT NOT NULL,
  status       TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed', 'skipped')),
  started_at   TIMESTAMPTZ DEFAULT now(),
  finished_at  TIMESTAMPTZ,
  records_upserted INT DEFAULT 0,
  error_message    TEXT
);
```

---

## Priority Order to be built

1. **TSE** — seeds all `Politician` nodes, everything depends on this
2. **Câmara + Senado APIs** — enriches politician profiles, current roles
3. **DataJud (CNJ)** — seeds `LegalProceeding` nodes
4. **Portal da Transparência** — CEIS/CNEP for `Organization` nodes
5. **DOU** — daily updates, new sanctions
6. **STF + STJ** — full decision text for NLP
7. **Agência Pública + Brasil.IO** — scandal context and cross-reference
8. **News outlets** — bulk of scandal narrative, NER-heavy
