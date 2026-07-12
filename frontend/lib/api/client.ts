// ─── Shared API client ────────────────────────────────────────────────────────
// Single source of truth for the API base URL and JSON fetching.
//
// Empty NEXT_PUBLIC_API_URL → same-origin Next.js mock routes (default).
// Set it to the Go API base URL (e.g. http://localhost:8080) to hit the backend.

export const API_BASE = (process.env.NEXT_PUBLIC_API_URL ?? "")
  .trim()
  .replace(/\/+$/, "") // tolerate a trailing slash in the configured base

// IS_MOCK_MODE is true when no API base is configured, so every fetch is served
// by the Next.js mock routes under app/api/v1.
//
// This has to be loud. The mocks return 26 nodes and 39 edges of entirely
// fabricated graph — plausible names, plausible scandals, node types the real
// database does not even contain — and they are the DEFAULT. Docker sets the base
// so compose is unaffected, but `pnpm dev` on its own serves invented corruption
// data with nothing on screen to say so. Someone debugging "why does the graph
// look wrong" can spend an afternoon reading a graph that was never real.
export const IS_MOCK_MODE = API_BASE === ""

if (IS_MOCK_MODE && typeof window !== "undefined") {
  console.warn(
    "[corruption.center] MOCK DATA. NEXT_PUBLIC_API_URL is unset, so every " +
      "response is fabricated by app/api/v1/*, not the Go API. Nothing you see " +
      "is real. Set NEXT_PUBLIC_API_URL=http://localhost:8080 to use the backend.",
  )
}

export async function fetchJson<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, init)
  if (!res.ok) {
    throw new Error(`API error ${res.status}: ${res.statusText} (${path})`)
  }
  return res.json() as Promise<T>
}
