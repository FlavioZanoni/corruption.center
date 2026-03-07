import { NextRequest, NextResponse } from "next/server"
import { mockExpand } from "@/lib/mock/data"

export function GET(
  req: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  return params.then(({ id }) => {
    const hops = parseInt(req.nextUrl.searchParams.get("hops") ?? "2")
    return NextResponse.json(mockExpand(id, isNaN(hops) ? 2 : Math.min(hops, 5)))
  })
}
