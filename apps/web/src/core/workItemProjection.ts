import {
  createWorkItems,
  groupWorkItemsByStatus,
  updateWorkItemPriority,
  updateWorkItemStatus
} from "./workboard";
import type {
  WorkItem,
  WorkItemPriority,
  WorkItemStatus,
  WorkItemGroup
} from "./workboard";
import type { PipelineRun } from "./types";

export type ProjectedWorkItem = WorkItem;
export type ProjectedWorkItemStatus = WorkItemStatus;
export type ProjectedWorkItemPriority = WorkItemPriority;
export interface ProjectedWorkItemGroup {
  status: WorkItemStatus;
  items: WorkItem[];
}

export function createProjectedWorkItems(run: PipelineRun): ProjectedWorkItem[] {
  return createWorkItems(run);
}

export function groupProjectedWorkItemsByStatus(items: ProjectedWorkItem[]): ProjectedWorkItemGroup[] {
  return groupWorkItemsByStatus(items).map((group: WorkItemGroup) => ({
    status: group.status,
    items: group.items
  }));
}

export function updateProjectedWorkItemStatus(
  items: ProjectedWorkItem[],
  itemId: string,
  status: ProjectedWorkItemStatus
): ProjectedWorkItem[] {
  return updateWorkItemStatus(items, itemId, status);
}

export function updateProjectedWorkItemPriority(
  items: ProjectedWorkItem[],
  itemId: string,
  priority: ProjectedWorkItemPriority
): ProjectedWorkItem[] {
  return updateWorkItemPriority(items, itemId, priority);
}
