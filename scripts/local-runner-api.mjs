import { createServer } from "http";
import { mkdir, readFile, readdir, writeFile } from "fs/promises";
import { dirname, join, resolve } from "path";
import { tmpdir } from "os";
import { spawn } from "child_process";

function argValue(name, fallback) {
  const index = process.argv.indexOf(name);
  return index >= 0 ? process.argv[index + 1] : fallback;
}

function safeSegment(input) {
  return String(input).replace(/[^a-zA-Z0-9._-]+/g, "-");
}

function readBody(request) {
  return new Promise((resolveBody) => {
    let body = "";
    request.on("data", (chunk) => {
      body += chunk.toString();
    });
    request.on("end", () => resolveBody(body));
  });
}

function setCors(response) {
  response.setHeader("access-control-allow-origin", "*");
  response.setHeader("access-control-allow-methods", "GET,POST,PUT,PATCH,OPTIONS");
  response.setHeader("access-control-allow-headers", "content-type");
}

function sendJson(response, statusCode, body) {
  response.statusCode = statusCode;
  setCors(response);
  response.setHeader("content-type", "application/json");
  response.end(JSON.stringify(body));
}

function nowIso() {
  return new Date().toISOString();
}

function createRequirementFromWorkItem(item) {
  return {
    id: `req_${item.id}`,
    identifier: item.key,
    title: item.title,
    description: item.description,
    source: "manual",
    priority: item.priority === "Urgent" || item.priority === "High" ? "high" : item.priority === "Medium" ? "medium" : "low",
    requester: item.assignee,
    labels: item.labels,
    createdAt: nowIso()
  };
}

function createPipelineRecordFromWorkItem(item) {
  const runId = `run_${item.id}`;
  const createdAt = nowIso();
  return {
    id: `pipeline_${item.id}`,
    workItemId: item.id,
    runId,
    status: "draft",
    run: {
      id: runId,
      requirement: createRequirementFromWorkItem(item),
      goal: `Deliver ${item.key}: ${item.title}`,
      successCriteria: [
        "All pipeline stages are passed",
        "All human gates are approved",
        "Testing and review evidence is attached",
        "Delivery notes and rollback plan are attached"
      ],
      stages: [
        { id: "intake", title: "Intake", description: "Clarify problem and acceptance criteria.", ownerAgentId: "requirement", status: "ready", humanGate: true, acceptanceCriteria: ["Scope is clear"], evidence: [] },
        { id: "solution", title: "Solution", description: "Design implementation plan.", ownerAgentId: "architect", status: "waiting", humanGate: true, acceptanceCriteria: ["Plan is explicit"], evidence: [] },
        { id: "coding", title: "Implementation", description: "Modify code in workspace.", ownerAgentId: "coding", status: "waiting", humanGate: false, acceptanceCriteria: ["Diff is reviewable"], evidence: [] },
        { id: "testing", title: "Testing", description: "Generate and run tests.", ownerAgentId: "testing", status: "waiting", humanGate: true, acceptanceCriteria: ["Tests cover acceptance criteria"], evidence: [] },
        { id: "review", title: "Review", description: "Review code and test outcome.", ownerAgentId: "review", status: "waiting", humanGate: true, acceptanceCriteria: ["No blocking review comments"], evidence: [] },
        { id: "delivery", title: "Delivery", description: "Prepare final delivery summary.", ownerAgentId: "delivery", status: "waiting", humanGate: true, acceptanceCriteria: ["Delivery note is complete"], evidence: [] }
      ],
      agents: [],
      selectedCapabilities: {},
      events: [{ id: `event_${runId}_1`, type: "run.created", message: `Pipeline created for ${item.key}`, timestamp: createdAt, stageId: "intake", agentId: "master" }],
      createdAt,
      updatedAt: createdAt
    },
    createdAt,
    updatedAt: createdAt
  };
}

function appendPipelineToDatabase(database, pipeline) {
  return {
    ...database,
    savedAt: nowIso(),
    tables: {
      ...database.tables,
      pipelines: [...(database.tables.pipelines ?? []), pipeline]
    }
  };
}

function updatePipelineInDatabase(database, pipelineId, updater) {
  return {
    ...database,
    savedAt: nowIso(),
    tables: {
      ...database.tables,
      pipelines: (database.tables.pipelines ?? []).map((pipeline) => pipeline.id === pipelineId ? updater(pipeline) : pipeline)
    }
  };
}

function upsertCheckpointInDatabase(database, checkpoint) {
  const checkpoints = database.tables.checkpoints ?? [];
  const exists = checkpoints.some((candidate) => candidate.id === checkpoint.id);
  return {
    ...database,
    savedAt: nowIso(),
    tables: {
      ...database.tables,
      checkpoints: exists
        ? checkpoints.map((candidate) => candidate.id === checkpoint.id ? checkpoint : candidate)
        : [...checkpoints, checkpoint]
    }
  };
}

function pendingCheckpointFromPipeline(pipeline) {
  const stage = pipeline.run.stages.find((candidate) => candidate.status === "needs-human");
  if (!stage) return undefined;
  const timestamp = nowIso();
  return {
    id: `${pipeline.id}:${stage.id}`,
    pipelineId: pipeline.id,
    stageId: stage.id,
    status: "pending",
    title: `${stage.title} 审批`,
    summary: `${stage.title} 需要人工确认后才能继续`,
    createdAt: timestamp,
    updatedAt: timestamp
  };
}

function replacePipelineRun(database, pipelineId, nextRun, nextStatus, checkpointDecision) {
  let nextDatabase = updatePipelineInDatabase(database, pipelineId, (pipeline) => ({
    ...pipeline,
    run: nextRun,
    status: nextStatus,
    updatedAt: nowIso()
  }));
  const pipeline = nextDatabase.tables.pipelines.find((candidate) => candidate.id === pipelineId);
  const pending = pipeline ? pendingCheckpointFromPipeline(pipeline) : undefined;
  if (pending) {
    nextDatabase = upsertCheckpointInDatabase(nextDatabase, pending);
  }
  if (checkpointDecision?.stageId) {
    const existing = (nextDatabase.tables.checkpoints ?? []).find((checkpoint) =>
      checkpoint.pipelineId === pipelineId && checkpoint.stageId === checkpointDecision.stageId
    );
    if (existing) {
      nextDatabase = upsertCheckpointInDatabase(nextDatabase, {
        ...existing,
        status: checkpointDecision.status,
        decisionNote: checkpointDecision.note,
        updatedAt: nowIso()
      });
    }
  }
  return nextDatabase;
}

function appendWorkItemToDatabase(database, item) {
  const timestamp = nowIso();
  const projectId = database.tables.projects[0]?.id ?? "project_unknown";
  return {
    ...database,
    savedAt: timestamp,
    tables: {
      ...database.tables,
      workItems: [...database.tables.workItems, { ...item, projectId, createdAt: timestamp, updatedAt: timestamp }],
      missionControlStates: database.tables.missionControlStates.length
        ? database.tables.missionControlStates.map((state, index) => index === 0
          ? { ...state, workItems: [...state.workItems, item], updatedAt: timestamp }
          : state)
        : database.tables.missionControlStates
    }
  };
}

function updateWorkItemInDatabase(database, itemId, patch) {
  const timestamp = nowIso();
  return {
    ...database,
    savedAt: timestamp,
    tables: {
      ...database.tables,
      workItems: database.tables.workItems.map((item) => item.id === itemId ? { ...item, ...patch, updatedAt: timestamp } : item),
      missionControlStates: database.tables.missionControlStates.map((state) => ({
        ...state,
        workItems: state.workItems.map((item) => item.id === itemId ? { ...item, ...patch } : item),
        updatedAt: timestamp
      }))
    }
  };
}

function workboardStatusFromEventType(type) {
  if (type === "operation.started") return "In Review";
  if (type === "checkpoint.requested") return "Ready";
  if (type === "operation.completed") return "Done";
  if (type === "operation.failed") return "Blocked";
  return null;
}

function syncIntentsFromEvents(events) {
  return events.flatMap((event) => {
    if (event.type === "operation.started") {
      return [{ provider: "workboard", action: "update-status", targetId: event.workItemId, payload: { status: "In Review" } }];
    }
    if (event.type === "operation.completed") {
      return [{ provider: "workboard", action: "update-status", targetId: event.workItemId, payload: { status: "Done" } }];
    }
    if (event.type === "operation.failed") {
      return [{ provider: "workboard", action: "update-status", targetId: event.workItemId, payload: { status: "Blocked" } }];
    }
    if (event.type === "checkpoint.requested") {
      return [{ provider: "workboard", action: "update-status", targetId: event.workItemId, payload: { status: "Ready" } }];
    }
    return [];
  });
}

function applyMissionEventsToDatabase(database, events) {
  const current = database.tables.missionControlStates[0];
  if (!current) return database;

  const timestamp = nowIso();
  let workItems = current.workItems;
  for (const event of events) {
    const nextStatus = workboardStatusFromEventType(event.type);
    if (nextStatus) {
      workItems = workItems.map((item) => item.id === event.workItemId ? { ...item, status: nextStatus } : item);
    }
  }

  const nextEvents = [...current.events, ...events];
  const nextSyncIntents = [...current.syncIntents, ...syncIntentsFromEvents(events)];

  return {
    ...database,
    savedAt: timestamp,
    tables: {
      ...database.tables,
      workItems: database.tables.workItems.map((record) => {
        const next = workItems.find((item) => item.id === record.id);
        return next ? { ...record, ...next, updatedAt: timestamp } : record;
      }),
      missionControlStates: [{
        ...current,
        workItems,
        events: nextEvents,
        syncIntents: nextSyncIntents,
        updatedAt: timestamp
      }],
      missionEvents: nextEvents.map((event, index) => ({ id: `${current.runId}:event:${index + 1}`, runId: current.runId, sequence: index + 1, event })),
      syncIntents: nextSyncIntents.map((intent, index) => ({ id: `${current.runId}:sync:${index + 1}`, runId: current.runId, sequence: index + 1, intent }))
    }
  };
}

function sqlQuote(value) {
  return `'${String(value).replace(/'/g, "''")}'`;
}

function runSqlite(databasePath, sql) {
  return new Promise((resolveRun, rejectRun) => {
    const child = spawn("sqlite3", [databasePath], { shell: false });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => {
      stdout += chunk.toString();
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString();
    });
    child.on("close", (exitCode) => {
      if (exitCode === 0) resolveRun(stdout);
      else rejectRun(new Error(stderr || `sqlite3 exited with ${exitCode}`));
    });
    child.on("error", rejectRun);
    child.stdin.write(sql);
    child.stdin.end();
  });
}

async function initializeDatabase(databasePath) {
  await mkdir(dirname(databasePath), { recursive: true });
  await runSqlite(databasePath, `
    PRAGMA journal_mode = WAL;
    CREATE TABLE IF NOT EXISTS workspace_snapshots (id TEXT PRIMARY KEY, database_json TEXT NOT NULL, saved_at TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS projects (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL, team TEXT NOT NULL, status TEXT NOT NULL, labels_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS work_items (id TEXT PRIMARY KEY, project_id TEXT NOT NULL, key TEXT NOT NULL, title TEXT NOT NULL, description TEXT NOT NULL, status TEXT NOT NULL, priority TEXT NOT NULL, assignee TEXT NOT NULL, labels_json TEXT NOT NULL, team TEXT NOT NULL, stage_id TEXT NOT NULL, target TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS mission_control_states (run_id TEXT PRIMARY KEY, project_id TEXT NOT NULL, work_items_json TEXT NOT NULL, events_json TEXT NOT NULL, sync_intents_json TEXT NOT NULL, updated_at TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS mission_events (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_json TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS sync_intents (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, sequence INTEGER NOT NULL, intent_json TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS connections (provider_id TEXT PRIMARY KEY, status TEXT NOT NULL, granted_permissions_json TEXT NOT NULL, connected_as TEXT, updated_at TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS ui_preferences (id TEXT PRIMARY KEY, active_nav TEXT NOT NULL, selected_provider_id TEXT NOT NULL, selected_work_item_id TEXT NOT NULL, inspector_open INTEGER NOT NULL, active_inspector_panel TEXT NOT NULL, runner_preset TEXT NOT NULL, status_filter TEXT NOT NULL, assignee_filter TEXT NOT NULL, sort_direction TEXT NOT NULL, collapsed_groups_json TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS pipelines (id TEXT PRIMARY KEY, work_item_id TEXT NOT NULL, run_id TEXT NOT NULL, status TEXT NOT NULL, run_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS checkpoints (id TEXT PRIMARY KEY, pipeline_id TEXT NOT NULL, stage_id TEXT NOT NULL, status TEXT NOT NULL, title TEXT NOT NULL, summary TEXT NOT NULL, decision_note TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
  `);
}

async function saveWorkspaceDatabase(databasePath, database) {
  await initializeDatabase(databasePath);
  const rows = [
    "BEGIN;",
    "DELETE FROM workspace_snapshots;",
    "DELETE FROM sync_intents;",
    "DELETE FROM mission_events;",
    "DELETE FROM mission_control_states;",
    "DELETE FROM work_items;",
    "DELETE FROM projects;",
    "DELETE FROM connections;",
    "DELETE FROM ui_preferences;",
    "DELETE FROM checkpoints;",
    "DELETE FROM pipelines;",
    `INSERT INTO workspace_snapshots (id, database_json, saved_at) VALUES ('default', ${sqlQuote(JSON.stringify(database))}, ${sqlQuote(database.savedAt ?? new Date().toISOString())});`,
    ...database.tables.projects.map((project) =>
      `INSERT INTO projects VALUES (${sqlQuote(project.id)}, ${sqlQuote(project.name)}, ${sqlQuote(project.description)}, ${sqlQuote(project.team)}, ${sqlQuote(project.status)}, ${sqlQuote(JSON.stringify(project.labels))}, ${sqlQuote(project.createdAt)}, ${sqlQuote(project.updatedAt)});`
    ),
    ...database.tables.workItems.map((item) =>
      `INSERT INTO work_items VALUES (${sqlQuote(item.id)}, ${sqlQuote(item.projectId)}, ${sqlQuote(item.key)}, ${sqlQuote(item.title)}, ${sqlQuote(item.description)}, ${sqlQuote(item.status)}, ${sqlQuote(item.priority)}, ${sqlQuote(item.assignee)}, ${sqlQuote(JSON.stringify(item.labels))}, ${sqlQuote(item.team)}, ${sqlQuote(item.stageId)}, ${sqlQuote(item.target)}, ${sqlQuote(item.createdAt)}, ${sqlQuote(item.updatedAt)});`
    ),
    ...database.tables.missionControlStates.map((state) =>
      `INSERT INTO mission_control_states VALUES (${sqlQuote(state.runId)}, ${sqlQuote(state.projectId)}, ${sqlQuote(JSON.stringify(state.workItems))}, ${sqlQuote(JSON.stringify(state.events))}, ${sqlQuote(JSON.stringify(state.syncIntents))}, ${sqlQuote(state.updatedAt)});`
    ),
    ...database.tables.missionEvents.map((record) =>
      `INSERT INTO mission_events VALUES (${sqlQuote(record.id)}, ${sqlQuote(record.runId)}, ${record.sequence}, ${sqlQuote(JSON.stringify(record.event))});`
    ),
    ...database.tables.syncIntents.map((record) =>
      `INSERT INTO sync_intents VALUES (${sqlQuote(record.id)}, ${sqlQuote(record.runId)}, ${record.sequence}, ${sqlQuote(JSON.stringify(record.intent))});`
    ),
    ...database.tables.connections.map((connection) =>
      `INSERT INTO connections VALUES (${sqlQuote(connection.providerId)}, ${sqlQuote(connection.status)}, ${sqlQuote(JSON.stringify(connection.grantedPermissions))}, ${connection.connectedAs ? sqlQuote(connection.connectedAs) : "NULL"}, ${sqlQuote(connection.updatedAt)});`
    ),
    ...database.tables.uiPreferences.map((ui) =>
      `INSERT INTO ui_preferences VALUES (${sqlQuote(ui.id)}, ${sqlQuote(ui.activeNav)}, ${sqlQuote(ui.selectedProviderId)}, ${sqlQuote(ui.selectedWorkItemId)}, ${ui.inspectorOpen ? 1 : 0}, ${sqlQuote(ui.activeInspectorPanel)}, ${sqlQuote(ui.runnerPreset)}, ${sqlQuote(ui.statusFilter)}, ${sqlQuote(ui.assigneeFilter)}, ${sqlQuote(ui.sortDirection)}, ${sqlQuote(JSON.stringify(ui.collapsedGroups))});`
    ),
    ...(database.tables.pipelines ?? []).map((pipeline) =>
      `INSERT INTO pipelines VALUES (${sqlQuote(pipeline.id)}, ${sqlQuote(pipeline.workItemId)}, ${sqlQuote(pipeline.runId)}, ${sqlQuote(pipeline.status)}, ${sqlQuote(JSON.stringify(pipeline.run))}, ${sqlQuote(pipeline.createdAt)}, ${sqlQuote(pipeline.updatedAt)});`
    ),
    ...(database.tables.checkpoints ?? []).map((checkpoint) =>
      `INSERT INTO checkpoints VALUES (${sqlQuote(checkpoint.id)}, ${sqlQuote(checkpoint.pipelineId)}, ${sqlQuote(checkpoint.stageId)}, ${sqlQuote(checkpoint.status)}, ${sqlQuote(checkpoint.title)}, ${sqlQuote(checkpoint.summary)}, ${checkpoint.decisionNote ? sqlQuote(checkpoint.decisionNote) : "NULL"}, ${sqlQuote(checkpoint.createdAt)}, ${sqlQuote(checkpoint.updatedAt)});`
    ),
    "COMMIT;"
  ];
  await runSqlite(databasePath, rows.join("\n"));
}

async function loadWorkspaceDatabase(databasePath) {
  await initializeDatabase(databasePath);
  const output = await runSqlite(databasePath, "SELECT database_json FROM workspace_snapshots WHERE id = 'default';");
  const raw = output.trim();
  return raw ? JSON.parse(raw) : undefined;
}

async function collectFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(
    entries.map(async (entry) => {
      const fullPath = join(directory, entry.name);
      return entry.isDirectory() ? collectFiles(fullPath) : [fullPath];
    })
  );
  return nested.flat();
}

function runCommand(command, cwd) {
  return new Promise((resolveRun) => {
    const child = spawn(command.executable, command.args, { cwd, shell: false });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => {
      stdout += chunk.toString();
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString();
    });
    if (command.stdinFile) {
      readFile(join(cwd, command.stdinFile), "utf8")
        .then((content) => {
          child.stdin.write(content);
          child.stdin.end();
        })
        .catch((error) => {
          stderr += error.message;
          child.stdin.end();
        });
    } else {
      child.stdin.end();
    }
    child.on("close", (exitCode) => resolveRun({ exitCode, stdout, stderr }));
  });
}

function commandFromPreset(runner) {
  if (runner === "local-proof") {
    return {
      executable: process.execPath,
      args: [
        "-e",
        "const fs=require('fs'); fs.writeFileSync('.omega/proof/local-proof.txt','local proof'); console.log('local proof complete')"
      ]
    };
  }

  if (runner === "codex") {
    return {
      executable: "codex",
      args: [
        "--ask-for-approval",
        "never",
        "exec",
        "--model",
        "gpt-5.4-mini",
        "-c",
        "model_reasoning_effort=\"medium\"",
        "--skip-git-repo-check",
        "--sandbox",
        "workspace-write",
        "--output-last-message",
        ".omega/proof/codex-last-message.txt",
        "-"
      ],
      stdinFile: ".omega/prompt.md"
    };
  }

  throw new Error(`Unknown runner preset: ${runner}`);
}

async function runLocalProof(workspaceRoot, mission, operationId) {
  const operation = mission.operations.find((candidate) => candidate.id === operationId);
  if (!operation) {
    throw new Error(`Unknown operation: ${operationId}`);
  }

  const workspace = join(
    workspaceRoot,
    `${safeSegment(mission.sourceIssueKey)}-${safeSegment(operation.stageId)}`
  );
  const omega = join(workspace, ".omega");
  const proof = join(omega, "proof");
  await mkdir(proof, { recursive: true });
  await writeFile(join(omega, "job.json"), JSON.stringify({ missionId: mission.id, operation }, null, 2));
  await writeFile(join(omega, "prompt.md"), operation.prompt);

  const command = commandFromPreset(mission.runner ?? "local-proof");
  const result = await runCommand(command, workspace);

  return {
    operationId,
    status: result.exitCode === 0 ? "passed" : "failed",
    workspacePath: workspace,
    proofFiles: await collectFiles(proof),
    stdout: result.stdout,
    stderr: result.stderr,
    events: [
      {
        type: "operation.started",
        missionId: mission.id,
        workItemId: mission.sourceWorkItemId,
        operationId,
        operationTitle: mission.title,
        timestamp: new Date().toISOString()
      },
      {
        type: "operation.proof-attached",
        missionId: mission.id,
        workItemId: mission.sourceWorkItemId,
        operationId,
        operationTitle: mission.title,
        proofFiles: await collectFiles(proof),
        summary: result.exitCode === 0 ? "Proof collected." : "Runner failed before proof completed.",
        timestamp: new Date().toISOString()
      },
      {
        type: result.exitCode === 0 ? "operation.completed" : "operation.failed",
        missionId: mission.id,
        workItemId: mission.sourceWorkItemId,
        operationId,
        operationTitle: mission.title,
        timestamp: new Date().toISOString()
      }
    ]
  };
}

const workspaceRoot = resolve(argValue("--workspace-root", join(tmpdir(), "omega-mission-control-api")));
const databasePath = resolve(argValue("--database", join(process.cwd(), ".omega", "omega.db")));
const port = Number(argValue("--port", "3888"));
const host = argValue("--host", "127.0.0.1");

const server = createServer(async (request, response) => {
  if (request.method === "OPTIONS") {
    response.statusCode = 204;
    setCors(response);
    response.end("");
    return;
  }

  if (request.method === "GET" && request.url === "/health") {
    sendJson(response, 200, { ok: true, persistence: "sqlite", databasePath });
    return;
  }

  if (request.method === "GET" && request.url === "/openapi.yaml") {
    try {
      const body = await readFile(resolve(process.cwd(), "docs", "openapi.yaml"), "utf8");
      response.statusCode = 200;
      setCors(response);
      response.setHeader("content-type", "application/yaml; charset=utf-8");
      response.end(body);
    } catch (error) {
      sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
    }
    return;
  }

  if (request.method === "GET" && request.url === "/workspace") {
    try {
      const database = await loadWorkspaceDatabase(databasePath);
      if (!database) {
        sendJson(response, 404, { error: "workspace not found" });
        return;
      }
      sendJson(response, 200, database);
    } catch (error) {
      sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
    }
    return;
  }

  if (request.method === "PUT" && request.url === "/workspace") {
    try {
      await saveWorkspaceDatabase(databasePath, JSON.parse(await readBody(request)));
      sendJson(response, 200, { ok: true });
    } catch (error) {
      sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
    }
    return;
  }

  if (request.method === "GET" && request.url === "/events") {
    try {
      const database = await loadWorkspaceDatabase(databasePath);
      sendJson(response, 200, database?.tables.missionEvents ?? []);
    } catch (error) {
      sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
    }
    return;
  }

  if (request.method === "POST" && request.url === "/work-items") {
    try {
      const database = await loadWorkspaceDatabase(databasePath);
      if (!database) {
        sendJson(response, 404, { error: "workspace not found" });
        return;
      }
      const payload = JSON.parse(await readBody(request));
      const next = appendWorkItemToDatabase(database, payload.item);
      await saveWorkspaceDatabase(databasePath, next);
      sendJson(response, 200, next);
    } catch (error) {
      sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
    }
    return;
  }

  if (request.method === "PATCH" && request.url?.startsWith("/work-items/")) {
    try {
      const itemId = request.url.split("/")[2];
      const database = await loadWorkspaceDatabase(databasePath);
      if (!database) {
        sendJson(response, 404, { error: "workspace not found" });
        return;
      }
      const patch = JSON.parse(await readBody(request));
      const next = updateWorkItemInDatabase(database, itemId, patch);
      await saveWorkspaceDatabase(databasePath, next);
      sendJson(response, 200, next);
    } catch (error) {
      sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
    }
    return;
  }

  if (request.method === "POST" && request.url === "/missions/from-work-item") {
    try {
      const payload = JSON.parse(await readBody(request));
      const item = payload.item;
      const mission = {
        id: `mission_${item.key}_${item.stageId}`,
        sourceIssueKey: item.key,
        sourceWorkItemId: item.id,
        title: item.title,
        status: item.status === "Done" ? "done" : item.status === "Blocked" ? "blocked" : item.status === "In Review" ? "running" : "ready",
        checkpointRequired: item.stageId !== "coding",
        operations: [{
          id: `operation_${item.stageId}`,
          stageId: item.stageId,
          agentId: item.assignee,
          status: item.status === "Done" ? "done" : item.status === "Blocked" ? "blocked" : item.status === "In Review" ? "running" : "ready",
          prompt: `Mission: ${item.title}\nSource work item: ${item.key}\nStage: ${item.stageId}\nAgent: ${item.assignee}`,
          requiredProof: ["proof"]
        }],
        links: []
      };
      sendJson(response, 200, mission);
    } catch (error) {
      sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
    }
    return;
  }

  if (request.method === "POST" && request.url === "/operations/run") {
    try {
      const payload = JSON.parse(await readBody(request));
      const mission = { ...payload.mission, runner: payload.runner ?? "local-proof" };
      const result = await runLocalProof(workspaceRoot, mission, payload.operationId);
      const database = await loadWorkspaceDatabase(databasePath);
      if (database) {
        const next = applyMissionEventsToDatabase(database, result.events);
        await saveWorkspaceDatabase(databasePath, next);
      }
      sendJson(response, 200, result);
    } catch (error) {
      sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
    }
    return;
  }

  if (request.method === "POST" && request.url === "/run-operation") {
    try {
      const payload = JSON.parse(await readBody(request));
      const mission = { ...payload.mission, runner: payload.runner ?? "local-proof" };
      sendJson(response, 200, await runLocalProof(workspaceRoot, mission, payload.operationId));
    } catch (error) {
      sendJson(response, 500, { error: error instanceof Error ? error.message : "unknown error" });
    }
    return;
  }

  sendJson(response, 404, { error: "not found" });
});

server.listen(port, host, () => {
  const address = server.address();
  const actualPort = typeof address === "object" && address ? address.port : port;
  console.log(`Mission Control API listening: http://${host}:${actualPort}`);
  console.log(`Omega SQLite database: ${databasePath}`);
});
