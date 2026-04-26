import { describe, expect, it } from "vitest";
import { planSyncIntents } from "../missionEvents";

describe("planSyncIntents", () => {
  it("plans workboard and GitHub sync when an operation starts", () => {
    const intents = planSyncIntents({
      type: "operation.started",
      missionId: "mission_1",
      workItemId: "item_intake",
      operationId: "operation_intake",
      operationTitle: "Intake",
      timestamp: "2026-04-21T00:00:00.000Z"
    });

    expect(intents).toEqual([
      {
        provider: "workboard",
        action: "update-status",
        targetId: "item_intake",
        payload: { status: "In Review" }
      },
      {
        provider: "github",
        action: "annotate-branch",
        targetId: "mission_1",
        payload: { operationId: "operation_intake", status: "started" }
      }
    ]);
  });

  it("plans proof comments when proof is attached", () => {
    const intents = planSyncIntents({
      type: "operation.proof-attached",
      missionId: "mission_1",
      workItemId: "item_testing",
      operationId: "operation_testing",
      operationTitle: "Testing",
      proofFiles: [".omega/proof/coverage.txt"],
      summary: "Coverage passed.",
      timestamp: "2026-04-21T00:00:00.000Z"
    });

    expect(intents).toEqual([
      {
        provider: "workboard",
        action: "attach-proof",
        targetId: "item_testing",
        payload: {
          operationTitle: "Testing",
          proofFiles: [".omega/proof/coverage.txt"],
          summary: "Coverage passed."
        }
      },
      {
        provider: "github",
        action: "comment",
        targetId: "mission_1",
        payload: {
          body: "Mission Control proof for Testing\n\nCoverage passed.\n\nProof files:\n- .omega/proof/coverage.txt"
        }
      }
    ]);
  });

  it("plans Feishu approval when a checkpoint is requested", () => {
    const intents = planSyncIntents({
      type: "checkpoint.requested",
      missionId: "mission_1",
      workItemId: "item_review",
      operationId: "operation_review",
      operationTitle: "Review",
      checkpointReason: "Review needs human approval.",
      timestamp: "2026-04-21T00:00:00.000Z"
    });

    expect(intents).toEqual([
      {
        provider: "workboard",
        action: "update-status",
        targetId: "item_review",
        payload: { status: "Ready" }
      },
      {
        provider: "feishu",
        action: "request-approval",
        targetId: "mission_1",
        payload: {
          title: "Review checkpoint",
          reason: "Review needs human approval."
        }
      }
    ]);
  });
});
