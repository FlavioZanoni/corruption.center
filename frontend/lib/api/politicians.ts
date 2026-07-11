import type { PoliticianListResponse } from "@/lib/types"
import { fetchJson } from "@/lib/api/client"

export interface PoliticianBrowseParams {
  filter?: string
  party?: string
  uf?: string
  page?: number
  pageSize?: number
  sort?: "connections" | "name"
}

// Paginated, filterable politician browse. Backs the /politicos entry point so a
// fresh install (politicians synced, no scandals yet) still has content.
export async function fetchPoliticians(
  params: PoliticianBrowseParams = {},
): Promise<PoliticianListResponse> {
  const search = new URLSearchParams()
  if (params.filter) search.set("filter", params.filter)
  if (params.party) search.set("party", params.party)
  if (params.uf) search.set("uf", params.uf)
  if (params.sort) search.set("sort", params.sort)
  if (params.page) search.set("page", String(params.page))
  if (params.pageSize) search.set("page_size", String(params.pageSize))

  const qs = search.toString()
  return fetchJson<PoliticianListResponse>(
    `/api/v1/politicians${qs ? `?${qs}` : ""}`,
  )
}
