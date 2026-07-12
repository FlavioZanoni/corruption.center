import type { Metadata } from "next";
import { siteUrl } from "@/lib/seo";

export const metadata: Metadata = {
  title: "Sanções",
  description:
    "Sanções aplicadas a pessoas e empresas pelos cadastros oficiais brasileiros: CEIS, CNEP, CEAF, acordos de leniência e inabilitações do TCU.",
  alternates: { canonical: siteUrl("/sancoes") },
};

export default function SancoesLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return children;
}
