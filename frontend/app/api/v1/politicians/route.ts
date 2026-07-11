import { NextRequest, NextResponse } from "next/server";
import { mockPoliticians } from "@/lib/mock/data";

export function GET(req: NextRequest) {
  const sp = req.nextUrl.searchParams;
  const filter = sp.get("filter") ?? "";
  const party = sp.get("party") ?? "";
  const uf = sp.get("uf") ?? "";
  const page = parseInt(sp.get("page") ?? "1", 10) || 1;
  const pageSize = parseInt(sp.get("page_size") ?? "24", 10) || 24;
  const sort = sp.get("sort") === "name" ? "name" : "connections";

  return NextResponse.json(
    mockPoliticians(filter, party, uf, page, pageSize, sort),
  );
}
