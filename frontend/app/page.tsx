import { SearchBar } from "@/components/SearchBar";
import { FilterPanel } from "@/components/FilterPanel";
import { TimelineScrubber } from "@/components/TimelineScrubber";
import { DetailPanel } from "@/components/DetailPanel";
import { GraphCanvasWrapper } from "@/components/GraphCanvasWrapper";

export default function Home() {
  return (
    <main className="relative w-screen h-screen overflow-hidden bg-bg">
      <GraphCanvasWrapper />
      <SearchBar />
      <FilterPanel />
      <DetailPanel />
      <TimelineScrubber />

      <div className="absolute bottom-16 right-4 z-10 pointer-events-none select-none">
        <span className="text-[9px] font-mono text-[#333333] tracking-widest uppercase">
          corruption.center
        </span>
      </div>
    </main>
  );
}
