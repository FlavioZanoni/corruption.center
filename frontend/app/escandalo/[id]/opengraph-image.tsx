// Dynamic Open Graph card for a scandal: its name, period and record counts,
// rendered per-request (ISR-cached) in the site's own type. File-based metadata
// wins over the page's openGraph.images, so this replaces the generic brand
// card on /escandalo/[id] links only. /pessoa deliberately keeps the generic
// card: a personalized share image for a private citizen would cut against the
// page's own noindex/anti-defamation stance.
import { ImageResponse } from "next/og"
import { readFile } from "fs/promises"
import path from "path"

import { fetchScandalDetail } from "@/lib/api/entities"
import { plural } from "@/lib/entity-labels"

export const size = { width: 1200, height: 630 }
export const contentType = "image/png"
export const alt = "Escândalo no corruption.center"

const RED = "#cc2222"

function yearRange(start?: string | null, end?: string | null): string {
  const y = (d?: string | null) => (d ? d.slice(0, 4) : "")
  if (!y(start)) return ""
  return y(end) && y(end) !== y(start) ? `${y(start)}–${y(end)}` : y(start)
}

export default async function Image({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = await params
  const detail = await fetchScandalDetail(decodeURIComponent(id))

  const [serif, mono] = await Promise.all([
    readFile(path.join(process.cwd(), "lib/og/ibm-plex-serif-bold.ttf")),
    readFile(path.join(process.cwd(), "lib/og/ibm-plex-mono-regular.ttf")),
  ])

  const name = detail?.scandal.name ?? "corruption.center"
  const nodes = detail?.connections.nodes ?? []
  const proceedings = nodes.filter((n) => n.type === "legal_proceeding").length
  const politicians = nodes.filter((n) => n.type === "politician").length
  const years = yearRange(detail?.scandal.date_start, detail?.scandal.date_end)

  const counts = [
    proceedings > 0 ? plural(proceedings, "processo judicial", "processos judiciais") : "",
    politicians > 0 ? plural(politicians, "político citado", "políticos citados") : "",
  ].filter(Boolean)

  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          backgroundColor: "#0a0a0a",
          padding: "0 80px",
          position: "relative",
          fontFamily: "mono",
        }}
      >
        <div
          style={{
            position: "absolute",
            top: 0,
            left: 0,
            width: 1200,
            height: 6,
            backgroundColor: RED,
            display: "flex",
          }}
        />
        <div
          style={{
            display: "flex",
            flexDirection: "column",
            justifyContent: "center",
            flexGrow: 1,
          }}
        >
          <div
            style={{
              display: "flex",
              fontSize: 22,
              letterSpacing: "0.35em",
              color: RED,
              marginBottom: 26,
            }}
          >
            {`ESCÂNDALO${years ? `  ·  ${years}` : ""}`}
          </div>
          <div
            style={{
              display: "flex",
              fontFamily: "serif",
              fontSize: name.length > 28 ? 64 : 84,
              fontWeight: 700,
              color: "#e8e8e8",
              lineHeight: 1.1,
              maxWidth: 1040,
            }}
          >
            {name}
          </div>
          {counts.length > 0 && (
            <div
              style={{
                display: "flex",
                fontSize: 27,
                color: "#9a9a9a",
                marginTop: 30,
              }}
            >
              {counts.join("  ·  ")}
            </div>
          )}
        </div>
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            paddingBottom: 54,
            fontSize: 20,
            letterSpacing: "0.15em",
          }}
        >
          <div style={{ display: "flex", color: "#5a5a5a" }}>
            {"DADOS OFICIAIS  CNJ · CGU · TCU · TSE"}
          </div>
          <div style={{ display: "flex", color: "#8a8a8a" }}>
            corruption.center
          </div>
        </div>
      </div>
    ),
    {
      ...size,
      fonts: [
        { name: "serif", data: serif, weight: 700, style: "normal" },
        { name: "mono", data: mono, weight: 400, style: "normal" },
      ],
    },
  )
}
