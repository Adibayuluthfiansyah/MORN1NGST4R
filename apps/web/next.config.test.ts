import { afterEach, beforeEach, describe, expect, it } from "vitest";
import nextConfig from "./next.config";

const KEY = "MORNINGSTAR_API_ORIGIN";
let saved: string | undefined;

beforeEach(() => {
  saved = process.env[KEY];
  delete process.env[KEY];
});

afterEach(() => {
  if (saved === undefined) delete process.env[KEY];
  else process.env[KEY] = saved;
});

async function rewrites() {
  const result = await nextConfig.rewrites!();
  if (!Array.isArray(result)) throw new Error("rewrites() must return a plain list");
  return result;
}

describe("next.config rewrites", () => {
  it("forwards /api/v1/* to the default loopback API", async () => {
    expect(await rewrites()).toEqual([{ source: "/api/v1/:path*", destination: "http://127.0.0.1:8080/api/v1/:path*" }]);
  });

  it("uses the configured loopback origin", async () => {
    process.env[KEY] = "http://localhost:9191";
    expect(await rewrites()).toEqual([{ source: "/api/v1/:path*", destination: "http://localhost:9191/api/v1/:path*" }]);
  });

  it("rewrites only the API prefix", async () => {
    const [rule] = await rewrites();
    expect(rule?.source.startsWith("/api/v1/")).toBe(true);
    expect(await rewrites()).toHaveLength(1);
  });

  it.each(["http://example.com:8080", "https://127.0.0.1:8080", "http://0.0.0.0:8080", "http://127.0.0.1"])(
    "refuses a non-loopback or incomplete origin (%s) instead of forwarding to it",
    async (value) => {
      process.env[KEY] = value;
      await expect(nextConfig.rewrites!()).rejects.toThrow(/MORNINGSTAR_API_ORIGIN/);
    },
  );
});

describe("next.config headers", () => {
  async function entries() {
    const list = await nextConfig.headers!();
    return list;
  }

  it("applies the framing and sniffing protections to pages", async () => {
    const list = await entries();
    expect(list).toHaveLength(1);
    const headers = Object.fromEntries(list[0]!.headers.map((h) => [h.key.toLowerCase(), h.value]));
    expect(headers["x-frame-options"]).toBe("DENY");
    expect(headers["x-content-type-options"]).toBe("nosniff");
    expect(headers["referrer-policy"]).toBe("no-referrer");
    expect(headers["content-security-policy"]).toBe("frame-ancestors 'none'");
  });

  it("does not set a second policy on the API paths that Go answers", async () => {
    const source = (await entries())[0]!.source;
    // Next compiles `source` with path-to-regexp. The one pattern used here is
    // a plain negative look-ahead, so a RegExp reads it the same way.
    const re = new RegExp("^" + source + "$");
    expect(re.test("/")).toBe(true);
    expect(re.test("/some/page")).toBe(true);
    expect(re.test("/_next/static/chunk.js")).toBe(true);
    expect(re.test("/api/v1/health/ready")).toBe(false);
    expect(re.test("/api/v1/anything/else")).toBe(false);
    expect(re.test("/api/")).toBe(false);
  });

  it("does not reveal the framework", () => {
    expect(nextConfig.poweredByHeader).toBe(false);
  });
});
