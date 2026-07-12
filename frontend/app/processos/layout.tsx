import type { Metadata } from "next";
import { siteUrl } from "@/lib/seo";

// The browse page itself is a client component (filters, pagination), and a client
// component cannot export metadata — hence a layout to carry it.
export const metadata: Metadata = {
  title: "Processos judiciais",
  description:
    "Processos judiciais de casos de corrupção no Brasil, com número CNJ, tribunal e situação da condenação. Dados oficiais do CNJ (DataJud) e do DJEN.",
  alternates: { canonical: siteUrl("/processos") },
};

export default function ProcessosLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return children;
}
