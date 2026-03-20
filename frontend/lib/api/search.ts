import type { GraphResponse, SearchResponse, NodeType } from "@/lib/types"

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? ""

function isSearchResponse(data: GraphResponse | SearchResponse): data is SearchResponse {
  return "results" in data && Array.isArray(data.results)
}

export async function searchNodes(
  query: string,
  type?: NodeType
): Promise<SearchResponse> {
  const params = new URLSearchParams({ q: query })
  if (type) params.set("type", type)

  const res = await fetch(`${API_BASE}/api/v1/search?${params.toString()}`)
  if (!res.ok) throw new Error(`Search API error ${res.status}: ${res.statusText}`)
  const data = (await res.json()) as GraphResponse | SearchResponse

  if (isSearchResponse(data)) {
    return data
  }

  const results = (data.nodes ?? []).map((node) => ({
    id: node.id,
    type: node.type,
    label: node.label,
    properties: node.properties,
  }))

  return {
    results,
    total: results.length,
  }
}
