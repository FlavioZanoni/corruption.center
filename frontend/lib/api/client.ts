// ─── Shared API client ────────────────────────────────────────────────────────
// Single source of truth for the API base URL and JSON fetching.
//
// Empty NEXT_PUBLIC_API_URL → same-origin Next.js mock routes (default).
// Set it to the Go API base URL (e.g. http://localhost:8080) to hit the backend.

export const API_BASE = (process.env.NEXT_PUBLIC_API_URL ?? "")
  .trim()
  .replace(/\/+$/, "") // tolerate a trailing slash in the configured base

export async function fetchJson<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, init)
  if (!res.ok) {
    throw new Error(`API error ${res.status}: ${res.statusText} (${path})`)
  }
  return res.json() as Promise<T>
}
