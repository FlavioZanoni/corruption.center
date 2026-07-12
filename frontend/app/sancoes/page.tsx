"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { BrowseNav } from "@/components/BrowseNav";
import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { Search, ExternalLink, Ban } from "lucide-react";
import { fetchSanctions, fetchSanctionRegistries } from "@/lib/api/sanctions";
import { formatDocument } from "@/lib/format";
import { entityHref } from "@/components/EntityUI";
import type { SanctionListItem, SanctionRegistry } from "@/lib/types";

const PAGE_SIZE = 24;

/**
 * Get the display name for a sanction subject.
 * If sanctioned_name is empty, format the document.
 */
function getSanctionSubject(item: SanctionListItem): string {
  if (item.sanctioned_name && item.sanctioned_name.trim()) {
    return item.sanctioned_name;
  }
  // Fall back to formatted document (CNPJ/CPF)
  if (item.sanctioned_document) {
    return formatDocument(item.sanctioned_document);
  }
  return "(Sem nome)";
}

function SanctionCard({ item }: { item: SanctionListItem }) {
  const entityUrl = entityHref(item.sanctioned_type.toLowerCase(), item.sanctioned_id);
  const subject = getSanctionSubject(item);

  const dateStart = item.date_start
    ? new Date(item.date_start).toLocaleDateString("pt-BR")
    : "";
  const dateEnd = item.date_end
    ? new Date(item.date_end).toLocaleDateString("pt-BR")
    : "";

  // The card is a plain <div> with SIBLING links, not a <Link> wrapping the whole
  // thing. Wrapping put the "registro oficial" anchor inside the entity anchor —
  // an <a> inside an <a>, which is invalid HTML and breaks hydration. It also stole
  // the click: the one link a reader most wants (the official record) was
  // unreachable, because the outer link swallowed it.
  return (
    <div className="flex flex-col gap-2 p-4 bg-surface border border-border rounded-sm hover:border-[#c8a96e]/60 transition-colors group h-full">
      <div className="min-w-0 flex-1">
        {entityUrl ? (
          <Link
            href={entityUrl}
            className="text-sm font-serif text-text leading-tight hover:text-white block truncate"
          >
            {subject}
          </Link>
        ) : (
          <div className="text-sm font-serif text-text leading-tight truncate">
            {subject}
          </div>
        )}
        <div className="text-[10px] font-mono text-text-muted uppercase tracking-wider mt-1 truncate">
          {[item.registry, item.organ].filter(Boolean).join(" \u00b7 ")}
        </div>
      </div>

      {item.sanction_type && (
        <Link
          href={`/sancao/${encodeURIComponent(item.id)}`}
          className="text-[10px] font-mono text-text-dim hover:text-white truncate"
        >
          {item.sanction_type}
        </Link>
      )}

      {(dateStart || dateEnd) && (
        <div className="text-[10px] font-mono text-text-dim mt-0.5">
          {dateStart && dateEnd
            ? `${dateStart} \u2014 ${dateEnd}`
            : dateStart
              ? `Desde ${dateStart}`
              : `At\u00e9 ${dateEnd}`}
        </div>
      )}

      {item.process_ref && (
        <div className="text-[10px] font-mono text-text-dim truncate">
          Ref: {item.process_ref}
        </div>
      )}

      {item.source_url && (
        <a
          href={item.source_url}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-[10px] font-mono text-text-muted hover:text-white transition-colors mt-auto pt-2"
          title={item.source_url}
        >
          <ExternalLink size={9} strokeWidth={1.5} />
          Registro oficial
        </a>
      )}
    </div>
  );
}

export default function SanctionsBrowsePage() {
  const [filterInput, setFilterInput] = useState("");
  const [filter, setFilter] = useState("");
  const [registry, setRegistry] = useState("");
  const [page, setPage] = useState(1);
  const [sort, setSort] = useState<"date" | "registry">("date");

  const [registryOptions, setRegistryOptions] = useState<SanctionRegistry[]>([]);
  const [loadingRegistries, setLoadingRegistries] = useState(true);

  // Load available registries on mount
  useEffect(() => {
    fetchSanctionRegistries()
      .then((res) => {
        setRegistryOptions(res.registries);
      })
      .catch((err) => {
        console.error("Failed to load registries:", err);
      })
      .finally(() => {
        setLoadingRegistries(false);
      });
  }, []);

  // debounce the search filter
  useEffect(() => {
    const t = setTimeout(() => {
      setFilter(filterInput);
      setPage(1);
    }, 350);
    return () => clearTimeout(t);
  }, [filterInput]);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["sanctions", filter, registry, sort, page],
    queryFn: () =>
      fetchSanctions({
        q: filter,
        registry: registry || undefined,
        sort,
        page,
        pageSize: PAGE_SIZE,
      }),
    placeholderData: keepPreviousData,
  });

  const totalPages = useMemo(
    () => (data ? Math.max(1, Math.ceil(data.total / data.page_size)) : 1),
    [data],
  );

  return (
    <main className="h-screen overflow-y-auto bg-bg text-text">
      <div className="max-w-6xl mx-auto px-6 py-8">
        {/* Header */}
        <div className="flex items-center justify-between gap-4 mb-2">
          <div>
            <h1 className="text-2xl font-serif font-semibold text-white">Sanções</h1>
            <p className="text-xs font-mono text-text-muted mt-1">
              Explore {data?.total.toLocaleString("pt-BR") || "as"} sanções registradas em bases oficiais: TCU, CEIS, CEAF, CNEP, entre outras.
            </p>
          </div>
          <BrowseNav current="/sancoes" />
        </div>

        {/* Controls */}
        <div className="flex flex-wrap items-center gap-3 mt-6 mb-6">
          <div className="relative flex-1 min-w-56">
            <Search
              size={14}
              strokeWidth={1.5}
              className="absolute left-3 top-1/2 -translate-y-1/2 text-text-dim"
            />
            <input
              value={filterInput}
              onChange={(e) => setFilterInput(e.target.value)}
              placeholder="Buscar por nome, órgão, tipo ou referência..."
              className="w-full bg-surface border border-border rounded-sm pl-9 pr-3 py-2 text-sm font-serif text-text placeholder:text-text-dim focus:outline-none focus:border-[#c8a96e]/60"
            />
          </div>
          <select
            value={registry}
            onChange={(e) => {
              setRegistry(e.target.value);
              setPage(1);
            }}
            className="bg-surface border border-border rounded-sm px-3 py-2 text-sm font-mono text-text focus:outline-none focus:border-[#c8a96e]/60"
            disabled={loadingRegistries}
          >
            <option value="">Registro: Todos</option>
            {registryOptions.map((r) => (
              <option key={r.registry} value={r.registry}>
                {r.registry} ({r.count.toLocaleString("pt-BR")})
              </option>
            ))}
          </select>
          <select
            value={sort}
            onChange={(e) => {
              setSort(e.target.value as "date" | "registry");
              setPage(1);
            }}
            className="bg-surface border border-border rounded-sm px-3 py-2 text-sm font-mono text-text focus:outline-none focus:border-[#c8a96e]/60"
          >
            <option value="date">Mais recentes</option>
            <option value="registry">Por registro</option>
          </select>
        </div>

        {/* Results */}
        {isError ? (
          <div className="py-16 text-center text-sm font-mono text-text-muted">
            Erro ao carregar sanções.
          </div>
        ) : isLoading && !data ? (
          <div className="py-16 text-center text-sm font-mono text-text-muted">
            Carregando...
          </div>
        ) : data && data.items.length === 0 ? (
          <div className="py-16 text-center text-sm font-mono text-text-muted">
            Nenhuma sanção encontrada.
          </div>
        ) : (
          <>
            <div className="text-[10px] font-mono text-text-dim uppercase tracking-widest mb-3">
              {data?.total ?? 0} resultado(s)
            </div>
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-3">
              {data?.items.map((item) => (
                <SanctionCard key={item.id} item={item} />
              ))}
            </div>

            {/* Pagination */}
            <div className="flex items-center justify-center gap-4 mt-8">
              <button
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page <= 1}
                className="px-3 py-1.5 text-xs font-mono border border-border rounded-sm text-text-muted hover:text-white hover:border-[#c8a96e]/60 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
              >
                Anterior
              </button>
              <span className="text-xs font-mono text-text-muted">
                Página {data?.page ?? page} de {totalPages}
              </span>
              <button
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                disabled={page >= totalPages}
                className="px-3 py-1.5 text-xs font-mono border border-border rounded-sm text-text-muted hover:text-white hover:border-[#c8a96e]/60 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
              >
                Próxima
              </button>
            </div>
          </>
        )}
      </div>
    </main>
  );
}
