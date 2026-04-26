import { describe, expect, it } from "vitest";
import { createSampleRun, PipelineEngine } from "..";

describe("PipelineEngine", () => {
  it("requests human approval after a gated stage and opens the next stage after approval", () => {
    const engine = new PipelineEngine(createSampleRun());

    engine.startStage("intake");
    const afterCompletion = engine.completeStage("intake", {
      passed: true,
      notes: "Requirement is clear.",
      evidence: [{ id: "ev-1", label: "Acceptance Criteria", value: "Ready" }]
    });

    expect(afterCompletion.stages.find((stage) => stage.id === "intake")?.status).toBe(
      "needs-human"
    );
    expect(afterCompletion.stages.find((stage) => stage.id === "solution")?.status).toBe(
      "waiting"
    );

    const afterApproval = engine.approveHumanGate("intake", "Product Owner");

    expect(afterApproval.stages.find((stage) => stage.id === "intake")?.status).toBe("passed");
    expect(afterApproval.stages.find((stage) => stage.id === "solution")?.status).toBe("ready");
  });

  it("automatically opens the next stage when no human gate is required", () => {
    const engine = new PipelineEngine(createSampleRun());

    engine.startStage("intake");
    engine.completeStage("intake", {
      passed: true,
      notes: "Requirement is clear.",
      evidence: [{ id: "ev-1", label: "Acceptance Criteria", value: "Ready" }]
    });
    engine.approveHumanGate("intake", "Product Owner");
    engine.startStage("solution");
    engine.completeStage("solution", {
      passed: true,
      notes: "Design approved.",
      evidence: [{ id: "ev-2", label: "Architecture", value: "Ready" }]
    });
    engine.approveHumanGate("solution", "Tech Lead");
    engine.startStage("coding");

    const afterCoding = engine.completeStage("coding", {
      passed: true,
      notes: "Implementation is ready.",
      evidence: [{ id: "ev-3", label: "Pull Request", value: "Draft PR" }]
    });

    expect(afterCoding.stages.find((stage) => stage.id === "coding")?.status).toBe("passed");
    expect(afterCoding.stages.find((stage) => stage.id === "testing")?.status).toBe("ready");
  });

  it("enforces agent capability boundaries", () => {
    const engine = new PipelineEngine(createSampleRun());

    const withOptional = engine.selectCapability("delivery", "mcp.release");
    expect(withOptional.selectedCapabilities.delivery).toContain("mcp.release");

    expect(() => engine.deselectCapability("delivery", "mcp.github")).toThrow(
      "Cannot remove default capability"
    );
    expect(() => engine.selectCapability("requirement", "mcp.release")).toThrow(
      "Requirement Agent cannot use Release MCP"
    );
  });
});
