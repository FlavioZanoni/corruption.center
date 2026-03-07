import type { GraphResponse } from "@/lib/types"

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? ""

async function fetchJson<T>(url: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) throw new Error(`API error ${res.status}: ${res.statusText}`)
  return res.json() as Promise<T>
}

export async function fetchScandalGraph(id: string): Promise<GraphResponse> {
  return fetchJson(`${API_BASE}/api/v1/graph/scandal/${encodeURIComponent(id)}`)
}

export async function fetchPoliticianGraph(id: string): Promise<GraphResponse> {
  return fetchJson(`${API_BASE}/api/v1/graph/politician/${encodeURIComponent(id)}`)
}

export async function fetchExpandGraph(id: string, hops = 2): Promise<GraphResponse> {
  return fetchJson(`${API_BASE}/api/v1/graph/expand/${encodeURIComponent(id)}?hops=${hops}`)
}
