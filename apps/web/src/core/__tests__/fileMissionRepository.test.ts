import { mkdtemp, readFile, rm } from "fs/promises";
import { join } from "path";
import { tmpdir } from "os";
import { describe, expect, it } from "vitest";
import {
  createMissionControlState,
  createSampleRun,
  createWorkItems
} from "..";
import { FileMissionRepository } from "../../local/fileMissionRepository";

describe("FileMissionRepository", () => {
  it("persists mission control state as JSON files", async () => {
    const root = await mkdtemp(join(tmpdir(), "omega-file-repo-"));
    const run = createSampleRun();
    const state = createMissionControlState(run, createWorkItems(run));
    const repo = new FileMissionRepository(root);

    try {
      await repo.saveState(state);

      expect(await repo.getState(run.id)).toEqual(state);
      const stored = JSON.parse(await readFile(join(root, `${run.id}.json`), "utf8"));
      expect(stored.runId).toBe(run.id);
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });

  it("appends events and updates stored sync intents", async () => {
    const root = await mkdtemp(join(tmpdir(), "omega-file-repo-"));
    const run = createSampleRun();
    const repo = new FileMissionRepository(root);

    try {
      await repo.saveState(createMissionControlState(run, createWorkItems(run)));
      const next = await repo.appendEvent(run.id, {
        type: "operation.started",
        missionId: "mission_1",
        workItemId: "item_intake",
        operationId: "operation_intake",
        operationTitle: "Intake",
        timestamp: "2026-04-22T00:00:00.000Z"
      });

      expect(next.events).toHaveLength(1);
      expect((await repo.getState(run.id))?.syncIntents).toHaveLength(2);
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });
});
