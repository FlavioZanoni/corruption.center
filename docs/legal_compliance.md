# Legal Compliance

---

## Public-facing requirements

### Contact page

A visible contact page or footer link on the site with:

- A public email address (dedicate one, e.g. `contato@[domain]` or `privacidade@[domain]`)
- A statement that data removal requests are accepted and will be processed within 15 days (LGPD art. 18 deadline is not fixed but 15 days is the standard adopted by ANPD)
- A link to the methodology page

### Methodology page

This is the most important legal protection. It must clearly explain:

1. **What the project is** — a transparency tool built exclusively from official Brazilian public records for public interest accountability purposes (LGPD art. 23)
2. **Where every data type comes from** — list each source (Câmara, Senado, TSE, DataJud/CNJ, Receita Federal via CNPJ.ws) with links
3. **What "pending confirmation" means** — explain that links between private individuals and politicians are never created automatically, always require human review before being displayed
4. **How to request removal** — direct link to contact, explain the process
5. **Limitations disclaimer** — data reflects official records and may contain errors from the source; the project does not add or infer information beyond what official records state

---

## CNJ notification

Send an email to CNJ before or at public launch. The notification must describe:

- What the project is and its public interest purpose
- That you are using the DataJud public API within the 120 req/min limit
- A link to the live project or a description if not yet live
- Your contact information

Contact: `https://www.cnj.jus.br/sistemas/datajud/api-publica/`

This is a notification, not a request for approval. You are not waiting for a response to proceed.

---

## Backoffice requirements

### Data removal requests

- A dedicated queue for incoming removal requests (email or form)
- For each request: record requester identity, date received, node/edge targeted, resolution, date resolved
- Ability to soft-delete or fully purge a `Person` node and all its edges with a full audit trail in Postgres
- Politicians cannot be removed — they are public officials and LGPD art. 23 applies; document this policy explicitly in the backoffice

### Person node controls

- Every `Person` node must display its creation reason (which proceeding or organization triggered it)
- A one-click purge that removes the node, all edges, and writes a deletion record to `audit_log`
- If a `Person` node is later found to have no legitimate connection to a scandal or proceeding, it must be removable without leaving orphaned edges

### Pending review discipline

- `possible_politician_in_qsa` items in `pending_review` must never auto-approve - always require explicit human action
- Rejected items must be recorded with reason so the same false match is not re-flagged by the watcher on the next run
- Display a clear "unconfirmed — pending review" label on any node or edge that has not yet been confirmed by a human

### Audit log visibility

- The backoffice must surface who created or modified each node/edge and when
- This is both an internal quality tool and LGPD compliance — if someone requests to know why their data is in the system, you can answer precisely

---

## What immediate action looks like and how to reduce the risk

| Actor | Can act immediately? | Likely trigger | Mitigation |
| --- | --- | --- | --- |
| CNJ | Yes — API revocation, no warning needed | Abuse of rate limit or commercial use | Stay within 120 req/min, keep project non-commercial |
| ANPD | Unlikely without prior notificação | Serious ongoing LGPD violation | Public interest framing, removal request process |
| Civil court (injunction) | Yes — tutela de urgência can be granted in hours | Politician claims defamation | Methodology page, "sourced from official records" disclaimer on every node, pending confirmation labels |
| Criminal | Unlikely for this type of project | — | — |

The methodology page and the "sourced from official records" label on every displayed fact are the primary defenses against a defamation injunction. A judge seeing that every claim links back to a DataJud case number or a Câmara API record is far less likely to grant emergency relief.
