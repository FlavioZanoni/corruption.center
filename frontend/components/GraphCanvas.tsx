// TODO: LOD — when the graph gets dense:
// - hiding politician/org nodes below a zoom threshold
// - switching SVG → Canvas renderer for >1000 nodes

"use client";

import { useEffect, useLayoutEffect, useRef, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import * as d3 from "d3";
import { useAppStore } from "@/lib/store";
import type { LayoutMode } from "@/lib/store";
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
  scandal: 24,
  politician: 13,
  organization: 11,
  legal_proceeding: 9,
};

// Edge thickness encodes conviction weight — the visual mass tells the story
// before the user clicks anything
const EDGE_WIDTH: Record<string, number> = {
  convicted: 2.5,
  under_investigation: 1.5,
  cited: 1.0,
  acquitted: 0.6,
  default: 1.0,
};

const EDGE_OPACITY: Record<string, number> = {
  convicted: 0.85,
  under_investigation: 0.55,
  cited: 0.3,
  acquitted: 0.12,
  default: 0.4,
};

const EDGE_COLORS: Record<string, string> = {
  convicted: "#cc2222",
  under_investigation: "#cc6600",
  cited: "#998822",
  acquitted: "#334433",
  INVOLVED_IN: "#cc6600",
  DEFENDANT_IN: "#cc2222",
  MEMBER_OF: "#334455",
  IMPLICATED_IN: "#885522",
  INVESTIGATES: "#553388",
  RELATED_TO: "#555555",
  SUPPORTS: "#1a4a2a",
  default: "#2a2a2a",
};

// Tighter distance = drawn closer to the scandal hub
const EDGE_DISTANCE: Record<string, number> = {
  INVESTIGATES: 70,
  IMPLICATED_IN: 100,
  INVOLVED_IN: 160,
  MEMBER_OF: 130,
  RELATED_TO: 230,
  default: 140,
};

const EDGE_STRENGTH: Record<string, number> = {
  INVESTIGATES: 0.8,
  IMPLICATED_IN: 0.6,
  INVOLVED_IN: 0.5,
  MEMBER_OF: 0.4,
  RELATED_TO: 0.2,
  default: 0.5,
};

function edgeColor(edge: GraphEdge): string {
  const s = edge.properties?.status as string | undefined;
  if (s && EDGE_COLORS[s]) return EDGE_COLORS[s];
  return EDGE_COLORS[edge.type] ?? EDGE_COLORS.default;
}

function edgeWidth(edge: GraphEdge): number {
  const s = edge.properties?.status as string | undefined;
  return EDGE_WIDTH[s ?? ""] ?? EDGE_WIDTH.default;
}

function edgeOpacity(edge: GraphEdge): number {
  const s = edge.properties?.status as string | undefined;
  return EDGE_OPACITY[s ?? ""] ?? EDGE_OPACITY.default;
}

function edgeDistance(edge: GraphEdge): number {
  return EDGE_DISTANCE[edge.type] ?? EDGE_DISTANCE.default;
}

function edgeStrength(edge: GraphEdge): number {
  return EDGE_STRENGTH[edge.type] ?? EDGE_STRENGTH.default;
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

function photoUrl(node: SimNode): string | null {
  const p = node.properties;
  const url =
    (p.photo_url as string | undefined) ?? (p.logo_url as string | undefined);
  return url && url !== "" ? url : null;
}

// ─── D3 simulation types ──────────────────────────────────────────────────────

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
  width: number;
  opacity: number;
  rawEdge: GraphEdge;
}

// ─── Expand state ─────────────────────────────────────────────────────────────
// Default view: only scandal nodes.
// Click a node: reveal its direct neighbours.
// Click background: collapse back to scandals only.
// Clicking further nodes accumulates — the visible set grows until collapse.

class ExpandState {
  private visibleIds: Set<string> = new Set();
  private allNodes: Map<string, SimNode> = new Map();
  private adjacency: Map<string, Set<string>> = new Map();

  init(nodes: SimNode[], links: SimLink[]) {
    this.allNodes = new Map(nodes.map((n) => [n.id, n]));
    this.adjacency = new Map(nodes.map((n) => [n.id, new Set<string>()]));

    for (const l of links) {
      // after simulation init source/target are objects, before init they're ids
      const srcId =
        typeof l.source === "object"
          ? (l.source as SimNode).id
          : (l.source as string);
      const tgtId =
        typeof l.target === "object"
          ? (l.target as SimNode).id
          : (l.target as string);
      this.adjacency.get(srcId)?.add(tgtId);
      this.adjacency.get(tgtId)?.add(srcId);
    }

    this.visibleIds = new Set(
      nodes.filter((n) => n.type === "scandal").map((n) => n.id),
    );
  }

  // Returns whether this node's neighbourhood was expanded (true) or collapsed (false)
  toggle(nodeId: string): { ids: Set<string>; expanded: boolean } {
    const neighbours = this.adjacency.get(nodeId) ?? new Set<string>();
    const alreadyExpanded = [...neighbours].every((n) =>
      this.visibleIds.has(n),
    );

    if (alreadyExpanded && this.visibleIds.has(nodeId)) {
      // collapse: remove neighbours that aren't scandals and aren't shared with other visible scandals
      for (const n of neighbours) {
        const node = this.allNodes.get(n);
        if (node?.type !== "scandal") this.visibleIds.delete(n);
      }
      return { ids: new Set(this.visibleIds), expanded: false };
    } else {
      // expand
      this.visibleIds.add(nodeId);
      for (const n of neighbours) this.visibleIds.add(n);
      return { ids: new Set(this.visibleIds), expanded: true };
    }
  }

  expand(nodeId: string): Set<string> {
    this.visibleIds.add(nodeId);
    for (const n of this.adjacency.get(nodeId) ?? []) {
      this.visibleIds.add(n);
    }
    return new Set(this.visibleIds);
  }

  collapse(): Set<string> {
    this.visibleIds = new Set(
      Array.from(this.allNodes.values())
        .filter((n) => n.type === "scandal")
        .map((n) => n.id),
    );
    return new Set(this.visibleIds);
  }

  get visible(): Set<string> {
    return new Set(this.visibleIds);
  }
}

// ─── Filter + visibility applicator ──────────────────────────────────────────

function applyVisibility(
  nodeSel: d3.Selection<SVGGElement, SimNode, SVGGElement, unknown> | null,
  linkSel: d3.Selection<SVGLineElement, SimLink, SVGGElement, unknown> | null,
  filters: ActiveFilters,
  visibleIds: Set<string>,
): void {
  if (!nodeSel || !linkSel) return;

  nodeSel.style("display", (d) => {
    if (!visibleIds.has(d.id)) return "none";
    if (filters.nodeTypes[d.type as keyof typeof filters.nodeTypes] === false)
      return "none";
    return "";
  });

  linkSel.style("display", (d) => {
    const src = d.source as SimNode;
    const tgt = d.target as SimNode;
    if (!visibleIds.has(src.id) || !visibleIds.has(tgt.id)) return "none";
    if (filters.nodeTypes[src.type as keyof typeof filters.nodeTypes] === false)
      return "none";
    if (filters.nodeTypes[tgt.type as keyof typeof filters.nodeTypes] === false)
      return "none";
    const status = d.edgeStatus as keyof typeof filters.edgeStatus;
    const reliability = d.edgeReliability as keyof typeof filters.reliability;
    if (filters.edgeStatus[status] === false) return "none";
    if (reliability && filters.reliability[reliability] === false)
      return "none";
    return "";
  });
}

// ─── Component ────────────────────────────────────────────────────────────────

export function GraphCanvas() {
  const svgRef = useRef<SVGSVGElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const simRef = useRef<d3.Simulation<SimNode, SimLink> | null>(null);
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
  const filtersRef = useRef<ActiveFilters | null>(null);
  const expandRef = useRef<ExpandState>(new ExpandState());
  const visibleRef = useRef<Set<string>>(new Set());

  const {
    timelineRange,
    focusedNodeId,
    filters,
    selectedNode,
    layoutMode,
    setSelectedNode,
  } = useAppStore();

  useLayoutEffect(() => {
    filtersRef.current = filters;
  });

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

  // refreshVisible: update DOM display without rebuilding the simulation
  const refreshVisible = useCallback((ids: Set<string>) => {
    visibleRef.current = ids;
    if (filtersRef.current) {
      applyVisibility(
        nodeSelRef.current,
        linkSelRef.current,
        filtersRef.current,
        ids,
      );
    }
    // gentle reheat so newly revealed nodes find space
    simRef.current?.alpha(0.25).restart();
  }, []);

  // ── Main build effect — runs once per data load ───────────────────────────
  useEffect(() => {
    if (!timelineData || !svgRef.current || !containerRef.current) return;

    const container = containerRef.current;
    const width = container.clientWidth;
    const height = container.clientHeight;
    const data: GraphResponse = timelineData;

    // ── Nodes ─────────────────────────────────────────────────────────────────

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

    // ── Links ─────────────────────────────────────────────────────────────────

    const links: SimLink[] = data.edges
      .filter((e) => nodeMap.has(e.from) && nodeMap.has(e.to))
      .map((e) => ({
        id: e.id,
        source: e.from,
        target: e.to,
        edgeType: e.type,
        edgeStatus: (e.properties?.status as string | undefined) ?? "default",
        edgeReliability:
          (e.properties?.reliability as string | undefined) ?? "high",
        color: edgeColor(e),
        width: edgeWidth(e),
        opacity: edgeOpacity(e),
        rawEdge: e,
      }));

    // ── Expand state init ─────────────────────────────────────────────────────

    expandRef.current.init(nodes, links);
    visibleRef.current = expandRef.current.visible; // scandals only

    // ── Clear ─────────────────────────────────────────────────────────────────

    simRef.current?.stop();
    simRef.current = null;

    const svg = d3.select(svgRef.current);
    svg.selectAll("*").remove();
    svg
      .attr("width", width)
      .attr("height", height)
      .style("background", "#0a0a0a");

    // ── Defs ──────────────────────────────────────────────────────────────────

    const defs = svg.append("defs");
    for (const node of nodes) {
      defs
        .append("clipPath")
        .attr("id", `clip-${node.id}`)
        .append("circle")
        .attr("r", node.radius);
    }

    // ── Zoom / pan ────────────────────────────────────────────────────────────

    const g = svg.append("g");
    const zoom = d3
      .zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.05, 8])
      .on("zoom", (ev) => g.attr("transform", ev.transform));
    svg.call(zoom);

    // background click → collapse to scandals only
    svg.on("click", (ev) => {
      if (ev.target === svgRef.current) {
        setSelectedNode(null);
        refreshVisible(expandRef.current.collapse());
      }
    });

    // ── Edges ─────────────────────────────────────────────────────────────────

    const linkSel = g
      .append("g")
      .attr("class", "links")
      .selectAll<SVGLineElement, SimLink>("line")
      .data(links)
      .join("line")
      .attr("stroke", (d) => d.color)
      .attr("stroke-opacity", (d) => d.opacity)
      .attr("stroke-width", (d) => d.width)
      // scandal↔scandal relationship shown dashed
      .attr("stroke-dasharray", (d) =>
        d.edgeType === "RELATED_TO" ? "6 4" : null,
      );

    linkSelRef.current = linkSel;

    // ── Nodes ─────────────────────────────────────────────────────────────────

    const nodeSel = g
      .append("g")
      .attr("class", "nodes")
      .selectAll<SVGGElement, SimNode>("g")
      .data(nodes)
      .join("g")
      .attr("class", "node")
      .style("cursor", "pointer");

    nodeSelRef.current = nodeSel;

    // selection ring (behind fill circle)
    nodeSel
      .insert("circle", ":first-child")
      .attr("class", "node-ring")
      .attr("r", (d) => d.radius + 6)
      .attr("fill", "none")
      .attr("stroke", (d) => d.color)
      .attr("stroke-width", 2.5)
      .attr("stroke-opacity", 0);

    // fill circle
    nodeSel
      .append("circle")
      .attr("class", "node-bg")
      .attr("r", (d) => d.radius)
      .attr("fill", (d) => d.color)
      .attr("stroke", (d) => d.color)
      .attr("stroke-width", 1.5);

    // photo or icon
    nodeSel
      .append("image")
      .attr("href", (d) => photoUrl(d) ?? iconDataURI(d.type))
      .attr("x", (d) => (photoUrl(d) ? -d.radius : -d.radius * 0.6))
      .attr("y", (d) => (photoUrl(d) ? -d.radius : -d.radius * 0.6))
      .attr("width", (d) => (photoUrl(d) ? d.radius * 2 : d.radius * 1.2))
      .attr("height", (d) => (photoUrl(d) ? d.radius * 2 : d.radius * 1.2))
      .attr("clip-path", (d) => `url(#clip-${d.id})`)
      .attr("preserveAspectRatio", "xMidYMid slice");

    // labels — scandals always visible and slightly larger
    nodeSel
      .append("text")
      .text((d) => d.label)
      .attr("y", (d) => d.radius + 14)
      .attr("text-anchor", "middle")
      .attr("fill", (d) => (d.type === "scandal" ? "#cccccc" : "#888888"))
      .attr("font-size", (d) => (d.type === "scandal" ? "12px" : "10px"))
      .attr("font-family", "IBM Plex Mono, monospace")
      .attr("font-weight", (d) => (d.type === "scandal" ? "500" : "400"))
      .style("pointer-events", "none")
      .style("user-select", "none");

    // ── Click: expand neighbourhood ───────────────────────────────────────────

    nodeSel.on("click", (ev, d) => {
      ev.stopPropagation();

      setSelectedNode({
        id: d.id,
        type: d.type as GraphNode["type"],
        label: d.label,
        properties: d.properties,
      });

      const { ids } = expandRef.current.toggle(d.id);
      refreshVisible(ids);

      // pulse
      const origR = d.radius;
      d3.select<SVGGElement, SimNode>(ev.currentTarget)
        .select<SVGCircleElement>(".node-bg")
        .transition()
        .duration(120)
        .attr("r", origR * 1.5)
        .transition()
        .duration(220)
        .attr("r", origR);
    });

    const drag = d3
      .drag<SVGGElement, SimNode>()
      .on("start", (ev, d) => {
        if (!ev.active) simulation.alphaTarget(0.3).restart();
        d.fx = d.x;
        d.fy = d.y;
      })
      .on("drag", (ev, d) => {
        d.fx = ev.x;
        d.fy = ev.y;
      })
      .on("end", (ev, d) => {
        if (!ev.active) simulation.alphaTarget(0);
        d.fx = null;
        d.fy = null;
      });
    nodeSel.call(drag);

    // ── Simulation ────────────────────────────────────────────────────────────
    // Per-edge distance and strength: proceedings sit tight to their scandal,
    // politicians fan out further, scandal↔scandal stay well apart.
    // Scandal nodes have high charge mass so they act as gravity wells.

    const simulation = d3
      .forceSimulation<SimNode>(nodes)
      .force(
        "link",
        d3
          .forceLink<SimNode, SimLink>(links)
          .id((d) => d.id)
          .distance((l) => edgeDistance(l.rawEdge))
          .strength((l) => edgeStrength(l.rawEdge)),
      )
      .force(
        "charge",
        d3
          .forceManyBody<SimNode>()
          .strength((d) => (d.type === "scandal" ? -800 : -300)),
      )
      .force(
        "collide",
        d3
          .forceCollide<SimNode>((d) => d.radius + 14)
          .strength(0.8)
          .iterations(3),
      )
      .force("center", d3.forceCenter(width / 2, height / 2))
      .alphaDecay(0.04)
      .velocityDecay(0.55);

    simRef.current = simulation;

    simulation.on("tick", () => {
      linkSel
        .attr("x1", (d) => (d.source as SimNode).x ?? 0)
        .attr("y1", (d) => (d.source as SimNode).y ?? 0)
        .attr("x2", (d) => (d.target as SimNode).x ?? 0)
        .attr("y2", (d) => (d.target as SimNode).y ?? 0);
      nodeSel.attr("transform", (d) => `translate(${d.x ?? 0},${d.y ?? 0})`);
    });

    // apply default visibility: scandals only
    if (filtersRef.current) {
      applyVisibility(nodeSel, linkSel, filtersRef.current, visibleRef.current);
    }

    return () => {
      simulation.stop();
      simRef.current = null;
      nodeSelRef.current = null;
      linkSelRef.current = null;
    };
  }, [timelineData, setSelectedNode, refreshVisible]);

  // ── Filter changes ────────────────────────────────────────────────────────
  useEffect(() => {
    applyVisibility(
      nodeSelRef.current,
      linkSelRef.current,
      filters,
      visibleRef.current,
    );
  }, [filters]);

  // ── Selection ring ────────────────────────────────────────────────────────
  useEffect(() => {
    if (!nodeSelRef.current) return;
    const id = selectedNode?.id ?? null;
    nodeSelRef.current
      .select<SVGCircleElement>(".node-ring")
      .attr("stroke-opacity", (d) => (d.id === id ? 0.85 : 0));
  }, [selectedNode]);

  // ── Layout mode ───────────────────────────────────────────────────────────
  useEffect(() => {
    const sim = simRef.current;
    if (!sim) return;

    // tear down previous layout forces
    sim.force("cluster", null);
    sim.force("x", null);
    sim.force("y", null);

    if (layoutMode !== "radial") {
      sim.nodes().forEach((n) => {
        n.fx = null;
        n.fy = null;
      });
    }

    switch (layoutMode as LayoutMode) {
      // ── Organic: scandal nodes repel hard, others pushed by links ───────────
      case "force":
        sim.force(
          "charge",
          d3
            .forceManyBody<SimNode>()
            .strength((d) => (d.type === "scandal" ? -800 : -300)),
        );
        break;

      // ── Cluster: gravity wells pull each node toward its primary scandal ────
      case "cluster": {
        // map each non-scandal node to its primary scandal
        const primaryScandal = new Map<string, string>();
        (sim.force("link") as d3.ForceLink<SimNode, SimLink>)
          .links()
          .forEach((l) => {
            const src = l.source as SimNode;
            const tgt = l.target as SimNode;
            if (src.type === "scandal" && !primaryScandal.has(tgt.id))
              primaryScandal.set(tgt.id, src.id);
            if (tgt.type === "scandal" && !primaryScandal.has(src.id))
              primaryScandal.set(src.id, tgt.id);
          });

        // custom force: each tick pulls nodes toward their scandal's current position
        sim.force("cluster", ((alpha: number) => {
          // rebuild scandal positions each tick so they move naturally
          const scandalPos = new Map<string, { x: number; y: number }>();
          sim.nodes().forEach((n) => {
            if (n.type === "scandal")
              scandalPos.set(n.id, { x: n.x ?? 0, y: n.y ?? 0 });
          });

          sim.nodes().forEach((n) => {
            const sid = primaryScandal.get(n.id);
            if (!sid) return;
            const pos = scandalPos.get(sid);
            if (!pos) return;
            // politicians pulled harder than orgs/proceedings
            const strength =
              n.type === "politician"
                ? 0.6
                : n.type === "organization"
                  ? 0.45
                  : 0.5;
            n.vx = (n.vx ?? 0) + (pos.x - (n.x ?? 0)) * strength * alpha;
            n.vy = (n.vy ?? 0) + (pos.y - (n.y ?? 0)) * strength * alpha;
          });
        }) as unknown as d3.Force<SimNode, SimLink>);

        // lower charge so cluster gravity dominates
        sim.force(
          "charge",
          d3
            .forceManyBody<SimNode>()
            .strength((d) => (d.type === "scandal" ? -600 : -80)),
        );
        break;
      }

      // ── Radial: scandals pinned to a ring, rest orbit freely ────────────────
      case "radial": {
        const scandals = sim.nodes().filter((n) => n.type === "scandal");
        const cx = (containerRef.current?.clientWidth ?? 800) / 2;
        const cy = (containerRef.current?.clientHeight ?? 600) / 2;
        const R = Math.min(cx, cy) * 0.42;
        scandals.forEach((n, i) => {
          const angle = (2 * Math.PI * i) / scandals.length - Math.PI / 2;
          n.fx = cx + R * Math.cos(angle);
          n.fy = cy + R * Math.sin(angle);
        });
        sim.force(
          "charge",
          d3
            .forceManyBody<SimNode>()
            .strength((d) => (d.type === "scandal" ? 0 : -250)),
        );
        break;
      }

      // ── Timeline: X = scandal start year, Y = free ───────────────────────────
      case "timeline": {
        const w = containerRef.current?.clientWidth ?? 800;
        const h = containerRef.current?.clientHeight ?? 600;
        const scandals = sim.nodes().filter((n) => n.type === "scandal");
        const years = scandals.map((n) =>
          new Date(
            (n.properties.date_start as string | undefined) ?? "2000-01-01",
          ).getFullYear(),
        );
        const minYear = Math.min(...years, 2000);
        const maxYear = Math.max(...years, 2025);
        const xScale = (year: number) =>
          ((year - minYear) / (maxYear - minYear)) * (w * 0.8) + w * 0.1;

        // build a map of scandal X targets for non-scandal nodes
        const scandalX = new Map<string, number>();
        scandals.forEach((n) => {
          const year = new Date(
            (n.properties.date_start as string | undefined) ?? "2000-01-01",
          ).getFullYear();
          scandalX.set(n.id, xScale(year));
        });

        const ls = (
          sim.force("link") as d3.ForceLink<SimNode, SimLink>
        ).links();
        const nodeTargetX = new Map<string, number>();
        ls.forEach((l) => {
          const src = l.source as SimNode;
          const tgt = l.target as SimNode;
          if (src.type === "scandal") {
            const prev = nodeTargetX.get(tgt.id) ?? 0;
            nodeTargetX.set(
              tgt.id,
              (prev + (scandalX.get(src.id) ?? w / 2)) / 2,
            );
          }
          if (tgt.type === "scandal") {
            const prev = nodeTargetX.get(src.id) ?? 0;
            nodeTargetX.set(
              src.id,
              (prev + (scandalX.get(tgt.id) ?? w / 2)) / 2,
            );
          }
        });

        sim.force(
          "x",
          d3
            .forceX<SimNode>((d) => {
              if (d.type === "scandal") return scandalX.get(d.id) ?? w / 2;
              return nodeTargetX.get(d.id) ?? w / 2;
            })
            .strength((d) => (d.type === "scandal" ? 0.9 : 0.5)),
        );

        sim.force(
          "y",
          d3
            .forceY<SimNode>(h / 2)
            .strength((d) => (d.type === "scandal" ? 0.3 : 0.05)),
        );

        sim.force("charge", d3.forceManyBody<SimNode>().strength(-150));
        break;
      }
    }

    // high alpha + slow decay so the new forces have time to reorganise layout
    sim.alpha(0.9).alphaDecay(0.02).restart();
  }, [layoutMode]);

  // ── Resize ────────────────────────────────────────────────────────────────
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => {
      if (!svgRef.current) return;
      const w = el.clientWidth;
      const h = el.clientHeight;
      d3.select(svgRef.current).attr("width", w).attr("height", h);
      simRef.current
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
