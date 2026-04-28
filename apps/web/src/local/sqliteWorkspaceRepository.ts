import { spawn } from "child_process";
import { mkdir } from "fs/promises";
import { dirname } from "path";
import type { WorkspaceDatabase } from "../core";

interface SqliteResult {
  stdout: string;
  stderr: string;
  exitCode: number | null;
}

function sqlQuote(value: string): string {
  return `'${value.replace(/'/g, "''")}'`;
}

function json(value: unknown): string {
  return JSON.stringify(value);
}

function runSqlite(databasePath: string, input: string): Promise<SqliteResult> {
  return new Promise((resolve) => {
    const child = spawn("sqlite3", [databasePath], { shell: false });
    let stdout = "";
    let stderr = "";
    if (!child.stdout || !child.stderr || !child.stdin) {
      resolve({ stdout, stderr: "sqlite3 stdio streams are unavailable", exitCode: null });
      return;
    }

    child.stdout.on("data", (chunk) => {
      stdout += chunk.toString();
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString();
    });
    child.on("close", (exitCode) => resolve({ stdout, stderr, exitCode }));
    child.on("error", (error) => resolve({ stdout, stderr: error.message, exitCode: null }));

    child.stdin.write(input);
    child.stdin.end();
  });
}

async function execSql(databasePath: string, sql: string): Promise<string> {
  await mkdir(dirname(databasePath), { recursive: true });
  const result = await runSqlite(databasePath, sql);
  if (result.exitCode !== 0) {
    throw new Error(result.stderr || `sqlite3 exited with ${result.exitCode}`);
  }
  return result.stdout;
}

async function tableColumns(databasePath: string, tableName: string): Promise<string[]> {
  const output = await execSql(databasePath, `.mode json\nPRAGMA table_info(${tableName});`);
  if (!output.trim()) {
    return [];
  }
  const rows = JSON.parse(output) as Array<{ name: string }>;
  return rows.map((row) => row.name);
}

async function addColumnIfMissing(databasePath: string, tableName: string, columnName: string, definition: string): Promise<void> {
  const columns = await tableColumns(databasePath, tableName);
  if (!columns.includes(columnName)) {
    await execSql(databasePath, `ALTER TABLE ${tableName} ADD COLUMN ${columnName} ${definition};`);
  }
}

const schemaSql = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS metadata (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL,
  team TEXT NOT NULL,
  status TEXT NOT NULL,
  labels_json TEXT NOT NULL,
  repository_targets_json TEXT NOT NULL,
  default_repository_target_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS requirements (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  repository_target_id TEXT,
  source TEXT NOT NULL,
  source_external_ref TEXT,
  title TEXT NOT NULL,
  raw_text TEXT NOT NULL,
  structured_json TEXT NOT NULL,
  acceptance_criteria_json TEXT NOT NULL,
  risks_json TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS work_items (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  key TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  status TEXT NOT NULL,
  priority TEXT NOT NULL,
  assignee TEXT NOT NULL,
  labels_json TEXT NOT NULL,
  team TEXT NOT NULL,
  stage_id TEXT NOT NULL,
  target TEXT NOT NULL,
  source TEXT NOT NULL,
  requirement_id TEXT,
  source_external_ref TEXT,
  repository_target_id TEXT,
  branch_name TEXT,
  acceptance_criteria_json TEXT NOT NULL,
  parent_item_id TEXT,
  blocked_by_item_ids_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS mission_control_states (
  run_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  work_items_json TEXT NOT NULL,
  events_json TEXT NOT NULL,
  sync_intents_json TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS mission_events (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  sequence INTEGER NOT NULL,
  event_json TEXT NOT NULL,
  FOREIGN KEY (run_id) REFERENCES mission_control_states(run_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS sync_intents (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  sequence INTEGER NOT NULL,
  intent_json TEXT NOT NULL,
  FOREIGN KEY (run_id) REFERENCES mission_control_states(run_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS connections (
  provider_id TEXT PRIMARY KEY,
  status TEXT NOT NULL,
  granted_permissions_json TEXT NOT NULL,
  connected_as TEXT,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS ui_preferences (
  id TEXT PRIMARY KEY,
  active_nav TEXT NOT NULL,
  selected_provider_id TEXT NOT NULL,
  selected_work_item_id TEXT NOT NULL,
  inspector_open INTEGER NOT NULL,
  active_inspector_panel TEXT NOT NULL,
  runner_preset TEXT NOT NULL,
  status_filter TEXT NOT NULL,
  assignee_filter TEXT NOT NULL,
  sort_direction TEXT NOT NULL,
  collapsed_groups_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS pipelines (
  id TEXT PRIMARY KEY,
  work_item_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  status TEXT NOT NULL,
  run_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS checkpoints (
  id TEXT PRIMARY KEY,
  pipeline_id TEXT NOT NULL,
  stage_id TEXT NOT NULL,
  status TEXT NOT NULL,
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  decision_note TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS missions (
  id TEXT PRIMARY KEY,
  pipeline_id TEXT NOT NULL,
  work_item_id TEXT NOT NULL,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  mission_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS operations (
  id TEXT PRIMARY KEY,
  mission_id TEXT NOT NULL,
  stage_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  status TEXT NOT NULL,
  prompt TEXT NOT NULL,
  required_proof_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS proof_records (
  id TEXT PRIMARY KEY,
  operation_id TEXT NOT NULL,
  label TEXT NOT NULL,
  value TEXT NOT NULL,
  source_path TEXT,
  created_at TEXT NOT NULL
);
`;

export class SqliteWorkspaceRepository {
  private initialization?: Promise<void>;

  constructor(private readonly databasePath: string) {}

  async initialize(): Promise<void> {
    this.initialization ??= this.initializeOnce();
    return this.initialization;
  }

  private async initializeOnce(): Promise<void> {
    await execSql(this.databasePath, schemaSql);
    await addColumnIfMissing(this.databasePath, "projects", "repository_targets_json", "TEXT NOT NULL DEFAULT '[]'");
    await addColumnIfMissing(this.databasePath, "projects", "default_repository_target_id", "TEXT");
    await addColumnIfMissing(this.databasePath, "work_items", "source", "TEXT NOT NULL DEFAULT 'manual'");
    await addColumnIfMissing(this.databasePath, "work_items", "requirement_id", "TEXT");
    await addColumnIfMissing(this.databasePath, "work_items", "source_external_ref", "TEXT");
    await addColumnIfMissing(this.databasePath, "work_items", "repository_target_id", "TEXT");
    await addColumnIfMissing(this.databasePath, "work_items", "branch_name", "TEXT");
    await addColumnIfMissing(this.databasePath, "work_items", "acceptance_criteria_json", "TEXT NOT NULL DEFAULT '[]'");
    await addColumnIfMissing(this.databasePath, "work_items", "parent_item_id", "TEXT");
    await addColumnIfMissing(this.databasePath, "work_items", "blocked_by_item_ids_json", "TEXT NOT NULL DEFAULT '[]'");
  }

  async saveDatabase(database: WorkspaceDatabase): Promise<void> {
    await this.initialize();
    const sql = [
      "BEGIN;",
      "DELETE FROM sync_intents;",
      "DELETE FROM mission_events;",
      "DELETE FROM mission_control_states;",
      "DELETE FROM work_items;",
      "DELETE FROM requirements;",
      "DELETE FROM projects;",
      "DELETE FROM connections;",
      "DELETE FROM ui_preferences;",
      "DELETE FROM checkpoints;",
      "DELETE FROM pipelines;",
      "DELETE FROM proof_records;",
      "DELETE FROM operations;",
      "DELETE FROM missions;",
      "DELETE FROM metadata;",
      `INSERT INTO metadata (key, value) VALUES ('schema_version', ${sqlQuote(String(database.schemaVersion))});`,
      `INSERT INTO metadata (key, value) VALUES ('saved_at', ${sqlQuote(database.savedAt)});`,
      ...database.tables.projects.map(
        (project) => `
          INSERT INTO projects (
            id, name, description, team, status, labels_json,
            repository_targets_json, default_repository_target_id, created_at, updated_at
          ) VALUES (
            ${sqlQuote(project.id)}, ${sqlQuote(project.name)}, ${sqlQuote(project.description)},
            ${sqlQuote(project.team)}, ${sqlQuote(project.status)}, ${sqlQuote(json(project.labels))},
            ${sqlQuote(json(project.repositoryTargets))},
            ${project.defaultRepositoryTargetId ? sqlQuote(project.defaultRepositoryTargetId) : "NULL"},
            ${sqlQuote(project.createdAt)}, ${sqlQuote(project.updatedAt)}
          );`
      ),
      ...database.tables.requirements.map(
        (requirement) => `
          INSERT INTO requirements (
            id, project_id, repository_target_id, source, source_external_ref, title,
            raw_text, structured_json, acceptance_criteria_json, risks_json, status,
            created_at, updated_at
          ) VALUES (
            ${sqlQuote(requirement.id)}, ${sqlQuote(requirement.projectId)},
            ${requirement.repositoryTargetId ? sqlQuote(requirement.repositoryTargetId) : "NULL"},
            ${sqlQuote(requirement.source)},
            ${requirement.sourceExternalRef ? sqlQuote(requirement.sourceExternalRef) : "NULL"},
            ${sqlQuote(requirement.title)}, ${sqlQuote(requirement.rawText)},
            ${sqlQuote(json(requirement.structured ?? {}))},
            ${sqlQuote(json(requirement.acceptanceCriteria))},
            ${sqlQuote(json(requirement.risks))},
            ${sqlQuote(requirement.status)}, ${sqlQuote(requirement.createdAt)}, ${sqlQuote(requirement.updatedAt)}
          );`
      ),
      ...database.tables.workItems.map(
        (item) => `
          INSERT INTO work_items (
            id, project_id, key, title, description, status, priority, assignee,
            labels_json, team, stage_id, target, source, requirement_id, source_external_ref,
            repository_target_id, branch_name, acceptance_criteria_json, parent_item_id,
            blocked_by_item_ids_json, created_at, updated_at
          ) VALUES (
            ${sqlQuote(item.id)}, ${sqlQuote(item.projectId)}, ${sqlQuote(item.key)},
            ${sqlQuote(item.title)}, ${sqlQuote(item.description)}, ${sqlQuote(item.status)},
            ${sqlQuote(item.priority)}, ${sqlQuote(item.assignee)}, ${sqlQuote(json(item.labels))},
            ${sqlQuote(item.team)}, ${sqlQuote(item.stageId)}, ${sqlQuote(item.target)},
            ${sqlQuote(item.source)}, ${item.requirementId ? sqlQuote(item.requirementId) : "NULL"},
            ${item.sourceExternalRef ? sqlQuote(item.sourceExternalRef) : "NULL"},
            ${item.repositoryTargetId ? sqlQuote(item.repositoryTargetId) : "NULL"},
            ${item.branchName ? sqlQuote(item.branchName) : "NULL"},
            ${sqlQuote(json(item.acceptanceCriteria))},
            ${item.parentItemId ? sqlQuote(item.parentItemId) : "NULL"},
            ${sqlQuote(json(item.blockedByItemIds))},
            ${sqlQuote(item.createdAt)}, ${sqlQuote(item.updatedAt)}
          );`
      ),
      ...database.tables.missionControlStates.map(
        (state) => `
          INSERT INTO mission_control_states (
            run_id, project_id, work_items_json, events_json, sync_intents_json, updated_at
          ) VALUES (
            ${sqlQuote(state.runId)}, ${sqlQuote(state.projectId)}, ${sqlQuote(json(state.workItems))},
            ${sqlQuote(json(state.events))}, ${sqlQuote(json(state.syncIntents))}, ${sqlQuote(state.updatedAt)}
          );`
      ),
      ...database.tables.missionEvents.map(
        (record) => `
          INSERT INTO mission_events (id, run_id, sequence, event_json)
          VALUES (${sqlQuote(record.id)}, ${sqlQuote(record.runId)}, ${record.sequence}, ${sqlQuote(json(record.event))});`
      ),
      ...database.tables.syncIntents.map(
        (record) => `
          INSERT INTO sync_intents (id, run_id, sequence, intent_json)
          VALUES (${sqlQuote(record.id)}, ${sqlQuote(record.runId)}, ${record.sequence}, ${sqlQuote(json(record.intent))});`
      ),
      ...database.tables.connections.map(
        (connection) => `
          INSERT INTO connections (provider_id, status, granted_permissions_json, connected_as, updated_at)
          VALUES (
            ${sqlQuote(connection.providerId)}, ${sqlQuote(connection.status)}, ${sqlQuote(json(connection.grantedPermissions))},
            ${connection.connectedAs ? sqlQuote(connection.connectedAs) : "NULL"}, ${sqlQuote(connection.updatedAt)}
          );`
      ),
      ...database.tables.uiPreferences.map(
        (ui) => `
          INSERT INTO ui_preferences (
            id, active_nav, selected_provider_id, selected_work_item_id, inspector_open,
            active_inspector_panel, runner_preset, status_filter, assignee_filter,
            sort_direction, collapsed_groups_json
          ) VALUES (
            ${sqlQuote(ui.id)}, ${sqlQuote(ui.activeNav)}, ${sqlQuote(ui.selectedProviderId)},
            ${sqlQuote(ui.selectedWorkItemId)}, ${ui.inspectorOpen ? 1 : 0},
            ${sqlQuote(ui.activeInspectorPanel)}, ${sqlQuote(ui.runnerPreset)},
            ${sqlQuote(ui.statusFilter)}, ${sqlQuote(ui.assigneeFilter)}, ${sqlQuote(ui.sortDirection)},
            ${sqlQuote(json(ui.collapsedGroups))}
          );`
      ),
      ...(database.tables.pipelines ?? []).map(
        (pipeline) => `
          INSERT INTO pipelines (id, work_item_id, run_id, status, run_json, created_at, updated_at)
          VALUES (
            ${sqlQuote(pipeline.id)}, ${sqlQuote(pipeline.workItemId)}, ${sqlQuote(pipeline.runId)},
            ${sqlQuote(pipeline.status)}, ${sqlQuote(json(pipeline.run))},
            ${sqlQuote(pipeline.createdAt)}, ${sqlQuote(pipeline.updatedAt)}
          );`
      ),
      ...(database.tables.checkpoints ?? []).map(
        (checkpoint) => `
          INSERT INTO checkpoints (id, pipeline_id, stage_id, status, title, summary, decision_note, created_at, updated_at)
          VALUES (
            ${sqlQuote(checkpoint.id)}, ${sqlQuote(checkpoint.pipelineId)}, ${sqlQuote(checkpoint.stageId)},
            ${sqlQuote(checkpoint.status)}, ${sqlQuote(checkpoint.title)}, ${sqlQuote(checkpoint.summary)},
            ${checkpoint.decisionNote ? sqlQuote(checkpoint.decisionNote) : "NULL"},
            ${sqlQuote(checkpoint.createdAt)}, ${sqlQuote(checkpoint.updatedAt)}
          );`
      ),
      ...(database.tables.missions ?? []).map(
        (mission) => `
          INSERT INTO missions (id, pipeline_id, work_item_id, title, status, mission_json, created_at, updated_at)
          VALUES (
            ${sqlQuote(mission.id)}, ${sqlQuote(mission.pipelineId)}, ${sqlQuote(mission.workItemId)},
            ${sqlQuote(mission.title)}, ${sqlQuote(mission.status)}, ${sqlQuote(json(mission.mission))},
            ${sqlQuote(mission.createdAt)}, ${sqlQuote(mission.updatedAt)}
          );`
      ),
      ...(database.tables.operations ?? []).map(
        (operation) => `
          INSERT INTO operations (id, mission_id, stage_id, agent_id, status, prompt, required_proof_json, created_at, updated_at)
          VALUES (
            ${sqlQuote(operation.id)}, ${sqlQuote(operation.missionId)}, ${sqlQuote(operation.stageId)},
            ${sqlQuote(operation.agentId)}, ${sqlQuote(operation.status)}, ${sqlQuote(operation.prompt)},
            ${sqlQuote(json(operation.requiredProof))}, ${sqlQuote(operation.createdAt)}, ${sqlQuote(operation.updatedAt)}
          );`
      ),
      ...(database.tables.proofRecords ?? []).map(
        (proof) => `
          INSERT INTO proof_records (id, operation_id, label, value, source_path, created_at)
          VALUES (
            ${sqlQuote(proof.id)}, ${sqlQuote(proof.operationId)}, ${sqlQuote(proof.label)},
            ${sqlQuote(proof.value)}, ${proof.sourcePath ? sqlQuote(proof.sourcePath) : "NULL"}, ${sqlQuote(proof.createdAt)}
          );`
      ),
      "COMMIT;"
    ].join("\n");

    await execSql(this.databasePath, sql);
  }

  async getDatabase(): Promise<WorkspaceDatabase | undefined> {
    await this.initialize();
    const output = await execSql(
      this.databasePath,
`.mode json
SELECT
        (SELECT value FROM metadata WHERE key = 'schema_version') AS schemaVersion,
        (SELECT value FROM metadata WHERE key = 'saved_at') AS savedAt,
        (SELECT json_group_array(json_object(
          'id', id,
          'name', name,
          'description', description,
          'team', team,
          'status', status,
          'labels', json(labels_json),
          'repositoryTargets', json(repository_targets_json),
          'defaultRepositoryTargetId', default_repository_target_id,
          'createdAt', created_at,
          'updatedAt', updated_at
        )) FROM projects) AS projects,
        (SELECT json_group_array(json_object(
          'id', id,
          'projectId', project_id,
          'repositoryTargetId', repository_target_id,
          'source', source,
          'sourceExternalRef', source_external_ref,
          'title', title,
          'rawText', raw_text,
          'structured', json(structured_json),
          'acceptanceCriteria', json(acceptance_criteria_json),
          'risks', json(risks_json),
          'status', status,
          'createdAt', created_at,
          'updatedAt', updated_at
        )) FROM requirements) AS requirements,
        (SELECT json_group_array(json_object(
          'id', id,
          'projectId', project_id,
          'key', key,
          'title', title,
          'description', description,
          'status', status,
          'priority', priority,
          'assignee', assignee,
          'labels', json(labels_json),
          'team', team,
          'stageId', stage_id,
          'target', target,
          'source', source,
          'requirementId', requirement_id,
          'sourceExternalRef', source_external_ref,
          'repositoryTargetId', repository_target_id,
          'branchName', branch_name,
          'acceptanceCriteria', json(acceptance_criteria_json),
          'parentItemId', parent_item_id,
          'blockedByItemIds', json(blocked_by_item_ids_json),
          'createdAt', created_at,
          'updatedAt', updated_at
        )) FROM work_items) AS workItems,
        (SELECT json_group_array(json_object(
          'runId', run_id,
          'projectId', project_id,
          'workItems', json(work_items_json),
          'events', json(events_json),
          'syncIntents', json(sync_intents_json),
          'updatedAt', updated_at
        )) FROM mission_control_states) AS missionControlStates,
        (SELECT json_group_array(json_object(
          'id', id,
          'runId', run_id,
          'sequence', sequence,
          'event', json(event_json)
        )) FROM mission_events ORDER BY sequence) AS missionEvents,
        (SELECT json_group_array(json_object(
          'id', id,
          'runId', run_id,
          'sequence', sequence,
          'intent', json(intent_json)
        )) FROM sync_intents ORDER BY sequence) AS syncIntents,
        (SELECT json_group_array(json_object(
          'providerId', provider_id,
          'status', status,
          'grantedPermissions', json(granted_permissions_json),
          'connectedAs', connected_as,
          'updatedAt', updated_at
        )) FROM connections) AS connections,
        (SELECT json_group_array(json_object(
          'id', id,
          'activeNav', active_nav,
          'selectedProviderId', selected_provider_id,
          'selectedWorkItemId', selected_work_item_id,
          'inspectorOpen', inspector_open != 0,
          'activeInspectorPanel', active_inspector_panel,
          'runnerPreset', runner_preset,
          'statusFilter', status_filter,
          'assigneeFilter', assignee_filter,
          'sortDirection', sort_direction,
          'collapsedGroups', json(collapsed_groups_json)
        )) FROM ui_preferences) AS uiPreferences,
        (SELECT json_group_array(json_object(
          'id', id,
          'workItemId', work_item_id,
          'runId', run_id,
          'status', status,
          'run', json(run_json),
          'createdAt', created_at,
          'updatedAt', updated_at
        )) FROM pipelines) AS pipelines,
        (SELECT json_group_array(json_object(
          'id', id,
          'pipelineId', pipeline_id,
          'stageId', stage_id,
          'status', status,
          'title', title,
          'summary', summary,
          'decisionNote', decision_note,
          'createdAt', created_at,
          'updatedAt', updated_at
        )) FROM checkpoints) AS checkpoints,
        (SELECT json_group_array(json_object(
          'id', id,
          'pipelineId', pipeline_id,
          'workItemId', work_item_id,
          'title', title,
          'status', status,
          'mission', json(mission_json),
          'createdAt', created_at,
          'updatedAt', updated_at
        )) FROM missions) AS missions,
        (SELECT json_group_array(json_object(
          'id', id,
          'missionId', mission_id,
          'stageId', stage_id,
          'agentId', agent_id,
          'status', status,
          'prompt', prompt,
          'requiredProof', json(required_proof_json),
          'createdAt', created_at,
          'updatedAt', updated_at
        )) FROM operations) AS operations,
        (SELECT json_group_array(json_object(
          'id', id,
          'operationId', operation_id,
          'label', label,
          'value', value,
          'sourcePath', source_path,
          'createdAt', created_at
        )) FROM proof_records) AS proofRecords;
`
    );

    const rows = JSON.parse(output || "[]") as Array<Record<string, string | number | null>>;
    const row = rows[0];
    if (!row?.schemaVersion || !row.savedAt) {
      return undefined;
    }

    return {
      schemaVersion: Number(row.schemaVersion),
      savedAt: String(row.savedAt),
      tables: {
        projects: JSON.parse(String(row.projects ?? "[]")),
        requirements: JSON.parse(String(row.requirements ?? "[]")),
        workItems: JSON.parse(String(row.workItems ?? "[]")),
        missionControlStates: JSON.parse(String(row.missionControlStates ?? "[]")),
        missionEvents: JSON.parse(String(row.missionEvents ?? "[]")),
        syncIntents: JSON.parse(String(row.syncIntents ?? "[]")),
        connections: JSON.parse(String(row.connections ?? "[]")),
        uiPreferences: JSON.parse(String(row.uiPreferences ?? "[]")),
        pipelines: JSON.parse(String(row.pipelines ?? "[]")),
        checkpoints: JSON.parse(String(row.checkpoints ?? "[]")),
        missions: JSON.parse(String(row.missions ?? "[]")),
        operations: JSON.parse(String(row.operations ?? "[]")),
        proofRecords: JSON.parse(String(row.proofRecords ?? "[]"))
      }
    };
  }
}
