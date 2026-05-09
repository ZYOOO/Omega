import type { Mission, PipelineRun, WorkItem, WorkspaceDatabase, WorkspaceSession } from "./core";
import { workspaceSessionFromDatabase } from "./core";
import type { RunnerPreset, RunOperationResponse } from "./runnerTypes";

export async function fetchWorkspaceSession(
  apiUrl: string,
  run: PipelineRun,
  fetchImpl: typeof fetch = fetch
): Promise<WorkspaceSession | undefined> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/workspace?scope=session`);
  if (response.status === 404) {
    return undefined;
  }
  if (!response.ok) {
    throw new Error(`Workspace API failed: ${response.status}`);
  }

  const database = await response.json() as WorkspaceDatabase;
  return workspaceSessionFromDatabase(run, database);
}

export async function createWorkItemViaApi(
  apiUrl: string,
  run: PipelineRun,
  item: WorkItem,
  fetchImpl: typeof fetch = fetch
): Promise<WorkspaceSession> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/work-items`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ item })
  });

  if (!response.ok) {
    throw new Error(`Create work item API failed: ${response.status}`);
  }

  return workspaceSessionFromDatabase(run, await response.json() as WorkspaceDatabase);
}

export async function createProjectViaApi(
  apiUrl: string,
  run: PipelineRun,
  input: {
    name: string;
    description?: string;
    team?: string;
    labels?: string[];
  },
  fetchImpl: typeof fetch = fetch
): Promise<WorkspaceSession> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/projects`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(input)
  });

  if (!response.ok) {
    throw new Error(`Create project API failed: ${response.status}`);
  }

  return workspaceSessionFromDatabase(run, await response.json() as WorkspaceDatabase);
}

export async function patchWorkItemViaApi(
  apiUrl: string,
  run: PipelineRun,
  itemId: string,
  patch: Partial<Pick<WorkItem, "status" | "priority">>,
  fetchImpl: typeof fetch = fetch
): Promise<WorkspaceSession> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/work-items/${itemId}`, {
    method: "PATCH",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(patch)
  });

  if (!response.ok) {
    throw new Error(`Patch work item API failed: ${response.status}`);
  }

  return workspaceSessionFromDatabase(run, await response.json() as WorkspaceDatabase);
}

export async function deleteWorkItemViaApi(
  apiUrl: string,
  run: PipelineRun,
  itemId: string,
  fetchImpl: typeof fetch = fetch
): Promise<WorkspaceSession> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/work-items/${itemId}`, {
    method: "DELETE"
  });

  if (!response.ok) {
    throw new Error(`Delete work item API failed: ${response.status}`);
  }

  return workspaceSessionFromDatabase(run, await response.json() as WorkspaceDatabase);
}

export async function importGitHubIssuesViaApi(
  apiUrl: string,
  run: PipelineRun,
  owner: string,
  repo: string,
  fetchImpl: typeof fetch = fetch
): Promise<WorkspaceSession> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/github/import-issues`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ owner, repo })
  });

  if (!response.ok) {
    throw new Error(`GitHub issue import API failed: ${response.status}`);
  }

  return workspaceSessionFromDatabase(run, await response.json() as WorkspaceDatabase);
}

export async function bindGitHubRepositoryTargetViaApi(
  apiUrl: string,
  run: PipelineRun,
  input: {
    projectId?: string;
    owner: string;
    repo: string;
    nameWithOwner?: string;
    defaultBranch?: string;
    url?: string;
  },
  fetchImpl: typeof fetch = fetch
): Promise<WorkspaceSession> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/github/bind-repository-target`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(input)
  });

  if (!response.ok) {
    throw new Error(`GitHub repository target API failed: ${response.status}`);
  }

  return workspaceSessionFromDatabase(run, await response.json() as WorkspaceDatabase);
}

export async function bindLocalRepositoryTargetViaApi(
  apiUrl: string,
  run: PipelineRun,
  input: {
    projectId?: string;
    path: string;
    defaultBranch?: string;
  },
  fetchImpl: typeof fetch = fetch
): Promise<WorkspaceSession> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/repository-targets/local`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(input)
  });

  if (!response.ok) {
    throw new Error(`Local repository workspace API failed: ${response.status}`);
  }

  return workspaceSessionFromDatabase(run, await response.json() as WorkspaceDatabase);
}

export async function deleteRepositoryTargetViaApi(
  apiUrl: string,
  run: PipelineRun,
  repositoryTargetId: string,
  options: { deleteLocalWorkspaces?: boolean } = {},
  fetchImpl: typeof fetch = fetch
): Promise<WorkspaceSession> {
  const params = new URLSearchParams();
  if (options.deleteLocalWorkspaces) {
    params.set("deleteLocalWorkspaces", "true");
  }
  const suffix = params.toString() ? `?${params.toString()}` : "";
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/github/repository-targets/${encodeURIComponent(repositoryTargetId)}${suffix}`, {
    method: "DELETE"
  });

  if (!response.ok) {
    throw new Error(`Delete repository workspace API failed: ${response.status}`);
  }

  return workspaceSessionFromDatabase(run, await response.json() as WorkspaceDatabase);
}

export async function runOperationViaWorkspaceApi(
  apiUrl: string,
  mission: Mission,
  operationId: string,
  runner: RunnerPreset,
  fetchImpl: typeof fetch = fetch
): Promise<RunOperationResponse> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/operations/run`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      mission,
      operationId,
      runner
    })
  });

  if (!response.ok) {
    throw new Error(`Run operation API failed: ${response.status}`);
  }

  return response.json() as Promise<RunOperationResponse>;
}

export async function fetchMissionFromWorkItem(
  apiUrl: string,
  item: WorkItem,
  fetchImpl: typeof fetch = fetch
): Promise<Mission> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/missions/from-work-item`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ item })
  });

  if (!response.ok) {
    throw new Error(`Mission build API failed: ${response.status}`);
  }

  return response.json() as Promise<Mission>;
}
