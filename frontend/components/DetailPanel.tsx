"use client";

import { useEffect, useCallback, useMemo } from "react";
import {
  X,
  ExternalLink,
  User,
  Users,
  AlertTriangle,
  Building2,
  Scale,
  FileText,
  Ban,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/lib/store";
import { fetchExpandGraph } from "@/lib/api/graph";
import type { GraphNode, GraphEdge } from "@/lib/types";
import { NODE_COLORS } from "@/lib/constants";
import { porExtenso, estilo } from "numero-por-extenso";

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
      return <User {...props} className="text-node-politician" />;
    case "person":
      return <Users {...props} className="text-[#6dbf8f]" />;
    case "scandal":
      return <AlertTriangle {...props} className="text-[#cc2222]" />;
    case "organization":
      return <Building2 {...props} className="text-node-organization" />;
    case "legal_proceeding":
      return <Scale {...props} className="text-node-legal" />;
    case "source":
      return <FileText {...props} className="text-[#7a7a7a]" />;
    case "sanction":
      return <Ban {...props} className="text-[#d98a4b]" />;
  }
}

// Legally required label for any node/edge not yet confirmed by a human
// (see docs/legal_compliance.md — "unconfirmed — pending review" discipline).
function ReviewBadge() {
  return (
    <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-sm border border-[#ccaa22]/40 bg-[#ccaa22]/10">
      <AlertTriangle size={9} strokeWidth={1.5} className="text-[#ccaa22] shrink-0" />
      <span className="text-[8px] font-mono uppercase tracking-wider text-[#ccaa22] leading-none">
        não confirmado — pendente de revisão
      </span>
    </span>
  );
}

// A node is treated as unconfirmed when it carries machine-ingest provenance
// (DJEN-created Person/Organization) or is a Sanction whose subject linkage is
// established automatically and routed through human review.
function isUnconfirmed(node: GraphNode | null): boolean {
  if (!node) return false;
  if (node.type === "sanction") return true;
  return Boolean(node.properties.provenance_source);
}

const NODE_TYPE_LABELS: Record<string, string> = {
  politician: "Político",
  person: "Pessoa",
  scandal: "Escândalo",
  organization: "Organização",
  legal_proceeding: "Processo",
  source: "Fonte",
  sanction: "Sanção",
};

const EDGE_TYPE_LABELS: Record<string, string> = {
  INVOLVED_IN: "Envolvido em",
  DEFENDANT_IN: "Réu em",
  MEMBER_OF: "Membro de",
  CONTROLS: "Controla",
  OWNED_BY: "Pertence a",
  IMPLICATED_IN: "Implicado em",
  INVESTIGATES: "Investigado por",
  RELATED_TO: "Relacionado a",
  SUPPORTS: "Apoia",
  SANCTIONED_IN: "Sancionado em",
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
  total_amount_brl: "Valor total",
  currency: "Moeda",
  confidence: "Confiança",
  tags: "Tags",
  aliases: "Aliases",
  name: "Nome",
  registry: "Cadastro",
  sanction_type: "Tipo de sanção",
  organ: "Órgão sancionador",
  process_ref: "Processo de referência",
  provenance_source: "Origem do dado",
  provenance_tribunal: "Tribunal (origem)",
};

// ─── Date formatting ──────────────────────────────────────────────────────────

const DATE_FIELDS = new Set([
  "birth_date",
  "start_date",
  "end_date",
  "date_start",
  "date_end",
  "date_filed",
  "date_concluded",
  "date_from",
  "judgment_date",
]);

const DATE_FORMATTER = new Intl.DateTimeFormat("pt-BR", {
  day: "numeric",
  month: "long",
  year: "numeric",
  timeZone: "UTC",
});

function formatDate(raw: string): string {
  // Accept YYYY-MM-DD or YYYY-MM or YYYY
  const d = new Date(raw.length === 4 ? `${raw}-01-01` : raw.length === 7 ? `${raw}-01` : raw);
  if (isNaN(d.getTime())) return raw;
  if (raw.length === 4) return raw; // bare year — no need to expand
  return DATE_FORMATTER.format(d);
}

// ─── Money formatting ─────────────────────────────────────────────────────────

const BRL_FORMATTER = new Intl.NumberFormat("pt-BR", {
  style: "currency",
  currency: "BRL",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

// Keys whose values should be rendered as BRL currency
const MONEY_FIELDS = new Set(["total_amount_brl", "total_amount"]);

function moneyTooltip(raw: string): string {
  const n = Number(raw.replace(/[^\d.-]/g, ""));
  if (isNaN(n)) return raw;
  return porExtenso(n, estilo.monetario);
}

function formatValue(key: string, raw: string): string {
  if (MONEY_FIELDS.has(key)) {
    const n = Number(raw.replace(/[^\d.-]/g, ""));
    if (!isNaN(n)) return BRL_FORMATTER.format(n);
  }
  if (DATE_FIELDS.has(key)) return formatDate(raw);
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

// How a link between a person and a case/sanction came to exist. Official
// sources publish names, and often mask documents, so tying a record to a
// specific politician is an inference — the reader is told which one it was and
// how strong the evidence behind it is (backend: package matching).
const CONFIDENCE_SIGNAL_LABELS: Record<string, string> = {
  full_document: "CPF/CNPJ completo na fonte",
  masked_cpf_middle6: "CPF parcial da fonte confere",
  exact_name: "nome idêntico",
  long_name: "nome completo (4+ partes)",
  ambiguous_match: "evidência serve a mais de uma pessoa",
};

function ProvenanceBadge({ properties }: { properties: Record<string, unknown> }) {
  const source = properties.source as string | undefined;
  if (source === "backoffice_review") {
    return (
      <span
        className="mt-1.5 inline-block text-[9px] font-mono uppercase tracking-wider text-[#7aa87a]"
        title="Vínculo criado por revisão humana a partir de registro oficial."
      >
        ✓ Confirmado por revisão humana
      </span>
    );
  }

  const confidence = properties.confidence as number | undefined;
  if (typeof confidence !== "number") return null;

  const signals = Array.isArray(properties.confidence_signals)
    ? (properties.confidence_signals as string[])
    : [];
  const reasons = signals
    .map((s) => CONFIDENCE_SIGNAL_LABELS[s] ?? s)
    .join(" · ");

  return (
    <span
      className="mt-1.5 inline-block text-[9px] font-mono uppercase tracking-wider text-text-muted"
      title={
        reasons
          ? `Identificação automática. Evidência: ${reasons}.`
          : "Identificação automática a partir de registro oficial."
      }
    >
      Identificação: {Math.round(confidence * 100)}%
      {reasons ? ` · ${reasons}` : ""}
    </span>
  );
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
        title={MONEY_FIELDS.has(fieldKey) ? moneyTooltip(raw) : undefined}
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
      <div className="shrink-0 mt-0.5 self-center">
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
        <ProvenanceBadge properties={edge.properties} />
      </div>
    </button>
  );
}

// Dedicated rendering for a Sanction connection: registry, type, sanctioning
// organ, effective period and — crucially — the official source_url deep link
// plus the pending-review provenance label required for compliance.
function SanctionItem({ node }: { node: GraphNode }) {
  const p = node.properties;
  const registry = (p.registry as string | undefined) ?? node.label;
  const sanctionType = p.sanction_type as string | undefined;
  const organ = p.organ as string | undefined;
  const dateStart = p.date_start as string | undefined;
  const dateEnd = p.date_end as string | undefined;
  const sourceUrl = p.source_url as string | undefined;
  const period = [dateStart, dateEnd]
    .filter((d): d is string => Boolean(d))
    .map((d) => formatDate(d))
    .join(" — ");

  return (
    <div className="px-4 py-3 border-b border-[#1a1a1a]">
      <div className="flex items-center gap-2 mb-1 flex-wrap">
        <span className="text-[10px] font-mono uppercase tracking-wider text-[#d98a4b]">
          {registry}
        </span>
        {sanctionType && (
          <span className="text-sm font-serif text-text leading-tight">
            {PROP_VALUES[sanctionType] ?? sanctionType}
          </span>
        )}
      </div>
      {organ && (
        <div className="text-[11px] font-mono text-text-muted">{organ}</div>
      )}
      {period && (
        <div className="text-[11px] font-mono text-text-muted mt-0.5">
          {period}
        </div>
      )}
      <div className="flex items-center gap-2 mt-2 flex-wrap">
        {sourceUrl && (
          <a
            href={sourceUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 text-[11px] font-mono text-text-muted hover:text-[#cc2222] transition-colors"
          >
            <ExternalLink size={10} strokeWidth={1.5} className="shrink-0" />
            <span>Registro oficial</span>
          </a>
        )}
        <ReviewBadge />
      </div>
    </div>
  );
}

export function DetailPanel() {
  const isDetailPanelOpen = useAppStore((s) => s.isDetailPanelOpen);
  const selectedNode = useAppStore((s) => s.selectedNode);
  const closeDetailPanel = useAppStore((s) => s.closeDetailPanel);
  const setSelectedNode = useAppStore((s) => s.setSelectedNode);

  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape" && isDetailPanelOpen) closeDetailPanel();
    }
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [isDetailPanelOpen, closeDetailPanel]);

  const {
    data: expandData,
    isLoading,
    isError,
  } = useQuery({
    queryKey: ["expand", selectedNode?.id, 1],
    queryFn: () => fetchExpandGraph(selectedNode!.id, 1),
    enabled: !!selectedNode?.id && isDetailPanelOpen,
  });

  const handleConnectedNodeClick = useCallback(
    (node: GraphNode) => setSelectedNode(node),
    [setSelectedNode],
  );

  const connectedNodes = useMemo<
    Array<{ node: GraphNode; edge: GraphEdge }>
  >(() => {
    if (!expandData || !selectedNode) return [];
    const list: Array<{ node: GraphNode; edge: GraphEdge }> = [];
    const nodeMap = new Map(expandData.nodes.map((n) => [n.id, n]));
    for (const edge of expandData.edges) {
      const otherId = edge.from === selectedNode.id ? edge.to : edge.from;
      const other = nodeMap.get(otherId);
      if (other && otherId !== selectedNode.id) {
        list.push({ node: other, edge });
      }
    }
    return list;
  }, [expandData, selectedNode]);

  const node = selectedNode;
  const properties = node?.properties ?? {};

  const photoUrl =
    (properties.photo_url as string | undefined) ??
    (properties.logo_url as string | undefined);
  const photoAttribution = properties.photo_attribution as string | undefined;

  const tseUrls = properties.tse_profile_urls as string[] | undefined;
  const tseUrl = tseUrls?.[0];
  const wikipediaUrl = properties.wikipedia_url as string | undefined;
  const sourceUrl = properties.source_url as string | undefined;
  const provenanceLink = properties.provenance_link as string | undefined;
  const sourceUrls = (properties.source_urls as string[] | undefined) ?? [];
  const allLinks = [
    ...(tseUrl ? [tseUrl] : []),
    ...(wikipediaUrl ? [wikipediaUrl] : []),
    ...(sourceUrl ? [sourceUrl] : []),
    ...(provenanceLink ? [provenanceLink] : []),
    ...sourceUrls,
  ];

  const unconfirmed = isUnconfirmed(node);

  const sanctionConnections = connectedNodes.filter(
    (c) => c.node.type === "sanction",
  );
  const otherConnections = connectedNodes.filter(
    (c) => c.node.type !== "sanction",
  );

  const ringColor = NODE_COLORS[node?.type ?? ""] ?? "#555555";

  return (
    <>
      <div
        className={`absolute top-0 right-0 bottom-0 z-40 w-xl bg-surface border-l border-border flex flex-col panel-slide-right ${
          isDetailPanelOpen ? "open" : ""
        }`}
      >
        {node && (
          <>
            {/* ── Header ── */}
            <div className="shrink-0 border-b border-border bg-[#0d0d0d] px-6 pt-6 pb-5">
              <div className="flex items-start gap-5">
                {/* Avatar */}
                <div className="shrink-0 flex flex-col items-center gap-1.5 w-24">
                  <div
                    className="rounded-full overflow-hidden"
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
                        loading="lazy"
                        decoding="async"
                        referrerPolicy="no-referrer"
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
                  {photoUrl && photoAttribution && (
                    <span
                      className="text-[8px] font-mono text-text-dim text-center leading-tight max-w-24 line-clamp-2"
                      title={photoAttribution}
                    >
                      {photoAttribution}
                    </span>
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
                  {unconfirmed && (
                    <div className="mt-2">
                      <ReviewBadge />
                    </div>
                  )}
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
                        key !== "tse_profile_urls" &&
                        key !== "wikipedia_url" &&
                        key !== "url" &&
                        key !== "source_url" &&
                        key !== "provenance_link" &&
                        key !== "photo_attribution" &&
                        key !== "photo_source" &&
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

              {sanctionConnections.length > 0 && (
                <div className="py-3 border-t border-[#141414]">
                  <h3 className="text-[9px] font-mono uppercase tracking-widest text-[#d98a4b] mb-2 px-4">
                    Sanções ({sanctionConnections.length})
                  </h3>
                  <div>
                    {sanctionConnections.map(({ node: sancNode, edge }) => (
                      <SanctionItem key={`${sancNode.id}-${edge.id}`} node={sancNode} />
                    ))}
                  </div>
                </div>
              )}

              <div className="py-3">
                <h3 className="text-[9px] font-mono uppercase tracking-widest text-text-muted mb-2 px-4">
                  Conexões{" "}
                  {!isLoading && otherConnections.length > 0 && (
                    <span className="text-text-muted">
                      ({otherConnections.length})
                    </span>
                  )}
                </h3>

                {isLoading ? (
                  <div className="px-4 py-3 text-xs font-mono text-text-muted">
                    Carregando...
                  </div>
                ) : isError ? (
                  <div
                    role="alert"
                    className="px-4 py-3 text-xs font-mono text-text-muted"
                  >
                    Erro ao carregar conexões.
                  </div>
                ) : otherConnections.length === 0 ? (
                  <div className="px-4 py-3 text-xs font-mono text-text-muted">
                    Nenhuma conexão encontrada.
                  </div>
                ) : (
                  <div>
                    {otherConnections.map(({ node: connNode, edge }) => (
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
