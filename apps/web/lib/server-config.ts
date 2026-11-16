/**
 * Build-time configuration shared by next.config.ts and its tests. It runs in
 * the Node.js process of Next.js only and is never bundled for the browser.
 */

const API_ORIGIN_KEY = "MORNINGSTAR_API_ORIGIN";
const DEFAULT_API_ORIGIN = "http://127.0.0.1:8080";
const LOOPBACK_HOSTS = new Set(["127.0.0.1", "localhost", "[::1]"]);

function refuse(reason: string): never {
  // The rejected value is never echoed. It could carry credentials.
  throw new Error(`${API_ORIGIN_KEY}: ${reason}`);
}

/**
 * Returns the origin of the Go API that the same-origin rewrite targets.
 * Fails closed: anything that is not a plain `http://<loopback host>:<port>`
 * is refused, so the web server can never be pointed at a remote host by a
 * stray environment variable.
 */
export function resolveApiOrigin(env: Record<string, string | undefined>): string {
  const raw = env[API_ORIGIN_KEY] ?? DEFAULT_API_ORIGIN;

  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return refuse("is not a valid URL");
  }
  if (url.protocol !== "http:") refuse("must use http on the loopback interface");
  if (url.username !== "" || url.password !== "") refuse("must not contain credentials");
  if (!LOOPBACK_HOSTS.has(url.hostname)) refuse("must be 127.0.0.1, localhost or [::1]");
  if (url.port === "") refuse("must include an explicit port");
  if (url.pathname !== "/" || url.search !== "" || url.hash !== "") {
    refuse("must be an origin without path, query or fragment");
  }
  return url.origin;
}

export interface HeaderEntry {
  key: string;
  value: string;
}

/**
 * Next.js `source` pattern for the pages that the web server answers itself.
 * It excludes /api/ so that responses proxied from the Go API keep the single
 * policy that Go sets (API-19). Without the exclusion a browser could receive
 * two policies for one response.
 */
export const PAGE_HEADERS_SOURCE = "/((?!api/).*)";

/**
 * Static response headers for every page. The nonce-based Content-Security-
 * Policy that the plan assigns to milestone M13 is not part of this baseline.
 * Only the framing restriction is set here, because it needs no nonce.
 */
export function securityHeaders(): HeaderEntry[] {
  return [
    { key: "X-Frame-Options", value: "DENY" },
    { key: "X-Content-Type-Options", value: "nosniff" },
    { key: "Referrer-Policy", value: "no-referrer" },
    { key: "Content-Security-Policy", value: "frame-ancestors 'none'" },
  ];
}
