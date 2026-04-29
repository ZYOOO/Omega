import type { AttemptRecordInfo } from "./omegaControlApiClient";

export function retryReasonForAttempt(attempt?: AttemptRecordInfo): string {
  const parts = ["Retry requested from Work Item detail."];
  if (!attempt) return parts[0];
  const reason = attempt.failureReason ?? attempt.errorMessage ?? attempt.statusReason;
  if (reason) parts.push(`Failure reason: ${reason}`);
  if (attempt.failureStageId || attempt.failureAgentId) {
    const stage = attempt.failureStageId ? `stage=${attempt.failureStageId}` : "";
    const agent = attempt.failureAgentId ? `agent=${attempt.failureAgentId}` : "";
    parts.push(["Failure location:", stage, agent].filter(Boolean).join(" "));
  }
  const feedback = attempt.failureReviewFeedback?.trim();
  if (feedback) {
    parts.push(`Review feedback:\n${feedback.slice(0, 1600)}`);
  } else if (attempt.failureDetail?.trim()) {
    parts.push(`Failure detail:\n${attempt.failureDetail.trim().slice(0, 1200)}`);
  } else if (attempt.stderrSummary?.trim()) {
    parts.push(`Runner stderr:\n${attempt.stderrSummary.trim().slice(0, 800)}`);
  }
  return parts.join("\n\n");
}
