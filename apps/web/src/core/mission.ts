import type { WorkItem } from "./workboard";
import type { AgentId, PipelineRun, PipelineStage, StageStatus } from "./types";

export type MissionStatus = "ready" | "running" | "waiting-human" | "done" | "blocked";
export type LinkProvider = "workboard" | "github" | "ci" | "feishu";

export interface MissionLink {
  provider: LinkProvider;
  label: string;
  target: string;
}

export interface MissionOperation {
  id: string;
  stageId: PipelineStage["id"];
  agentId: AgentId;
  status: MissionStatus;
  prompt: string;
  requiredProof: string[];
}

export interface Mission {
  id: string;
  sourceIssueKey: string;
  sourceWorkItemId: string;
  title: string;
  target: string;
  repositoryTargetId?: string;
  status: MissionStatus;
  checkpointRequired: boolean;
  operations: MissionOperation[];
  links: MissionLink[];
}

function missionStatusFromStage(status: StageStatus): MissionStatus {
  if (status === "running") return "running";
  if (status === "needs-human") return "waiting-human";
  if (status === "passed") return "done";
  if (status === "failed" || status === "blocked") return "blocked";
  return "ready";
}

function buildOperationPrompt(run: PipelineRun, item: WorkItem, stage: PipelineStage): string {
  return [
    `Mission: ${run.goal}`,
    `Source work item: ${item.key}`,
    `Work item title: ${item.title}`,
    `Repository target ID: ${item.repositoryTargetId ?? "unscoped"}`,
    `Repository target: ${item.target || "No target"}`,
    `Stage: ${stage.title}`,
    `Agent: ${stage.ownerAgentId}`,
    "",
    "Acceptance criteria:",
    ...stage.acceptanceCriteria.map((criterion) => `- ${criterion}`),
    "",
    "Instructions:",
    "Work only inside the isolated workspace.",
    "Attach proof under .omega/proof before reporting completion.",
    "Do not mark the operation done without proof."
  ].join("\n");
}

export function createMissionFromRun(run: PipelineRun, item: WorkItem): Mission {
  const stage = run.stages.find((candidate) => candidate.id === item.stageId) ?? run.stages[0];

  return {
    id: `mission_${run.requirement.identifier}_${stage.id}`,
    sourceIssueKey: item.key,
    sourceWorkItemId: item.id,
    title: item.title,
    target: item.target,
    repositoryTargetId: item.repositoryTargetId,
    status: missionStatusFromStage(stage.status),
    checkpointRequired: stage.humanGate,
    operations: [
      {
        id: `operation_${stage.id}`,
        stageId: stage.id,
        agentId: stage.ownerAgentId,
        status: missionStatusFromStage(stage.status),
        prompt: buildOperationPrompt(run, item, stage),
        requiredProof: stage.acceptanceCriteria
      }
    ],
    links: [
      { provider: "workboard", label: "Source work item", target: item.key },
      { provider: "github", label: "Workspace branch", target: `omega/${run.requirement.identifier.toLowerCase()}` },
      { provider: "ci", label: "Required checks", target: "lint, coverage, build" }
    ]
  };
}
