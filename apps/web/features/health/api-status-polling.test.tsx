import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { ApiStatus } from "./api-status";
import type { ApiState } from "../../services/health";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

/** Lets pending promises and the timers due within `ms` run. */
async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

function sequence(...states: ApiState[]) {
  const check = vi.fn(async () => states[Math.min(check.mock.calls.length - 1, states.length - 1)] as ApiState);
  return check;
}

describe("ApiStatus failure handling", () => {
  it("shows unreachable when the check rejects instead of staying on Checking", async () => {
    const check = vi.fn(async () => {
      throw new Error("boom");
    });
    render(<ApiStatus check={check} retryMs={1000} />);
    await advance(0);
    expect(screen.getByRole("status").textContent).toBe("Backend unreachable");
  });

  it("shows unreachable when the check throws synchronously", async () => {
    const check = vi.fn((): Promise<ApiState> => {
      throw new Error("sync boom");
    });
    render(<ApiStatus check={check} retryMs={1000} />);
    await advance(0);
    expect(screen.getByRole("status").textContent).toBe("Backend unreachable");
  });
});

describe("ApiStatus re-polling", () => {
  it("checks again after the retry delay while the backend is not ready", async () => {
    const check = sequence("not_ready", "unreachable", "ready");
    render(<ApiStatus check={check} retryMs={1000} />);
    await advance(0);
    expect(check).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("status").textContent).toBe("Backend not ready");

    await advance(999);
    expect(check).toHaveBeenCalledTimes(1);
    await advance(1);
    expect(check).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("status").textContent).toBe("Backend unreachable");

    await advance(1000);
    expect(check).toHaveBeenCalledTimes(3);
    expect(screen.getByRole("status").textContent).toBe("Backend ready");
  });

  it("stops polling once the backend is ready", async () => {
    const check = sequence("ready");
    render(<ApiStatus check={check} retryMs={1000} />);
    await advance(0);
    await advance(10_000);
    expect(check).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("does not overlap checks: the next delay starts after the previous check ends", async () => {
    let release: (s: ApiState) => void = () => {};
    const check = vi.fn(() => new Promise<ApiState>((resolve) => (release = resolve)));
    render(<ApiStatus check={check} retryMs={1000} />);
    await advance(5000);
    expect(check).toHaveBeenCalledTimes(1); // still waiting for the first answer
    release("not_ready");
    await advance(0);
    await advance(1000);
    expect(check).toHaveBeenCalledTimes(2);
  });

  it("stops polling on unmount", async () => {
    const check = sequence("unreachable");
    const { unmount } = render(<ApiStatus check={check} retryMs={1000} />);
    await advance(0);
    expect(check).toHaveBeenCalledTimes(1);
    unmount();
    expect(vi.getTimerCount()).toBe(0);
    await advance(10_000);
    expect(check).toHaveBeenCalledTimes(1);
  });

  it("ignores an answer that arrives after unmount", async () => {
    let release: (s: ApiState) => void = () => {};
    const check = vi.fn(() => new Promise<ApiState>((resolve) => (release = resolve)));
    const errors = vi.spyOn(console, "error").mockImplementation(() => {});
    const { unmount } = render(<ApiStatus check={check} retryMs={1000} />);
    unmount();
    release("not_ready");
    await advance(5000);
    expect(check).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(0);
    expect(errors).not.toHaveBeenCalled();
    errors.mockRestore();
  });
});
