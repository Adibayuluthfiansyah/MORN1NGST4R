import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { ApiStatus } from "./api-status";

afterEach(cleanup);

describe("ApiStatus", () => {
  it("shows a checking state first and then the result", async () => {
    render(<ApiStatus check={async () => "ready"} />);
    expect(screen.getByRole("status").textContent).toContain("Checking");
    expect((await screen.findByText(/ready/i)).textContent).toContain("Backend ready");
  });

  it("shows not ready", async () => {
    render(<ApiStatus check={async () => "not_ready"} />);
    expect((await screen.findByText(/not ready/i)).textContent).toContain("Backend not ready");
  });

  it("shows unreachable", async () => {
    render(<ApiStatus check={async () => "unreachable"} />);
    expect((await screen.findByText(/unreachable/i)).textContent).toContain("Backend unreachable");
  });
});
