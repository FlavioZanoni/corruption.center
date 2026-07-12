import Link from "next/link";
import { Users, Scale, Ban } from "lucide-react";
import { SearchBar } from "@/components/SearchBar";
import { FilterPanel } from "@/components/FilterPanel";
import { TimelineScrubber } from "@/components/TimelineScrubber";
import { DetailPanel } from "@/components/DetailPanel";
import { GraphCanvasWrapper } from "@/components/GraphCanvasWrapper";

export default function Home() {
  const apiURL = process.env.NEXT_PUBLIC_API_URL?.trim() ?? "";
  const isMock = apiURL === "" || apiURL.startsWith("/");

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

      {/* The graph opens on 3 scandals. Everything else we hold -- 6.5k court cases,
          65k sanctions, 4.9k politicians -- was reachable only by expanding a
          scandal node, i.e. not reachable. These are the doors to the rest. */}
      <nav className="absolute top-16 left-4 z-30 flex flex-wrap gap-2">
        <Link
          href="/politicos"
          className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-surface/90 backdrop-blur border border-border rounded-sm text-[11px] font-mono uppercase tracking-wider text-text-muted hover:text-white hover:border-[#c8a96e]/60 transition-colors"
        >
          <Users size={13} strokeWidth={1.5} />
          Políticos
        </Link>
        <Link
          href="/processos"
          className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-surface/90 backdrop-blur border border-border rounded-sm text-[11px] font-mono uppercase tracking-wider text-text-muted hover:text-white hover:border-[#8b7ec8]/60 transition-colors"
        >
          <Scale size={13} strokeWidth={1.5} />
          Processos
        </Link>
        <Link
          href="/sancoes"
          className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-surface/90 backdrop-blur border border-border rounded-sm text-[11px] font-mono uppercase tracking-wider text-text-muted hover:text-white hover:border-[#d98a4b]/60 transition-colors"
        >
          <Ban size={13} strokeWidth={1.5} />
          Sanções
        </Link>
      </nav>

      <div className="absolute bottom-16 right-4 z-10 flex flex-col items-end gap-1.5">
        <span className="text-[9px] font-mono text-[#333333] tracking-widest uppercase pointer-events-none select-none">
          corruption.center
        </span>
        <div className="flex gap-2 text-[8px] font-mono text-[#555555] pointer-events-auto">
          <Link
            href="/metodologia"
            className="hover:text-[#999999] transition-colors underline"
          >
            Metodologia e fontes
          </Link>
          <span className="text-[#333333]">·</span>
          <a
            href="mailto:contato@corruption.center"
            className="hover:text-[#999999] transition-colors underline"
          >
            Contato
          </a>
        </div>
      </div>
    </main>
  );
}
