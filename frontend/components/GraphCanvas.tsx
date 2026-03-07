"use client";

import { useEffect, useRef, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { Sigma } from "sigma";
import { EdgeLineProgram, NodeCircleProgram } from "sigma/rendering";
import { createNodeImageProgram } from "@sigma/node-image";
import { animateNodes } from "sigma/utils";
import Graph from "graphology";
import { useAppStore } from "@/lib/store";
import { fetchTimeline } from "@/lib/api/timeline";
import { fetchExpandGraph } from "@/lib/api/graph";
import type {
  GraphNode,
  GraphEdge,
  GraphResponse,
  ActiveFilters,
} from "@/lib/types";
import { NODE_COLORS } from "@/lib/constants";

// createNodeImageProgram must be called once at module level so the texture
// atlas is shared across sigma instances (kills + recreations included).
//
// Two programs:
//   NodeImageProgram    — for nodes with real photo/logo URLs (needs crossOrigin
//                         "anonymous" so WebGL canvas doesn't get tainted)
//   NodePictogramProgram — for icon fallbacks (data: URIs must NOT have the
//                         crossOrigin attribute set, or the browser silently
//                         fails to load them; crossOrigin: "" becomes `undefined`
//                         via the `|| undefined` guard in TextureManager)
const NodeImageProgram = createNodeImageProgram({
  padding: 0.05,
  keepWithinCircle: true,
  correctCentering: true,
  // default crossOrigin: "anonymous" — fine for external photo URLs
});

const NodePictogramProgram = createNodeImageProgram({
  padding: 0.15,          // give the icon some breathing room inside the circle
  keepWithinCircle: true,
  correctCentering: true,
  // drawingMode defaults to "background": renders node color circle behind the
  // image, so the node is always visible even before the texture loads.
  // (drawingMode: "color" makes nodes invisible when texture hasn't loaded.)
  crossOrigin: null,      // null → no crossOrigin attr → data: URIs load correctly
});

const EDGE_COLORS: Record<string, string> = {
  convicted: "#cc2222",
  indicted: "#cc6600",
  cited: "#888822",
  membership: "#333333",
  INVOLVED_IN: "#cc6600",
  DEFENDANT_IN: "#cc2222",
  MEMBER_OF: "#333333",
  IMPLICATED_IN: "#888822",
  INVESTIGATES: "#553388",
  RELATED_TO: "#333333",
  SUPPORTS: "#1a4a2a",
};

function edgeColor(edge: GraphEdge): string {
  const s = edge.properties?.status as string | undefined;
  if (s && EDGE_COLORS[s]) return EDGE_COLORS[s];
  return EDGE_COLORS[edge.type] ?? "#333333";
}

// ─── Icon textures ────────────────────────────────────────────────────────────

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

function getIconURI(nodeType: string): string {
  const paths = LUCIDE_PATHS[nodeType] ?? "";
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256" viewBox="0 0 24 24" fill="none" stroke="#ffffff" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">${paths}</svg>`;
  return `data:image/svg+xml;base64,${btoa(svg)}`;
}

function nodeSize(node: GraphNode, connections: number): number {
  const bases: Record<string, number> = {
    scandal: 16,
    politician: 9,
    organization: 7,
    legal_proceeding: 6,
  };
  return Math.min((bases[node.type] ?? 7) + Math.sqrt(connections) * 1.4, 22);
}

// ─── dim helper ───────────────────────────────────────────────────────────────

function dim(hex: string, alpha: number): string {
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  const m = (c: number) => Math.round(c * alpha + 10 * (1 - alpha));
  return `rgb(${m(r)},${m(g)},${m(b)})`;
}

// ─── animated dim via nodeReducer ────────────────────────────────────────────

function animateDim(
  sigma: Sigma,
  hood: Set<string>,
  restore: boolean,
): ReturnType<typeof setInterval> {
  const STEPS = 12;
  const INTERVAL = 25;
  let step = 0;

  const timer = setInterval(() => {
    step++;
    const progress = step / STEPS;

    sigma.setSetting("nodeReducer", (node, attrs) => {
      const base = (attrs.originalColor as string) ?? attrs.color ?? "#555555";
      if (restore) {
        const alpha = 0.08 + 0.92 * progress;
        return { ...attrs, color: dim(base, Math.min(1, alpha)) };
      }
      const inHood = hood.has(node);
      const alpha = inHood
        ? 0.08 + 0.92 * progress
        : 1.0 - 0.92 * progress;
      return { ...attrs, color: dim(base, Math.max(0.05, Math.min(1, alpha))) };
    });

    if (step >= STEPS) {
      clearInterval(timer);
      if (restore) {
        sigma.setSetting("nodeReducer", null);
        sigma.setSetting("edgeReducer", null);
      } else {
        sigma.setSetting("nodeReducer", (node, attrs) => {
          const base =
            (attrs.originalColor as string) ?? attrs.color ?? "#555555";
          return { ...attrs, color: hood.has(node) ? base : dim(base, 0.08) };
        });
      }
    }
  }, INTERVAL);

  return timer;
}

// ─── ego layout ──────────────────────────────────────────────────────────────
// Places all revealed (non-hidden) nodes around the expanded roots.
// Returns { nodeId → { x, y } } suitable for animateNodes().
//
// Strategy:
//   • 1 root  → root at origin; all visible neighbors in a full circle at
//               RING_STEP; their visible neighbors at 2×RING_STEP; etc.
//   • N roots → roots evenly spaced on a ROOT_RING circle; each root owns
//               a 2π/N angular slice and fans its neighbours outward within it.
//
// The BFS is global (shared `placed` set) so nodes aren't double-counted when
// two roots share the same neighbor.

// Node-type ring offsets — different types land on different radii so they
// don't overlap even when they share the same parent scandal.
const TYPE_RING_OFFSET: Record<string, number> = {
  politician:       0,
  organization:     80,
  legal_proceeding: 160,
  scandal:          0,
};

const RING_BASE = 240;    // base radius of the first neighbour ring
const ROOT_RING = 200;    // radius of the ring that holds multiple roots
const MIN_ARC_PX = 56;    // minimum arc-length gap between siblings at any ring

function egoLayout(
  graph: Graph,
  roots: Set<string>,
): Record<string, { x: number; y: number }> {
  const positions: Record<string, { x: number; y: number }> = {};
  const placed = new Set<string>();

  const rootArr = Array.from(roots).filter((r) => graph.hasNode(r));
  if (rootArr.length === 0) return positions;

  // ── 1. place roots ─────────────────────────────────────────────────────────
  const rootCenter = (i: number): { x: number; y: number } => {
    if (rootArr.length === 1) return { x: 0, y: 0 };
    const angle = (2 * Math.PI * i) / rootArr.length - Math.PI / 2;
    const r = Math.max(ROOT_RING, (MIN_ARC_PX * rootArr.length) / (2 * Math.PI));
    return { x: r * Math.cos(angle), y: r * Math.sin(angle) };
  };

  rootArr.forEach((id, i) => {
    positions[id] = rootCenter(i);
    placed.add(id);
  });

  // ── 2. per-root BFS fan-out ────────────────────────────────────────────────
  rootArr.forEach((rootId, rootIdx) => {
    const rp = positions[rootId];

    const sliceCenter =
      rootArr.length === 1
        ? 0
        : (2 * Math.PI * rootIdx) / rootArr.length - Math.PI / 2;
    const sliceArc =
      rootArr.length === 1 ? 2 * Math.PI : (2 * Math.PI * 0.88) / rootArr.length;

    type Item = { id: string; angle: number; depth: number };
    let frontier: Item[] = [{ id: rootId, angle: sliceCenter, depth: 0 }];

    const visited = new Set<string>([rootId]);

    while (frontier.length > 0) {
      const nextFrontier: Item[] = [];

      for (const { id: parentId, angle: parentAngle, depth } of frontier) {
        // Collect unplaced visible neighbours, grouped by node type
        const byType: Record<string, string[]> = {};
        graph.forEachNeighbor(parentId, (nbr) => {
          if (placed.has(nbr)) return;
          if (visited.has(nbr)) return;
          if (graph.getNodeAttribute(nbr, "hidden") as boolean) return;
          visited.add(nbr);
          placed.add(nbr);
          const t = (graph.getNodeAttribute(nbr, "nodeType") as string) ?? "unknown";
          if (!byType[t]) byType[t] = [];
          byType[t].push(nbr);
        });

        for (const [nodeType, siblings] of Object.entries(byType)) {
          if (siblings.length === 0) continue;

          const childDepth = depth + 1;
          const ringOffset = TYPE_RING_OFFSET[nodeType] ?? 0;
          const radius = RING_BASE * childDepth + ringOffset;

          // Arc available for this group — full slice at depth 1, narrows deeper
          const availableArc = childDepth === 1 ? sliceArc : sliceArc / childDepth;
          // Ensure minimum spacing between nodes
          const minArc = (MIN_ARC_PX / radius) * siblings.length;
          const arc = Math.max(availableArc, minArc);

          // Spread siblings evenly across the arc, leaving a small gap at ends
          // Use (i+0.5)/n * arc instead of i/(n-1) * arc to avoid wrap-overlap
          // when arc approaches 2π.
          const step = arc / siblings.length;
          const startAngle = parentAngle - arc / 2 + step * 0.5;

          siblings.forEach((nbrId, si) => {
            const angle = startAngle + si * step;
            positions[nbrId] = {
              x: rp.x + radius * Math.cos(angle),
              y: rp.y + radius * Math.sin(angle),
            };
            nextFrontier.push({ id: nbrId, angle, depth: childDepth });
          });
        }
      }

      frontier = nextFrontier;
    }
  });

  return positions;
}

// ─── initial seed layout ──────────────────────────────────────────────────────
// On first load only scandal (seed) nodes are visible.
// Spread them evenly on a circle so the user has clear targets to click.

const SEED_RING = 280;

function seedLayout(graph: Graph): void {
  const scandals: string[] = [];
  graph.forEachNode((id, attrs) => {
    if (attrs.nodeType === "scandal") scandals.push(id);
  });
  if (scandals.length === 0) return;

  scandals.forEach((id, i) => {
    const angle = (2 * Math.PI * i) / scandals.length - Math.PI / 2;
    const r = scandals.length === 1 ? 0 : SEED_RING;
    graph.setNodeAttribute(id, "x", r * Math.cos(angle) + jitter(20));
    graph.setNodeAttribute(id, "y", r * Math.sin(angle) + jitter(20));
  });
}

function jitter(range: number): number {
  return (Math.random() - 0.5) * 2 * range;
}

// ─── graph builder ────────────────────────────────────────────────────────────
// Merges a GraphResponse into an existing graphology Graph (or builds fresh).
// New nodes start hidden; only scandal seeds are visible on first load.

function mergeGraphData(
  graph: Graph,
  data: GraphResponse,
  filters: ActiveFilters,
  hidden = true,
): void {
  const counts = new Map<string, number>();
  for (const e of data.edges) {
    counts.set(e.from, (counts.get(e.from) ?? 0) + 1);
    counts.set(e.to, (counts.get(e.to) ?? 0) + 1);
  }

  for (const node of data.nodes) {
    if (
      filters.nodeTypes[node.type as keyof typeof filters.nodeTypes] === false
    )
      continue;
    if (graph.hasNode(node.id)) continue;

    const size = nodeSize(node, counts.get(node.id) ?? 0);
    const color = NODE_COLORS[node.type] ?? "#555555";

    const photoUrl =
      (node.properties.photo_url as string | undefined) ??
      (node.properties.logo_url as string | undefined);
    const nodeRenderType = photoUrl ? "image" : "pictogram";
    const imageUrl = photoUrl ?? getIconURI(node.type);

    // Scandals are always visible (seeds); everything else starts hidden
    const isHidden = hidden && node.type !== "scandal";

    graph.addNode(node.id, {
      label: node.label,
      size,
      color,
      x: 0,
      y: 0,
      nodeType: node.type,
      originalColor: color,
      originalSize: size,
      properties: node.properties,
      type: nodeRenderType,
      image: imageUrl,
      borderColor: color,
      hidden: isHidden,
    });
  }

  for (const edge of data.edges) {
    const status = (edge.properties?.status as string) ?? "membership";
    if (filters.edgeStatus[status as keyof typeof filters.edgeStatus] === false)
      continue;
    if (!graph.hasNode(edge.from) || !graph.hasNode(edge.to)) continue;
    if (graph.hasEdge(edge.from, edge.to)) continue;
    graph.addEdge(edge.from, edge.to, {
      color: edgeColor(edge) + "44",
      size: 1,
      edgeStatus: status,
      edgeType: edge.type,
      hidden: true, // edges start hidden; revealed when both endpoints are visible
    });
  }
}

// Reveal edges whose both endpoints are now visible
function revealEdges(graph: Graph): void {
  graph.forEachEdge((edge) => {
    const src = graph.source(edge);
    const tgt = graph.target(edge);
    const srcHidden = graph.getNodeAttribute(src, "hidden") as boolean;
    const tgtHidden = graph.getNodeAttribute(tgt, "hidden") as boolean;
    graph.setEdgeAttribute(edge, "hidden", srcHidden || tgtHidden);
  });
}

// ─── component ───────────────────────────────────────────────────────────────

export function GraphCanvas() {
  const containerRef = useRef<HTMLDivElement>(null);
  const sigmaRef = useRef<Sigma | null>(null);
  const graphRef = useRef<Graph | null>(null);
  const selectedRef = useRef<string | null>(null);
  const animTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // tracks which nodes have been expanded (clicked to reveal neighbors)
  const expandedRootsRef = useRef<Set<string>>(new Set());
  // cancel fn returned by animateNodes
  const layoutAnimCancelRef = useRef<(() => void) | null>(null);

  const { filters, timelineRange, focusedNodeId, setSelectedNode } =
    useAppStore();

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

  const activeData = timelineData;

  // ── shared layout+animate helper ─────────────────────────────────────────
  const runLayout = useCallback(() => {
    const graph = graphRef.current;
    if (!graph) return;

    if (layoutAnimCancelRef.current) {
      layoutAnimCancelRef.current();
      layoutAnimCancelRef.current = null;
    }

    if (expandedRootsRef.current.size === 0) {
      // back to seed — spread scandals evenly
      const scandals: string[] = [];
      graph.forEachNode((id, attrs) => {
        if (attrs.nodeType === "scandal") scandals.push(id);
      });
      const SEED_R = 280;
      const seedTargets: Record<string, { x: number; y: number }> = {};
      scandals.forEach((id, i) => {
        const angle = (2 * Math.PI * i) / scandals.length - Math.PI / 2;
        seedTargets[id] = {
          x: (scandals.length === 1 ? 0 : SEED_R) * Math.cos(angle),
          y: (scandals.length === 1 ? 0 : SEED_R) * Math.sin(angle),
        };
      });
      layoutAnimCancelRef.current = animateNodes(graph, seedTargets, {
        easing: "cubicInOut",
        duration: 500,
      });
      return;
    }

    const targets = egoLayout(graph, expandedRootsRef.current);

    // snap any node sitting at origin to its target before animating
    for (const [id, pos] of Object.entries(targets)) {
      const x = graph.getNodeAttribute(id, "x") as number;
      const y = graph.getNodeAttribute(id, "y") as number;
      if (x === 0 && y === 0) {
        graph.setNodeAttribute(id, "x", pos.x);
        graph.setNodeAttribute(id, "y", pos.y);
      }
    }

    const visibleTargets: Record<string, { x: number; y: number }> = {};
    for (const [id, pos] of Object.entries(targets)) {
      if (!(graph.getNodeAttribute(id, "hidden") as boolean)) {
        visibleTargets[id] = pos;
      }
    }

    layoutAnimCancelRef.current = animateNodes(graph, visibleTargets, {
      easing: "cubicInOut",
      duration: 600,
    });
  }, []);

  // ── expand: BFS un-hide all reachable nodes from this root ───────────────
  const expandNode = useCallback(
    (nodeId: string) => {
      const graph = graphRef.current;
      if (!graph) return;

      expandedRootsRef.current.add(nodeId);

      // BFS over local graph — all nodes already loaded, just need un-hiding
      const queue = [nodeId];
      const visited = new Set<string>([nodeId]);
      while (queue.length > 0) {
        const current = queue.shift()!;
        graph.setNodeAttribute(current, "hidden", false);
        graph.forEachNeighbor(current, (nbr) => {
          if (visited.has(nbr)) return;
          const nType = graph.getNodeAttribute(nbr, "nodeType") as string;
          if (filters.nodeTypes[nType as keyof typeof filters.nodeTypes] === false) return;
          visited.add(nbr);
          queue.push(nbr);
        });
      }

      revealEdges(graph);
      runLayout();
    },
    [filters, runLayout],
  );

  // ── collapse: hide exclusive nodes, re-layout ────────────────────────────
  const collapseNode = useCallback(
    (nodeId: string) => {
      const graph = graphRef.current;
      if (!graph) return;

      expandedRootsRef.current.delete(nodeId);

      // Nodes still reachable from remaining expanded roots
      const stillReachable = new Set<string>();
      for (const rootId of expandedRootsRef.current) {
        if (!graph.hasNode(rootId)) continue;
        const q = [rootId];
        const vis = new Set<string>([rootId]);
        while (q.length > 0) {
          const cur = q.shift()!;
          stillReachable.add(cur);
          graph.forEachNeighbor(cur, (nbr) => {
            if (vis.has(nbr)) return;
            vis.add(nbr);
            q.push(nbr);
          });
        }
      }

      // Hide everything not reachable from remaining roots (except scandal seeds)
      graph.forEachNode((id, attrs) => {
        if (attrs.nodeType === "scandal") return; // seeds always visible
        if (!stillReachable.has(id)) {
          graph.setNodeAttribute(id, "hidden", true);
        }
      });

      revealEdges(graph);
      runLayout();
    },
    [runLayout],
  );

  const renderGraph = useCallback(
    (data: GraphResponse) => {
      if (!containerRef.current) return;

      expandedRootsRef.current = new Set();
      if (layoutAnimCancelRef.current) {
        layoutAnimCancelRef.current();
        layoutAnimCancelRef.current = null;
      }

      const graph = new Graph({ multi: false, allowSelfLoops: false });
      mergeGraphData(graph, data, filters, true);
      seedLayout(graph);
      graphRef.current = graph;
      selectedRef.current = null;

      sigmaRef.current?.kill();
      sigmaRef.current = null;

      const sigma = new Sigma(graph, containerRef.current, {
        renderLabels: true,
        labelFont: "IBM Plex Mono, monospace",
        labelSize: 11,
        labelWeight: "400",
        labelColor: { color: "#777777" },
        labelRenderedSizeThreshold: 8,
        renderEdgeLabels: false,
        defaultNodeColor: "#444444",
        defaultEdgeColor: "#1a1a1a",
        minCameraRatio: 0.04,
        maxCameraRatio: 8,
        nodeProgramClasses: {
          image: NodeImageProgram,
          pictogram: NodePictogramProgram,
          circle: NodeCircleProgram,
        },
        edgeProgramClasses: { line: EdgeLineProgram },
        defaultNodeType: "circle",
        defaultEdgeType: "line",
      });

      sigmaRef.current = sigma;

      // ── click node ─────────────────────────────────────────────────────────
      sigma.on("clickNode", ({ node, event }) => {
        event.preventSigmaDefault();

        const attrs = graph.getNodeAttributes(node);
        selectedRef.current = node;

        setSelectedNode({
          id: node,
          type: attrs.nodeType,
          label: attrs.label ?? node,
          properties: attrs.properties ?? {},
        });

        // brief scale pulse
        const orig = attrs.originalSize ?? attrs.size;
        graph.setNodeAttribute(node, "size", orig * 1.9);
        setTimeout(() => {
          if (graphRef.current?.hasNode(node)) {
            graphRef.current.setNodeAttribute(node, "size", orig);
          }
        }, 320);

        // expand or collapse if this is a scandal node
        if (attrs.nodeType === "scandal") {
          if (expandedRootsRef.current.has(node)) {
            collapseNode(node);
          } else {
            expandNode(node);
          }
        }
      });

      // ── click background → deselect ────────────────────────────────────────
      sigma.on("clickStage", () => {
        if (!selectedRef.current) return;
        selectedRef.current = null;
        setSelectedNode(null);
      });

      setTimeout(() => sigma.getCamera().animatedReset(), 80);
    },
    [filters, setSelectedNode, expandNode, collapseNode],
  );

  useEffect(() => {
    if (!activeData) return;
    renderGraph(activeData);
  }, [activeData, renderGraph]);

  // ── focus dimming — animated, independent of renderGraph ──────────────────
  useEffect(() => {
    const graph = graphRef.current;
    const sigma = sigmaRef.current;
    if (!graph || !sigma) return;

    if (animTimerRef.current) {
      clearInterval(animTimerRef.current);
      animTimerRef.current = null;
    }

    if (!focusedNodeId) {
      animTimerRef.current = animateDim(sigma, new Set(), true);
      return;
    }

    if (!graph.hasNode(focusedNodeId)) return;

    const hood = new Set<string>([focusedNodeId]);
    graph.forEachNeighbor(focusedNodeId, (n1) => {
      hood.add(n1);
      graph.forEachNeighbor(n1, (n2) => hood.add(n2));
    });

    sigma.setSetting("edgeReducer", (edge, attrs) => {
      const src = graph.source(edge);
      const tgt = graph.target(edge);
      return { ...attrs, hidden: !(hood.has(src) && hood.has(tgt)) };
    });

    animTimerRef.current = animateDim(sigma, hood, false);
  }, [focusedNodeId]);

  useEffect(
    () => () => {
      if (animTimerRef.current) clearInterval(animTimerRef.current);
      if (layoutAnimCancelRef.current) layoutAnimCancelRef.current();
      sigmaRef.current?.kill();
      sigmaRef.current = null;
    },
    [],
  );

  return (
    <div
      ref={containerRef}
      className="absolute inset-0 w-full h-full"
      style={{ background: "#0a0a0a", pointerEvents: "all" }}
    />
  );
}
