"use client";

import { useEffect, useState } from "react";
import { fetchReadiness, type ApiState } from "../../services/health";

const LABELS: Record<ApiState | "checking", string> = {
  checking: "Checking backend…",
  ready: "Backend ready",
  not_ready: "Backend not ready",
  unreachable: "Backend unreachable",
};

/** Delay before the next check while the backend is not ready. Matches the `Retry-After` the API sends. */
export const RETRY_MS = 5000;

interface ApiStatusProps {
  /** Replaceable for tests. */
  check?: () => Promise<ApiState>;
  /** Replaceable for tests. */
  retryMs?: number;
}

/**
 * Shows whether the backend is ready. While it is not ready the check repeats
 * after `retryMs`, one check at a time, until the backend is ready or the
 * component unmounts. A check that throws counts as unreachable.
 */
export function ApiStatus({ check = () => fetchReadiness(), retryMs = RETRY_MS }: ApiStatusProps) {
  const [state, setState] = useState<ApiState | "checking">("checking");

  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const run = async () => {
      let result: ApiState;
      try {
        result = await check();
      } catch {
        result = "unreachable";
      }
      if (!active) return;
      setState(result);
      if (result !== "ready") timer = setTimeout(run, retryMs);
    };
    void run();

    return () => {
      active = false;
      if (timer !== undefined) clearTimeout(timer);
    };
    // `check` and `retryMs` are read once on mount. A new function identity must not restart the probe.
  }, []);

  return (
    <p role="status" data-state={state}>
      {LABELS[state]}
    </p>
  );
}
