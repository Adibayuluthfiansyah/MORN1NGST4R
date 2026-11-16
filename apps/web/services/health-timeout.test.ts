import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchReadiness, READINESS_TIMEOUT_MS } from "./health";

afterEach(() => {
  vi.useRealTimers();
});

/** A fetch that never answers and rejects like the real one when aborted. */
function hanging(): typeof fetch {
  return (_input, init) =>
    new Promise<Response>((_resolve, reject) => {
      init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
    });
}

/** A fetch that answers the headers at once and then stalls the body until aborted. */
function stallingBody(): typeof fetch {
  return async (_input, init) => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        init?.signal?.addEventListener("abort", () => controller.error(new DOMException("aborted", "AbortError")));
      },
    });
    return new Response(body, { status: 200 });
  };
}

describe("fetchReadiness timeout", () => {
  it("has a bounded default", () => {
    expect(READINESS_TIMEOUT_MS).toBeGreaterThan(0);
    expect(READINESS_TIMEOUT_MS).toBeLessThanOrEqual(10_000);
  });

  it("gives up on a request that never answers", async () => {
    const started = Date.now();
    expect(await fetchReadiness(hanging(), { timeoutMs: 25 })).toBe("unreachable");
    expect(Date.now() - started).toBeLessThan(2000);
  });

  it("gives up on a body that never finishes", async () => {
    const started = Date.now();
    expect(await fetchReadiness(stallingBody(), { timeoutMs: 25 })).toBe("unreachable");
    expect(Date.now() - started).toBeLessThan(2000);
  });

  it("hands an abort signal to fetch", async () => {
    let signal: AbortSignal | null | undefined;
    const spy: typeof fetch = async (_input, init) => {
      signal = init?.signal;
      return new Response('{"status":"ready"}', { status: 200 });
    };
    await fetchReadiness(spy);
    expect(signal).toBeInstanceOf(AbortSignal);
  });

  it("uses the default timeout when none is given", async () => {
    vi.useFakeTimers();
    const pending = fetchReadiness(hanging());
    await vi.advanceTimersByTimeAsync(READINESS_TIMEOUT_MS - 1);
    let settled = false;
    void pending.then(() => (settled = true));
    await Promise.resolve();
    expect(settled).toBe(false);
    await vi.advanceTimersByTimeAsync(2);
    expect(await pending).toBe("unreachable");
  });

  it("leaves no timer behind after a normal answer", async () => {
    vi.useFakeTimers();
    const ok: typeof fetch = async () => new Response('{"status":"ready"}', { status: 200 });
    expect(await fetchReadiness(ok)).toBe("ready");
    expect(vi.getTimerCount()).toBe(0);
  });

  it("leaves no timer behind after a failure", async () => {
    vi.useFakeTimers();
    const failing: typeof fetch = async () => {
      throw new TypeError("fetch failed");
    };
    expect(await fetchReadiness(failing)).toBe("unreachable");
    expect(vi.getTimerCount()).toBe(0);
  });
});
