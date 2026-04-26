import { describe, expect, it } from "vitest";
import { planWorkspaceCleanup } from "../workspaceCleanup";

describe("planWorkspaceCleanup", () => {
  it("plans cleanup only for completed or failed old workspaces", () => {
    const plan = planWorkspaceCleanup({
      now: "2026-04-22T10:00:00.000Z",
      maxAgeHours: 24,
      workspaces: [
        { path: "/tmp/a", status: "completed", updatedAt: "2026-04-20T00:00:00.000Z" },
        { path: "/tmp/b", status: "active", updatedAt: "2026-04-20T00:00:00.000Z" },
        { path: "/tmp/c", status: "failed", updatedAt: "2026-04-22T09:00:00.000Z" }
      ]
    });

    expect(plan).toEqual({
      candidates: [{ path: "/tmp/a", reason: "completed workspace older than 24h" }],
      retained: [
        { path: "/tmp/b", reason: "workspace is active" },
        { path: "/tmp/c", reason: "workspace age is below threshold" }
      ]
    });
  });
});
