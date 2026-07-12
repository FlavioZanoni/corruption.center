import Link from "next/link";
import { ArrowLeft } from "lucide-react";

// The three browse pages are siblings, so they link to each other. Without this,
// switching from cases to sanctions meant going back to the graph and out again --
// and the graph is the one view that shows almost none of this data.
const VIEWS = [
  { href: "/politicos", label: "Políticos" },
  { href: "/processos", label: "Processos" },
  { href: "/sancoes", label: "Sanções" },
];

export function BrowseNav({ current }: { current: string }) {
  return (
    <div className="flex items-center gap-4 shrink-0">
      <div className="flex items-center gap-3 text-xs font-mono">
        {VIEWS.filter((v) => v.href !== current).map((v) => (
          <Link
            key={v.href}
            href={v.href}
            className="text-text-muted hover:text-white transition-colors"
          >
            {v.label}
          </Link>
        ))}
      </div>
      <Link
        href="/"
        className="inline-flex items-center gap-1.5 text-xs font-mono text-text-muted hover:text-white transition-colors"
      >
        <ArrowLeft size={13} strokeWidth={1.5} />
        Voltar ao grafo
      </Link>
    </div>
  );
}
