import { describe, expect, it } from "vitest";
import { resolveApiOrigin, securityHeaders } from "./server-config";

describe("resolveApiOrigin", () => {
  it("defaults to the loopback API address", () => {
    expect(resolveApiOrigin({})).toBe("http://127.0.0.1:8080");
  });

  it.each([
    ["http://127.0.0.1:9090", "http://127.0.0.1:9090"],
    ["http://localhost:8080", "http://localhost:8080"],
    ["http://[::1]:8080", "http://[::1]:8080"],
    ["http://127.0.0.1:8080/", "http://127.0.0.1:8080"],
  ])("accepts %s", (value, expected) => {
    expect(resolveApiOrigin({ MORNINGSTAR_API_ORIGIN: value })).toBe(expected);
  });

  it.each([
    ["a remote host", "http://example.com:8080"],
    ["a LAN address", "http://192.168.1.5:8080"],
    ["the wildcard address", "http://0.0.0.0:8080"],
    ["https", "https://127.0.0.1:8080"],
    ["credentials in the URL", ["http:", "//u:p@127.0.0.1:8080"].join("")],
    ["a path", "http://127.0.0.1:8080/api"],
    ["a query", "http://127.0.0.1:8080/?x=1"],
    ["a fragment", "http://127.0.0.1:8080/#x"],
    ["a host that only starts like localhost", "http://localhost.evil.example:8080"],
    ["no port", "http://127.0.0.1"],
    ["garbage", "not a url"],
    ["an empty value", ""],
  ])("refuses %s", (_name, value) => {
    expect(() => resolveApiOrigin({ MORNINGSTAR_API_ORIGIN: value })).toThrow(/MORNINGSTAR_API_ORIGIN/);
  });

  it("does not echo credentials from a refused value", () => {
    let message = "";
    try {
      resolveApiOrigin({ MORNINGSTAR_API_ORIGIN: ["http:", "//u:hunter2@example.com:8080"].join("") });
    } catch (e) {
      message = String(e);
    }
    expect(message).not.toContain("hunter2");
  });
});

describe("securityHeaders", () => {
  const headers = Object.fromEntries(securityHeaders().map((h) => [h.key.toLowerCase(), h.value]));

  it("denies framing, sniffing and referrers", () => {
    expect(headers["x-frame-options"]).toBe("DENY");
    expect(headers["x-content-type-options"]).toBe("nosniff");
    expect(headers["referrer-policy"]).toBe("no-referrer");
    expect(headers["content-security-policy"]).toBe("frame-ancestors 'none'");
  });
});
