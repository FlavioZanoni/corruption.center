"use client"

import dynamic from "next/dynamic"

const GraphCanvas = dynamic(
  () => import("@/components/GraphCanvas").then((m) => m.GraphCanvas),
  { ssr: false }
)

export function GraphCanvasWrapper() {
  return <GraphCanvas />
}
