import { describe, expect, it } from "vitest";
import { retryReasonForAttempt } from "../attemptRetryReason";

describe("retryReasonForAttempt", () => {
  it("carries review feedback into retry requests", () => {
    const reason = retryReasonForAttempt({
      id: "attempt_1",
      itemId: "item_1",
      pipelineId: "pipeline_1",
      status: "failed",
      failureReason: "Rework agent failed while applying review feedback.",
      failureStageId: "rework",
      failureAgentId: "coding",
      failureReviewFeedback: "# Review\n\nVerdict: CHANGES_REQUESTED\n\nThe save button still does not persist user data."
    });

    expect(reason).toContain("Failure reason: Rework agent failed while applying review feedback.");
    expect(reason).toContain("stage=rework");
    expect(reason).toContain("agent=coding");
    expect(reason).toContain("The save button still does not persist user data.");
  });
});
