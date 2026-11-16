import type { NextConfig } from "next";
import { PAGE_HEADERS_SOURCE, resolveApiOrigin, securityHeaders } from "./lib/server-config";

const nextConfig: NextConfig = {
  poweredByHeader: false,
  reactStrictMode: true,
  // The browser talks only to this origin. Requests to /api/v1/* are
  // forwarded to the Go API on loopback (D-79). The Go server checks Host and
  // Origin itself. Whether the Host that it sees through this rewrite is
  // usable is a milestone M13 check, not settled here.
  async rewrites() {
    const apiOrigin = resolveApiOrigin(process.env);
    return [{ source: "/api/v1/:path*", destination: `${apiOrigin}/api/v1/:path*` }];
  },
  async headers() {
    return [{ source: PAGE_HEADERS_SOURCE, headers: securityHeaders() }];
  },
};

export default nextConfig;
