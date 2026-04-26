import type { MasterDecision, PipelineRun, PipelineStage } from "./types";

function stageNeedsHuman(stage: PipelineStage): boolean {
  return stage.status === "needs-human";
}

function stageNeedsRework(stage: PipelineStage): boolean {
  return stage.status === "failed" || stage.status === "blocked";
}

export function evaluateRun(run: PipelineRun): MasterDecision {
  const humanStage = run.stages.find(stageNeedsHuman);
  if (humanStage) {
    return {
      action: "request-human",
      reason: `${humanStage.title} needs human validation before the pipeline can continue.`,
      nextStageId: humanStage.id,
      nextAgentId: "master"
    };
  }

  const failedStage = run.stages.find(stageNeedsRework);
  if (failedStage) {
    return {
      action: "rework",
      reason: `${failedStage.title} did not satisfy its acceptance criteria.`,
      nextStageId: failedStage.id,
      nextAgentId: failedStage.ownerAgentId
    };
  }

  const readyStage = run.stages.find((stage) => stage.status === "ready");
  if (readyStage) {
    return {
      action: "continue",
      reason: `${readyStage.title} is ready for ${readyStage.ownerAgentId}.`,
      nextStageId: readyStage.id,
      nextAgentId: readyStage.ownerAgentId
    };
  }

  const runningStage = run.stages.find((stage) => stage.status === "running");
  if (runningStage) {
    return {
      action: "continue",
      reason: `${runningStage.title} is still running.`,
      nextStageId: runningStage.id,
      nextAgentId: runningStage.ownerAgentId
    };
  }

  const allPassed = run.stages.every((stage) => stage.status === "passed");
  const hasTestingEvidence = run.stages
    .find((stage) => stage.id === "testing")
    ?.evidence.some((evidence) => evidence.label.toLowerCase().includes("coverage"));
  const hasDeliveryEvidence = run.stages
    .find((stage) => stage.id === "delivery")
    ?.evidence.some((evidence) => evidence.label.toLowerCase().includes("release"));

  if (allPassed && hasTestingEvidence && hasDeliveryEvidence) {
    return {
      action: "succeed",
      reason: "All success criteria are met with testing and delivery evidence.",
      nextAgentId: "master"
    };
  }

  return {
    action: "rework",
    reason: "All stages passed, but required evidence is incomplete.",
    nextAgentId: "master"
  };
}
