import type { PipelineRun, PipelineStage } from "./types";

export type WorkItemStatus = "Planning" | "Ready" | "In Review" | "Human Review" | "Backlog" | "Blocked" | "Done";
export type WorkItemPriority = "No priority" | "Low" | "Medium" | "High" | "Urgent";
export type WorkboardProjectStatus = "Active" | "Paused" | "Completed";
export type WorkItemSource = "manual" | "github_issue" | "feishu_message" | "ai_generated" | "page_pilot";

export type RepositoryTarget =
  | {
      id: string;
      kind: "github";
      owner: string;
      repo: string;
      defaultBranch: string;
      url?: string;
    }
  | {
      id: string;
      kind: "local";
      path: string;
      defaultBranch: string;
    };

export interface WorkboardProject {
  id: string;
  name: string;
  description: string;
  team: string;
  status: WorkboardProjectStatus;
  labels: string[];
  repositoryTargets: RepositoryTarget[];
  defaultRepositoryTargetId?: string;
}

export interface WorkItem {
  id: string;
  key: string;
  title: string;
  description: string;
  status: WorkItemStatus;
  priority: WorkItemPriority;
  assignee: string;
  labels: string[];
  team: string;
  stageId: PipelineStage["id"];
  target: string;
  source: WorkItemSource;
  requirementId?: string;
  sourceExternalRef?: string;
  repositoryTargetId?: string;
  branchName?: string;
  acceptanceCriteria: string[];
  parentItemId?: string;
  blockedByItemIds: string[];
}

export interface WorkItemGroup {
  status: WorkItemStatus;
  items: WorkItem[];
}

const statusOrder: WorkItemStatus[] = ["Planning", "Ready", "In Review", "Human Review", "Backlog", "Blocked", "Done"];

function workItemStatusFromStage(stage: PipelineStage): WorkItemStatus {
  if (stage.status === "ready" || stage.status === "running" || stage.status === "needs-human") {
    return "Ready";
  }

  if (stage.status === "passed") {
    return "Done";
  }

  if (stage.status === "failed" || stage.status === "blocked") {
    return "Blocked";
  }

  return "Backlog";
}

function workItemPriorityFromRun(run: PipelineRun): WorkItemPriority {
  if (run.requirement.priority === "urgent") return "Urgent";
  if (run.requirement.priority === "high") return "High";
  if (run.requirement.priority === "medium") return "Medium";
  if (run.requirement.priority === "low") return "Low";
  return "No priority";
}

export function createWorkboardProject(run: PipelineRun): WorkboardProject {
  return {
    id: `project_${run.requirement.id}`,
    name: run.requirement.identifier,
    description: run.requirement.description,
    team: "Omega",
    status: "Active",
    labels: run.requirement.labels,
    repositoryTargets: []
  };
}

export function createWorkItems(run: PipelineRun): WorkItem[] {
  return run.stages.map((stage, index) => ({
    id: `item_${stage.id}`,
    key: `OMG-${index + 1}`,
    title: stage.title,
    description: stage.description,
    status: workItemStatusFromStage(stage),
    priority: workItemPriorityFromRun(run),
    assignee: stage.ownerAgentId,
    labels: stage.humanGate ? ["human-gate", "ai-delivery"] : ["automation", "ai-delivery"],
    team: "Omega",
    stageId: stage.id,
    target: index < 3 ? "Apr 22" : "Apr 23",
    source: "ai_generated",
    acceptanceCriteria: stage.acceptanceCriteria,
    blockedByItemIds: []
  }));
}

export function groupWorkItemsByStatus(items: WorkItem[]): WorkItemGroup[] {
  return statusOrder
    .map((status) => ({
      status,
      items: items.filter((item) => item.status === status)
    }))
    .filter((group) => group.items.length > 0);
}

export function updateWorkItemStatus(
  items: WorkItem[],
  itemId: string,
  status: WorkItemStatus
): WorkItem[] {
  return items.map((item) => (item.id === itemId ? { ...item, status } : item));
}

export function updateWorkItemPriority(
  items: WorkItem[],
  itemId: string,
  priority: WorkItemPriority
): WorkItem[] {
  return items.map((item) => (item.id === itemId ? { ...item, priority } : item));
}
