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

// The same network motif as the static brand card (and the favicon): the
// scandal node in red at the center of its connections, drawn like the real
// force graph — gently curved edges, a soft glow on the hub, and size/opacity
// falling off toward the periphery. Satori renders inline SVG natively.
function Network() {
  return (
    <svg
      width="500"
      height="630"
      viewBox="60 0 500 630"
      style={{ position: "absolute", right: -40, top: 0 }}
    >
      {/* hub edges */}
      <g stroke="#4a4a4a" strokeWidth="3" fill="none">
        <path d="M300 310 L445 175" />
        <path d="M300 310 L475 405" />
        <path d="M300 310 L185 205" />
        <path d="M300 310 L215 475" />
      </g>
      {/* second-tier edges, dimmer and thinner */}
      <g stroke="#333333" strokeWidth="2" fill="none">
        <path d="M445 175 L475 405" />
        <path d="M445 175 L540 95" />
        <path d="M445 175 L555 300" />
        <path d="M475 405 L555 300" />
        <path d="M475 405 L530 520" />
        <path d="M185 205 L125 95" />
        <path d="M215 475 L120 530" />
      </g>
      {/* glow behind the scandal hub */}
      <circle cx="300" cy="310" r="82" fill={RED} opacity="0.04" />
      <circle cx="300" cy="310" r="60" fill={RED} opacity="0.07" />
      <circle cx="300" cy="310" r="40" fill={RED} opacity="0.12" />
      {/* inner ring: the entity types, in the graph's own colors */}
      <circle cx="445" cy="175" r="18" fill="#d8d8d8" />
      <circle cx="475" cy="405" r="16" fill="#8b7ec8" />
      <circle cx="185" cy="205" r="15" fill="#4488ff" />
      <circle cx="215" cy="475" r="14" fill="#22aa66" />
      {/* periphery, fading out */}
      <circle cx="530" cy="520" r="11" fill="#d98a4b" />
      <circle cx="540" cy="95" r="9" fill="#6a6a6a" />
      <circle cx="555" cy="300" r="9" fill="#7a7a7a" />
      <circle cx="125" cy="95" r="8" fill="#6a6a6a" opacity="0.9" />
      <circle cx="120" cy="530" r="7" fill="#5a5a5a" opacity="0.85" />
      <circle cx="300" cy="310" r="28" fill={RED} />
    </svg>
  )
}

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
        <Network />
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
              fontSize: name.length > 28 ? 60 : 80,
              fontWeight: 700,
              color: "#e8e8e8",
              lineHeight: 1.1,
              // leave the right side to the network
              maxWidth: 700,
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
