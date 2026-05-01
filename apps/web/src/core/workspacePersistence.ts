import type { ConnectionState, ProviderId } from "./connections";
import { connectionProviders, createInitialConnectionState } from "./connections";
import { applyMissionControlEvents, createMissionControlState, type MissionControlState } from "./missionControlState";
import type { SyncIntent, MissionEvent } from "./missionEvents";
import type { Mission } from "./mission";
import { createWorkboardProject, type WorkItem, type WorkItemStatus, type WorkboardProject } from "./workboard";
import type { PipelineRun, PipelineStageId } from "./types";

export const workspacePersistenceSchemaVersion = 1;

export type InspectorPanelPersistence = "properties" | "provider";
export type PrimaryNavPersistence = "Projects" | "Views" | "Issues" | "Page Pilot";
export type RunnerPresetPersistence = "local-proof" | "demo-code" | "codex" | "opencode" | "claude-code" | "trae-agent";
type LegacyRunnerPresetPersistence = RunnerPresetPersistence | "claude";

export interface ProjectRecord extends WorkboardProject {
  createdAt: string;
  updatedAt: string;
}

export interface WorkItemRecord extends WorkItem {
  projectId: string;
  createdAt: string;
  updatedAt: string;
}

export type RequirementRecordSource = "manual" | "github_issue" | "feishu_message" | "api" | "ai_generated" | string;
export type RequirementRecordStatus = "draft" | "structured" | "approved" | "converted" | "archived" | string;

export interface RequirementRecord {
  id: string;
  projectId: string;
  repositoryTargetId?: string;
  source: RequirementRecordSource;
  sourceExternalRef?: string;
  title: string;
  rawText: string;
  structured?: Record<string, unknown>;
  acceptanceCriteria: string[];
  risks: string[];
  status: RequirementRecordStatus;
  createdAt: string;
  updatedAt: string;
}

export interface MissionControlStateRecord extends MissionControlState {
  projectId: string;
  updatedAt: string;
}

export interface MissionEventRecord {
  id: string;
  runId: string;
  sequence: number;
  event: MissionEvent;
}

export interface SyncIntentRecord {
  id: string;
  runId: string;
  sequence: number;
  intent: SyncIntent;
}

export interface ConnectionRecord {
  providerId: ProviderId;
  status: ConnectionState[ProviderId]["status"];
  grantedPermissions: string[];
  connectedAs?: string;
  updatedAt: string;
}

export interface UiPreferenceRecord {
  id: "default";
  activeNav: PrimaryNavPersistence;
  selectedProviderId: ProviderId;
  selectedWorkItemId: string;
  inspectorOpen: boolean;
  activeInspectorPanel: InspectorPanelPersistence;
  runnerPreset: LegacyRunnerPresetPersistence;
  statusFilter: "All" | WorkItemStatus;
  assigneeFilter: string;
  sortDirection: "asc" | "desc";
  collapsedGroups: WorkItemStatus[];
}

function normalizeRunnerPreset(runner: string | undefined): RunnerPresetPersistence {
  if (runner === "demo-code" || runner === "codex" || runner === "opencode" || runner === "claude-code" || runner === "trae-agent") {
    return runner;
  }
  if (runner === "claude") return "claude-code";
  return "local-proof";
}

export type PipelineLifecycleStatus =
  | "draft"
  | "running"
  | "paused"
  | "waiting-human"
  | "completed"
  | "failed"
  | "terminated";

export interface PipelineRecord {
  id: string;
  workItemId: string;
  runId: string;
  status: PipelineLifecycleStatus;
  run: PipelineRun;
  createdAt: string;
  updatedAt: string;
}

export type CheckpointStatus = "pending" | "approved" | "rejected";

export interface CheckpointRecord {
  id: string;
  pipelineId: string;
  stageId: PipelineStageId;
  status: CheckpointStatus;
  title: string;
  summary: string;
  decisionNote?: string;
  createdAt: string;
  updatedAt: string;
}

export interface MissionRecord {
  id: string;
  pipelineId: string;
  workItemId: string;
  title: string;
  status: Mission["status"];
  mission: Mission;
  createdAt: string;
  updatedAt: string;
}

export interface OperationRecord {
  id: string;
  missionId: string;
  stageId: string;
  agentId: string;
  status: string;
  prompt: string;
  requiredProof: string[];
  createdAt: string;
  updatedAt: string;
}

export interface ProofRecord {
  id: string;
  operationId: string;
  label: string;
  value: string;
  sourcePath?: string;
  createdAt: string;
}

export interface WorkspaceDatabaseTables {
  projects: ProjectRecord[];
  requirements: RequirementRecord[];
  workItems: WorkItemRecord[];
  missionControlStates: MissionControlStateRecord[];
  missionEvents: MissionEventRecord[];
  syncIntents: SyncIntentRecord[];
  connections: ConnectionRecord[];
  uiPreferences: UiPreferenceRecord[];
  pipelines: PipelineRecord[];
  checkpoints: CheckpointRecord[];
  missions: MissionRecord[];
  operations: OperationRecord[];
  proofRecords: ProofRecord[];
}

export interface WorkspaceDatabase {
  schemaVersion: number;
  savedAt: string;
  tables: WorkspaceDatabaseTables;
}

export interface WorkspaceSession {
  projects: ProjectRecord[];
  requirements: RequirementRecord[];
  workItems: WorkItem[];
  missionState: MissionControlState;
  connections: ConnectionState;
  activeNav: PrimaryNavPersistence;
  selectedProviderId: ProviderId;
  selectedWorkItemId: string;
  inspectorOpen: boolean;
  activeInspectorPanel: InspectorPanelPersistence;
  runnerPreset: RunnerPresetPersistence;
  statusFilter: "All" | WorkItemStatus;
  assigneeFilter: string;
  sortDirection: "asc" | "desc";
  collapsedGroups: WorkItemStatus[];
}

export const workspaceTableDesign = {
  projects: "Product or engineering goals that can bind one or more repository targets.",
  requirements: "User needs or imported external issues that own one or more executable work items.",
  workItems: "Workboard items under a project; each can start one or more pipeline runs.",
  missionControlStates: "Current reducer state for each mission control run.",
  missionEvents: "Append-only operation/checkpoint event log.",
  syncIntents: "Planned connector sync actions derived from mission events.",
  connections: "Provider connection state and granted permissions.",
  uiPreferences: "Durable user workspace preferences.",
  pipelines: "Pipeline lifecycle records linked to work items.",
  checkpoints: "Human approval records linked to pipeline stages.",
  missions: "Mission records linked to pipelines and work items.",
  operations: "Operation execution records linked to missions.",
  proofRecords: "Structured proof entries linked to operations."
} as const;

function storageKey(runId: string): string {
  return `omega.workspace.${runId}.v${workspacePersistenceSchemaVersion}`;
}

export function nowIso(): string {
  return new Date().toISOString();
}

function projectRecordFromRun(run: PipelineRun, timestamp: string): ProjectRecord {
  return {
    ...createWorkboardProject(run),
    createdAt: timestamp,
    updatedAt: timestamp
  };
}

function normalizeWorkItem(item: Partial<WorkItem>): WorkItem {
  return {
    id: item.id ?? "item_unknown",
    key: item.key ?? "OMG-0",
    title: item.title ?? "Untitled work item",
    description: item.description ?? "",
    status: item.status ?? "Ready",
    priority: item.priority ?? "Medium",
    assignee: item.assignee ?? "requirement",
    labels: item.labels ?? [],
    team: item.team ?? "Omega",
    stageId: item.stageId ?? "intake",
    target: item.target ?? "No target",
    source: item.source ?? "manual",
    requirementId: item.requirementId,
    sourceExternalRef: item.sourceExternalRef,
    repositoryTargetId: item.repositoryTargetId,
    branchName: item.branchName,
    acceptanceCriteria:
      item.acceptanceCriteria && item.acceptanceCriteria.length > 0
        ? item.acceptanceCriteria
        : ["Request is described clearly", "Human can verify the result"],
    parentItemId: item.parentItemId,
    blockedByItemIds: item.blockedByItemIds ?? []
  };
}

function normalizeRequirement(record: Partial<RequirementRecord>): RequirementRecord {
  const timestamp = nowIso();
  return {
    id: record.id ?? `req_${Date.now()}`,
    projectId: record.projectId ?? "project_unknown",
    repositoryTargetId: record.repositoryTargetId,
    source: record.source ?? "manual",
    sourceExternalRef: record.sourceExternalRef,
    title: record.title ?? "Untitled requirement",
    rawText: record.rawText ?? "",
    structured: record.structured ?? {},
    acceptanceCriteria: record.acceptanceCriteria ?? [],
    risks: record.risks ?? [],
    status: record.status ?? "converted",
    createdAt: record.createdAt ?? timestamp,
    updatedAt: record.updatedAt ?? timestamp
  };
}

function workItemRecordFromItem(projectId: string, item: WorkItem, timestamp: string): WorkItemRecord {
  const normalized = normalizeWorkItem(item);
  return {
    ...normalized,
    projectId,
    createdAt: timestamp,
    updatedAt: timestamp
  };
}

function connectionRecordsFromState(state: ConnectionState, timestamp: string): ConnectionRecord[] {
  return Object.entries(state).map(([providerId, connection]) => ({
    providerId: providerId as ProviderId,
    status: connection.status,
    grantedPermissions: connection.grantedPermissions,
    connectedAs: connection.connectedAs,
    updatedAt: timestamp
  }));
}

function connectionStateFromRecords(records: ConnectionRecord[]): ConnectionState {
  const initial = createInitialConnectionState();
  const knownProviderIds = new Set(connectionProviders.map((provider) => provider.id));
  return records.reduce<ConnectionState>(
    (state, record) => {
      if (!knownProviderIds.has(record.providerId)) {
        return state;
      }

      return {
        ...state,
        [record.providerId]: {
        status: record.status,
        grantedPermissions: record.grantedPermissions,
        connectedAs: record.connectedAs
      }
      };
    },
    initial
  );
}

function eventRecordsFromState(runId: string, events: MissionEvent[]): MissionEventRecord[] {
  return events.map((event, index) => ({
    id: `${runId}:event:${index + 1}`,
    runId,
    sequence: index + 1,
    event
  }));
}

function syncIntentRecordsFromState(runId: string, intents: SyncIntent[]): SyncIntentRecord[] {
  return intents.map((intent, index) => ({
    id: `${runId}:sync:${index + 1}`,
    runId,
    sequence: index + 1,
    intent
  }));
}

export function createInitialWorkspaceSession(run: PipelineRun): WorkspaceSession {
  const workItems: WorkItem[] = [];
  const timestamp = nowIso();
  return {
    projects: [projectRecordFromRun(run, timestamp)],
    requirements: [],
    workItems,
    missionState: createMissionControlState(run, workItems),
    connections: createInitialConnectionState(),
    activeNav: "Issues",
    selectedProviderId: "github",
    selectedWorkItemId: "",
    inspectorOpen: true,
    activeInspectorPanel: "properties",
    runnerPreset: "local-proof",
    statusFilter: "All",
    assigneeFilter: "All",
    sortDirection: "desc",
    collapsedGroups: []
  };
}

export function databaseFromWorkspaceSession(run: PipelineRun, session: WorkspaceSession): WorkspaceDatabase {
  const timestamp = nowIso();
  const project = {
    ...projectRecordFromRun(run, timestamp),
    ...session.projects[0],
    updatedAt: timestamp
  };
  return {
    schemaVersion: workspacePersistenceSchemaVersion,
    savedAt: timestamp,
    tables: {
      projects: [project],
      requirements: session.requirements.map(normalizeRequirement),
      workItems: session.workItems.map((item) => workItemRecordFromItem(project.id, item, timestamp)),
      missionControlStates: [
        {
          ...session.missionState,
          projectId: project.id,
          updatedAt: timestamp
        }
      ],
      missionEvents: eventRecordsFromState(session.missionState.runId, session.missionState.events),
      syncIntents: syncIntentRecordsFromState(session.missionState.runId, session.missionState.syncIntents),
      connections: connectionRecordsFromState(session.connections, timestamp),
      uiPreferences: [
        {
          id: "default",
          activeNav: session.activeNav,
          selectedProviderId: session.selectedProviderId,
          selectedWorkItemId: session.selectedWorkItemId,
          inspectorOpen: session.inspectorOpen,
          activeInspectorPanel: session.activeInspectorPanel,
          runnerPreset: session.runnerPreset,
          statusFilter: session.statusFilter,
          assigneeFilter: session.assigneeFilter,
          sortDirection: session.sortDirection,
          collapsedGroups: session.collapsedGroups
        }
      ],
      pipelines: [],
      checkpoints: [],
      missions: [],
      operations: [],
      proofRecords: []
    }
  };
}

export function workspaceSessionFromDatabase(run: PipelineRun, database: WorkspaceDatabase): WorkspaceSession {
  const initial = createInitialWorkspaceSession(run);
  const tables = database.tables;
  const projects = (tables.projects ?? []).length > 0 ? tables.projects : initial.projects;
  const requirements = (tables.requirements ?? []).map(normalizeRequirement);
  const workItems = (tables.workItems ?? []).map(({ projectId, createdAt, updatedAt, ...item }) =>
    normalizeWorkItem(item)
  );
  const missionState = (tables.missionControlStates ?? [])[0] ?? createMissionControlState(run, workItems);
  const ui = (tables.uiPreferences ?? [])[0];
  const selectedProviderId = connectionProviders.some((provider) => provider.id === ui?.selectedProviderId)
    ? ui.selectedProviderId
    : initial.selectedProviderId;

  return {
    ...initial,
    projects,
    requirements,
    workItems,
    missionState: {
      runId: missionState.runId,
      workItems,
      events: missionState.events,
      syncIntents: missionState.syncIntents
    },
    connections: connectionStateFromRecords(tables.connections ?? []),
    activeNav: ui?.activeNav ?? initial.activeNav,
    selectedProviderId,
    selectedWorkItemId: ui?.selectedWorkItemId ?? initial.selectedWorkItemId,
    inspectorOpen: ui?.inspectorOpen ?? initial.inspectorOpen,
    activeInspectorPanel: ui?.activeInspectorPanel ?? initial.activeInspectorPanel,
    runnerPreset: normalizeRunnerPreset(ui?.runnerPreset),
    statusFilter: ui?.statusFilter ?? initial.statusFilter,
    assigneeFilter: ui?.assigneeFilter ?? initial.assigneeFilter,
    sortDirection: ui?.sortDirection ?? initial.sortDirection,
    collapsedGroups: ui?.collapsedGroups ?? initial.collapsedGroups
  };
}

export function loadWorkspaceSession(run: PipelineRun, storage: Storage = window.localStorage): WorkspaceSession {
  const raw = storage.getItem(storageKey(run.id));
  if (!raw) {
    return createInitialWorkspaceSession(run);
  }

  try {
    const database = JSON.parse(raw) as WorkspaceDatabase;
    if (database.schemaVersion !== workspacePersistenceSchemaVersion) {
      return createInitialWorkspaceSession(run);
    }
    return workspaceSessionFromDatabase(run, database);
  } catch {
    return createInitialWorkspaceSession(run);
  }
}

export function saveWorkspaceSession(
  run: PipelineRun,
  session: WorkspaceSession,
  storage: Storage = window.localStorage
): void {
  storage.setItem(storageKey(run.id), JSON.stringify(databaseFromWorkspaceSession(run, session)));
}

export function appendWorkItemToDatabase(database: WorkspaceDatabase, item: WorkItem): WorkspaceDatabase {
  const timestamp = nowIso();
  const projectId = database.tables.projects[0]?.id ?? "project_unknown";
  const normalized = normalizeWorkItem(item);
  const record: WorkItemRecord = {
    ...normalized,
    projectId,
    createdAt: timestamp,
    updatedAt: timestamp
  };

  const nextMissionState = database.tables.missionControlStates[0]
    ? {
        ...database.tables.missionControlStates[0],
        workItems: [...database.tables.missionControlStates[0].workItems.map(normalizeWorkItem), normalized],
        updatedAt: timestamp
      }
    : undefined;

  return {
    ...database,
    savedAt: timestamp,
    tables: {
      ...database.tables,
      requirements: database.tables.requirements ?? [],
      workItems: [...(database.tables.workItems ?? []), record],
      missionControlStates: nextMissionState ? [nextMissionState] : database.tables.missionControlStates
    }
  };
}

export function updateWorkItemInDatabase(
  database: WorkspaceDatabase,
  itemId: string,
  patch: Partial<Pick<WorkItem, "status" | "priority">>
): WorkspaceDatabase {
  const timestamp = nowIso();
  const workItems = database.tables.workItems.map((item) =>
    item.id === itemId ? { ...item, ...patch, updatedAt: timestamp } : item
  );
  const missionControlStates = database.tables.missionControlStates.map((state) => ({
    ...state,
    workItems: state.workItems.map((item) => (item.id === itemId ? { ...item, ...patch } : item)),
    updatedAt: timestamp
  }));

  return {
    ...database,
    savedAt: timestamp,
    tables: {
      ...database.tables,
      workItems,
      missionControlStates
    }
  };
}

export function deleteWorkItemFromDatabase(database: WorkspaceDatabase, itemId: string): WorkspaceDatabase {
  const timestamp = nowIso();
  const item = database.tables.workItems.find((candidate) => candidate.id === itemId);
  const workItems = database.tables.workItems.filter((candidate) => candidate.id !== itemId);
  const requirementId = item?.requirementId;
  const requirements =
    requirementId && !workItems.some((candidate) => candidate.requirementId === requirementId)
      ? database.tables.requirements.filter((requirement) => requirement.id !== requirementId)
      : database.tables.requirements;
  const missionControlStates = database.tables.missionControlStates.map((state) => ({
    ...state,
    workItems: state.workItems.filter((candidate) => candidate.id !== itemId),
    updatedAt: timestamp
  }));

  return {
    ...database,
    savedAt: timestamp,
    tables: {
      ...database.tables,
      requirements,
      workItems,
      missionControlStates
    }
  };
}

export function applyMissionEventsToDatabase(
  database: WorkspaceDatabase,
  events: MissionEvent[]
): WorkspaceDatabase {
  const currentState = database.tables.missionControlStates[0];
  if (!currentState) {
    return database;
  }

  const timestamp = nowIso();
  const nextState = applyMissionControlEvents(
    {
      runId: currentState.runId,
      workItems: currentState.workItems,
      events: currentState.events,
      syncIntents: currentState.syncIntents
    },
    events
  );

  return {
    ...database,
    savedAt: timestamp,
    tables: {
      ...database.tables,
      workItems: database.tables.workItems.map((record) => {
        const next = nextState.workItems.find((item) => item.id === record.id);
        return next ? { ...record, ...next, updatedAt: timestamp } : record;
      }),
      missionControlStates: [
        {
          ...currentState,
          workItems: nextState.workItems,
          events: nextState.events,
          syncIntents: nextState.syncIntents,
          updatedAt: timestamp
        }
      ],
      missionEvents: eventRecordsFromState(nextState.runId, nextState.events),
      syncIntents: syncIntentRecordsFromState(nextState.runId, nextState.syncIntents)
    }
  };
}

export function appendPipelineToDatabase(database: WorkspaceDatabase, pipeline: PipelineRecord): WorkspaceDatabase {
  return {
    ...database,
    savedAt: nowIso(),
    tables: {
      ...database.tables,
      pipelines: [...(database.tables.pipelines ?? []), pipeline]
    }
  };
}

export function updatePipelineInDatabase(
  database: WorkspaceDatabase,
  pipelineId: string,
  updater: (pipeline: PipelineRecord) => PipelineRecord
): WorkspaceDatabase {
  return {
    ...database,
    savedAt: nowIso(),
    tables: {
      ...database.tables,
      pipelines: (database.tables.pipelines ?? []).map((pipeline) =>
        pipeline.id === pipelineId ? updater(pipeline) : pipeline
      )
    }
  };
}

export function upsertCheckpointInDatabase(
  database: WorkspaceDatabase,
  checkpoint: CheckpointRecord
): WorkspaceDatabase {
  const checkpoints = database.tables.checkpoints ?? [];
  const exists = checkpoints.some((candidate) => candidate.id === checkpoint.id);
  return {
    ...database,
    savedAt: nowIso(),
    tables: {
      ...database.tables,
      checkpoints: exists
        ? checkpoints.map((candidate) => (candidate.id === checkpoint.id ? checkpoint : candidate))
        : [...checkpoints, checkpoint]
    }
  };
}

export function appendMissionToDatabase(database: WorkspaceDatabase, mission: MissionRecord): WorkspaceDatabase {
  return {
    ...database,
    savedAt: nowIso(),
    tables: {
      ...database.tables,
      missions: [...(database.tables.missions ?? []), mission]
    }
  };
}

export function upsertOperationInDatabase(database: WorkspaceDatabase, operation: OperationRecord): WorkspaceDatabase {
  const operations = database.tables.operations ?? [];
  const exists = operations.some((candidate) => candidate.id === operation.id);
  return {
    ...database,
    savedAt: nowIso(),
    tables: {
      ...database.tables,
      operations: exists
        ? operations.map((candidate) => (candidate.id === operation.id ? operation : candidate))
        : [...operations, operation]
    }
  };
}

export function appendProofRecordToDatabase(database: WorkspaceDatabase, proofRecord: ProofRecord): WorkspaceDatabase {
  return {
    ...database,
    savedAt: nowIso(),
    tables: {
      ...database.tables,
      proofRecords: [...(database.tables.proofRecords ?? []), proofRecord]
    }
  };
}
