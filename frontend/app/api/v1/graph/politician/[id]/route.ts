import { NextResponse } from "next/server";
import { mockExpand } from "@/lib/mock/data";

export function GET(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  return params.then(({ id }) => NextResponse.json(mockExpand(id, 2)));
}
