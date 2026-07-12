"use client";

import { useState } from "react";

// Only what the canvas can actually draw.
//
// The legend used to promise a "Fonte" node (no such node type exists anywhere in
// the database), a "Sanção" node (sanctions exist, but the timeline that feeds the
// canvas never returns one), and edges marked Condenado / Sob investigação /
// Absolvido / Escândalos relacionados — none of which any worker writes. The only
// edge outcome that exists is "citado". A legend for things that never appear does
// not describe the graph, it describes a graph we do not have.
const LEGEND_ENTRIES = [
  // node types, all reachable from the timeline
  { kind: "node", color: "#e05252", label: "Escândalo" },
  { kind: "node", color: "#8b7ec8", label: "Processo judicial" },
  { kind: "node", color: "#6dbf8f", label: "Pessoa" },
  { kind: "node", color: "#c8a96e", label: "Político" },
  { kind: "node", color: "#5b9bd5", label: "Organização" },
  { kind: "node", color: "#d98a4b", label: "Sanção" },
  // The ring says the CASE ended in a conviction. It never says which defendant
  // was convicted, because no official source we use publishes that.
  { kind: "ring", color: "#cc2222", label: "Processo com condenação" },
  // edge types, all of which the graph really contains
  {
    kind: "edge",
    color: "#553388",
    width: 1.6,
    label: "Investiga (processo → escândalo)",
  },
  {
    kind: "edge",
    color: "#998822",
    width: 1.0,
    label: "Citado (réu no processo)",
  },
  {
    kind: "edge",
    color: "#d98a4b",
    width: 1.0,
    label: "Sancionado (CGU/TCU)",
  },
] as const;

export function GraphLegend() {
  const [open, setOpen] = useState(true);

  return (
    <div className="absolute bottom-16 left-10 z-50 rounded-md border border-white/10 bg-black/70 backdrop-blur-sm">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="flex w-full items-center justify-between gap-6 px-3 py-2 text-sm font-medium uppercase tracking-widest text-white/40 transition-colors hover:text-white/70"
      >
        Legenda
        <span aria-hidden className="text-xs leading-none text-white/50">
          {open ? "−" : "+"}
        </span>
      </button>

      {open && (
        <div className="flex flex-col gap-0.5 px-3 pb-2.5">
          {LEGEND_ENTRIES.map((e) => (
            <div key={e.label} className="flex items-center gap-2">
              {e.kind === "node" ? (
                <svg width="14" height="14" viewBox="0 0 14 14" className="shrink-0">
                  <circle cx="7" cy="7" r="5" fill={e.color} />
                </svg>
              ) : e.kind === "ring" ? (
                <svg width="14" height="14" viewBox="0 0 14 14" className="shrink-0">
                  <circle cx="7" cy="7" r="3.5" fill="#8b7ec8" />
                  <circle
                    cx="7"
                    cy="7"
                    r="6"
                    fill="none"
                    stroke={e.color}
                    strokeWidth="1.5"
                  />
                </svg>
              ) : (
                <svg width="14" height="14" viewBox="0 0 22 10" className="shrink-0">
                  <line
                    x1="1"
                    y1="5"
                    x2="21"
                    y2="5"
                    stroke={e.color}
                    strokeWidth={e.width}
                    strokeOpacity={0.9}
                  />
                </svg>
              )}
              <span className="font-mono text-md text-white/60">{e.label}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
