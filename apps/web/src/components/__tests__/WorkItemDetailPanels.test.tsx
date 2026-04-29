import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkItemAttemptPanel } from "../WorkItemDetailPanels";

describe("WorkItemAttemptPanel", () => {
  afterEach(() => cleanup());

  it("shows runner stderr as execution evidence instead of filtering environment lines", () => {
    render(
      <WorkItemAttemptPanel
        agentShortLabel={(agentId) => agentId}
        attempt={{
          id: "attempt_failed",
          itemId: "item_1",
          pipelineId: "pipeline_1",
          status: "failed",
          currentStageId: "code_review_round_1",
          failureReason: "Review agent failed before issuing a verdict."
        }}
        attemptStatusLabel={(status) => status}
        failedStages={[{ id: "code_review_round_1", title: "Code Review Round 1", status: "failed" }]}
        failureOperations={[{
          id: "operation_review",
          missionId: "mission_1",
          stageId: "code_review_round_1",
          agentId: "review",
          status: "failed",
          prompt: "Review the diff",
          requiredProof: ["review-report"],
          runnerProcess: {
            runner: "codex",
            command: "codex",
            status: "failed",
            stderr: "ERROR codex_core::skills::loader failed to stat skills entry /Users/demo/.agents/skills/superpowers"
          }
        }]}
        failureProofCards={[]}
        humanReviewArtifacts={[]}
        humanReviewEvents={[]}
        onApproveCheckpoint={vi.fn()}
        onRequestCheckpointChanges={vi.fn()}
        operationStatusLabel={(status) => status}
        pipelineStageClassName={(status) => status}
        pipelineStageLabel={(status) => status}
        displayText={(value) => value}
      />
    );

    expect(screen.getByText(/failed to stat skills entry/)).toBeInTheDocument();
  });
});
