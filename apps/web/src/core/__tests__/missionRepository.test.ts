import { describe, expect, it } from "vitest";
import {
  InMemoryMissionRepository,
  createMissionControlState,
  createSampleRun,
  createWorkItems
} from "..";

describe("InMemoryMissionRepository", () => {
  it("saves and loads mission control state by run id", async () => {
    const run = createSampleRun();
    const state = createMissionControlState(run, createWorkItems(run));
    const repo = new InMemoryMissionRepository();

    await repo.saveState(state);

    expect(await repo.getState(run.id)).toEqual(state);
  });

  it("appends events and sync intents without mutating the stored state object", async () => {
    const run = createSampleRun();
    const state = createMissionControlState(run, createWorkItems(run));
    const repo = new InMemoryMissionRepository();
    await repo.saveState(state);

    await repo.appendEvent(run.id, {
      type: "operation.started",
      missionId: "mission_1",
      workItemId: "item_intake",
      operationId: "operation_intake",
      operationTitle: "Intake",
      timestamp: "2026-04-22T00:00:00.000Z"
    });

    const next = await repo.getState(run.id);
    expect(next?.events).toHaveLength(1);
    expect(state.events).toHaveLength(0);
  });
});
