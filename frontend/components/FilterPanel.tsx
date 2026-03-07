"use client";

import { X } from "lucide-react";
import { useAppStore } from "@/lib/store";
import { NODE_COLORS } from "@/lib/constants";

// Edge status colors — kept local since they're only used here
const EDGE_STATUS_COLORS: Record<string, string> = {
  convicted: "#cc2222",
  indicted: "#cc7722",
  cited: "#ccaa22",
  membership: "#444444",
};

function Toggle({
  label,
  checked,
  onChange,
  color,
}: {
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
  color?: string;
}) {
  return (
    <label className="flex items-center gap-2.5 cursor-pointer group select-none">
      <button
        role="checkbox"
        aria-checked={checked}
        onClick={() => onChange(!checked)}
        className={`w-8 h-4 rounded-full transition-colors relative shrink-0 ${
          checked ? "bg-[#cc2222]" : "bg-[#2a2a2a]"
        }`}
      >
        <span
          className={`absolute top-0.5 left-0 w-3 h-3 rounded-full transition-transform bg-text ${
            checked ? "translate-x-4.5" : "translate-x-0.5"
          }`}
        />
      </button>
      <span className="flex items-center gap-1.5 text-sm text-text-muted group-hover:text-text transition-colors">
        {color && (
          <span
            className="w-2 h-2 rounded-full shrink-0"
            style={{ backgroundColor: color }}
          />
        )}
        {label}
      </span>
    </label>
  );
}

function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-3">
      <h3 className="text-[9px] font-mono uppercase tracking-widest text-text-muted">
        {title}
      </h3>
      <div className="space-y-2.5">{children}</div>
    </div>
  );
}

export function FilterPanel() {
  const {
    isFilterPanelOpen,
    setFilterPanelOpen,
    filters,
    setNodeTypeFilter,
    setEdgeStatusFilter,
    setReliabilityFilter,
    resetFilters,
  } = useAppStore();

  return (
    <>
      {isFilterPanelOpen && (
        <div
          className="absolute inset-0 z-30"
          onClick={() => setFilterPanelOpen(false)}
        />
      )}

      <div
        className={`absolute top-0 left-0 bottom-0 z-40 w-64 bg-surface border-r border-border flex flex-col panel-slide-left ${
          isFilterPanelOpen ? "open" : ""
        }`}
      >
        <div className="flex items-center justify-between px-4 py-3 border-b border-border">
          <span className="text-[10px] font-mono uppercase tracking-widest text-text-muted">
            Filtros
          </span>
          <button
            onClick={() => setFilterPanelOpen(false)}
            className="text-text-muted hover:text-text transition-colors"
            aria-label="Fechar filtros"
          >
            <X size={14} strokeWidth={1.5} />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto px-4 py-4 space-y-6">
          <Section title="Tipo de nó">
            <Toggle
              label="Político"
              checked={filters.nodeTypes.politician}
              onChange={(v) => setNodeTypeFilter("politician", v)}
              color={NODE_COLORS.politician}
            />
            <Toggle
              label="Escândalo"
              checked={filters.nodeTypes.scandal}
              onChange={(v) => setNodeTypeFilter("scandal", v)}
              color={NODE_COLORS.scandal}
            />
            <Toggle
              label="Organização"
              checked={filters.nodeTypes.organization}
              onChange={(v) => setNodeTypeFilter("organization", v)}
              color={NODE_COLORS.organization}
            />
            <Toggle
              label="Processo"
              checked={filters.nodeTypes.legal_proceeding}
              onChange={(v) => setNodeTypeFilter("legal_proceeding", v)}
              color={NODE_COLORS.legal_proceeding}
            />
          </Section>

          <Section title="Status da aresta">
            <Toggle
              label="Condenado"
              checked={filters.edgeStatus.convicted}
              onChange={(v) => setEdgeStatusFilter("convicted", v)}
              color={EDGE_STATUS_COLORS.convicted}
            />
            <Toggle
              label="Indiciado"
              checked={filters.edgeStatus.indicted}
              onChange={(v) => setEdgeStatusFilter("indicted", v)}
              color={EDGE_STATUS_COLORS.indicted}
            />
            <Toggle
              label="Citado"
              checked={filters.edgeStatus.cited}
              onChange={(v) => setEdgeStatusFilter("cited", v)}
              color={EDGE_STATUS_COLORS.cited}
            />
            <Toggle
              label="Membro"
              checked={filters.edgeStatus.membership}
              onChange={(v) => setEdgeStatusFilter("membership", v)}
              color={EDGE_STATUS_COLORS.membership}
            />
          </Section>

          <Section title="Confiabilidade da fonte">
            <Toggle
              label="Alta"
              checked={filters.reliability.high}
              onChange={(v) => setReliabilityFilter("high", v)}
            />
            <Toggle
              label="Média"
              checked={filters.reliability.medium}
              onChange={(v) => setReliabilityFilter("medium", v)}
            />
            <Toggle
              label="Baixa"
              checked={filters.reliability.low}
              onChange={(v) => setReliabilityFilter("low", v)}
            />
          </Section>
        </div>

        <div className="px-4 py-3 border-t border-border">
          <button
            onClick={resetFilters}
            className="w-full text-xs font-mono text-text-muted hover:text-text transition-colors py-1"
          >
            Redefinir filtros
          </button>
        </div>
      </div>
    </>
  );
}
