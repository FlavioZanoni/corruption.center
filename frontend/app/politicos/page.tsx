"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { ArrowLeft, Search, Ban, Scale } from "lucide-react";
import { fetchPoliticians } from "@/lib/api/politicians";
import { useAppStore } from "@/lib/store";
import { NODE_COLORS } from "@/lib/constants";
import type { PoliticianListItem, GraphNode } from "@/lib/types";

const PAGE_SIZE = 24;

// Brazilian states (UF). "" = all.
const UFS = [
  "AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA", "MT", "MS",
  "MG", "PA", "PB", "PR", "PE", "PI", "RJ", "RN", "RS", "RO", "RR", "SC",
  "SP", "SE", "TO",
];

function PoliticianCard({
  item,
  onOpen,
}: {
  item: PoliticianListItem;
  onOpen: (item: PoliticianListItem) => void;
}) {
  const ring = NODE_COLORS.politician;
  return (
    <button
      onClick={() => onOpen(item)}
      className="flex flex-col items-center text-center gap-2 p-4 bg-surface border border-border rounded-sm hover:border-[#c8a96e]/60 hover:bg-[#161616] transition-colors group"
    >
      <div
        className="rounded-full overflow-hidden shrink-0"
        style={{ width: 72, height: 72, boxShadow: `0 0 0 2px ${ring}, 0 0 0 4px #0d0d0d` }}
      >
        {item.photo_url ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={item.photo_url}
            alt={item.name}
            loading="lazy"
            decoding="async"
            referrerPolicy="no-referrer"
            className="w-full h-full object-cover object-center"
          />
        ) : (
          <div
            className="w-full h-full flex items-center justify-center text-[10px] font-mono text-text-dim"
            style={{ background: `${ring}22` }}
          >
            s/ foto
          </div>
        )}
      </div>
      {item.photo_attribution && (
        <span
          className="text-[8px] font-mono text-text-dim leading-tight line-clamp-1 max-w-full"
          title={item.photo_attribution}
        >
          {item.photo_attribution}
        </span>
      )}
      <div className="min-w-0 w-full">
        <div className="text-sm font-serif text-text leading-tight group-hover:text-white truncate">
          {item.name}
        </div>
        <div className="text-[10px] font-mono text-text-muted uppercase tracking-wider mt-1 truncate">
          {[item.party_current, item.state].filter(Boolean).join(" · ")}
        </div>
        {item.role_current && (
          <div className="text-[10px] font-mono text-text-dim mt-0.5 truncate">
            {item.role_current}
          </div>
        )}
      </div>
      {(item.sanction_count > 0 || item.proceeding_count > 0) && (
        <div className="flex items-center gap-3 mt-1">
          {item.sanction_count > 0 && (
            <span className="inline-flex items-center gap-1 text-[10px] font-mono text-[#d98a4b]">
              <Ban size={11} strokeWidth={1.5} />
              {item.sanction_count}
            </span>
          )}
          {item.proceeding_count > 0 && (
            <span className="inline-flex items-center gap-1 text-[10px] font-mono text-[#8b7ec8]">
              <Scale size={11} strokeWidth={1.5} />
              {item.proceeding_count}
            </span>
          )}
        </div>
      )}
    </button>
  );
}

export default function PoliticiansBrowsePage() {
  const router = useRouter();
  const setSelectedNode = useAppStore((s) => s.setSelectedNode);

  const [filterInput, setFilterInput] = useState("");
  const [filter, setFilter] = useState("");
  const [party, setParty] = useState("");
  const [uf, setUf] = useState("");
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
    queryKey: ["politicians", filter, party, uf, page],
    queryFn: () =>
      fetchPoliticians({ filter, party, uf, page, pageSize: PAGE_SIZE }),
    placeholderData: keepPreviousData,
  });

  const totalPages = useMemo(
    () => (data ? Math.max(1, Math.ceil(data.total / data.page_size)) : 1),
    [data],
  );

  function openInGraph(item: PoliticianListItem) {
    const node: GraphNode = {
      id: item.id,
      type: "politician",
      label: item.name,
      properties: {
        name: item.name,
        party_current: item.party_current,
        role_current: item.role_current,
        state: item.state,
        photo_url: item.photo_url,
        photo_attribution: item.photo_attribution,
      },
    };
    setSelectedNode(node);
    router.push("/");
  }

  return (
    <main className="min-h-screen bg-bg text-text">
      <div className="max-w-6xl mx-auto px-6 py-8">
        {/* Header */}
        <div className="flex items-center justify-between gap-4 mb-2">
          <div>
            <h1 className="text-2xl font-serif font-semibold text-white">Políticos</h1>
            <p className="text-xs font-mono text-text-muted mt-1">
              Explore os políticos monitorados — mesmo antes de qualquer escândalo mapeado.
            </p>
          </div>
          <Link
            href="/"
            className="inline-flex items-center gap-1.5 text-xs font-mono text-text-muted hover:text-white transition-colors shrink-0"
          >
            <ArrowLeft size={13} strokeWidth={1.5} />
            Voltar ao grafo
          </Link>
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
              placeholder="Buscar por nome..."
              className="w-full bg-surface border border-border rounded-sm pl-9 pr-3 py-2 text-sm font-serif text-text placeholder:text-text-dim focus:outline-none focus:border-[#c8a96e]/60"
            />
          </div>
          <input
            value={party}
            onChange={(e) => {
              setParty(e.target.value);
              setPage(1);
            }}
            placeholder="Partido"
            className="w-32 bg-surface border border-border rounded-sm px-3 py-2 text-sm font-mono text-text placeholder:text-text-dim focus:outline-none focus:border-[#c8a96e]/60"
          />
          <select
            value={uf}
            onChange={(e) => {
              setUf(e.target.value);
              setPage(1);
            }}
            className="bg-surface border border-border rounded-sm px-3 py-2 text-sm font-mono text-text focus:outline-none focus:border-[#c8a96e]/60"
          >
            <option value="">UF: Todas</option>
            {UFS.map((u) => (
              <option key={u} value={u}>
                {u}
              </option>
            ))}
          </select>
        </div>

        {/* Results */}
        {isError ? (
          <div className="py-16 text-center text-sm font-mono text-text-muted">
            Erro ao carregar políticos.
          </div>
        ) : isLoading && !data ? (
          <div className="py-16 text-center text-sm font-mono text-text-muted">
            Carregando...
          </div>
        ) : data && data.items.length === 0 ? (
          <div className="py-16 text-center text-sm font-mono text-text-muted">
            Nenhum político encontrado.
          </div>
        ) : (
          <>
            <div className="text-[10px] font-mono text-text-dim uppercase tracking-widest mb-3">
              {data?.total ?? 0} resultado(s)
            </div>
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-3">
              {data?.items.map((item) => (
                <PoliticianCard key={item.id} item={item} onOpen={openInGraph} />
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
