import type { ProceedingListResponse } from "@/lib/types"
import { fetchJson } from "@/lib/api/client"

// The conviction state is three-valued, so the filter is too. A boolean cannot
// express "DataJud has never looked at this case", which is the state 6,489 of our
// 6,503 cases are in — and the one a reader must never confuse with an acquittal.
export type ConvictionFilter = "true" | "false" | "unknown"

export interface ProceedingBrowseParams {
  q?: string
  court?: string
  has_conviction?: ConvictionFilter
  sort?: "case_number" | "court"
  page?: number
  pageSize?: number
}

/**
 * Fetch paginated, filterable court proceedings.
 * Supports search by CNJ number (formatted or bare), class name, or court.
 */
export async function fetchProceedings(
  params: ProceedingBrowseParams = {},
): Promise<ProceedingListResponse> {
  const search = new URLSearchParams()
  if (params.q) search.set("q", params.q)
  if (params.court) search.set("court", params.court)
  if (params.has_conviction) search.set("has_conviction", params.has_conviction)
  if (params.sort) search.set("sort", params.sort)
  if (params.page) search.set("page", String(params.page))
  if (params.pageSize) search.set("page_size", String(params.pageSize))

  const qs = search.toString()
  return fetchJson<ProceedingListResponse>(
    `/api/v1/proceedings${qs ? `?${qs}` : ""}`,
  )
}
