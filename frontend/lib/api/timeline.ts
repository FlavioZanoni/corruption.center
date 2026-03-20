import type { GraphResponse } from "@/lib/types"

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? ""

export async function fetchTimeline(from: number, to: number): Promise<GraphResponse> {
  const params = new URLSearchParams({
    from: `${from}-01-01`,
    to: `${to}-12-31`,
  })

  const res = await fetch(`${API_BASE}/api/v1/timeline?${params.toString()}`)
  if (!res.ok) throw new Error(`Timeline API error ${res.status}: ${res.statusText}`)
  return res.json() as Promise<GraphResponse>
}
