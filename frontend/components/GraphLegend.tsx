const LEGEND_ENTRIES = [
  // node types
  { kind: "node", color: "#e05252", label: "Escândalo" },
  { kind: "node", color: "#c8a96e", label: "Político" },
  { kind: "node", color: "#6dbf8f", label: "Pessoa" },
  { kind: "node", color: "#5b9bd5", label: "Organização" },
  { kind: "node", color: "#8b7ec8", label: "Processo judicial" },
  { kind: "node", color: "#7a7a7a", label: "Fonte" },
  { kind: "node", color: "#d98a4b", label: "Sanção" },
  // edge statuses
  {
    kind: "edge",
    color: "#cc2222",
    width: 2.5,
    dash: false,
    label: "Condenado",
  },
  {
    kind: "edge",
    color: "#cc6600",
    width: 1.5,
    dash: false,
    label: "Sob investigação",
  },
  { kind: "edge", color: "#998822", width: 1.0, dash: false, label: "Citado" },
  {
    kind: "edge",
    color: "#334433",
    width: 0.8,
    dash: false,
    label: "Absolvido",
  },
  {
    kind: "edge",
    color: "#555555",
    width: 1.0,
    dash: true,
    label: "Escândalos relacionados",
  },
] as const;

export function GraphLegend() {
  return (
    <div className="absolute bottom-16 left-10 z-50 flex flex-col gap-1 rounded-md border border-white/10 bg-black/70 px-3 py-2.5 backdrop-blur-sm">
      <p className="mb-1 text-sm font-medium uppercase tracking-widest text-white/40">
        Legenda
      </p>

      <div className="flex flex-col gap-0.5">
        {LEGEND_ENTRIES.map((e) => (
          <div key={e.label} className="flex items-center gap-2">
            {e.kind === "node" ? (
              <svg width="14" height="14" viewBox="0 0 14 14">
                <circle cx="7" cy="7" r="5" fill={e.color} />
              </svg>
            ) : (
              <svg width="22" height="10" viewBox="0 0 22 10">
                <line
                  x1="1"
                  y1="5"
                  x2="21"
                  y2="5"
                  stroke={e.color}
                  strokeWidth={e.width}
                  strokeDasharray={e.dash ? "4 3" : undefined}
                  strokeOpacity={0.9}
                />
              </svg>
            )}
            <span className="text-md text-white/60 font-mono">{e.label}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
