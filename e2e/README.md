# e2e — Playwright frontend smoke tests

End-to-end tests that drive the real public frontend in a browser against the
live API and graph. They assert the load-bearing invariants: search works, a
result opens its page, and the legal-safety guarantees hold (the
"não é condenação" disclaimer shows, no "Réu em" label, no unmasked CPF).

## Running

The stack normally binds `:3000` (frontend) and `:8080` (api). If those are
taken by other projects, bring it up on the remapped ports with the override:

```sh
# from the repo root
docker compose -f docker-compose.dev.yml -f docker-compose.playwright.yml \
  up -d postgres memgraph api frontend
```

That serves the **frontend on :3001** and the **api on :8090**, and adds
`http://localhost:3001` to the API's CORS allowlist (the browser calls the API
cross-origin via `NEXT_PUBLIC_API_URL`).

Then:

```sh
cd e2e
npm install
npx playwright install chromium
npx playwright test
```

Point at a different host with `E2E_BASE_URL` / `E2E_API_URL` if your ports
differ (defaults: `http://localhost:3001` and `http://localhost:8090`).

The person-detail specs resolve a real Person id from the API at runtime, so no
test hardcodes data that a re-import could change.
