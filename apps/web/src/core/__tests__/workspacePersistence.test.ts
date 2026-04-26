import { describe, expect, it } from "vitest";
import {
  createInitialWorkspaceSession,
  createSampleRun,
  databaseFromWorkspaceSession,
  loadWorkspaceSession,
  saveWorkspaceSession,
  updateWorkItemStatus,
  workspaceSessionFromDatabase
} from "..";

function createMemoryStorage(): Storage {
  const values = new Map<string, string>();
  return {
    get length() {
      return values.size;
    },
    clear: () => values.clear(),
    getItem: (key: string) => values.get(key) ?? null,
    key: (index: number) => [...values.keys()][index] ?? null,
    removeItem: (key: string) => {
      values.delete(key);
    },
    setItem: (key: string, value: string) => {
      values.set(key, value);
    }
  };
}

describe("workspace persistence", () => {
  it("uses a stable sample run id so the same workspace can be restored", () => {
    expect(createSampleRun().id).toBe(createSampleRun().id);
  });

  it("serializes workspace state into table-shaped records", () => {
    const run = createSampleRun();
    const session = createInitialWorkspaceSession(run);
    const database = databaseFromWorkspaceSession(run, {
      ...session,
      workItems: [
        {
          id: "item_manual_1",
          key: "OMG-1",
          title: "Persist work item",
          description: "Keep it after refresh.",
          status: "Ready",
          priority: "High",
          assignee: "requirement",
          labels: ["manual"],
          team: "Omega",
          stageId: "intake",
          target: "No target",
          source: "manual",
          acceptanceCriteria: ["The item survives refresh."],
          blockedByItemIds: []
        }
      ],
      selectedWorkItemId: "item_manual_1"
    });

    expect(database.tables.projects).toHaveLength(1);
    expect(database.tables.projects[0]).toMatchObject({
      repositoryTargets: []
    });
    expect(database.tables.workItems[0]).toMatchObject({
      projectId: "project_req_omega_001",
      id: "item_manual_1",
      title: "Persist work item",
      source: "manual",
      acceptanceCriteria: ["The item survives refresh."],
      blockedByItemIds: []
    });
    expect(database.tables.uiPreferences[0]).toMatchObject({
      selectedWorkItemId: "item_manual_1",
      activeNav: "Issues"
    });
  });

  it("round-trips session state through storage", () => {
    const run = createSampleRun();
    const storage = createMemoryStorage();
    const session = createInitialWorkspaceSession(run);
    const workItems = [
      {
        id: "item_manual_1",
        key: "OMG-1",
        title: "Persist work item",
        description: "Keep it after refresh.",
        status: "Ready" as const,
        priority: "High" as const,
        assignee: "requirement",
        labels: ["manual"],
        team: "Omega",
        stageId: "intake" as const,
        target: "No target",
        source: "manual" as const,
        acceptanceCriteria: ["The item survives refresh."],
        blockedByItemIds: []
      }
    ];

    saveWorkspaceSession(
      run,
      {
        ...session,
        workItems: updateWorkItemStatus(workItems, "item_manual_1", "In Review"),
        selectedWorkItemId: "item_manual_1",
        inspectorOpen: false,
        activeInspectorPanel: "provider"
      },
      storage
    );

    const restored = loadWorkspaceSession(run, storage);

    expect(restored.workItems[0]).toMatchObject({
      id: "item_manual_1",
      status: "In Review"
    });
    expect(restored.selectedWorkItemId).toBe("item_manual_1");
    expect(restored.inspectorOpen).toBe(false);
    expect(restored.activeInspectorPanel).toBe("provider");
  });

  it("uses persisted work items as the mission control workboard projection", () => {
    const run = createSampleRun();
    const session = createInitialWorkspaceSession(run);
    const database = databaseFromWorkspaceSession(run, {
      ...session,
      workItems: [
        {
          id: "item_manual_1",
          key: "OMG-1",
          title: "Persist work item",
          description: "Keep it after refresh.",
          status: "Done",
          priority: "High",
          assignee: "requirement",
          labels: ["manual"],
          team: "Omega",
          stageId: "intake",
          target: "No target",
          source: "manual",
          acceptanceCriteria: ["The item survives refresh."],
          blockedByItemIds: []
        }
      ]
    });

    const restored = workspaceSessionFromDatabase(run, database);

    expect(restored.missionState.workItems).toEqual(restored.workItems);
  });
});
