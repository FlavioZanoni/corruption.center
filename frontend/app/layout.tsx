import type { Metadata } from "next";
import { IBM_Plex_Mono, IBM_Plex_Serif } from "next/font/google";
import "./globals.css";
import { Providers } from "./providers";
import { SITE_URL } from "@/lib/seo";

const ibmPlexSerif = IBM_Plex_Serif({
  variable: "--font-serif",
  subsets: ["latin"],
  weight: ["400", "600", "700"],
});

const ibmPlexMono = IBM_Plex_Mono({
  variable: "--font-mono",
  subsets: ["latin"],
  weight: ["400", "500"],
});

const SITE_NAME = "corruption.center";
const SITE_DESCRIPTION =
  "Mapeamento interativo de escândalos de corrupção, políticos e processos judiciais no Brasil, a partir de fontes públicas e oficiais.";

export const metadata: Metadata = {
  // Resolves every relative URL below (canonicals, Open Graph images) against
  // the deploy origin. Without it, Next falls back to localhost in production.
  metadataBase: new URL(SITE_URL),
  title: {
    default: `${SITE_NAME}: Rede de Corrupção Brasil`,
    // Entity pages set only their own title: "Fulano de Tal | corruption.center"
    template: `%s | ${SITE_NAME}`,
  },
  description: SITE_DESCRIPTION,
  keywords: [
    "corrupção",
    "Brasil",
    "escândalos",
    "políticos",
    "transparência",
    "dados públicos",
    "processos judiciais",
    "Lava Jato",
    "prestação de contas",
  ],
  authors: [{ name: SITE_NAME }],
  creator: SITE_NAME,
  publisher: SITE_NAME,
  alternates: {
    canonical: "/",
  },
  openGraph: {
    type: "website",
    locale: "pt_BR",
    url: "/",
    siteName: SITE_NAME,
    title: `${SITE_NAME}: Rede de Corrupção Brasil`,
    description: SITE_DESCRIPTION,
  },
  twitter: {
    card: "summary_large_image",
    title: `${SITE_NAME}: Rede de Corrupção Brasil`,
    description: SITE_DESCRIPTION,
  },
  robots: {
    index: true,
    follow: true,
    googleBot: {
      index: true,
      follow: true,
      "max-image-preview": "large",
      "max-snippet": -1,
      "max-video-preview": -1,
    },
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="pt-BR" className="dark">
      <body
        className={`${ibmPlexSerif.variable} ${ibmPlexMono.variable} antialiased`}
      >
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
