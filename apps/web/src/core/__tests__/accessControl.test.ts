import { describe, expect, it } from "vitest";
import { canPerformAction } from "../accessControl";

describe("canPerformAction", () => {
  it("allows operators to run operations but not manage providers", () => {
    expect(canPerformAction({ role: "operator" }, "run-operation")).toBe(true);
    expect(canPerformAction({ role: "operator" }, "manage-provider")).toBe(false);
  });

  it("allows reviewers to approve checkpoints", () => {
    expect(canPerformAction({ role: "reviewer" }, "approve-checkpoint")).toBe(true);
  });

  it("allows admins to perform every action", () => {
    expect(canPerformAction({ role: "admin" }, "manage-provider")).toBe(true);
    expect(canPerformAction({ role: "admin" }, "run-codex")).toBe(true);
  });
});
