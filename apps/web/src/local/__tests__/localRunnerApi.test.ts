import { mkdtemp, rm } from "fs/promises";
import { join } from "path";
import { tmpdir } from "os";
import { describe, expect, it } from "vitest";
import {
  createInitialWorkspaceSession,
  createMissionFromRun,
  createSampleRun,
  createWorkItems,
  databaseFromWorkspaceSession
} from "../../core";
import { commandFromRunnerPreset, startLocalRunnerApi } from "../localRunnerApi";
import { SqliteWorkspaceRepository } from "../sqliteWorkspaceRepository";

describe("startLocalRunnerApi", () => {
  it("creates a Codex command from the codex runner preset", () => {
    const command = commandFromRunnerPreset("codex");

    expect(command.executable).toBe("codex");
    expect(command.args).toEqual(
      expect.arrayContaining([
        "--ask-for-approval",
        "never",
        "exec",
        "--sandbox",
        "workspace-write",
        "--output-last-message",
        ".omega/proof/codex-last-message.txt",
        "-"
      ])
    );
    expect(command.stdinFile).toBe(".omega/prompt.md");
  });

  it("serves POST /run-operation and returns runner proof", async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), "omega-api-"));
    const run = createSampleRun();
    const mission = createMissionFromRun(run, createWorkItems(run)[0]);
    const api = await startLocalRunnerApi({ workspaceRoot, port: 0 });

    try {
      const response = await fetch(`${api.url}/run-operation`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          mission,
          operationId: "operation_intake",
          command: {
            executable: process.execPath,
            args: [
              "-e",
              "const fs=require('fs'); fs.writeFileSync('.omega/proof/api.txt','api proof'); console.log('api complete')"
            ]
          }
        })
      });

      expect(response.status).toBe(200);
      const body = await response.json();
      expect(body.status).toBe("passed");
      expect(body.stdout).toContain("api complete");
      expect(body.proofFiles).toEqual(expect.arrayContaining([expect.stringContaining("api.txt")]));
    } finally {
      await api.close();
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });

  it("persists and returns a workspace database through SQLite", async () => {
    const root = await mkdtemp(join(tmpdir(), "omega-api-db-"));
    const workspaceRoot = join(root, "workspace");
    const databasePath = join(root, "omega.db");
    const run = createSampleRun();
    const session = createInitialWorkspaceSession(run);
    const database = databaseFromWorkspaceSession(run, {
      ...session,
      workItems: [
        {
          id: "item_manual_1",
          key: "OMG-1",
          title: "Server persisted item",
          description: "Stored by Local Server.",
          status: "Ready",
          priority: "High",
          assignee: "requirement",
          labels: ["manual"],
          team: "Omega",
          stageId: "intake",
          target: "No target",
          source: "manual",
          acceptanceCriteria: ["The item survives SQLite persistence."],
          blockedByItemIds: []
        }
      ]
    });
    const api = await startLocalRunnerApi({ workspaceRoot, databasePath, port: 0 });

    try {
      const saveResponse = await fetch(`${api.url}/workspace`, {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(database)
      });
      expect(saveResponse.status).toBe(200);

      const loadResponse = await fetch(`${api.url}/workspace`);
      expect(loadResponse.status).toBe(200);
      const body = await loadResponse.json();
      expect(body.tables.workItems[0].title).toBe("Server persisted item");

      const repo = new SqliteWorkspaceRepository(databasePath);
      const stored = await repo.getDatabase();
      expect(stored?.tables.workItems[0].title).toBe("Server persisted item");
    } finally {
      await api.close();
      await rm(root, { recursive: true, force: true });
    }
  });

  it("creates and patches work items through API endpoints", async () => {
    const root = await mkdtemp(join(tmpdir(), "omega-api-items-"));
    const workspaceRoot = join(root, "workspace");
    const databasePath = join(root, "omega.db");
    const run = createSampleRun();
    const session = createInitialWorkspaceSession(run);
    const database = databaseFromWorkspaceSession(run, session);
    const api = await startLocalRunnerApi({ workspaceRoot, databasePath, port: 0 });

    try {
      await fetch(`${api.url}/workspace`, {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(database)
      });

      const createResponse = await fetch(`${api.url}/work-items`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          item: {
            id: "item_manual_1",
            key: "OMG-1",
            title: "API created item",
            description: "Created through REST endpoint.",
            status: "Ready",
            priority: "High",
            assignee: "requirement",
            labels: ["manual"],
            team: "Omega",
            stageId: "intake",
            target: "No target"
          }
        })
      });
      expect(createResponse.status).toBe(200);

      const patchResponse = await fetch(`${api.url}/work-items/item_manual_1`, {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ status: "Done", priority: "Urgent" })
      });
      expect(patchResponse.status).toBe(200);
      const patched = await patchResponse.json();
      expect(patched.tables.workItems[0]).toMatchObject({
        title: "API created item",
        status: "Done",
        priority: "Urgent"
      });
    } finally {
      await api.close();
      await rm(root, { recursive: true, force: true });
    }
  });

  it("runs an operation through the workspace API and persists events", async () => {
    const root = await mkdtemp(join(tmpdir(), "omega-api-run-"));
    const workspaceRoot = join(root, "workspace");
    const databasePath = join(root, "omega.db");
    const run = createSampleRun();
    const workItem = createWorkItems(run)[0];
    const mission = createMissionFromRun(run, workItem);
    const database = databaseFromWorkspaceSession(run, {
      ...createInitialWorkspaceSession(run),
      workItems: [workItem],
      missionState: {
        runId: run.id,
        workItems: [workItem],
        events: [],
        syncIntents: []
      }
    });
    const api = await startLocalRunnerApi({ workspaceRoot, databasePath, port: 0 });

    try {
      await fetch(`${api.url}/workspace`, {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(database)
      });

      const response = await fetch(`${api.url}/operations/run`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          mission,
          operationId: "operation_intake",
          runner: "local-proof"
        })
      });

      expect(response.status).toBe(200);
      const missionBuildResponse = await fetch(`${api.url}/missions/from-work-item`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ item: workItem })
      });
      expect(missionBuildResponse.status).toBe(200);

      const eventsResponse = await fetch(`${api.url}/events`);
      const events = await eventsResponse.json();
      expect(events.map((event: { event: { type: string } }) => event.event.type)).toEqual([
        "operation.started",
        "operation.proof-attached",
        "operation.completed"
      ]);

      const workspace = await fetch(`${api.url}/workspace`).then((response) => response.json());
      expect(workspace.tables.missions).toHaveLength(1);
      expect(workspace.tables.operations).toEqual(
        expect.arrayContaining([
          expect.objectContaining({ id: "operation_intake", missionId: mission.id, status: "done" })
        ])
      );
      expect(workspace.tables.proofRecords).toEqual(
        expect.arrayContaining([
          expect.objectContaining({ operationId: "operation_intake", label: "proof-file" })
        ])
      );
    } finally {
      await api.close();
      await rm(root, { recursive: true, force: true });
    }
  });

  it("creates a pipeline from a work item and supports lifecycle transitions", async () => {
    const root = await mkdtemp(join(tmpdir(), "omega-api-pipeline-"));
    const workspaceRoot = join(root, "workspace");
    const databasePath = join(root, "omega.db");
    const run = createSampleRun();
    const workItem = {
      id: "item_manual_1",
      key: "OMG-1",
      title: "Pipeline item",
      description: "Needs a pipeline.",
      status: "Ready" as const,
      priority: "High" as const,
      assignee: "requirement",
      labels: ["manual"],
      team: "Omega",
      stageId: "intake" as const,
      target: "No target",
      source: "manual" as const,
      acceptanceCriteria: ["The pipeline can be started."],
      blockedByItemIds: []
    };
    const database = databaseFromWorkspaceSession(run, {
      ...createInitialWorkspaceSession(run),
      workItems: [workItem],
      missionState: {
        runId: run.id,
        workItems: [workItem],
        events: [],
        syncIntents: []
      }
    });
    const api = await startLocalRunnerApi({ workspaceRoot, databasePath, port: 0 });

    try {
      await fetch(`${api.url}/workspace`, {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(database)
      });

      const created = await fetch(`${api.url}/pipelines/from-work-item`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ item: workItem })
      }).then((response) => response.json());

      expect(created.status).toBe("draft");

      const started = await fetch(`${api.url}/pipelines/${created.id}/start`, {
        method: "POST"
      }).then((response) => response.json());
      expect(started.status).toBe("running");

      const paused = await fetch(`${api.url}/pipelines/${created.id}/pause`, {
        method: "POST"
      }).then((response) => response.json());
      expect(paused.status).toBe("paused");

      const resumed = await fetch(`${api.url}/pipelines/${created.id}/resume`, {
        method: "POST"
      }).then((response) => response.json());
      expect(resumed.status).toBe("running");

      const terminated = await fetch(`${api.url}/pipelines/${created.id}/terminate`, {
        method: "POST"
      }).then((response) => response.json());
      expect(terminated.status).toBe("terminated");
    } finally {
      await api.close();
      await rm(root, { recursive: true, force: true });
    }
  });

  it("supports checkpoint approve and request-changes actions", async () => {
    const root = await mkdtemp(join(tmpdir(), "omega-api-checkpoint-"));
    const workspaceRoot = join(root, "workspace");
    const databasePath = join(root, "omega.db");
    const run = createSampleRun();
    const workItem = {
      id: "item_manual_1",
      key: "OMG-1",
      title: "Checkpoint item",
      description: "Needs approval.",
      status: "Ready" as const,
      priority: "High" as const,
      assignee: "requirement",
      labels: ["manual"],
      team: "Omega",
      stageId: "intake" as const,
      target: "No target",
      source: "manual" as const,
      acceptanceCriteria: ["The checkpoint can be approved or rejected."],
      blockedByItemIds: []
    };
    const database = databaseFromWorkspaceSession(run, {
      ...createInitialWorkspaceSession(run),
      workItems: [workItem],
      missionState: {
        runId: run.id,
        workItems: [workItem],
        events: [],
        syncIntents: []
      }
    });
    const api = await startLocalRunnerApi({ workspaceRoot, databasePath, port: 0 });

    try {
      await fetch(`${api.url}/workspace`, {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(database)
      });

      const pipeline = await fetch(`${api.url}/pipelines/from-work-item`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ item: workItem })
      }).then((response) => response.json());

      await fetch(`${api.url}/pipelines/${pipeline.id}/start`, { method: "POST" });
      await fetch(`${api.url}/pipelines/${pipeline.id}/complete-stage`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ passed: true, notes: "Ready for approval" })
      });

      let checkpoints = await fetch(`${api.url}/checkpoints`).then((response) => response.json());
      expect(checkpoints).toHaveLength(1);

      const approved = await fetch(`${api.url}/checkpoints/${checkpoints[0].id}/approve`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ reviewer: "alice" })
      }).then((response) => response.json());
      expect(approved.status).toBe("approved");

      const pipeline2 = await fetch(`${api.url}/pipelines/from-work-item`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ item: { ...workItem, id: "item_manual_2", key: "OMG-2" } })
      }).then((response) => response.json());

      await fetch(`${api.url}/pipelines/${pipeline2.id}/start`, { method: "POST" });
      await fetch(`${api.url}/pipelines/${pipeline2.id}/complete-stage`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ passed: true, notes: "Needs changes" })
      });

      checkpoints = await fetch(`${api.url}/checkpoints`).then((response) => response.json());
      const pendingForSecond = checkpoints.find((checkpoint: { pipelineId: string; status: string }) =>
        checkpoint.pipelineId === pipeline2.id && checkpoint.status === "pending"
      );
      const rejected = await fetch(`${api.url}/checkpoints/${pendingForSecond.id}/request-changes`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ reason: "Need more detail" })
      }).then((response) => response.json());
      expect(rejected.status).toBe("rejected");
    } finally {
      await api.close();
      await rm(root, { recursive: true, force: true });
    }
  });

  it("supports a safe local-proof runner preset without accepting arbitrary browser commands", async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), "omega-api-"));
    const run = createSampleRun();
    const mission = createMissionFromRun(run, createWorkItems(run)[0]);
    const api = await startLocalRunnerApi({ workspaceRoot, port: 0 });

    try {
      const response = await fetch(`${api.url}/run-operation`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          mission,
          operationId: "operation_intake",
          runner: "local-proof"
        })
      });

      expect(response.status).toBe(200);
      const body = await response.json();
      expect(body.status).toBe("passed");
      expect(body.proofFiles).toEqual(expect.arrayContaining([expect.stringContaining("local-proof.txt")]));
    } finally {
      await api.close();
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });

  it("returns 404 for unknown routes", async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), "omega-api-"));
    const api = await startLocalRunnerApi({ workspaceRoot, port: 0 });

    try {
      const response = await fetch(`${api.url}/missing`);
      expect(response.status).toBe(404);
    } finally {
      await api.close();
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });

  it("serves the OpenAPI document", async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), "omega-api-"));
    const api = await startLocalRunnerApi({ workspaceRoot, port: 0 });

    try {
      const response = await fetch(`${api.url}/openapi.yaml`);
      expect(response.status).toBe(200);
      expect(await response.text()).toContain("openapi: 3.1.0");
    } finally {
      await api.close();
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });

  it("responds to CORS preflight requests", async () => {
    const workspaceRoot = await mkdtemp(join(tmpdir(), "omega-api-"));
    const api = await startLocalRunnerApi({ workspaceRoot, port: 0 });

    try {
      const response = await fetch(`${api.url}/run-operation`, { method: "OPTIONS" });
      expect(response.status).toBe(204);
      expect(response.headers.get("access-control-allow-origin")).toBe("*");
    } finally {
      await api.close();
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });
});
