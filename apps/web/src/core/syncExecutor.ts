import type { SyncIntent } from "./missionEvents";
import { updateWorkItemStatus, type WorkItem, type WorkItemStatus } from "./workboard";

function unique(values: string[]): string[] {
  return [...new Set(values)];
}

export function applyWorkboardSyncIntent(items: WorkItem[], intent: SyncIntent): WorkItem[] {
  if (intent.provider !== "workboard") {
    return items;
  }

  if (intent.action === "update-status") {
    return updateWorkItemStatus(items, intent.targetId, intent.payload.status as WorkItemStatus);
  }

  if (intent.action === "attach-proof") {
    return items.map((item) =>
      item.id === intent.targetId
        ? { ...item, labels: unique([...item.labels, "proof-attached"]) }
        : item
    );
  }

  return items;
}
