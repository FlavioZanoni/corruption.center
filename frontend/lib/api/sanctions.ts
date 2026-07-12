import type { SanctionListResponse, SanctionRegistryResponse } from "@/lib/types"
import { fetchJson } from "@/lib/api/client"

export interface SanctionBrowseParams {
  q?: string
  registry?: string
  organ?: string
  sort?: "date" | "registry"
  page?: number
  pageSize?: number
}

/**
 * Fetch paginated, filterable sanctions.
 * Supports search by sanctioned party name, organ, registry, type, or process reference.
 */
export async function fetchSanctions(
  params: SanctionBrowseParams = {},
): Promise<SanctionListResponse> {
  const search = new URLSearchParams()
  if (params.q) search.set("q", params.q)
  if (params.registry) search.set("registry", params.registry)
  if (params.organ) search.set("organ", params.organ)
  if (params.sort) search.set("sort", params.sort)
  if (params.page) search.set("page", String(params.page))
  if (params.pageSize) search.set("page_size", String(params.pageSize))

  const qs = search.toString()
  return fetchJson<SanctionListResponse>(
    `/api/v1/sanctions${qs ? `?${qs}` : ""}`,
  )
}

/**
 * Fetch available sanction registries for filtering.
 */
export async function fetchSanctionRegistries(): Promise<SanctionRegistryResponse> {
  return fetchJson<SanctionRegistryResponse>("/api/v1/sanction-registries")
}
