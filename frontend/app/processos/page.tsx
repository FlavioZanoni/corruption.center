"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { formatCNJ } from "@/lib/format";
import { BrowseNav } from "@/components/BrowseNav";
import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { Search, Scale } from "lucide-react";
import { fetchProceedings } from "@/lib/api/proceedings";
import type { ProceedingListItem } from "@/lib/types";

const PAGE_SIZE = 24;

// Brazilian courts - common values
const COURTS = [
  "STF",
  "STJ",
  "TSE",
  "STM",
  "OAB",
  "TRF",
  "TJ",
  "MP",
];

function ProceedingCard({ item }: { item: ProceedingListItem }) {
  // Determine conviction state color and label
  const convictionState = useMemo(() => {
    if (!item.polled) {
      return {
        label: "Não verificado",
        color: "#7a7a7a",
        bg: "rgba(122, 122, 122, 0.1)",
        description: "DataJud ainda não verificou este caso",
      }
    }
    if (item.has_conviction) {
      return {
        label: "Com condenação",
        color: "#e05252",
        bg: "rgba(224, 82, 82, 0.1)",
        description: "Há condenação registrada",
      }
    }
    return {
      label: "Sem condenação",
      color: "#6dbf8f",
      bg: "rgba(109, 191, 143, 0.1)",
      description: "A corte não condenou",
    }
  }, [item.polled, item.has_conviction])

  // The whole card is the link: a list of 6,506 cases you can read but not open is
  // a catalogue of dead ends. There is no nested anchor here, so wrapping is safe.
  return (
    <Link
      href={`/processo/${encodeURIComponent(item.id)}`}
      className="flex flex-col gap-2 p-4 bg-surface border border-border rounded-sm hover:border-[#c8a96e]/60 hover:bg-[#161616] transition-colors group"
      style={{ borderColor: convictionState.color }}
    >
      {/* Case number and court */}
      <div className="min-w-0">
        <div className="text-sm font-serif text-text leading-tight group-hover:text-white truncate">
          {formatCNJ(item.case_number)}
        </div>
        <div className="text-[10px] font-mono text-text-muted uppercase tracking-wider mt-1 truncate">
          {[item.court, item.type].filter(Boolean).join(" · ")}
        </div>
      </div>

      {/* Phase / Status */}
      {item.phase && (
        <div className="text-[10px] font-mono text-text-dim mt-0.5 truncate">
          {item.phase}
        </div>
      )}

      {/* Conviction state badge */}
      <div
        className="inline-flex items-center gap-2 px-2 py-1 rounded-sm text-[10px] font-mono font-medium w-fit mt-1"
        style={{ backgroundColor: convictionState.bg, color: convictionState.color }}
        title={convictionState.description}
      >
        <Scale size={10} strokeWidth={1.5} />
        {convictionState.label}
      </div>
    </Link>
  );
}

export default function ProceedingsBrowsePage() {
  const [filterInput, setFilterInput] = useState("");
  const [filter, setFilter] = useState("");
  const [court, setCourt] = useState("");
  const [convictionFilter, setConvictionFilter] = useState<"" | "convicted" | "not_convicted" | "not_polled">("");
  const [page, setPage] = useState(1);

  // debounce the name filter
  useEffect(() => {
    const t = setTimeout(() => {
      setFilter(filterInput);
      setPage(1);
    }, 350);
    return () => clearTimeout(t);
  }, [filterInput]);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["proceedings", filter, court, convictionFilter, page],
    queryFn: () => {
      // "not_polled" is its own server-side filter, not the absence of one. Sending
      // undefined returned every case, convictions included, under a "Não
      // verificado" heading.
      const has_conviction =
        convictionFilter === "convicted"
          ? "true"
          : convictionFilter === "not_convicted"
            ? "false"
            : convictionFilter === "not_polled"
              ? "unknown"
              : undefined

      return fetchProceedings({
        q: filter,
        court: court || undefined,
        has_conviction,
        page,
        pageSize: PAGE_SIZE,
      })
    },
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
            <h1 className="text-2xl font-serif font-semibold text-white">Processos</h1>
            <p className="text-xs font-mono text-text-muted mt-1">
              Explore {data?.total.toLocaleString("pt-BR") || "os"} processos judiciais. <span className="block mt-1">Os três estados: <strong>com condenação</strong> (corte decidiu), <strong>sem condenação</strong> (corte não condenou), <strong>não verificado</strong> (DataJud ainda não consultou).</span>
            </p>
          </div>
          <BrowseNav current="/processos" />
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
              placeholder="Buscar por número CNJ, classe ou corte..."
              className="w-full bg-surface border border-border rounded-sm pl-9 pr-3 py-2 text-sm font-serif text-text placeholder:text-text-dim focus:outline-none focus:border-[#c8a96e]/60"
            />
          </div>
          <select
            value={court}
            onChange={(e) => {
              setCourt(e.target.value);
              setPage(1);
            }}
            className="bg-surface border border-border rounded-sm px-3 py-2 text-sm font-mono text-text focus:outline-none focus:border-[#c8a96e]/60"
          >
            <option value="">Corte: Todas</option>
            {COURTS.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
          <select
            value={convictionFilter}
            onChange={(e) => {
              setConvictionFilter(e.target.value as typeof convictionFilter);
              setPage(1);
            }}
            className="bg-surface border border-border rounded-sm px-3 py-2 text-sm font-mono text-text focus:outline-none focus:border-[#c8a96e]/60"
          >
            <option value="">Estado: Todos</option>
            <option value="convicted">Com condenação</option>
            <option value="not_convicted">Sem condenação</option>
            <option value="not_polled">Não verificado</option>
          </select>
        </div>

        {/* Results */}
        {isError ? (
          <div className="py-16 text-center text-sm font-mono text-text-muted">
            Erro ao carregar processos.
          </div>
        ) : isLoading && !data ? (
          <div className="py-16 text-center text-sm font-mono text-text-muted">
            Carregando...
          </div>
        ) : data && data.items.length === 0 ? (
          <div className="py-16 text-center text-sm font-mono text-text-muted">
            Nenhum processo encontrado.
          </div>
        ) : (
          <>
            <div className="text-[10px] font-mono text-text-dim uppercase tracking-widest mb-3">
              {data?.total ?? 0} resultado(s)
            </div>
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-3">
              {data?.items.map((item) => (
                <ProceedingCard key={item.id} item={item} />
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
