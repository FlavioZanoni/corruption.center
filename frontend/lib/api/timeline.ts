import type { TimelineResponse } from "@/lib/types"

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? ""

export async function fetchTimeline(from: number, to: number): Promise<TimelineResponse> {
  const params = new URLSearchParams({ from: String(from), to: String(to) })

  const res = await fetch(`${API_BASE}/api/v1/timeline?${params.toString()}`)
  if (!res.ok) throw new Error(`Timeline API error ${res.status}: ${res.statusText}`)
  return res.json() as Promise<TimelineResponse>
}
