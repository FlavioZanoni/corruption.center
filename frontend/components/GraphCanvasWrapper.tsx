"use client";

import dynamic from "next/dynamic";
import { GraphLegend } from "./GraphLegend";

const GraphCanvas = dynamic(
  () => import("@/components/GraphCanvas").then((m) => m.GraphCanvas),
  { ssr: false },
);

export function GraphCanvasWrapper() {
  return (
    <>
      <GraphLegend />
      <GraphCanvas />
    </>
  );
}
