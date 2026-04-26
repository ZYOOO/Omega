import type { MissionEvent, SyncIntent } from "./missionEvents";

export type ActivityKind = "event" | "sync";

export interface ActivityItem {
  id: string;
  kind: ActivityKind;
  title: string;
  detail: string;
  timestamp?: string;
}

export interface ActivityFeedInput {
  events: MissionEvent[];
  syncIntents: SyncIntent[];
}

function eventTitle(event: MissionEvent): string {
  if (event.type === "operation.started") return `${event.operationTitle} started`;
  if (event.type === "operation.completed") return `${event.operationTitle} completed`;
  if (event.type === "operation.failed") return `${event.operationTitle} failed`;
  if (event.type === "operation.proof-attached") return `${event.operationTitle} proof attached`;
  return `${event.operationTitle} checkpoint requested`;
}

function syncTitle(intent: SyncIntent): string {
  if (intent.provider === "workboard" && intent.action === "update-status") return "Work item status updated";
  if (intent.provider === "workboard" && intent.action === "attach-proof") return "Proof attached to work item";
  if (intent.provider === "github" && intent.action === "comment") return "GitHub proof comment queued";
  if (intent.provider === "github" && intent.action === "annotate-branch") return "GitHub branch annotation queued";
  if (intent.provider === "feishu" && intent.action === "request-approval") return "Feishu approval requested";
  return `${intent.provider} ${intent.action}`;
}

export function createActivityFeed(input: ActivityFeedInput): ActivityItem[] {
  const eventItems: ActivityItem[] = input.events.map((event, index) => ({
    id: `event_${index}`,
    kind: "event",
    title: eventTitle(event),
    detail: event.operationId,
    timestamp: event.timestamp
  }));

  const syncItems: ActivityItem[] = input.syncIntents.map((intent, index) => ({
    id: `sync_${index}`,
    kind: "sync",
    title: syncTitle(intent),
    detail: intent.targetId
  }));

  return [...eventItems, ...syncItems];
}
