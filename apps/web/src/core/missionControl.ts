import { evaluateRun } from "./masterAgent";
import type { AgentId, PipelineRun, PipelineStage, PipelineStageId, StageStatus } from "./types";

export type WorkspaceStatus = "idle" | "active" | "waiting-human" | "failed" | "completed";
export type RunnerJobStatus = "queued" | "running" | "waiting-human" | "done" | "failed";

export interface WorkspaceRef {
  id: string;
  label: string;
  repository: string;
  branch: string;
  path: string;
  status: WorkspaceStatus;
}

export interface RunnerJob {
  id: string;
  stageId: PipelineStageId;
  agentId: AgentId;
  title: string;
  status: RunnerJobStatus;
  requiredProof: string[];
  proofCount: number;
}

export interface MissionControlSnapshot {
  trackerIssue: string;
  workspace: WorkspaceRef;
  jobs: RunnerJob[];
  nextAction: string;
  proofOfWork: string[];
}

function workspaceStatusFromStages(stages: PipelineStage[]): WorkspaceStatus {
  if (stages.some((stage) => stage.status === "failed" || stage.status === "blocked")) {
    return "failed";
  }

  if (stages.some((stage) => stage.status === "needs-human")) {
    return "waiting-human";
  }

  if (stages.every((stage) => stage.status === "passed")) {
    return "completed";
  }

  if (stages.some((stage) => stage.status === "running")) {
    return "active";
  }

  return "idle";
}

function jobStatusFromStage(status: StageStatus): RunnerJobStatus {
  const mapping: Record<StageStatus, RunnerJobStatus> = {
    waiting: "queued",
    ready: "queued",
    running: "running",
    "needs-human": "waiting-human",
    passed: "done",
    failed: "failed",
    blocked: "failed"
  };

  return mapping[status];
}

function slugify(input: string): string {
  return input
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "");
}

export function createMissionControlSnapshot(run: PipelineRun): MissionControlSnapshot {
  const decision = evaluateRun(run);
  const branchSlug = slugify(run.requirement.identifier);
  const workspace: WorkspaceRef = {
    id: `ws_${run.requirement.id}`,
    label: `${run.requirement.identifier} workspace`,
    repository: "omega/product-delivery",
    branch: `omega/${branchSlug}`,
    path: `.omega/workspaces/${branchSlug}`,
    status: workspaceStatusFromStages(run.stages)
  };

  const jobs: RunnerJob[] = run.stages.map((stage, index) => ({
    id: `job_${index + 1}_${stage.id}`,
    stageId: stage.id,
    agentId: stage.ownerAgentId,
    title: stage.title,
    status: jobStatusFromStage(stage.status),
    requiredProof: stage.acceptanceCriteria,
    proofCount: stage.evidence.length
  }));

  const proofOfWork = run.stages.flatMap((stage) =>
    stage.evidence.map((evidence) => `${stage.title} / ${evidence.label}: ${evidence.value}`)
  );

  return {
    trackerIssue: run.requirement.identifier,
    workspace,
    jobs,
    nextAction: decision.reason,
    proofOfWork
  };
}
