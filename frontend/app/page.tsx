import { SearchBar } from "@/components/SearchBar";
import { FilterPanel } from "@/components/FilterPanel";
import { TimelineScrubber } from "@/components/TimelineScrubber";
import { DetailPanel } from "@/components/DetailPanel";
import { GraphCanvasWrapper } from "@/components/GraphCanvasWrapper";

export default function Home() {
  const isMock =
    !process.env.NEXT_PUBLIC_API_URL ||
    new URL(process.env.NEXT_PUBLIC_API_URL, window.location.origin).origin ===
      window.location.origin;

  return (
    <main className="relative w-screen h-screen overflow-hidden bg-bg">
      <GraphCanvasWrapper />
      <SearchBar />
      <FilterPanel />
      <DetailPanel />
      <TimelineScrubber />

      {isMock && (
        <div className="absolute inset-0 z-50 flex items-center justify-center pointer-events-none select-none">
          <p className="text-center font-mono font-bold uppercase tracking-widest leading-tight opacity-5 rotate-[-19deg] text-[clamp(3rem,5vw,4rem)] text-text whitespace-pre-line">
            {"DADOS DE TESTE!\nINFOS NÃO SÃO REAIS"}
          </p>
        </div>
      )}

      <div className="absolute bottom-16 right-4 z-10 pointer-events-none select-none">
        <span className="text-[9px] font-mono text-[#333333] tracking-widest uppercase">
          corruption.center
        </span>
      </div>
    </main>
  );
}
