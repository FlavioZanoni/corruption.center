"use client";

import { useEffect, useCallback } from "react";
import {
  X,
  ExternalLink,
  User,
  AlertTriangle,
  Building2,
  Scale,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/lib/store";
import { fetchExpandGraph } from "@/lib/api/graph";
import type { GraphNode, GraphEdge } from "@/lib/types";
import { NODE_COLORS } from "@/lib/constants";

function NodeIcon({
  type,
  size = 16,
}: {
  type: GraphNode["type"];
  size?: number;
}) {
  const props = { size, strokeWidth: 1.5 };
  switch (type) {
    case "politician":
      return <User {...props} className="text-[#4488ff]" />;
    case "scandal":
      return <AlertTriangle {...props} className="text-[#cc2222]" />;
    case "organization":
      return <Building2 {...props} className="text-node-organization" />;
    case "legal_proceeding":
      return <Scale {...props} className="text-node-legal" />;
  }
}

const NODE_TYPE_LABELS: Record<string, string> = {
  politician: "Político",
  scandal: "Escândalo",
  organization: "Organização",
  legal_proceeding: "Processo",
};

const EDGE_TYPE_LABELS: Record<string, string> = {
  INVOLVED_IN: "Envolvido em",
  DEFENDANT_IN: "Réu em",
  MEMBER_OF: "Membro de",
  IMPLICATED_IN: "Implicado em",
  INVESTIGATES: "Investigado por",
  RELATED_TO: "Relacionado a",
  SUPPORTS: "Apoia",
};

const PROP_LABELS: Record<string, string> = {
  party: "Partido",
  party_current: "Partido atual",
  party_at_time: "Partido na época",
  role: "Cargo",
  role_current: "Cargo atual",
  role_at_time: "Cargo na época",
  state: "Estado",
  birth_date: "Nascimento",
  start_date: "Início",
  end_date: "Fim",
  date_start: "Início",
  date_end: "Fim",
  date_filed: "Autuado em",
  date_concluded: "Concluído em",
  status: "Status",
  description: "Descrição",
  cnpj: "CNPJ",
  sector: "Setor",
  type: "Tipo",
  court: "Tribunal",
  case_number: "Número do processo",
  judgment_date: "Julgamento",
  outcome: "Resultado",
  total_amount: "Valor total",
  total_amount_brl: "Valor total (R$)",
  currency: "Moeda",
  confidence: "Confiança",
  tags: "Tags",
  aliases: "Aliases",
  name: "Nome",
};

// ─── Money formatting ─────────────────────────────────────────────────────────

const BRL_FORMATTER = new Intl.NumberFormat("pt-BR", {
  style: "currency",
  currency: "BRL",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

// Keys whose values should be rendered as BRL currency
const MONEY_FIELDS = new Set(["total_amount_brl", "total_amount"]);

function formatValue(key: string, raw: string): string {
  if (MONEY_FIELDS.has(key)) {
    const n = Number(raw.replace(/[^\d.-]/g, ""));
    if (!isNaN(n)) return BRL_FORMATTER.format(n);
  }
  return PROP_VALUES[raw] ?? raw;
}

const PROP_VALUES: Record<string, string> = {
  // status
  concluded: "Concluído",
  ongoing: "Em andamento",
  prescribed: "Prescrito",
  convicted: "Condenado",
  acquitted: "Absolvido",
  under_investigation: "Sob investigação",
  cited: "Citado",
  indicted: "Indiciado",
  pending: "Pendente",
  // org type
  party: "Partido político",
  company: "Empresa",
  shell: "Empresa de fachada",
  ngo: "ONG",
  public_agency: "Estatal / Órgão público",
  // legal proceeding type
  criminal: "Criminal",
  administrative: "Administrativo",
  cpi: "CPI",
  // membership
  membership: "Membro",
};

function edgeStatusColor(properties: Record<string, unknown>): string {
  const status = properties.status as string | undefined;
  switch (status) {
    case "convicted":
      return "#cc2222";
    case "indicted":
      return "#cc7722";
    case "cited":
      return "#ccaa22";
    default:
      return "#999999";
  }
}

function PropRow({
  label,
  value,
  fieldKey,
}: {
  label: string;
  value: unknown;
  fieldKey: string;
}) {
  if (value === null || value === undefined || value === "") return null;
  const raw = Array.isArray(value) ? value.join(", ") : String(value);
  const display = formatValue(fieldKey, raw);
  return (
    <div className="flex gap-2 py-1.5 border-b border-[#1a1a1a]">
      <span className="text-[10px] font-mono text-text-muted uppercase tracking-wider min-w-22.5 shrink-0 pt-0.5">
        {label}
      </span>
      <span
        className={`text-xs break-all ${MONEY_FIELDS.has(fieldKey) ? "font-mono text-[#e8c97a]" : "text-text"}`}
      >
        {display}
      </span>
    </div>
  );
}

function ConnectedNodeItem({
  node,
  edge,
  onClick,
}: {
  node: GraphNode;
  edge: GraphEdge;
  onClick: (node: GraphNode) => void;
}) {
  const edgeLabel =
    EDGE_TYPE_LABELS[edge.type] ?? edge.type.replace(/_/g, " ").toLowerCase();
  const statusColor = edgeStatusColor(edge.properties);

  const roleAtTime = edge.properties.role_at_time as string | undefined;
  const partyAtTime = edge.properties.party_at_time as string | undefined;

  return (
    <button
      onClick={() => onClick(node)}
      className="w-full flex items-start gap-2.5 px-3 py-2.5 hover:bg-[#161616] text-left transition-colors group rounded-sm"
    >
      <div className="shrink-0 mt-0.5">
        <NodeIcon type={node.type} />
      </div>
      <div className="min-w-0">
        {/* Relationship type */}
        <div className="flex items-center gap-1.5 mb-1">
          <span
            className="w-1.5 h-1.5 rounded-full shrink-0"
            style={{ backgroundColor: statusColor }}
          />
          <span
            className="text-[10px] font-mono uppercase tracking-wider"
            style={{ color: statusColor }}
          >
            {edgeLabel}
          </span>
        </div>
        <div className="text-sm font-serif text-text leading-tight truncate group-hover:text-white">
          {node.label}
        </div>
        {/* Contextual edge fields */}
        {(roleAtTime || partyAtTime) && (
          <div className="mt-1.5 space-y-0.5 w-full">
            {roleAtTime && (
              <div className="flex gap-2 w-full">
                <span className="text-[10px] font-mono text-text-muted uppercase tracking-wider shrink-0 ">
                  Cargo no escândalo:
                </span>
                <span className="text-[10px] font-mono text-text">
                  {roleAtTime}
                </span>
              </div>
            )}
            {partyAtTime && (
              <div className="flex gap-2 w-full">
                <span className="text-[10px] font-mono text-text-muted uppercase tracking-wider shrink-0 ">
                  Partido no escândalo:
                </span>
                <span className="text-[10px] font-mono text-text">
                  {partyAtTime}
                </span>
              </div>
            )}
          </div>
        )}
      </div>
    </button>
  );
}

export function DetailPanel() {
  const { isDetailPanelOpen, selectedNode, closeDetailPanel, setSelectedNode } =
    useAppStore();

  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape" && isDetailPanelOpen) closeDetailPanel();
    }
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [isDetailPanelOpen, closeDetailPanel]);

  const { data: expandData, isLoading } = useQuery({
    queryKey: ["expand", selectedNode?.id, 1],
    queryFn: () => fetchExpandGraph(selectedNode!.id, 1),
    enabled: !!selectedNode?.id && isDetailPanelOpen,
  });

  const handleConnectedNodeClick = useCallback(
    (node: GraphNode) => setSelectedNode(node),
    [setSelectedNode],
  );

  const connectedNodes: Array<{ node: GraphNode; edge: GraphEdge }> = [];
  if (expandData && selectedNode) {
    const nodeMap = new Map(expandData.nodes.map((n) => [n.id, n]));
    for (const edge of expandData.edges) {
      const otherId = edge.from === selectedNode.id ? edge.to : edge.from;
      const other = nodeMap.get(otherId);
      if (other && otherId !== selectedNode.id) {
        connectedNodes.push({ node: other, edge });
      }
    }
  }

  const node = selectedNode;
  const properties = node?.properties ?? {};

  const photoUrl =
    (properties.photo_url as string | undefined) ??
    (properties.logo_url as string | undefined);

  const tseUrl = properties.tse_profile_url as string | undefined;
  const wikipediaUrl = properties.wikipedia_url as string | undefined;
  const sourceUrls = (properties.source_urls as string[] | undefined) ?? [];
  const allLinks = [
    ...(tseUrl ? [tseUrl] : []),
    ...(wikipediaUrl ? [wikipediaUrl] : []),
    ...sourceUrls,
  ];

  const ringColor = NODE_COLORS[node?.type ?? ""] ?? "#555555";

  return (
    <>
      <div
        className={`absolute top-0 right-0 bottom-0 z-40 w-[576px] bg-surface border-l border-border flex flex-col panel-slide-right ${
          isDetailPanelOpen ? "open" : ""
        }`}
      >
        {node && (
          <>
            {/* ── Header ── */}
            <div className="shrink-0 border-b border-border bg-[#0d0d0d] px-6 pt-6 pb-5">
              <div className="flex items-start gap-5">
                {/* Avatar */}
                <div
                  className="shrink-0 rounded-full overflow-hidden"
                  style={{
                    width: 96,
                    height: 96,
                    boxShadow: `0 0 0 3px ${ringColor}, 0 0 0 5px #0d0d0d`,
                  }}
                >
                  {photoUrl ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img
                      src={photoUrl}
                      alt={node.label}
                      className="w-full h-full object-cover object-center"
                    />
                  ) : (
                    <div
                      className="w-full h-full flex items-center justify-center"
                      style={{ background: `${ringColor}22` }}
                    >
                      <NodeIcon type={node.type} size={36} />
                    </div>
                  )}
                </div>

                {/* Title block */}
                <div className="flex-1 min-w-0 pt-1">
                  <div
                    className="text-[9px] font-mono uppercase tracking-widest mb-1"
                    style={{ color: ringColor }}
                  >
                    {NODE_TYPE_LABELS[node.type] ?? node.type}
                  </div>
                  <h2 className="text-lg font-serif font-semibold text-white leading-snug">
                    {node.label}
                  </h2>
                  <div className="text-[10px] font-mono text-text-muted mt-1">
                    {node.id}
                  </div>
                </div>

                <button
                  onClick={closeDetailPanel}
                  className="shrink-0 text-text-muted hover:text-white transition-colors mt-1"
                  aria-label="Fechar painel"
                >
                  <X size={14} strokeWidth={1.5} />
                </button>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto">
              <div className="px-4 py-3">
                <h3 className="text-[9px] font-mono uppercase tracking-widest text-text-muted mb-2">
                  Detalhes e Links
                </h3>
                <div className="space-y-0">
                  {Object.entries(properties)
                    .filter(
                      ([key]) =>
                        key !== "photo_url" &&
                        key !== "logo_url" &&
                        key !== "source_urls" &&
                        key !== "name_aliases" &&
                        key !== "tse_profile_url" &&
                        key !== "wikipedia_url" &&
                        key !== "url" &&
                        key !== "active" &&
                        key !== "name",
                    )
                    .map(([key, value]) => (
                      <PropRow
                        key={key}
                        fieldKey={key}
                        label={PROP_LABELS[key] ?? key}
                        value={value}
                      />
                    ))}
                </div>

                {allLinks.length > 0 && (
                  <div className="mt-3 space-y-1">
                    <h4 className="text-[9px] font-mono uppercase tracking-widest text-text-muted mb-2">
                      Links
                    </h4>
                    {allLinks.map((url, i) => (
                      <a
                        key={i}
                        href={url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="flex items-center gap-1.5 text-xs font-mono text-text-muted hover:text-[#cc2222] transition-colors truncate"
                      >
                        <ExternalLink
                          size={10}
                          strokeWidth={1.5}
                          className="shrink-0"
                        />
                        <span className="truncate">{url}</span>
                      </a>
                    ))}
                  </div>
                )}
              </div>

              <div className="py-3">
                <h3 className="text-[9px] font-mono uppercase tracking-widest text-text-muted mb-2 px-4">
                  Conexões{" "}
                  {!isLoading && connectedNodes.length > 0 && (
                    <span className="text-text-muted">
                      ({connectedNodes.length})
                    </span>
                  )}
                </h3>

                {isLoading ? (
                  <div className="px-4 py-3 text-xs font-mono text-text-muted">
                    Carregando...
                  </div>
                ) : connectedNodes.length === 0 ? (
                  <div className="px-4 py-3 text-xs font-mono text-text-muted">
                    Nenhuma conexão encontrada.
                  </div>
                ) : (
                  <div>
                    {connectedNodes.map(({ node: connNode, edge }) => (
                      <ConnectedNodeItem
                        key={`${connNode.id}-${edge.id}`}
                        node={connNode}
                        edge={edge}
                        onClick={handleConnectedNodeClick}
                      />
                    ))}
                  </div>
                )}
              </div>
            </div>
          </>
        )}
      </div>
    </>
  );
}
