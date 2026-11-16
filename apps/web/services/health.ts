/**
 * Typed client for the public health probe of the Go API.
 *
 * Hand-written on purpose. The generated client of plan milestone M13 waits
 * for gate G-openapi, and the health routes are not in the OpenAPI document.
 */

export type ApiState = "ready" | "not_ready" | "unreachable";

export const READINESS_PATH = "/api/v1/health/ready";

/** Upper bound for one probe, request and body together. */
export const READINESS_TIMEOUT_MS = 5000;

export interface ReadinessOptions {
  /** Overrides READINESS_TIMEOUT_MS. */
  timeoutMs?: number;
}

/**
 * Asks the API whether it is ready. Only two answers count: 200 with a body
 * whose `status` is "ready", and 503 with `status` "not_ready". Any other
 * status, body, redirect, timeout or network error is "unreachable", so a
 * proxy error page can never read as healthy and a stalled request can never
 * leave the caller waiting.
 */
export async function fetchReadiness(
  fetchImpl: typeof fetch = fetch,
  { timeoutMs = READINESS_TIMEOUT_MS }: ReadinessOptions = {},
): Promise<ApiState> {
  // The timer covers the body as well as the headers: the signal also aborts
  // a response whose body stalls. It is cleared on every exit path.
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetchImpl(READINESS_PATH, {
      method: "GET",
      credentials: "omit", // the probe is public. Send no cookie.
      cache: "no-store",
      redirect: "error",
      signal: controller.signal,
    });
    const body: unknown = await response.json();
    const status =
      typeof body === "object" && body !== null && "status" in body ? (body as { status: unknown }).status : undefined;
    if (response.status === 200 && status === "ready") return "ready";
    if (response.status === 503 && status === "not_ready") return "not_ready";
    return "unreachable";
  } catch {
    return "unreachable";
  } finally {
    clearTimeout(timer);
  }
}
