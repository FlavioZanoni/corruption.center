import type { GraphResponse, SearchResponse, NodeType } from "@/lib/types"
import { fetchJson } from "@/lib/api/client"

function isSearchResponse(
  data: GraphResponse | SearchResponse,
): data is SearchResponse {
  return "results" in data && Array.isArray(data.results)
}

export async function searchNodes(
  query: string,
  type?: NodeType,
): Promise<SearchResponse> {
  const params = new URLSearchParams({ q: query })
  if (type) params.set("type", type)

  const data = await fetchJson<GraphResponse | SearchResponse>(
    `/api/v1/search?${params.toString()}`,
  )

  if (isSearchResponse(data)) {
    return data
  }

  // Backend returns a GraphResponse — adapt it to the UI SearchResponse shape.
  const results = (data.nodes ?? []).map((node) => ({
    id: node.id,
    type: node.type,
    label: node.label,
    properties: node.properties,
  }))

  return { results, total: results.length }
}
