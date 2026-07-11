import type { GraphResponse } from "@/lib/types"
import { fetchJson } from "@/lib/api/client"

export function fetchExpandGraph(id: string, hops = 2): Promise<GraphResponse> {
  return fetchJson<GraphResponse>(
    `/api/v1/graph/expand/${encodeURIComponent(id)}?hops=${hops}`,
  )
}
