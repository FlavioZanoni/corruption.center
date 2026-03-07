"use client";

import { useRef, useState, useMemo, useCallback } from "react";
import { Menu, Search, X } from "lucide-react";
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

export function SearchBar() {
  const { toggleFilterPanel, setSelectedNode, setSearchQuery, searchQuery } =
    useAppStore();

  const [inputValue, setInputValue] = useState("");
  const [isDropdownOpen, setIsDropdownOpen] = useState(false);
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
    <div
      ref={containerRef}
      onBlur={handleContainerBlur}
      className="absolute top-0 left-0 right-0 z-30 flex items-start pt-3 px-3"
    >
      <button
        onClick={toggleFilterPanel}
        className="shrink-0 w-9 h-9 flex items-center justify-center text-text-muted hover:text-text transition-colors"
        aria-label="Abrir filtros"
      >
        <Menu size={18} strokeWidth={1.5} />
      </button>

      <div className="flex-1 flex justify-center">
        <div className="relative w-full max-w-xl search-glow">
        <div className="flex items-center bg-surface border border-border rounded-sm h-9 px-3 gap-2">
          <Search
            size={14}
            strokeWidth={1.5}
            className={`shrink-0 transition-colors ${
              isFetching ? "text-[#cc2222]" : "text-[#888888]"
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
            className="flex-1 bg-transparent text-text placeholder-[#444444] text-sm outline-none caret-[#cc2222] font-mono"
          />
          {inputValue && (
            <button
              onClick={handleClear}
              className="shrink-0 text-[#888888] hover:text-text transition-colors"
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
                    <div className="text-[#666666] text-xs mt-0.5 line-clamp-2 font-mono">
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
    </div>
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
      return "bg-[#1a1a1a] text-[#888888] border border-[#333333]";
  }
}
