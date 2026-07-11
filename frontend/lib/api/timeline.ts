import type { GraphResponse } from "@/lib/types"
import { fetchJson } from "@/lib/api/client"

export function fetchTimeline(from: number, to: number): Promise<GraphResponse> {
  const params = new URLSearchParams({
    from: `${from}-01-01`,
    to: `${to}-12-31`,
  })
  return fetchJson<GraphResponse>(`/api/v1/timeline?${params.toString()}`)
}
