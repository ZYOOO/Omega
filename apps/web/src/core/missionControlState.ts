import type { PipelineRun } from "./types";
import type { MissionEvent, SyncIntent } from "./missionEvents";
import { planSyncIntents } from "./missionEvents";
import { applyWorkboardSyncIntent } from "./syncExecutor";
import type { WorkItem } from "./workboard";

export interface MissionControlState {
  runId: string;
  workItems: WorkItem[];
  events: MissionEvent[];
  syncIntents: SyncIntent[];
}

export function createMissionControlState(
  run: PipelineRun,
  workItems: WorkItem[]
): MissionControlState {
  return {
    runId: run.id,
    workItems,
    events: [],
    syncIntents: []
  };
}

export function applyMissionControlEvents(
  state: MissionControlState,
  events: MissionEvent[]
): MissionControlState {
  return events.reduce<MissionControlState>((current, event) => {
    const intents = planSyncIntents(event);
    const workItems = intents.reduce(
      (items, intent) => applyWorkboardSyncIntent(items, intent),
      current.workItems
    );

    return {
      ...current,
      workItems,
      events: [...current.events, event],
      syncIntents: [...current.syncIntents, ...intents]
    };
  }, state);
}
