import { describe, expect, it } from "vitest";
import { createSampleRun, createMissionControlSnapshot, PipelineEngine } from "..";

describe("createMissionControlSnapshot", () => {
  it("projects pipeline stages into Mission Control runner jobs", () => {
    const snapshot = createMissionControlSnapshot(createSampleRun());

    expect(snapshot.workspace.branch).toBe("omega/omega-1");
    expect(snapshot.workspace.status).toBe("idle");
    expect(snapshot.jobs).toHaveLength(6);
    expect(snapshot.jobs.map((job) => job.stageId)).toEqual([
      "intake",
      "solution",
      "coding",
      "testing",
      "review",
      "delivery"
    ]);
  });

  it("marks the isolated workspace as waiting for humans at gates", () => {
    const engine = new PipelineEngine(createSampleRun());

    engine.startStage("intake");
    const run = engine.completeStage("intake", {
      passed: true,
      notes: "Ready for approval.",
      evidence: [{ id: "ev-1", label: "Acceptance Criteria", value: "Ready" }]
    });

    const snapshot = createMissionControlSnapshot(run);

    expect(snapshot.workspace.status).toBe("waiting-human");
    expect(snapshot.jobs.find((job) => job.stageId === "intake")?.status).toBe("waiting-human");
    expect(snapshot.proofOfWork).toContain("Intake / Acceptance Criteria: Ready");
  });
});
