// ─── Shared API client ────────────────────────────────────────────────────────
// Single source of truth for the API base URL and JSON fetching.
//
// NEXT_PUBLIC_API_URL must point at the Go API (e.g. http://localhost:8080).
// There is deliberately no fallback: this app once shipped a mock-data mode
// that silently served fabricated corruption records whenever the variable was
// unset, and a fake graph that looks plausible is worse than an error. A
// misconfigured base now fails loudly on the first fetch.

export const API_BASE = (process.env.NEXT_PUBLIC_API_URL ?? "")
  .trim()
  .replace(/\/+$/, "") // tolerate a trailing slash in the configured base

export async function fetchJson<T>(path: string, init?: RequestInit): Promise<T> {
  if (API_BASE === "") {
    throw new Error(
      "NEXT_PUBLIC_API_URL is unset — the frontend has no API to talk to. " +
        "Set it to the Go API base URL (e.g. http://localhost:8080).",
    )
  }
  const res = await fetch(`${API_BASE}${path}`, init)
  if (!res.ok) {
    throw new Error(`API error ${res.status}: ${res.statusText} (${path})`)
  }
  return res.json() as Promise<T>
}
