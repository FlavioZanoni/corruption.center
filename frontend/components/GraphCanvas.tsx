// TODO: LOD — when the graph gets dense, consider:
// - hiding politician/org nodes below a zoom threshold
// - adding a custom clustering force to pull politicians toward their primary scandal
// - switching SVG → Canvas renderer for >1000 nodes

"use client";

import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import * as d3 from "d3";
import { useAppStore } from "@/lib/store";
import { fetchTimeline } from "@/lib/api/timeline";
import { fetchExpandGraph } from "@/lib/api/graph";
import { NODE_COLORS } from "@/lib/constants";
import type {
  GraphNode,
  GraphEdge,
  GraphResponse,
  ActiveFilters,
} from "@/lib/types";

// ─── Visual constants ─────────────────────────────────────────────────────────

const RADII: Record<string, number> = {
  scandal: 22,
  politician: 12,
  organization: 10,
  legal_proceeding: 8,
};

const EDGE_COLORS: Record<string, string> = {
  convicted: "#cc2222",
  indicted: "#cc6600",
  cited: "#998822",
  INVOLVED_IN: "#cc6600",
  DEFENDANT_IN: "#cc2222",
  MEMBER_OF: "#553333",
  default: "#2a2a2a",
};

function edgeColor(edge: GraphEdge): string {
  const s = edge.properties?.status as string | undefined;
  if (s && EDGE_COLORS[s]) return EDGE_COLORS[s];
  return EDGE_COLORS[edge.type] ?? EDGE_COLORS.default;
}

// ─── Lucide icon data URIs ────────────────────────────────────────────────────

const LUCIDE_PATHS: Record<string, string> = {
  politician: `
    <path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2"/>
    <circle cx="12" cy="7" r="4"/>`,
  scandal: `
    <path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/>
    <path d="M12 9v4"/>
    <path d="M12 17h.01"/>`,
  organization: `
    <path d="M10 12h4"/>
    <path d="M10 8h4"/>
    <path d="M14 21v-3a2 2 0 0 0-4 0v3"/>
    <path d="M6 10H4a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-2"/>
    <path d="M6 21V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v16"/>`,
  legal_proceeding: `
    <path d="M12 3v18"/>
    <path d="m19 8 3 8a5 5 0 0 1-6 0"/>
    <path d="M3 7h1a17 17 0 0 0 8-2 17 17 0 0 0 8 2h1"/>
    <path d="m5 8 3 8a5 5 0 0 1-6 0"/>
    <path d="M7 21h10"/>`,
};

function iconDataURI(nodeType: string): string {
  const paths = LUCIDE_PATHS[nodeType] ?? "";
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256" viewBox="0 0 24 24" fill="none" stroke="#ffffff" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">${paths}</svg>`;
  return `data:image/svg+xml;base64,${btoa(svg)}`;
}

// Returns the photo/logo URL for a node if it has one, otherwise null.
function photoUrl(node: SimNode): string | null {
  const p = node.properties;
  const url =
    (p.photo_url as string | undefined) ?? (p.logo_url as string | undefined);
  return url && url !== "" ? url : null;
}

// ─── D3 simulation node/link types ───────────────────────────────────────────

interface SimNode extends d3.SimulationNodeDatum {
  id: string;
  type: string;
  label: string;
  radius: number;
  color: string;
  properties: Record<string, unknown>;
}

interface SimLink extends d3.SimulationLinkDatum<SimNode> {
  id: string;
  edgeType: string;
  edgeStatus: string;
  edgeReliability: string;
  color: string;
}

// ─── Filter applicator ────────────────────────────────────────────────────────
// Called both after initial render and whenever filters change.
// Operates on live D3 selections — no simulation rebuild needed.

function applyFilters(
  nodeSel: d3.Selection<SVGGElement, SimNode, SVGGElement, unknown> | null,
  linkSel: d3.Selection<SVGLineElement, SimLink, SVGGElement, unknown> | null,
  filters: ActiveFilters,
): void {
  if (!nodeSel || !linkSel) return;

  // Show/hide nodes by type
  nodeSel.style("display", (d) =>
    filters.nodeTypes[d.type as keyof typeof filters.nodeTypes] === false
      ? "none"
      : "",
  );

  // Show/hide edges: hide if either endpoint type is filtered out,
  // if the edge status itself is filtered out, or if reliability is filtered out.
  linkSel.style("display", (d) => {
    const src = d.source as SimNode;
    const tgt = d.target as SimNode;
    if (filters.nodeTypes[src.type as keyof typeof filters.nodeTypes] === false)
      return "none";
    if (filters.nodeTypes[tgt.type as keyof typeof filters.nodeTypes] === false)
      return "none";
    const status = d.edgeStatus as keyof typeof filters.edgeStatus;
    if (filters.edgeStatus[status] === false) return "none";
    const reliability = d.edgeReliability as keyof typeof filters.reliability;
    if (
      reliability &&
      filters.reliability[reliability] === false
    )
      return "none";
    return "";
  });
}

// ─── Component ────────────────────────────────────────────────────────────────

export function GraphCanvas() {
  const svgRef = useRef<SVGSVGElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const simulationRef = useRef<d3.Simulation<SimNode, SimLink> | null>(null);
  // Keep live selections so the filter/highlight effects can update them without rebuilding
  const nodeSelRef = useRef<d3.Selection<
    SVGGElement,
    SimNode,
    SVGGElement,
    unknown
  > | null>(null);
  const linkSelRef = useRef<d3.Selection<
    SVGLineElement,
    SimLink,
    SVGGElement,
    unknown
  > | null>(null);
  // Mirror of filters kept in a ref so the simulation build effect can read
  // the latest value without needing filters in its dep array.
  const filtersRef = useRef<ActiveFilters | null>(null);

  const { timelineRange, focusedNodeId, filters, selectedNode, setSelectedNode } =
    useAppStore();

  // Keep filtersRef in sync with Zustand state
  filtersRef.current = filters;

  const { data: timelineData } = useQuery({
    queryKey: ["timeline", timelineRange.from, timelineRange.to],
    queryFn: () => fetchTimeline(timelineRange.from, timelineRange.to),
    staleTime: 2 * 60 * 1000,
  });

  useQuery({
    queryKey: ["expand", focusedNodeId, 2],
    queryFn: () => fetchExpandGraph(focusedNodeId!, 2),
    enabled: !!focusedNodeId,
  });

  useEffect(() => {
    if (!timelineData || !svgRef.current || !containerRef.current) return;

    const container = containerRef.current;
    const width = container.clientWidth;
    const height = container.clientHeight;

    // ── Build sim nodes/links ─────────────────────────────────────────────────

    const data: GraphResponse = timelineData;

    const nodeMap = new Map<string, SimNode>();
    for (const n of data.nodes) {
      nodeMap.set(n.id, {
        id: n.id,
        type: n.type,
        label: n.label,
        radius: RADII[n.type] ?? 8,
        color: NODE_COLORS[n.type] ?? "#555555",
        properties: n.properties,
      });
    }

    const nodes: SimNode[] = Array.from(nodeMap.values());

    const links: SimLink[] = data.edges
      .filter((e) => nodeMap.has(e.from) && nodeMap.has(e.to))
      .map((e) => ({
        id: e.id,
        source: e.from,
        target: e.to,
        edgeType: e.type,
        edgeStatus:
          (e.properties?.status as string | undefined) ?? "membership",
        edgeReliability:
          (e.properties?.reliability as string | undefined) ?? "high",
        color: edgeColor(e),
      }));

    // ── Clear previous render ─────────────────────────────────────────────────

    if (simulationRef.current) {
      simulationRef.current.stop();
      simulationRef.current = null;
    }

    const svg = d3.select(svgRef.current);
    svg.selectAll("*").remove();

    svg
      .attr("width", width)
      .attr("height", height)
      .style("background", "#0a0a0a");

    // ── Defs: clip paths for all nodes (used by photo/icon images) ───────────

    const defs = svg.append("defs");

    for (const node of nodes) {
      defs
        .append("clipPath")
        .attr("id", `clip-${node.id}`)
        .append("circle")
        .attr("r", node.radius);
    }

    // ── Zoom + pan ────────────────────────────────────────────────────────────

    const g = svg.append("g");

    const zoom = d3
      .zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.05, 8])
      .on("zoom", (event) => {
        g.attr("transform", event.transform);
      });

    svg.call(zoom);

    // Click on SVG background → deselect
    svg.on("click", (event) => {
      if (event.target === svgRef.current) {
        setSelectedNode(null);
      }
    });

    // ── Edge layer ────────────────────────────────────────────────────────────

    const linkSel = g
      .append("g")
      .attr("class", "links")
      .selectAll<SVGLineElement, SimLink>("line")
      .data(links)
      .join("line")
      .attr("stroke", (d) => d.color)
      .attr("stroke-opacity", 0.5)
      .attr("stroke-width", 1);

    linkSelRef.current = linkSel;

    // ── Node layer ────────────────────────────────────────────────────────────

    const nodeSel = g
      .append("g")
      .attr("class", "nodes")
      .selectAll<SVGGElement, SimNode>("g")
      .data(nodes)
      .join("g")
      .attr("class", "node")
      .style("cursor", "pointer");

    nodeSelRef.current = nodeSel;

    // Circle background (always rendered)
    nodeSel
      .append("circle")
      .attr("class", "node-bg")
      .attr("r", (d) => d.radius)
      .attr("fill", (d) => d.color)
      .attr("stroke", (d) => d.color)
      .attr("stroke-width", 1.5);

    // Selection highlight ring — rendered behind the fill circle, initially hidden
    nodeSel
      .insert("circle", ".node-bg")
      .attr("class", "node-ring")
      .attr("r", (d) => d.radius + 5)
      .attr("fill", "none")
      .attr("stroke", (d) => d.color)
      .attr("stroke-width", 2)
      .attr("stroke-opacity", 0);

    // Photo/logo for nodes that have a URL, icon fallback for the rest.
    // Both are clipped to the node circle.
    nodeSel
      .append("image")
      .attr("href", (d) => photoUrl(d) ?? iconDataURI(d.type))
      // photos fill the circle edge-to-edge; icons use a little padding
      .attr("x", (d) => (photoUrl(d) ? -d.radius : -d.radius * 0.6))
      .attr("y", (d) => (photoUrl(d) ? -d.radius : -d.radius * 0.6))
      .attr("width", (d) => (photoUrl(d) ? d.radius * 2 : d.radius * 1.2))
      .attr("height", (d) => (photoUrl(d) ? d.radius * 2 : d.radius * 1.2))
      .attr("clip-path", (d) => `url(#clip-${d.id})`)
      .attr("preserveAspectRatio", "xMidYMid slice");

    // ── Labels: all nodes ─────────────────────────────────────────────────────

    nodeSel
      .append("text")
      .text((d) => d.label)
      .attr("y", (d) => d.radius + 14)
      .attr("text-anchor", "middle")
      .attr("fill", "#999999")
      .attr("font-size", "11px")
      .attr("font-family", "IBM Plex Mono, monospace")
      .attr("font-weight", "400")
      .style("pointer-events", "none")
      .style("user-select", "none");

    // ── Click handler ─────────────────────────────────────────────────────────

    nodeSel.on("click", (event, d) => {
      event.stopPropagation();

      setSelectedNode({
        id: d.id,
        type: d.type as GraphNode["type"],
        label: d.label,
        properties: d.properties,
      });

      // Brief scale pulse 1.4x → 1x
      const circleEl = d3
        .select(event.currentTarget)
        .select<SVGCircleElement>("circle");
      const origR = d.radius;
      circleEl
        .transition()
        .duration(120)
        .attr("r", origR * 1.4)
        .transition()
        .duration(200)
        .attr("r", origR);
    });

    // ── Drag ──────────────────────────────────────────────────────────────────

    const drag = d3
      .drag<SVGGElement, SimNode>()
      .on("start", (event, d) => {
        if (!event.active) simulation.alphaTarget(0.3).restart();
        d.fx = d.x;
        d.fy = d.y;
      })
      .on("drag", (event, d) => {
        d.fx = event.x;
        d.fy = event.y;
      })
      .on("end", (event, d) => {
        if (!event.active) simulation.alphaTarget(0);
        d.fx = null;
        d.fy = null;
      });

    nodeSel.call(drag);

    // ── Force simulation ──────────────────────────────────────────────────────

    const simulation = d3
      .forceSimulation<SimNode>(nodes)
      .force(
        "link",
        d3
          .forceLink<SimNode, SimLink>(links)
          .id((d) => d.id)
          .distance(130)
          .strength(0.5),
      )
      .force("charge", d3.forceManyBody<SimNode>().strength(-350))
      .force(
        "collide",
        d3.forceCollide<SimNode>((d) => d.radius + 6),
      )
      .force("center", d3.forceCenter(width / 2, height / 2));

    simulationRef.current = simulation;

    simulation.on("tick", () => {
      linkSel
        .attr("x1", (d) => (d.source as SimNode).x ?? 0)
        .attr("y1", (d) => (d.source as SimNode).y ?? 0)
        .attr("x2", (d) => (d.target as SimNode).x ?? 0)
        .attr("y2", (d) => (d.target as SimNode).y ?? 0);

      nodeSel.attr("transform", (d) => `translate(${d.x ?? 0},${d.y ?? 0})`);
    });

    // Apply any filters that were active before this render (e.g. persisted state)
    if (filtersRef.current) {
      applyFilters(nodeSel, linkSel, filtersRef.current);
    }

    return () => {
      simulation.stop();
      simulationRef.current = null;
      nodeSelRef.current = null;
      linkSelRef.current = null;
    };
  }, [timelineData, setSelectedNode]);

  // ── Apply filters reactively without rebuilding the simulation ───────────────
  useEffect(() => {
    applyFilters(nodeSelRef.current, linkSelRef.current, filters);
  }, [filters]);

  // ── Highlight the selected node with a persistent glow ring ──────────────────
  useEffect(() => {
    if (!nodeSelRef.current) return;
    const selectedId = selectedNode?.id ?? null;
    nodeSelRef.current.select<SVGCircleElement>(".node-ring").attr(
      "stroke-opacity",
      (d) => (d.id === selectedId ? 0.85 : 0),
    );
  }, [selectedNode]);

  // Resize observer — restart simulation on container resize
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => {
      if (!svgRef.current) return;
      const w = el.clientWidth;
      const h = el.clientHeight;
      d3.select(svgRef.current).attr("width", w).attr("height", h);
      simulationRef.current
        ?.force("center", d3.forceCenter(w / 2, h / 2))
        .alpha(0.3)
        .restart();
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  return (
    <div
      ref={containerRef}
      className="absolute inset-0 w-full h-full"
      style={{ background: "#0a0a0a", pointerEvents: "all" }}
    >
      <svg ref={svgRef} style={{ width: "100%", height: "100%" }} />
    </div>
  );
}
