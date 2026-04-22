# DataJud Worker (Watcher Skeleton)

Implements the initial DataJud watcher foundation with explicit verification
steps before state-machine writes.

## Security and key handling

- `DATAJUD_API_KEY` is read from env if provided.
- If missing, worker attempts to fetch key from DataJud wiki on startup.
- Key is not hardcoded.

## Verification modes (less brittle by default)

Before full state-machine automation, this worker verifies:

1. Movement codes `51`, `901`, `981` against TPU public pages.
2. Field presence from a real DataJud case probe response:
   - required core fields: `_source.numeroProcesso` and `_source.movimentos`
   - optional capability fields: `_source.processoRelacionado` and
     `_source.partes[].documento` (can be absent in some tribunals/datasets)

Defaults are non-strict to avoid brittle false failures caused by tribunal
payload variability. Use `--strict-verify=true` when you want fail-fast startup
behavior.

Use flags:

```bash
cd backend
DATABASE_URL="postgres://..." DATAJUD_API_KEY="..." \
go run ./workers/datajud/cmd \
  --verify-tpu=true \
  --strict-verify=false \
  --probe-case "5046512-94.2016.4.04.7000" \
  --probe-tribunal "api_publica_trf4" \
  --poll-limit 10
```

Enable write path (proceedings/defendants/related-case expansion):

```bash
cd backend
DATABASE_URL="postgres://..." \
MEMGRAPH_URI="bolt://localhost:7687" \
MEMGRAPH_USER="" MEMGRAPH_PASS="" \
DATAJUD_API_KEY="..." \
go run ./workers/datajud/cmd \
  --verify-tpu=true \
  --strict-verify=false \
  --probe-case "50001655420174047004" \
  --probe-tribunal "api_publica_trf4" \
  --enable-writes \
  --poll-limit 50
```

## Current scope

- Loads cases from `watcher_tracking` according to active/weekly-concluded rule.
- Polls DataJud by `numeroProcesso`.
- Skips restricted (`nivelSigilo > 0`).
- Updates `last_movement_id` and `last_polled_at` in Postgres.

State-machine writes for outcomes/defendants/case-tree expansion are available
behind `--enable-writes`.

## Live verification notes

- With the currently provided key, TRF/STJ endpoints respond; `api_publica_stf`
  returned 404 in live probing.
- Movement codes `51` and `901` were observed in TRF/STJ samples.
- Movement code `981` was observed in multiple TRF samples.
- Sampled responses did not always expose `processoRelacionado` and `partes`, so
  the worker guards for tribunal/dataset variability.
