// Turns the flat {nodes, edges} graph returned by the API into the grouped,
// provenance-carrying shape the server-rendered entity pages need.

import type { GraphEdge, GraphNode, GraphResponse, NodeType } from "@/lib/types"

export interface Connection {
  node: GraphNode
  // The edge that ties this node to the page's entity. When the node is not
  // directly linked to it (e.g. a person who is a defendant in a proceeding that
  // investigates this scandal), this is the edge that brings the node into the
  // graph and `via` names the node on the other end of it.
  edge?: GraphEdge
  via?: GraphNode
}

export function buildConnections(
  graph: GraphResponse | undefined,
  rootId: string,
): Connection[] {
  if (!graph) return []
  const nodeMap = new Map(graph.nodes.map((n) => [n.id, n]))

  return graph.nodes
    .filter((n) => n.id !== rootId)
    .map((node) => {
      const direct = graph.edges.find(
        (e) =>
          (e.from === rootId && e.to === node.id) ||
          (e.to === rootId && e.from === node.id),
      )
      if (direct) return { node, edge: direct }

      const indirect = graph.edges.find(
        (e) => e.from === node.id || e.to === node.id,
      )
      if (!indirect) return { node }

      const otherId = indirect.from === node.id ? indirect.to : indirect.from
      return { node, edge: indirect, via: nodeMap.get(otherId) }
    })
}

export function ofType(
  connections: Connection[],
  ...types: NodeType[]
): Connection[] {
  return connections.filter((c) => types.includes(c.node.type))
}

export function prop(node: GraphNode, key: string): string | undefined {
  const v = node.properties[key]
  if (v === null || v === undefined || v === "") return undefined
  return String(v)
}
