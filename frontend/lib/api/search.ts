import type { SearchResponse, NodeType } from "@/lib/types"

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? ""

export async function searchNodes(
  query: string,
  type?: NodeType
): Promise<SearchResponse> {
  const params = new URLSearchParams({ q: query })
  if (type) params.set("type", type)

  const res = await fetch(`${API_BASE}/api/v1/search?${params.toString()}`)
  if (!res.ok) throw new Error(`Search API error ${res.status}: ${res.statusText}`)
  return res.json() as Promise<SearchResponse>
}
