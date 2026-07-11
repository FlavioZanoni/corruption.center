import type { NextConfig } from "next";
import { ALLOWED_IMAGE_HOST_SUFFIXES } from "./lib/images";

const nextConfig: NextConfig = {
  output: "standalone",
  images: {
    // Official photo hosts only (see lib/images.ts). Politician portraits are
    // hotlinked from the source that publishes them, never re-hosted.
    remotePatterns: ALLOWED_IMAGE_HOST_SUFFIXES.flatMap((suffix) => [
      { protocol: "https" as const, hostname: suffix },
      { protocol: "https" as const, hostname: `**.${suffix}` },
    ]),
  },
};

export default nextConfig;
