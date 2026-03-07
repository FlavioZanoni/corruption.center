import { NextRequest, NextResponse } from "next/server";
import { mockTimeline } from "@/lib/mock/data";

export function GET(req: NextRequest) {
  const fromParam = req.nextUrl.searchParams.get("from");
  const toParam = req.nextUrl.searchParams.get("to");

  const from = fromParam ? parseInt(fromParam) : 2000;
  const to = toParam ? parseInt(toParam) : 2025;

  if (isNaN(from) || isNaN(to)) {
    return NextResponse.json(
      { error: "invalid from/to parameters" },
      { status: 400 },
    );
  }

  return NextResponse.json(mockTimeline(from, to));
}
