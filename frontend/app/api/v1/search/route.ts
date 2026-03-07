import { NextRequest, NextResponse } from "next/server";
import { mockSearch } from "@/lib/mock/data";

export function GET(req: NextRequest) {
  const q = req.nextUrl.searchParams.get("q") ?? "";
  const type = req.nextUrl.searchParams.get("type") ?? undefined;

  if (q.trim().length < 2) {
    return NextResponse.json({ results: [], total: 0 });
  }

  let { results, total } = mockSearch(q);
  if (type) {
    results = results.filter((r) => r.type === type);
    total = results.length;
  }

  return NextResponse.json({ results, total });
}
