# corruption.center

## Contract Verification

Run this before opening a PR:

```bash
./verify-contracts.sh
```

The script does three checks:

1. Regenerates backend Swagger docs from handler annotations.
2. Runs backend Go tests.
3. Builds frontend (includes TypeScript checks).

## Frontend API Modes

The frontend supports two API modes:

- Mock Next.js API routes (default): leave `NEXT_PUBLIC_API_URL` empty.
- Real Go API: set `NEXT_PUBLIC_API_URL` to the backend base URL (example: `http://localhost:8080`).

Current compatibility notes:

- Search: frontend adapter accepts backend `GraphResponse` and maps it to UI `SearchResponse`.
- Timeline: frontend sends `from`/`to` as `YYYY-MM-DD`, matching Go API expectations.
