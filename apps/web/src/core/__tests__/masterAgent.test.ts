import { describe, expect, it } from "vitest";
import { createSampleRun, evaluateRun, PipelineEngine } from "..";
import type { PipelineStageId } from "../types";

const stageSequence: PipelineStageId[] = [
  "intake",
  "solution",
  "coding",
  "testing",
  "review",
  "delivery"
];

function passStage(engine: PipelineEngine, stageId: PipelineStageId) {
  engine.startStage(stageId);
  engine.completeStage(stageId, {
    passed: true,
    notes: `${stageId} passed`,
    evidence: [{ id: `${stageId}-ev`, label: stageId, value: "ok" }]
  });

  const stage = engine.snapshot().stages.find((candidate) => candidate.id === stageId);
  if (stage?.status === "needs-human") {
    engine.approveHumanGate(stageId, "Reviewer");
  }
}

describe("evaluateRun", () => {
  it("continues with the first ready stage", () => {
    const decision = evaluateRun(createSampleRun());

    expect(decision.action).toBe("continue");
    expect(decision.nextStageId).toBe("intake");
    expect(decision.nextAgentId).toBe("requirement");
  });

  it("asks humans when a gate is waiting", () => {
    const engine = new PipelineEngine(createSampleRun());
    engine.startStage("intake");
    const run = engine.completeStage("intake", {
      passed: true,
      notes: "Ready for approval.",
      evidence: [{ id: "ev-1", label: "Acceptance Criteria", value: "Ready" }]
    });

    const decision = evaluateRun(run);

    expect(decision.action).toBe("request-human");
    expect(decision.nextStageId).toBe("intake");
  });

  it("requests rework when a stage fails", () => {
    const engine = new PipelineEngine(createSampleRun());
    engine.startStage("intake");
    const run = engine.completeStage("intake", {
      passed: false,
      notes: "Acceptance criteria are vague.",
      evidence: [{ id: "ev-1", label: "Gap", value: "Missing target users" }]
    });

    const decision = evaluateRun(run);

    expect(decision.action).toBe("rework");
    expect(decision.nextAgentId).toBe("requirement");
  });

  it("succeeds only when all stages and required evidence are present", () => {
    const engine = new PipelineEngine(createSampleRun());

    for (const stageId of stageSequence) {
      passStage(engine, stageId);
    }

    const current = engine.snapshot();
    current.stages = current.stages.map((stage) => {
      if (stage.id === "testing") {
        return {
          ...stage,
          evidence: [...stage.evidence, { id: "coverage", label: "Coverage", value: "84%" }]
        };
      }
      if (stage.id === "delivery") {
        return {
          ...stage,
          evidence: [...stage.evidence, { id: "release", label: "Release Notes", value: "Ready" }]
        };
      }
      return stage;
    });

    const decision = evaluateRun(current);

    expect(decision.action).toBe("succeed");
  });
});
