export interface ObservabilitySummary {
  counts: {
    workItems: number;
    pipelines: number;
    checkpoints: number;
    missions: number;
    operations: number;
    proofRecords: number;
    events: number;
  };
  pipelineStatus: Record<string, number>;
  checkpointStatus: Record<string, number>;
  operationStatus: Record<string, number>;
  workItemStatus: Record<string, number>;
  attention: {
    waitingHuman: number;
    failed: number;
    blocked: number;
  };
}

export interface LlmProviderInfo {
  id: string;
  name: string;
  kind: string;
  models: string[];
  defaultModel: string;
  envHint: string;
}

export interface LlmProviderSelection {
  providerId: string;
  model: string;
  reasoningEffort: string;
}

export interface PipelineTemplateInfo {
  id: string;
  name: string;
  description: string;
  stages: Array<{
    id: string;
    title: string;
    agentId: string;
    humanGate: boolean;
  }>;
}

export interface AgentDefinitionInfo {
  id: string;
  name: string;
  stageId: string;
  systemPrompt: string;
  inputContract: string[];
  outputContract: string[];
  defaultTools: string[];
  defaultModel: LlmProviderSelection;
}

export interface AgentProfileDraftInfo {
  id: string;
  label: string;
  runner: string;
  model: string;
  skills: string;
  mcp: string;
  stageNotes: string;
  codexPolicy: string;
  claudePolicy: string;
}

export interface ProjectAgentProfileInfo {
  projectId: string;
  repositoryTargetId?: string;
  workflowTemplate: string;
  workflowMarkdown: string;
  stagePolicy: string;
  skillAllowlist: string;
  mcpAllowlist: string;
  codexPolicy: string;
  claudePolicy: string;
  agentProfiles: AgentProfileDraftInfo[];
  source?: string;
  updatedAt?: string;
}

export interface LocalCapabilityInfo {
  id: string;
  command: string;
  category: string;
  description: string;
  available: boolean;
  path?: string;
  version?: string;
  required: boolean;
}

export interface LocalWorkspaceRootInfo {
  workspaceRoot: string;
}

export interface FeishuNotificationResult {
  status: string;
  provider: string;
  tool: string;
  chatId: string;
  messageId?: string;
  raw?: string;
}

export interface GitHubOAuthStartResult {
  configured: boolean;
  authorizeUrl?: string;
  state?: string;
  redirectUri?: string;
  scopes?: string[];
  reason?: string;
}

export interface GitHubCliLoginStartResult {
  started: boolean;
  method?: "gh-cli" | string;
  message?: string;
  reason?: string;
  command?: string;
  verificationUrl?: string;
}

export interface GitHubStatusInfo {
  available: boolean;
  authenticated: boolean;
  output: string;
  account?: string;
  oauthConfigured: boolean;
  oauthAuthenticated: boolean;
  oauthConnectedAs?: string;
}

export interface GitHubOAuthConfigInfo {
  configured: boolean;
  clientId: string;
  redirectUri: string;
  tokenUrl: string;
  secretConfigured: boolean;
  source: "app" | "env" | "empty" | string;
}

export interface GitHubOAuthConfigUpdate {
  clientId: string;
  clientSecret?: string;
  redirectUri: string;
  tokenUrl?: string;
}

export interface GitHubRepositoryInfo {
  name: string;
  nameWithOwner?: string;
  owner?: {
    login?: string;
  };
  description?: string;
  url?: string;
  isPrivate?: boolean;
  defaultBranchRef?: {
    name?: string;
  };
}

export interface GitHubPullRequestInput {
  workspacePath?: string;
  repositoryPath?: string;
  repositoryOwner?: string;
  repositoryName?: string;
  title: string;
  body?: string;
  branchName: string;
  baseBranch?: string;
  draft?: boolean;
  changedFiles?: string[];
}

export interface GitHubPullRequestResult {
  status: string;
  url: string;
  title?: string;
  branchName?: string;
  baseBranch?: string;
  repositoryPath?: string;
  bodyPath?: string;
}

export interface GitHubPullRequestStatusInput {
  repositoryOwner?: string;
  repositoryName?: string;
  number?: number;
  url?: string;
}

export interface GitHubPullRequestStatusResult {
  number?: number;
  title?: string;
  state: string;
  mergeable?: string;
  reviewDecision?: string;
  headRefName?: string;
  baseRefName?: string;
  url?: string;
  deliveryGate: "ready" | "pending" | "closed" | string;
  checks: Array<{
    name: string;
    state: string;
    link?: string;
  }>;
  proofRecords: Array<{
    id?: string;
    label: string;
    value?: string;
    sourceUrl?: string;
    status?: string;
  }>;
}

export interface RequirementDecompositionInput {
  title: string;
  description?: string;
  repositoryTarget?: string;
  source?: string;
}

export interface RequirementDecompositionResult {
  id?: string;
  summary: string;
  description?: string;
  source?: string;
  repositoryTarget?: string;
  acceptanceCriteria: string[];
  risks: string[];
  assumptions?: string[];
  suggestedWorkItems: Array<{
    id?: string;
    key?: string;
    title?: string;
    description?: string;
    stageId: string;
    assignee?: string;
    priority?: string;
    status?: string;
    acceptanceCriteria?: string[];
    target?: string;
    source?: string;
  }>;
  pipelineStages?: string[];
  createdAt?: string;
}

export interface PipelineRecordInfo {
  id: string;
  workItemId: string;
  runId: string;
  status: string;
  templateId?: string;
  run?: {
    stages?: Array<{
      id: string;
      title: string;
      status: string;
      agentId?: string;
      agentIds?: string[];
      humanGate?: boolean;
      approvedBy?: string;
      dependsOn?: string[];
      inputArtifacts?: string[];
      outputArtifacts?: string[];
    }>;
    events?: Array<{
      type: string;
      message: string;
      stageId?: string;
      agentId?: string;
      timestamp?: string;
    }>;
  };
  createdAt?: string;
  updatedAt?: string;
}

export interface AttemptRecordInfo {
  id: string;
  itemId: string;
  pipelineId: string;
  repositoryTargetId?: string;
  status: string;
  trigger?: string;
  runner?: string;
  currentStageId?: string;
  workspacePath?: string;
  branchName?: string;
  pullRequestUrl?: string;
  startedAt?: string;
  finishedAt?: string;
  durationMs?: number;
  errorMessage?: string;
  stdoutSummary?: string;
  stderrSummary?: string;
  stages?: Array<{
    id: string;
    title?: string;
    status: string;
    agentIds?: string[];
    inputArtifacts?: string[];
    outputArtifacts?: string[];
    evidence?: string[];
    startedAt?: string;
    completedAt?: string;
  }>;
  events?: Array<{
    type?: string;
    message?: string;
    stageId?: string;
    createdAt?: string;
  }>;
  createdAt?: string;
  updatedAt?: string;
}

export interface ProofRecordInfo {
  id: string;
  operationId?: string;
  label: string;
  value?: string;
  sourcePath?: string;
  sourceUrl?: string;
  status?: string;
  createdAt?: string;
}

export interface RequirementRecordInfo {
  id: string;
  projectId: string;
  repositoryTargetId?: string;
  source: string;
  sourceExternalRef?: string;
  title: string;
  rawText: string;
  structured?: Record<string, unknown>;
  acceptanceCriteria: string[];
  risks: string[];
  status: string;
  createdAt: string;
  updatedAt: string;
}

export interface RunnerProcessInfo {
  runner?: string;
  command?: string;
  args?: string[];
  cwd?: string;
  pid?: number;
  status?: string;
  exitCode?: number;
  stdout?: string;
  stderr?: string;
  startedAt?: string;
  finishedAt?: string;
  durationMs?: number;
}

export interface OperationRecordInfo {
  id: string;
  missionId?: string;
  stageId?: string;
  agentId?: string;
  status: string;
  prompt?: string;
  summary?: string;
  requiredProof?: string[];
  runnerProcess?: RunnerProcessInfo;
  createdAt?: string;
  updatedAt?: string;
}

export interface ExecutionLockInfo {
  id: string;
  scope: string;
  repositoryTargetId?: string;
  sourceExternalRef?: string;
  workItemId?: string;
  pipelineId?: string;
  status: string;
  owner?: string;
  runnerProcessState?: string;
  createdAt?: string;
  updatedAt?: string;
  expiresAt?: string;
}

export interface CheckpointRecordInfo {
  id: string;
  pipelineId: string;
  stageId: string;
  status: "pending" | "approved" | "rejected" | string;
  title: string;
  summary: string;
  decisionNote?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface RunCurrentStageResult {
  pipeline: PipelineRecordInfo;
  operationResult: {
    operationId?: string;
    status: string;
    workspacePath?: string;
    proofFiles?: string[];
    stdout?: string;
    stderr?: string;
    events?: unknown[];
    runnerProcess?: RunnerProcessInfo;
  };
}

export interface RunDevFlowCycleResult {
  status: string;
  workspacePath?: string;
  repositoryPath?: string;
  branchName?: string;
  pullRequestUrl?: string;
  merged?: boolean;
  changedFiles?: string[];
  proofFiles?: string[];
  pipeline?: PipelineRecordInfo;
  attempt?: AttemptRecordInfo;
}

async function fetchJson<T>(apiUrl: string, path: string, fetchImpl: typeof fetch): Promise<T> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}${path}`);
  if (!response.ok) {
    throw new Error(`Omega control API failed: ${path} ${response.status}`);
  }
  return response.json() as Promise<T>;
}

async function postJson<T>(
  apiUrl: string,
  path: string,
  body: unknown,
  fetchImpl: typeof fetch = fetch
): Promise<T> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}${path}`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body ?? {})
  });
  if (!response.ok) {
    throw new Error(`Omega control API failed: ${path} ${response.status}`);
  }
  return response.json() as Promise<T>;
}

async function putJson<T>(
  apiUrl: string,
  path: string,
  body: unknown,
  fetchImpl: typeof fetch = fetch
): Promise<T> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}${path}`, {
    method: "PUT",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body ?? {})
  });
  if (!response.ok) {
    throw new Error(`Omega control API failed: ${path} ${response.status}`);
  }
  return response.json() as Promise<T>;
}

export async function fetchObservability(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<ObservabilitySummary> {
  return fetchJson<ObservabilitySummary>(apiUrl, "/observability", fetchImpl);
}

export async function fetchLlmProviders(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<LlmProviderInfo[]> {
  return fetchJson<LlmProviderInfo[]>(apiUrl, "/llm-providers", fetchImpl);
}

export async function fetchLlmProviderSelection(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<LlmProviderSelection> {
  return fetchJson<LlmProviderSelection>(apiUrl, "/llm-provider-selection", fetchImpl);
}

export async function updateLlmProviderSelection(
  apiUrl: string,
  selection: LlmProviderSelection,
  fetchImpl: typeof fetch = fetch
): Promise<LlmProviderSelection> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/llm-provider-selection`, {
    method: "PUT",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(selection)
  });
  if (!response.ok) {
    throw new Error(`Provider selection API failed: ${response.status}`);
  }
  return response.json() as Promise<LlmProviderSelection>;
}

export async function fetchPipelineTemplates(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<PipelineTemplateInfo[]> {
  return fetchJson<PipelineTemplateInfo[]>(apiUrl, "/pipeline-templates", fetchImpl);
}

export async function fetchAgentDefinitions(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<AgentDefinitionInfo[]> {
  return fetchJson<AgentDefinitionInfo[]>(apiUrl, "/agent-definitions", fetchImpl);
}

export async function fetchProjectAgentProfile(
  apiUrl: string,
  scope: { projectId?: string; repositoryTargetId?: string } = {},
  fetchImpl: typeof fetch = fetch
): Promise<ProjectAgentProfileInfo> {
  const search = new URLSearchParams();
  if (scope.projectId) search.set("projectId", scope.projectId);
  if (scope.repositoryTargetId) search.set("repositoryTargetId", scope.repositoryTargetId);
  const suffix = search.toString() ? `?${search.toString()}` : "";
  return fetchJson<ProjectAgentProfileInfo>(apiUrl, `/agent-profile${suffix}`, fetchImpl);
}

export async function updateProjectAgentProfile(
  apiUrl: string,
  profile: ProjectAgentProfileInfo,
  fetchImpl: typeof fetch = fetch
): Promise<ProjectAgentProfileInfo> {
  return putJson<ProjectAgentProfileInfo>(apiUrl, "/agent-profile", profile, fetchImpl);
}

export async function fetchLocalCapabilities(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<LocalCapabilityInfo[]> {
  return fetchJson<LocalCapabilityInfo[]>(apiUrl, "/local-capabilities", fetchImpl);
}

export async function fetchLocalWorkspaceRoot(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<LocalWorkspaceRootInfo> {
  return fetchJson<LocalWorkspaceRootInfo>(apiUrl, "/local-workspace-root", fetchImpl);
}

export async function updateLocalWorkspaceRoot(
  apiUrl: string,
  workspaceRoot: string,
  fetchImpl: typeof fetch = fetch
): Promise<LocalWorkspaceRootInfo> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/local-workspace-root`, {
    method: "PUT",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ workspaceRoot })
  });
  if (!response.ok) {
    throw new Error(`Local workspace root API failed: ${response.status}`);
  }
  return response.json() as Promise<LocalWorkspaceRootInfo>;
}

export async function sendFeishuNotification(
  apiUrl: string,
  chatId: string,
  text: string,
  fetchImpl: typeof fetch = fetch
): Promise<FeishuNotificationResult> {
  return postJson<FeishuNotificationResult>(apiUrl, "/feishu/notify", { chatId, text }, fetchImpl);
}

export async function startGitHubOAuth(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<GitHubOAuthStartResult> {
  return postJson<GitHubOAuthStartResult>(apiUrl, "/github/oauth/start", {}, fetchImpl);
}

export async function startGitHubCliLogin(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<GitHubCliLoginStartResult> {
  return postJson<GitHubCliLoginStartResult>(apiUrl, "/github/cli-login/start", {}, fetchImpl);
}

export async function fetchGitHubOAuthConfig(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<GitHubOAuthConfigInfo> {
  return fetchJson<GitHubOAuthConfigInfo>(apiUrl, "/github/oauth/config", fetchImpl);
}

export async function fetchGitHubStatus(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<GitHubStatusInfo> {
  return fetchJson<GitHubStatusInfo>(apiUrl, "/github/status", fetchImpl);
}

export async function updateGitHubOAuthConfig(
  apiUrl: string,
  config: GitHubOAuthConfigUpdate,
  fetchImpl: typeof fetch = fetch
): Promise<GitHubOAuthConfigInfo> {
  const response = await fetchImpl(`${apiUrl.replace(/\/$/, "")}/github/oauth/config`, {
    method: "PUT",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(config)
  });
  if (!response.ok) {
    throw new Error(`GitHub OAuth config API failed: ${response.status}`);
  }
  return response.json() as Promise<GitHubOAuthConfigInfo>;
}

export async function fetchGitHubRepoInfo(
  apiUrl: string,
  owner: string,
  repo: string,
  fetchImpl: typeof fetch = fetch
): Promise<GitHubRepositoryInfo> {
  return postJson<GitHubRepositoryInfo>(apiUrl, "/github/repo-info", { owner, repo }, fetchImpl);
}

export async function fetchGitHubRepositories(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<GitHubRepositoryInfo[]> {
  return fetchJson<GitHubRepositoryInfo[]>(apiUrl, "/github/repositories", fetchImpl);
}

export async function createGitHubPullRequest(
  apiUrl: string,
  input: GitHubPullRequestInput,
  fetchImpl: typeof fetch = fetch
): Promise<GitHubPullRequestResult> {
  return postJson<GitHubPullRequestResult>(apiUrl, "/github/create-pr", input, fetchImpl);
}

export async function fetchGitHubPullRequestStatus(
  apiUrl: string,
  input: GitHubPullRequestStatusInput,
  fetchImpl: typeof fetch = fetch
): Promise<GitHubPullRequestStatusResult> {
  return postJson<GitHubPullRequestStatusResult>(apiUrl, "/github/pr-status", input, fetchImpl);
}

export async function decomposeRequirement(
  apiUrl: string,
  input: RequirementDecompositionInput,
  fetchImpl: typeof fetch = fetch
): Promise<RequirementDecompositionResult> {
  return postJson<RequirementDecompositionResult>(apiUrl, "/requirements/decompose", input, fetchImpl);
}

export async function fetchPipelines(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<PipelineRecordInfo[]> {
  return fetchJson<PipelineRecordInfo[]>(apiUrl, "/pipelines", fetchImpl);
}

export async function fetchAttempts(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<AttemptRecordInfo[]> {
  return fetchJson<AttemptRecordInfo[]>(apiUrl, "/attempts", fetchImpl);
}

export async function fetchProofRecords(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<ProofRecordInfo[]> {
  return fetchJson<ProofRecordInfo[]>(apiUrl, "/proof-records", fetchImpl);
}

export async function fetchRequirements(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<RequirementRecordInfo[]> {
  return fetchJson<RequirementRecordInfo[]>(apiUrl, "/requirements", fetchImpl);
}

export async function fetchCheckpoints(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<CheckpointRecordInfo[]> {
  return fetchJson<CheckpointRecordInfo[]>(apiUrl, "/checkpoints", fetchImpl);
}

export async function fetchOperations(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<OperationRecordInfo[]> {
  return fetchJson<OperationRecordInfo[]>(apiUrl, "/operations", fetchImpl);
}

export async function fetchExecutionLocks(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<ExecutionLockInfo[]> {
  return fetchJson<ExecutionLockInfo[]>(apiUrl, "/execution-locks", fetchImpl);
}

export async function releaseExecutionLock(
  apiUrl: string,
  lockId: string,
  fetchImpl: typeof fetch = fetch
): Promise<ExecutionLockInfo> {
  return postJson<ExecutionLockInfo>(apiUrl, `/execution-locks/${encodeURIComponent(lockId)}/release`, {}, fetchImpl);
}

export async function createPipelineFromTemplate(
  apiUrl: string,
  templateId: string,
  item: unknown,
  fetchImpl: typeof fetch = fetch
): Promise<PipelineRecordInfo> {
  return postJson<PipelineRecordInfo>(apiUrl, "/pipelines/from-template", { templateId, item }, fetchImpl);
}

export async function startPipeline(
  apiUrl: string,
  pipelineId: string,
  fetchImpl: typeof fetch = fetch
): Promise<PipelineRecordInfo> {
  return postJson<PipelineRecordInfo>(apiUrl, `/pipelines/${pipelineId}/start`, {}, fetchImpl);
}

export async function runCurrentPipelineStage(
  apiUrl: string,
  pipelineId: string,
  runner: "local-proof" | "demo-code" | "codex" = "local-proof",
  fetchImpl: typeof fetch = fetch
): Promise<RunCurrentStageResult> {
  return postJson<RunCurrentStageResult>(apiUrl, `/pipelines/${pipelineId}/run-current-stage`, { runner }, fetchImpl);
}

export async function runDevFlowCycle(
  apiUrl: string,
  pipelineId: string,
  fetchImpl: typeof fetch = fetch
): Promise<RunDevFlowCycleResult> {
  return postJson<RunDevFlowCycleResult>(
    apiUrl,
    `/pipelines/${pipelineId}/run-devflow-cycle`,
    { autoApproveHuman: false, autoMerge: false },
    fetchImpl
  );
}

export interface OrchestratorTickInput {
  repositoryTargetId?: string;
  limit?: string;
  autoRun?: boolean;
  autoApproveHuman?: boolean;
  autoMerge?: boolean;
}

export interface OrchestratorTickResult {
  status: string;
  repositoryTargetId?: string;
  reason?: string;
  workItem?: unknown;
  pipeline?: unknown;
  lock?: unknown;
  runResult?: RunDevFlowCycleResult;
}

export interface OrchestratorWatcherInfo {
  id: string;
  repositoryTargetId: string;
  status: "active" | "paused" | string;
  intervalSeconds: number;
  limit?: string;
  autoRun: boolean;
  autoApproveHuman: boolean;
  autoMerge: boolean;
  lastTickAt?: string;
  lastTickStatus?: string;
  lastTickReason?: string;
  lastError?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface OrchestratorWatcherUpdate {
  status: "active" | "paused";
  intervalSeconds?: number;
  limit?: string;
  autoRun?: boolean;
  autoApproveHuman?: boolean;
  autoMerge?: boolean;
}

export async function runOrchestratorTick(
  apiUrl: string,
  input: OrchestratorTickInput,
  fetchImpl: typeof fetch = fetch
): Promise<OrchestratorTickResult> {
  return postJson<OrchestratorTickResult>(apiUrl, "/orchestrator/tick", input, fetchImpl);
}

export async function fetchOrchestratorWatchers(
  apiUrl: string,
  fetchImpl: typeof fetch = fetch
): Promise<OrchestratorWatcherInfo[]> {
  return fetchJson<OrchestratorWatcherInfo[]>(apiUrl, "/orchestrator/watchers", fetchImpl);
}

export async function updateOrchestratorWatcher(
  apiUrl: string,
  repositoryTargetId: string,
  input: OrchestratorWatcherUpdate,
  fetchImpl: typeof fetch = fetch
): Promise<OrchestratorWatcherInfo> {
  return putJson<OrchestratorWatcherInfo>(
    apiUrl,
    `/orchestrator/watchers/${encodeURIComponent(repositoryTargetId)}`,
    input,
    fetchImpl
  );
}

export async function approveCheckpoint(
  apiUrl: string,
  checkpointId: string,
  reviewer = "human",
  fetchImpl: typeof fetch = fetch
): Promise<CheckpointRecordInfo> {
  return postJson<CheckpointRecordInfo>(apiUrl, `/checkpoints/${checkpointId}/approve`, { reviewer }, fetchImpl);
}

export async function requestCheckpointChanges(
  apiUrl: string,
  checkpointId: string,
  reason: string,
  fetchImpl: typeof fetch = fetch
): Promise<CheckpointRecordInfo> {
  return postJson<CheckpointRecordInfo>(apiUrl, `/checkpoints/${checkpointId}/request-changes`, { reason }, fetchImpl);
}
