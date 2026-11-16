import { describe, expect, it } from "vitest";
import { fetchReadiness } from "./health";

function respond(status: number, body: string): typeof fetch {
  return async () => new Response(body, { status, headers: { "content-type": "application/json" } });
}

describe("fetchReadiness", () => {
  it("reports ready for 200 with status ready", async () => {
    expect(await fetchReadiness(respond(200, '{"status":"ready"}'))).toBe("ready");
  });

  it("reports not_ready for 503 with status not_ready", async () => {
    expect(await fetchReadiness(respond(503, '{"status":"not_ready"}'))).toBe("not_ready");
  });

  it.each([
    ["a 200 that does not say ready", 200, '{"status":"ok"}'],
    ["a 503 that does not say not_ready", 503, '{"status":"ready"}'],
    ["an error envelope", 403, '{"error":{"code":"host_denied"}}'],
    ["a 500", 500, '{"status":"ready"}'],
    ["a body that is not JSON", 200, "<html>proxy error</html>"],
    ["an empty body", 200, ""],
    ["a JSON array", 200, "[]"],
    ["null", 200, "null"],
  ])("treats %s as unreachable", async (_name, status, body) => {
    expect(await fetchReadiness(respond(status, body))).toBe("unreachable");
  });

  it("treats a network failure as unreachable", async () => {
    const failing: typeof fetch = async () => {
      throw new TypeError("fetch failed");
    };
    expect(await fetchReadiness(failing)).toBe("unreachable");
  });

  it("calls the same-origin readiness path without credentials, caching or redirects", async () => {
    let seenUrl = "";
    let seenInit: RequestInit | undefined;
    const spy: typeof fetch = async (input, init) => {
      seenUrl = String(input);
      seenInit = init;
      return new Response('{"status":"ready"}', { status: 200 });
    };
    await fetchReadiness(spy);
    expect(seenUrl).toBe("/api/v1/health/ready");
    expect(seenInit?.method ?? "GET").toBe("GET");
    expect(seenInit?.credentials).toBe("omit");
    expect(seenInit?.cache).toBe("no-store");
    expect(seenInit?.redirect).toBe("error");
  });
});
