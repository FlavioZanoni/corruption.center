# Sources definition

## We will define sources categorizing them by `type` and `reliabilyty`

### Source Types

| Type | Description | Examples |
| --- | --- | --- |
| `news_outlet` | Journalism and press | Folha de S.Paulo, O Globo, Oeste, Agência Pública |
| `government_agency` | Federal/state administrative bodies | TCU, TSE, Portal da Transparência, CGU |
| `court_document` | Judiciary rulings and filings | STF, STJ, TRF1–6, TJs estaduais |
| `parliamentary` | Congressional records and hearings | CPIs, DCOs, Câmara, Senado |
| `official_gazette` | Official government publications | Diário Oficial da União, Diários estaduais |
| `ngo_watchdog` | Civil society and watchdog orgs | Transparência Internacional, Contas Abertas, OKBR |
| `academic` | Research and academic databases | FGV, CPDOC, peer-reviewed papers |

---

### Default Reliability by Type

| Type | Default Reliability | Rationale |
| --- | --- | --- |
| `court_document` | `high` | Judiciary rulings, legally binding |
| `official_gazette` | `high` | Official government record |
| `government_agency` | `high` | Authoritative administrative source |
| `parliamentary` | `high` | Official congressional record |
| `ngo_watchdog` | `medium` | Curated but not official |
| `news_outlet` | `medium` | Can report unverified accusations |
| `academic` | `medium` | Reliable but may lag current events |

---

### Table Definition

```sql
CREATE TABLE sources (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  url            TEXT NOT NULL UNIQUE,
  title          TEXT NOT NULL,
  publisher      TEXT NOT NULL,
  type           TEXT NOT NULL CHECK (type IN (
                   'news_outlet',
                   'government_agency',
                   'court_document',
                   'parliamentary',
                   'official_gazette',
                   'ngo_watchdog',
                   'academic'
                 )),
  reliability    TEXT NOT NULL CHECK (reliability IN ('high', 'medium', 'low')),
  date_published DATE,
  date_scraped   TIMESTAMPTZ DEFAULT now(),
  raw_content    TEXT,
  checksum       TEXT,
  active         BOOLEAN DEFAULT true
);
```

### Notes

- `raw_content` stores the original scraped text, allowing NLP re-runs without
re-scraping when the pipeline improves.
- `reliability` is set automatically by the Python ingestion pipeline based on
`type`, then can be adjusted manually.
- `active` allows soft-deleting sources that become unavailable or are found to
be unreliable without losing the reference.
- The `id` is referenced as `source_id` on Memgraph edge properties to link
graph relationships back to their backing documents.
