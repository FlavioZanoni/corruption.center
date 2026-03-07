"use client";

import { useRef, useState, useMemo, useCallback } from "react";
import { Filter, Search, X, Heart } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/lib/store";
import { searchNodes } from "@/lib/api/search";
import type { SearchResult } from "@/lib/types";

const NODE_TYPE_LABELS: Record<string, string> = {
  politician: "Político",
  scandal: "Escândalo",
  organization: "Organização",
  legal_proceeding: "Processo",
};

// ─── Open-source credits modal ────────────────────────────────────────────────

const FRONTEND_LIBS = [
  { name: "Next.js", url: "https://nextjs.org", desc: "React framework" },
  { name: "React", url: "https://react.dev", desc: "UI library" },
  {
    name: "D3.js",
    url: "https://d3js.org",
    desc: "Force graph & data visualization",
  },
  {
    name: "Tailwind CSS",
    url: "https://tailwindcss.com",
    desc: "Utility-first CSS framework",
  },
  { name: "Lucide", url: "https://lucide.dev", desc: "Icon library" },
  {
    name: "Zustand",
    url: "https://zustand-demo.pmnd.rs",
    desc: "State management",
  },
  {
    name: "TanStack Query",
    url: "https://tanstack.com/query",
    desc: "Async data fetching & caching",
  },
  {
    name: "TypeScript",
    url: "https://www.typescriptlang.org",
    desc: "Typed JavaScript",
  },
  {
    name: "pnpm",
    url: "https://pnpm.io",
    desc: "Fast, disk-efficient package manager",
  },
  {
    name: "IBM Plex Mono",
    url: "https://www.ibm.com/plex/",
    desc: "Monospace typeface by IBM",
  },
];

const BACKEND_LIBS = [
  { name: "Go", url: "https://go.dev", desc: "Backend language" },
  { name: "Gin", url: "https://gin-gonic.com", desc: "HTTP web framework" },
  {
    name: "PostgreSQL",
    url: "https://www.postgresql.org",
    desc: "Relational database",
  },
  {
    name: "Memgraph",
    url: "https://memgraph.com",
    desc: "In-memory graph database",
  },
  {
    name: "pgx",
    url: "https://github.com/jackc/pgx",
    desc: "PostgreSQL driver for Go",
  },
  {
    name: "neo4j-go-driver",
    url: "https://github.com/neo4j/neo4j-go-driver",
    desc: "Bolt driver (used with Memgraph)",
  },
  {
    name: "Swaggo",
    url: "https://github.com/swaggo/swag",
    desc: "Swagger docs generator for Go",
  },
  { name: "Docker", url: "https://www.docker.com", desc: "Containerization" },
];

function LibList({ libs }: { libs: typeof FRONTEND_LIBS }) {
  return (
    <ul>
      {libs.map((lib) => (
        <li key={lib.name}>
          <a
            href={lib.url}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-baseline justify-between gap-4 px-5 py-2 hover:bg-[#161616] transition-colors group"
          >
            <span className="text-sm font-mono text-text group-hover:text-white transition-colors shrink-0">
              {lib.name}
            </span>
            <span className="text-xs font-mono text-text-muted truncate text-right">
              {lib.desc}
            </span>
          </a>
        </li>
      ))}
    </ul>
  );
}

function CreditsModal({ onClose }: { onClose: () => void }) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      onClick={onClose}
    >
      {/* backdrop */}
      <div className="absolute inset-0 bg-black/60" />

      <div
        className="relative z-10 w-full max-w-sm mx-4 bg-surface border border-border rounded-sm shadow-2xl flex flex-col max-h-[80vh]"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="shrink-0 flex items-start justify-between px-5 py-4 border-b border-border">
          <div>
            <span className="text-[9px] font-mono uppercase tracking-widest text-text-muted block mb-1">
              código aberto
            </span>
            <h2 className="text-sm font-serif text-text leading-snug">
              Esse projeto não existiria
              <br />
              sem esses projetos
            </h2>
          </div>
          <button
            onClick={onClose}
            className="text-text-muted hover:text-text transition-colors mt-0.5"
            aria-label="Fechar"
          >
            <X size={14} strokeWidth={1.5} />
          </button>
        </div>

        {/* Scrollable body */}
        <div className="overflow-y-auto">
          {/* Frontend section */}
          <div className="pt-3 pb-1">
            <span className="px-5 text-[9px] font-mono uppercase tracking-widest text-text-muted">
              Frontend
            </span>
            <div className="mt-2">
              <LibList libs={FRONTEND_LIBS} />
            </div>
          </div>

          {/* Divider */}
          <div className="mx-5 border-t border-border" />

          {/* Backend section */}
          <div className="pt-3 pb-2">
            <span className="px-5 text-[9px] font-mono uppercase tracking-widest text-text-muted">
              Backend
            </span>
            <div className="mt-2">
              <LibList libs={BACKEND_LIBS} />
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="shrink-0 px-5 py-3 border-t border-border flex items-center gap-1.5">
          <Heart
            size={10}
            strokeWidth={1.5}
            className="text-[#cc2222] shrink-0"
          />
          <span className="text-[10px] font-mono text-text-muted">
            Open Source
          </span>
        </div>
      </div>
    </div>
  );
}

// ─── GitHub icon (plain SVG, no external dep) ─────────────────────────────────

function GitHubIcon({ size = 18 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden="true"
    >
      <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0 1 12 6.844a9.59 9.59 0 0 1 2.504.337c1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.02 10.02 0 0 0 22 12.017C22 6.484 17.522 2 12 2z" />
    </svg>
  );
}

// ─── Component ────────────────────────────────────────────────────────────────

export function SearchBar() {
  const { toggleFilterPanel, setSelectedNode, setSearchQuery, searchQuery } =
    useAppStore();

  const [inputValue, setInputValue] = useState("");
  const [isDropdownOpen, setIsDropdownOpen] = useState(false);
  const [isCreditsOpen, setIsCreditsOpen] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const debouncedSetQuery = useCallback(
    (value: string) => {
      if (debounceTimer.current) clearTimeout(debounceTimer.current);
      debounceTimer.current = setTimeout(() => setSearchQuery(value), 300);
    },
    [setSearchQuery],
  );

  const { data, isFetching } = useQuery({
    queryKey: ["search", searchQuery],
    queryFn: () => searchNodes(searchQuery),
    enabled: searchQuery.trim().length >= 2,
    placeholderData: (prev) => prev,
  });

  const results = useMemo(() => data?.results ?? [], [data]);

  const handleInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const value = e.target.value;
      setInputValue(value);
      debouncedSetQuery(value);
      if (value.trim().length >= 2) {
        setIsDropdownOpen(true);
      } else {
        setIsDropdownOpen(false);
      }
    },
    [debouncedSetQuery],
  );

  const handleFocus = useCallback(() => {
    if (results.length > 0 && searchQuery.trim().length >= 2) {
      setIsDropdownOpen(true);
    }
  }, [results.length, searchQuery]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Escape") {
        setInputValue("");
        setSearchQuery("");
        setIsDropdownOpen(false);
        inputRef.current?.blur();
      }
    },
    [setSearchQuery],
  );

  const handleSelect = useCallback(
    (result: SearchResult) => {
      setSelectedNode({
        id: result.id,
        type: result.type,
        label: result.label,
        properties: result.properties,
      });
      setInputValue(result.label);
      setIsDropdownOpen(false);
    },
    [setSelectedNode],
  );

  const handleClear = useCallback(() => {
    setInputValue("");
    setSearchQuery("");
    setIsDropdownOpen(false);
    inputRef.current?.focus();
  }, [setSearchQuery]);

  const handleContainerBlur = useCallback((e: React.FocusEvent) => {
    if (!containerRef.current?.contains(e.relatedTarget as Node)) {
      setIsDropdownOpen(false);
    }
  }, []);

  return (
    <>
      {isCreditsOpen && (
        <CreditsModal onClose={() => setIsCreditsOpen(false)} />
      )}

      <div
        ref={containerRef}
        onBlur={handleContainerBlur}
        className="absolute top-0 left-0 right-0 z-30 flex items-start pt-3 px-3"
      >
        {/* Left: filter toggle */}
        <button
          onClick={toggleFilterPanel}
          className="shrink-0 w-9 h-9 flex items-center justify-center text-text-muted hover:text-text transition-colors"
          aria-label="Abrir filtros"
        >
          <Filter size={18} strokeWidth={1.5} />
        </button>

        {/* Center: search input */}
        <div className="flex-1 flex justify-center">
          <div className="relative w-full max-w-xl search-glow">
            <div className="flex items-center bg-surface border border-border rounded-sm h-9 px-3 gap-2">
              <Search
                size={14}
                strokeWidth={1.5}
                className={`shrink-0 transition-colors ${
                  isFetching ? "text-[#cc2222]" : "text-text-muted"
                }`}
              />
              <input
                ref={inputRef}
                type="text"
                value={inputValue}
                onChange={handleInputChange}
                onFocus={handleFocus}
                onKeyDown={handleKeyDown}
                placeholder="Buscar político, escândalo ou organização..."
                className="flex-1 bg-transparent text-text placeholder-text-dim text-sm outline-none caret-[#cc2222] font-mono"
              />
              {inputValue && (
                <button
                  onClick={handleClear}
                  className="shrink-0 text-text-muted hover:text-text transition-colors"
                  aria-label="Limpar busca"
                >
                  <X size={12} strokeWidth={2} />
                </button>
              )}
            </div>

            {isDropdownOpen && results.length > 0 && (
              <div className="absolute top-full left-0 right-0 mt-1 bg-surface border border-border rounded-sm overflow-hidden shadow-2xl max-h-80 overflow-y-auto">
                {results.map((result) => (
                  <button
                    key={result.id}
                    onClick={() => handleSelect(result)}
                    className="w-full flex items-start gap-3 px-3 py-2.5 hover:bg-[#1a1a1a] text-left transition-colors group"
                  >
                    <span
                      className={`shrink-0 text-[9px] font-mono uppercase tracking-wider px-1.5 py-0.5 rounded-sm mt-0.5 ${getTypeStyle(result.type)}`}
                    >
                      {NODE_TYPE_LABELS[result.type] ?? result.type}
                    </span>
                    <div className="flex-1 min-w-0">
                      <div className="text-text text-sm font-serif leading-tight truncate">
                        {result.label}
                      </div>
                      {result.snippet && (
                        <div className="text-text-muted text-xs mt-0.5 line-clamp-2 font-mono">
                          {result.snippet}
                        </div>
                      )}
                    </div>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Right: credits + GitHub */}
        <div className="shrink-0 flex items-center">
          <button
            onClick={() => setIsCreditsOpen(true)}
            className="w-9 h-9 flex items-center justify-center text-text-muted hover:text-text transition-colors"
            aria-label="Bibliotecas open-source"
          >
            <Heart size={16} strokeWidth={1.5} />
          </button>
          <a
            href="https://github.com/FlavioZanoni/corruption.center"
            target="_blank"
            rel="noopener noreferrer"
            className="w-9 h-9 flex items-center justify-center text-text-muted hover:text-text transition-colors"
            aria-label="GitHub"
          >
            <GitHubIcon size={18} />
          </a>
        </div>
      </div>
    </>
  );
}

function getTypeStyle(type: string): string {
  switch (type) {
    case "politician":
      return "bg-[#0a1a3a] text-[#4488ff] border border-[#1a2a5a]";
    case "scandal":
      return "bg-[#3a0a0a] text-[#cc2222] border border-[#5a1a1a]";
    case "organization":
      return "bg-[#0a2a1a] text-[#22aa66] border border-[#1a4a2a]";
    case "legal_proceeding":
      return "bg-[#1a0a3a] text-[#8844cc] border border-[#2a1a5a]";
    default:
      return "bg-[#1a1a1a] text-text-muted border border-[#333333]";
  }
}
