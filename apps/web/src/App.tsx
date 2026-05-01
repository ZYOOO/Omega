import { type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import {
  buildAuthorizeUrl,
  connectionProviders,
  createActivityFeed,
  createMissionFromRun,
  createSampleRun,
  createManualWorkItem,
  createWorkboardView,
  applyMissionControlEvents,
  groupWorkItemsByStatus,
  grantProviderConnection,
  loadWorkspaceSession,
  revokeProviderConnection,
  saveWorkspaceSession,
  titleFromMarkdownDescription,
  updateWorkItemPriority,
  updateWorkItemStatus
} from "./core";
import { runOperationViaMissionControlApi } from "./missionControlApiClient";
import type { MissionControlRunnerPreset } from "./missionControlApiClient";
import { navigateToExternalUrl } from "./browserNavigation";
import { openExternalUrlInNewTab } from "./browserNavigation";
import { retryReasonForAttempt } from "./attemptRetryReason";
import { PagePilotPreview } from "./components/PagePilotPreview";
import { PortalHome } from "./components/PortalHome";
import { ProjectSurface } from "./components/ProjectSurface";
import { RequirementComposer } from "./components/RequirementComposer";
import { WorkspaceChrome, type PrimaryNav, type UiTheme } from "./components/WorkspaceChrome";
import { WorkItemDetailPage } from "./components/WorkItemDetailPage";
import { isWorkItemDetailHash, parseWorkItemDetailHash, workItemDetailHash } from "./workItemRoutes";
import {
  applyPagePilotInstruction,
  approveCheckpoint,
  createPipelineFromTemplate,
  deliverPagePilotChange,
  discardPagePilotRun,
  fetchAttempts,
  fetchAttemptTimeline,
  fetchExecutionLocks,
  fetchCheckpoints,
  fetchAgentDefinitions,
  fetchGitHubOAuthConfig,
  fetchGitHubPullRequestStatus,
  fetchGitHubRepositories,
  fetchLocalCapabilities,
  fetchLocalWorkspaceRoot,
  fetchGitHubStatus,
  fetchLlmProviderSelection,
  fetchLlmProviders,
  fetchObservability,
  fetchOperations,
  fetchOrchestratorWatchers,
  fetchPipelines,
  fetchPipelineTemplates,
  fetchPagePilotRuns,
  fetchProofRecords,
  fetchProjectAgentProfile,
  fetchRequirements,
  fetchRunWorkpads,
  fetchRuntimeLogs,
  patchRunWorkpad,
  releaseExecutionLock,
  requestCheckpointChanges,
  retryAttempt,
  runCurrentPipelineStage,
  runDevFlowCycle,
  sendFeishuNotification,
  startGitHubCliLogin,
  startPipeline,
  startGitHubOAuth,
  updateGitHubOAuthConfig,
  updateLocalWorkspaceRoot,
  updateLlmProviderSelection,
  updateOrchestratorWatcher,
  updateProjectAgentProfile,
  type AgentDefinitionInfo,
  type AttemptRecordInfo,
  type AttemptTimelineInfo,
  type CheckpointRecordInfo,
  type ExecutionLockInfo,
  type GitHubOAuthConfigInfo,
  type GitHubPullRequestStatusResult,
  type GitHubStatusInfo,
  type GitHubRepositoryInfo,
  type LocalCapabilityInfo,
  type LlmProviderInfo,
  type LlmProviderSelection,
  type ObservabilitySummary,
  type OperationRecordInfo,
  type PagePilotSelectionContext,
  type PatchRunWorkpadInput,
  type OrchestratorWatcherInfo,
  type PipelineRecordInfo,
  type PipelineTemplateInfo,
  type ProjectAgentProfileInfo,
  type ProofRecordInfo,
  type RequirementRecordInfo,
  type RunWorkpadRecordInfo,
  type RuntimeLogRecordInfo
} from "./omegaControlApiClient";
import {
  bindGitHubRepositoryTargetViaApi,
  createWorkItemViaApi,
  deleteWorkItemViaApi,
  deleteRepositoryTargetViaApi,
  fetchMissionFromWorkItem,
  fetchWorkspaceSession,
  importGitHubIssuesViaApi,
  patchWorkItemViaApi,
  runOperationViaWorkspaceApi,
  saveWorkspaceSessionViaApi
} from "./workspaceApiClient";
import type { ConnectionProvider, ProjectRecord, ProviderId, WorkItem, WorkItemPriority, WorkItemStatus, WorkboardViewSort } from "./core";
import "./styles.css";

type InspectorPanel = "properties" | "provider";
type AppSurface = "home" | "workboard";
type AgentConfigTab = "workflow" | "agents" | "runtime";
type RuntimeConfigTab = "omega" | "codex" | "claude";
type AgentProfileDraft = {
  id: string;
  label: string;
  runner: MissionControlRunnerPreset | "opencode" | "claude-code";
  model: string;
  skills: string;
  mcp: string;
  stageNotes: string;
  codexPolicy: string;
  claudePolicy: string;
};
type AgentConfigurationDraft = {
  projectId?: string;
  repositoryTargetId?: string;
  runner: MissionControlRunnerPreset | "opencode" | "claude-code";
  workflowTemplate: string;
  workflowMarkdown: string;
  stagePolicy: string;
  skillAllowlist: string;
  mcpAllowlist: string;
  codexPolicy: string;
  claudePolicy: string;
  agentProfiles: AgentProfileDraft[];
};

const agentRunnerOptions: Array<{
  value: AgentProfileDraft["runner"];
  label: string;
  capabilityId?: string;
  setupHint?: string;
}> = [
  { value: "codex", label: "Codex", capabilityId: "codex", setupHint: "Install codex or switch this Agent to an available runner." },
  { value: "opencode", label: "opencode", capabilityId: "opencode", setupHint: "Install opencode or choose Codex." },
  { value: "claude-code", label: "Claude Code", capabilityId: "claude-code", setupHint: "Install Claude Code CLI or choose Codex." },
  { value: "demo-code", label: "demo-code", capabilityId: "git", setupHint: "demo-code needs git." },
  { value: "local-proof", label: "local-proof" }
];

function InfoIcon() {
  return (
    <svg className="info-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <circle cx="12" cy="12" r="9" />
      <path d="M12 10.8v5.2" />
      <path d="M12 7.5h.01" />
    </svg>
  );
}

function initialAppSurface(): AppSurface {
  if (typeof window === "undefined") return "home";
  if (window.location.hash === "#home") return "home";
  if (window.location.hash === "#workboard") return "workboard";
  if (window.location.hash === "#page-pilot") return "workboard";
  if (isWorkItemDetailHash(window.location.hash)) return "workboard";
  return import.meta.env.MODE === "test" ? "workboard" : "home";
}

function initialActiveNav(savedNav: PrimaryNav): PrimaryNav {
  if (typeof window !== "undefined" && isWorkItemDetailHash(window.location.hash)) return "Issues";
  if (typeof window !== "undefined" && window.location.hash === "#page-pilot") return "Page Pilot";
  return savedNav;
}

function initialWorkItemDetailId(): string {
  if (typeof window === "undefined") return "";
  return parseWorkItemDetailHash(window.location.hash);
}

function initialUiTheme(): UiTheme {
  if (typeof window === "undefined") return "light";
  const saved = window.localStorage.getItem("omega-ui-theme");
  return saved === "dark" ? "dark" : "light";
}

const agentConfigurationStorageKey = "omega-agent-configuration-draft";

const defaultWorkflowMarkdown = `workflow: devflow-pr
stages:
  - requirement: requirement
  - implementation: architect + coding + testing
  - code_review: review
  - rework: coding + testing, then code_review
  - human_review: human gate
  - delivery: delivery
artifacts:
  - requirement
  - solution
  - diff
  - test-report
  - review-report
  - handoff-bundle`;

const defaultAgentProfiles: AgentProfileDraft[] = [
  {
    id: "requirement",
    label: "Requirement",
    runner: "codex",
    model: "gpt-5.4-mini",
    skills: "github:github\nbrowser-use",
    mcp: "github\nfilesystem:repository-workspace",
    stageNotes: "Clarify acceptance criteria, repository target, risks, and suggested work items before planning.",
    codexPolicy: "read requirement source, inspect repository context, write requirement artifact only",
    claudePolicy: "focus on ambiguity, acceptance criteria, and handoff clarity"
  },
  {
    id: "architect",
    label: "Architect",
    runner: "codex",
    model: "gpt-5.4-mini",
    skills: "github:github",
    mcp: "filesystem:repository-workspace",
    stageNotes: "Map affected files, data flow, integration risks, and verification plan.",
    codexPolicy: "prefer read-only analysis, generate solution-plan.md, no source edits unless explicitly allowed",
    claudePolicy: "produce concise architecture notes with file-level impact"
  },
  {
    id: "coding",
    label: "Coding",
    runner: "codex",
    model: "gpt-5.4-mini",
    skills: "github:gh-fix-ci\nbrowser-use",
    mcp: "filesystem:repository-workspace\nbrowser:localhost-preview",
    stageNotes: "Implement inside the locked repository workspace and keep changes scoped to the Work Item.",
    codexPolicy: "workspace-write only, never write outside repositoryTarget workspace, emit diff and summary",
    claudePolicy: "apply code edits conservatively and preserve existing project style"
  },
  {
    id: "testing",
    label: "Testing",
    runner: "codex",
    model: "gpt-5.4-mini",
    skills: "browser-use",
    mcp: "filesystem:repository-workspace\nbrowser:localhost-preview",
    stageNotes: "Run focused tests first, then broader checks when shared contracts changed.",
    codexPolicy: "run configured validation commands, capture test-report.md and failure traces",
    claudePolicy: "summarize validation evidence and remaining risk"
  },
  {
    id: "review",
    label: "Review",
    runner: "codex",
    model: "gpt-5.4-mini",
    skills: "github:github\ngithub:gh-fix-ci",
    mcp: "github\nfilesystem:repository-workspace",
    stageNotes: "Review correctness, safety, tests, and contract drift. Changes requested routes to Rework.",
    codexPolicy: "read diff and artifacts, write review report, do not mark attempt failed for changes_requested",
    claudePolicy: "return clear verdict, required fixes, and evidence"
  },
  {
    id: "delivery",
    label: "Delivery",
    runner: "codex",
    model: "gpt-5.4-mini",
    skills: "github:yeet\ngithub:github",
    mcp: "github\nfilesystem:repository-workspace",
    stageNotes: "After human approval, prepare PR or delivery proof and final handoff bundle.",
    codexPolicy: "require human gate approval before merge or delivery action",
    claudePolicy: "summarize shipped changes, verification, and caveats"
  }
];

const defaultAgentConfigurationDraft: AgentConfigurationDraft = {
  runner: "codex",
  workflowTemplate: "devflow-pr",
  workflowMarkdown: defaultWorkflowMarkdown,
  stagePolicy:
    "Requirement: clarify acceptance criteria and repository target before planning.\nArchitecture: list affected files and risky integration points.\nCoding: only edit inside the bound repository workspace.\nTesting: run focused tests first, then broaden when shared contracts change.\nReview: changes_requested must route to Rework, not fail the attempt.\nHuman Review: stop until explicit approval.",
  skillAllowlist: "browser-use\ngithub:github\ngithub:gh-fix-ci\ngithub:yeet",
  mcpAllowlist: "github\nfilesystem:repository-workspace\nbrowser:localhost-preview",
  codexPolicy:
    "sandbox: workspace-write\napproval: never inside automated stage\nrepo-scope: require repositoryTargetId match\nartifacts: requirement, solution, diff, test-report, review-report, handoff-bundle",
  claudePolicy:
    "workspace: repository target only\nreview: explain assumptions before edits\nhandoff: keep artifact names compatible with Omega workflow",
  agentProfiles: defaultAgentProfiles
};

function initialAgentConfigurationDraft(): AgentConfigurationDraft {
  if (typeof window === "undefined") return defaultAgentConfigurationDraft;
  const saved = window.localStorage.getItem(agentConfigurationStorageKey);
  if (!saved) return defaultAgentConfigurationDraft;
  try {
    const parsed = JSON.parse(saved) as Partial<AgentConfigurationDraft>;
    return normalizeAgentConfigurationDraft(parsed);
  } catch {
    return defaultAgentConfigurationDraft;
  }
}

function normalizeAgentConfigurationDraft(profile: Partial<ProjectAgentProfileInfo> | Partial<AgentConfigurationDraft>): AgentConfigurationDraft {
  const rawProfile = profile as Partial<ProjectAgentProfileInfo> & Partial<AgentConfigurationDraft>;
  return {
    ...defaultAgentConfigurationDraft,
    ...rawProfile,
    runner: (rawProfile.runner as AgentConfigurationDraft["runner"]) || defaultAgentConfigurationDraft.runner,
    agentProfiles: rawProfile.agentProfiles?.length
      ? rawProfile.agentProfiles.map((agent) => ({
          ...agent,
          runner: agent.runner as AgentProfileDraft["runner"]
        }))
      : defaultAgentProfiles
  };
}

function capabilityAvailable(capabilities: LocalCapabilityInfo[], capabilityId?: string) {
  if (!capabilityId || capabilities.length === 0) return true;
  return capabilities.some((capability) => capability.id === capabilityId && capability.available);
}

function runnerOptionFor(runner: string) {
  return agentRunnerOptions.find((option) => option.value === runner);
}

function runnerAvailabilityLabel(runner: string, capabilities: LocalCapabilityInfo[]) {
  const option = runnerOptionFor(runner);
  if (!option) return `Unsupported runner: ${runner}`;
  if (capabilityAvailable(capabilities, option.capabilityId)) return `${option.label} ready`;
  return option.setupHint ?? `${option.label} is not available.`;
}

function unavailableAgentProfiles(profile: AgentConfigurationDraft, capabilities: LocalCapabilityInfo[]) {
  if (capabilities.length === 0) return [];
  return profile.agentProfiles.filter((agent) => {
    const option = runnerOptionFor(agent.runner);
    return !option || !capabilityAvailable(capabilities, option.capabilityId);
  });
}

const providerClientIds: Partial<Record<ProviderId, string>> = {
 github: import.meta.env.VITE_GITHUB_CLIENT_ID,
  google: import.meta.env.VITE_GOOGLE_CLIENT_ID
};

const missionControlApiUrl = import.meta.env.VITE_MISSION_CONTROL_API_URL || (import.meta.env.DEV ? "/api" : "");

const defaultGitHubOAuthConfig: GitHubOAuthConfigInfo = {
  configured: false,
  clientId: "",
  redirectUri: "http://127.0.0.1:3888/auth/github/callback",
  tokenUrl: "https://github.com/login/oauth/access_token",
  secretConfigured: false,
  source: "empty"
};

const visibleConnectionProviders = connectionProviders.filter((provider) =>
  ["github", "feishu", "google", "ci"].includes(provider.id)
);

function agentShortLabel(agentId: string): string {
  const labels: Record<string, string> = {
    master: "Master",
    requirement: "Req",
    architect: "Arch",
    coding: "Code",
    testing: "Test",
    review: "Rev",
    delivery: "Ship"
  };
  return labels[agentId] ?? agentId.slice(0, 4);
}

function statusClassName(status: WorkItemStatus): string {
  return `status-${status.toLowerCase().replace(/\s+/g, "-")}`;
}

function workItemStatusLabel(status: WorkItemStatus): string {
  if (status === "Ready") return "Not started";
  if (status === "In Review") return "Running";
  return status;
}

function pipelineStageLabel(status: string): string {
  const labels: Record<string, string> = {
    ready: "Ready",
    waiting: "Waiting",
    running: "Running",
    "needs-human": "Review",
    passed: "Done",
    "changes-requested": "Changes requested",
    failed: "Failed",
    blocked: "Blocked"
  };
  return labels[status] ?? status;
}

function pipelineStageClassName(status: string): string {
  return `stage-${status.toLowerCase().replace(/\s+/g, "-")}`;
}

function attemptStatusLabel(status: string): string {
  const labels: Record<string, string> = {
    running: "Running",
    "waiting-human": "Waiting for review",
    "changes-requested": "Changes requested",
    failed: "Failed",
    done: "Done",
    completed: "Done",
    canceled: "Canceled"
  };
  return labels[status] ?? status;
}

function sourceLabel(item: WorkItem): string {
  if (item.source === "github_issue") return "GitHub";
  if (item.source === "feishu_message") return "Feishu";
  if (item.source === "ai_generated") return "AI";
  return "Omega";
}

function issueNumberFromReference(item: WorkItem): string {
  const sourceNumber = item.sourceExternalRef?.match(/#(\d+)$/)?.[1];
  if (sourceNumber) return sourceNumber;
  return item.key.match(/(?:GH|OMG)-(\d+)$/)?.[1] ?? "";
}

function workItemDisplayLabel(item: WorkItem): string {
  const number = issueNumberFromReference(item);
  if (item.source === "github_issue") return number ? `GitHub #${number}` : "GitHub issue";
  if (item.source === "feishu_message") return number ? `Feishu item ${number}` : "Feishu item";
  if (item.source === "ai_generated") return number ? `AI item ${number}` : "AI item";
  return number ? `Work item ${number}` : "Work item";
}

function displayText(value: string): string {
  return value.replace(/\\n/g, "\n");
}

function isCompletedWork(item: WorkItem, pipeline?: PipelineRecordInfo): boolean {
  return runtimeStatusForWorkItem(item, pipeline) === "Done" || pipeline?.status === "done" || pipeline?.status === "delivered";
}

function isFailedWork(item: WorkItem, pipeline?: PipelineRecordInfo): boolean {
  return runtimeStatusForWorkItem(item, pipeline) === "Blocked" || pipeline?.status === "failed" || pipeline?.status === "discarded";
}

function isPagePilotWorkItem(item: WorkItem): boolean {
  return item.labels.includes("page-pilot") || item.sourceExternalRef?.startsWith("page-pilot:") === true;
}

function runtimeStatusForWorkItem(item: WorkItem, pipeline?: PipelineRecordInfo): WorkItemStatus {
  if (!isPagePilotWorkItem(item)) return item.status;
  if (pipeline?.status === "waiting-human") return "In Review";
  if (pipeline?.status === "discarded" || pipeline?.status === "failed") return "Blocked";
  if (pipeline?.status === "delivered" || pipeline?.status === "done") return "Done";
  return item.status;
}

function applyRuntimeWorkItemStatus(item: WorkItem, pipeline?: PipelineRecordInfo): WorkItem {
  const status = runtimeStatusForWorkItem(item, pipeline);
  return status === item.status ? item : { ...item, status };
}

function operationStatusLabel(status: string): string {
  const labels: Record<string, string> = {
    ready: "Ready",
    running: "Running",
    passed: "Passed",
    done: "Done",
    "changes-requested": "Changes requested",
    "needs-human": "Needs human input",
    "waiting-human": "Waiting for review",
    failed: "Failed",
    blocked: "Blocked"
  };
  return labels[status] ?? status;
}

function summarizePipelineProgress(
  item: WorkItem,
  stages: NonNullable<PipelineRecordInfo["run"]>["stages"] = [],
  running: boolean
) {
  if (!stages.length) {
    return {
      label: running || item.status === "Planning" ? "Preparing pipeline" : workItemStatusLabel(item.status),
      percent: running || item.status === "Planning" ? 14 : item.status === "Done" ? 100 : 0,
      status: running || item.status === "Planning" ? "running" : item.status.toLowerCase()
    };
  }

  const activeIndex = stages.findIndex((stage) =>
    ["running", "needs-human", "changes-requested", "failed", "blocked", "ready"].includes(stage.status)
  );
  const safeIndex = activeIndex >= 0 ? activeIndex : stages.length - 1;
  const currentStage = stages[safeIndex];
  const passedCount = stages.filter((stage) => stage.status === "passed" || stage.status === "done").length;
  const percent =
    passedCount >= stages.length
      ? 100
      : Math.max(8, Math.round(((passedCount + (currentStage.status === "running" ? 0.55 : 0.25)) / stages.length) * 100));
  const agentIds = currentStage.agentIds ?? (currentStage.agentId ? [currentStage.agentId] : []);

  return {
    label: currentStage.status === "passed" || currentStage.status === "done"
      ? agentIds.length
        ? agentIds.map(agentShortLabel).join(" + ")
        : "Complete"
      : currentStage.title ?? currentStage.id,
    percent,
    status: currentStage.status
  };
}

function safeMarkdownHref(href: string): string | undefined {
  return /^(https?:|mailto:)/i.test(href) ? href : undefined;
}

function renderInlineMarkdown(text: string, keyPrefix: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  const pattern = /(`[^`]+`|\*\*[^*]+\*\*|\[[^\]]+\]\([^)]+\))/g;
  let cursor = 0;
  let match: RegExpExecArray | null;

  while ((match = pattern.exec(text))) {
    if (match.index > cursor) {
      nodes.push(text.slice(cursor, match.index));
    }

    const token = match[0];
    const key = `${keyPrefix}-${match.index}`;
    if (token.startsWith("`")) {
      nodes.push(<code key={key}>{token.slice(1, -1)}</code>);
    } else if (token.startsWith("**")) {
      nodes.push(<strong key={key}>{token.slice(2, -2)}</strong>);
    } else {
      const linkMatch = token.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
      const href = linkMatch ? safeMarkdownHref(linkMatch[2]) : undefined;
      nodes.push(
        href ? (
          <a key={key} href={href} target="_blank" rel="noreferrer">
            {linkMatch?.[1]}
          </a>
        ) : (
          <span key={key}>{linkMatch?.[1] ?? token}</span>
        )
      );
    }

    cursor = match.index + token.length;
  }

  if (cursor < text.length) {
    nodes.push(text.slice(cursor));
  }
  return nodes;
}

function renderMarkdown(value: string): ReactNode[] {
  const lines = displayText(value).split("\n");
  const nodes: ReactNode[] = [];
  let index = 0;

  while (index < lines.length) {
    const line = lines[index];
    const trimmed = line.trim();
    if (!trimmed) {
      index += 1;
      continue;
    }

    const heading = trimmed.match(/^(#{1,3})\s+(.+)$/);
    if (heading) {
      const level = heading[1].length;
      const content = renderInlineMarkdown(heading[2], `h-${index}`);
      nodes.push(level === 1 ? <h1 key={index}>{content}</h1> : level === 2 ? <h2 key={index}>{content}</h2> : <h3 key={index}>{content}</h3>);
      index += 1;
      continue;
    }

    if (trimmed.startsWith("```")) {
      const codeLines: string[] = [];
      index += 1;
      while (index < lines.length && !lines[index].trim().startsWith("```")) {
        codeLines.push(lines[index]);
        index += 1;
      }
      if (index < lines.length) index += 1;
      nodes.push(
        <pre key={index}>
          <code>{codeLines.join("\n")}</code>
        </pre>
      );
      continue;
    }

    if (/^[-*]\s+/.test(trimmed)) {
      const items: string[] = [];
      while (index < lines.length && /^[-*]\s+/.test(lines[index].trim())) {
        items.push(lines[index].trim().replace(/^[-*]\s+/, ""));
        index += 1;
      }
      nodes.push(
        <ul key={index}>
          {items.map((item, itemIndex) => (
            <li key={`${index}-${itemIndex}`}>{renderInlineMarkdown(item, `ul-${index}-${itemIndex}`)}</li>
          ))}
        </ul>
      );
      continue;
    }

    if (/^\d+\.\s+/.test(trimmed)) {
      const items: string[] = [];
      while (index < lines.length && /^\d+\.\s+/.test(lines[index].trim())) {
        items.push(lines[index].trim().replace(/^\d+\.\s+/, ""));
        index += 1;
      }
      nodes.push(
        <ol key={index}>
          {items.map((item, itemIndex) => (
            <li key={`${index}-${itemIndex}`}>{renderInlineMarkdown(item, `ol-${index}-${itemIndex}`)}</li>
          ))}
        </ol>
      );
      continue;
    }

    if (trimmed.startsWith(">")) {
      const quoteLines: string[] = [];
      while (index < lines.length && lines[index].trim().startsWith(">")) {
        quoteLines.push(lines[index].trim().replace(/^>\s?/, ""));
        index += 1;
      }
      nodes.push(<blockquote key={index}>{renderInlineMarkdown(quoteLines.join(" "), `quote-${index}`)}</blockquote>);
      continue;
    }

    const paragraphLines: string[] = [];
    while (
      index < lines.length &&
      lines[index].trim() &&
      !/^(#{1,3})\s+/.test(lines[index].trim()) &&
      !lines[index].trim().startsWith("```") &&
      !/^[-*]\s+/.test(lines[index].trim()) &&
      !/^\d+\.\s+/.test(lines[index].trim()) &&
      !lines[index].trim().startsWith(">")
    ) {
      paragraphLines.push(lines[index].trim());
      index += 1;
    }
    nodes.push(<p key={index}>{renderInlineMarkdown(paragraphLines.join(" "), `p-${index}`)}</p>);
  }

  return nodes;
}

function emptyObservability(): ObservabilitySummary {
  return {
    counts: {
      workItems: 0,
      pipelines: 0,
      checkpoints: 0,
      missions: 0,
      operations: 0,
      proofRecords: 0,
      events: 0,
      runtimeLogs: 0
    },
    pipelineStatus: {},
    checkpointStatus: {},
    operationStatus: {},
    workItemStatus: {},
    attention: { waitingHuman: 0, failed: 0, blocked: 0 },
    recentErrors: []
  };
}

function formatShortTimestamp(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false
  }).format(date);
}

function App() {
  const run = useMemo(() => createSampleRun(), []);
  const persistedSession = useMemo(() => loadWorkspaceSession(run), [run.id]);
  const [appSurface, setAppSurface] = useState<AppSurface>(() => initialAppSurface());
  const [uiTheme, setUiTheme] = useState<UiTheme>(() => initialUiTheme());
  const [activeNav, setActiveNav] = useState<PrimaryNav>(() => initialActiveNav(persistedSession.activeNav));
  const [connections, setConnections] = useState(persistedSession.connections);
  const [selectedProviderId, setSelectedProviderId] = useState<ProviderId>(persistedSession.selectedProviderId);
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [activeInspectorPanel, setActiveInspectorPanel] = useState<InspectorPanel>(persistedSession.activeInspectorPanel);
  const [projects, setProjects] = useState<ProjectRecord[]>(persistedSession.projects);
  const [requirements, setRequirements] = useState<RequirementRecordInfo[]>(persistedSession.requirements);
  const [workItems, setWorkItems] = useState<WorkItem[]>(persistedSession.workItems);
  const [selectedWorkItemId, setSelectedWorkItemId] = useState(persistedSession.selectedWorkItemId);
  const [activeWorkItemDetailId, setActiveWorkItemDetailId] = useState(() => initialWorkItemDetailId());
  const [newItemTitle, setNewItemTitle] = useState("");
  const [newItemDescription, setNewItemDescription] = useState("");
  const [newItemAssignee, setNewItemAssignee] = useState("requirement");
  const [newItemTarget, setNewItemTarget] = useState("");
  const [showInlineCreate, setShowInlineCreate] = useState(false);
  const [createComposerExpanded, setCreateComposerExpanded] = useState(false);
  const [createDescriptionMode, setCreateDescriptionMode] = useState<"write" | "preview">("write");
  const [isCreatingItem, setIsCreatingItem] = useState(false);
  const creatingItemRef = useRef(false);
  const [runnerMessage, setRunnerMessage] = useState("");
  const [runnerPreset, setRunnerPreset] = useState<MissionControlRunnerPreset>(persistedSession.runnerPreset);
  const [searchQuery, setSearchQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<"All" | WorkItemStatus>(persistedSession.statusFilter);
  const [assigneeFilter, setAssigneeFilter] = useState(persistedSession.assigneeFilter);
  const [sortDirection, setSortDirection] = useState<WorkboardViewSort["direction"]>(persistedSession.sortDirection);
  const [missionState, setMissionState] = useState(persistedSession.missionState);
  const [collapsedGroups, setCollapsedGroups] = useState<WorkItemStatus[]>(persistedSession.collapsedGroups);
  const [githubIssuesCollapsed, setGithubIssuesCollapsed] = useState(false);
  const [workspaceLoaded, setWorkspaceLoaded] = useState(!missionControlApiUrl);
  const [observability, setObservability] = useState<ObservabilitySummary>(() => emptyObservability());
  const [llmProviders, setLlmProviders] = useState<LlmProviderInfo[]>([]);
  const [llmSelection, setLlmSelection] = useState<LlmProviderSelection>({
    providerId: "openai",
    model: "gpt-5.4-mini",
    reasoningEffort: "medium"
  });
  const [pipelines, setPipelines] = useState<PipelineRecordInfo[]>([]);
  const [attempts, setAttempts] = useState<AttemptRecordInfo[]>([]);
  const [activeAttemptTimeline, setActiveAttemptTimeline] = useState<AttemptTimelineInfo | null>(null);
  const [activePullRequestStatus, setActivePullRequestStatus] = useState<GitHubPullRequestStatusResult | null>(null);
  const [proofRecords, setProofRecords] = useState<ProofRecordInfo[]>([]);
  const [runWorkpads, setRunWorkpads] = useState<RunWorkpadRecordInfo[]>([]);
  const [checkpoints, setCheckpoints] = useState<CheckpointRecordInfo[]>([]);
  const [operations, setOperations] = useState<OperationRecordInfo[]>([]);
  const [runtimeLogs, setRuntimeLogs] = useState<RuntimeLogRecordInfo[]>([]);
  const [executionLocks, setExecutionLocks] = useState<ExecutionLockInfo[]>([]);
  const [orchestratorWatchers, setOrchestratorWatchers] = useState<OrchestratorWatcherInfo[]>([]);
  const [localCapabilities, setLocalCapabilities] = useState<LocalCapabilityInfo[]>([]);
  const [localWorkspaceRoot, setLocalWorkspaceRoot] = useState("");
  const [localWorkspaceRootDraft, setLocalWorkspaceRootDraft] = useState("");
  const [localRunner, setLocalRunner] = useState<MissionControlRunnerPreset>("local-proof");
  const [pipelineTemplates, setPipelineTemplates] = useState<PipelineTemplateInfo[]>([]);
  const [agentDefinitions, setAgentDefinitions] = useState<AgentDefinitionInfo[]>([]);
  const [githubOAuthConfig, setGitHubOAuthConfig] = useState<GitHubOAuthConfigInfo>(defaultGitHubOAuthConfig);
  const [githubOAuthDraft, setGitHubOAuthDraft] = useState({
    clientId: defaultGitHubOAuthConfig.clientId,
    clientSecret: "",
    redirectUri: defaultGitHubOAuthConfig.redirectUri
  });
  const [githubOAuthSetupOpen, setGitHubOAuthSetupOpen] = useState(false);
  const [providerFeedback, setProviderFeedback] = useState("");
  const [githubDeviceLoginUrl, setGitHubDeviceLoginUrl] = useState("");
  const [githubStatus, setGitHubStatus] = useState<GitHubStatusInfo | null>(null);
  const [githubRepoOwner, setGitHubRepoOwner] = useState("");
  const [githubRepoName, setGitHubRepoName] = useState("");
  const [githubRepoInfo, setGitHubRepoInfo] = useState<GitHubRepositoryInfo | null>(null);
  const [githubRepositories, setGitHubRepositories] = useState<GitHubRepositoryInfo[]>([]);
  const [githubRepositoryQuery, setGitHubRepositoryQuery] = useState("");
  const [githubRepositoriesLoading, setGitHubRepositoriesLoading] = useState(false);
  const [activeRepositoryWorkspaceTargetId, setActiveRepositoryWorkspaceTargetId] = useState("");
  const [syncingRepositoryKey, setSyncingRepositoryKey] = useState("");
  const [runningWorkItemId, setRunningWorkItemId] = useState("");
  const [repositorySyncMessage, setRepositorySyncMessage] = useState("");
  const [feishuChatId, setFeishuChatId] = useState("");
  const [agentConfigOpen, setAgentConfigOpen] = useState(false);
  const [agentConfigSavedMessage, setAgentConfigSavedMessage] = useState("");
  const [agentConfigDraft, setAgentConfigDraft] = useState<AgentConfigurationDraft>(initialAgentConfigurationDraft);
  const [agentConfigTab, setAgentConfigTab] = useState<AgentConfigTab>("workflow");
  const [selectedAgentProfileId, setSelectedAgentProfileId] = useState(defaultAgentProfiles[0].id);
  const [runtimeConfigTab, setRuntimeConfigTab] = useState<RuntimeConfigTab>("codex");
  const [workspaceFolderPickerMessage, setWorkspaceFolderPickerMessage] = useState("");
  const [workspaceSectionOpen, setWorkspaceSectionOpen] = useState(true);
  const [connectionsSectionOpen, setConnectionsSectionOpen] = useState(true);

  useEffect(() => {
    function syncDetailRoute() {
      const itemId = parseWorkItemDetailHash(window.location.hash);
      if (itemId) {
        setAppSurface("workboard");
        setActiveNav("Issues");
        setActiveWorkItemDetailId(itemId);
        setInspectorOpen(false);
        return;
      }
      if (window.location.hash === "#page-pilot") {
        setAppSurface("workboard");
        setActiveNav("Page Pilot");
        setActiveWorkItemDetailId("");
        setInspectorOpen(false);
        return;
      }
      if (window.location.hash === "#workboard" || window.location.hash === "#home") {
        setActiveWorkItemDetailId("");
      }
    }
    window.addEventListener("hashchange", syncDetailRoute);
    return () => window.removeEventListener("hashchange", syncDetailRoute);
  }, []);

  const primaryProject = projects[0];
  const repositoryTargets = primaryProject?.repositoryTargets ?? [];
  const repositoryTargetCount = repositoryTargets.length;
  const effectiveRepositoryWorkspaceTargetId =
    activeRepositoryWorkspaceTargetId ||
    (activeNav === "Issues" || activeNav === "Page Pilot" ? primaryProject?.defaultRepositoryTargetId ?? (repositoryTargets.length === 1 ? repositoryTargets[0].id : "") : "");
  const activeRepositoryWorkspace =
    repositoryTargets.find((target) => target.id === effectiveRepositoryWorkspaceTargetId) ?? undefined;
  const activeRepositoryWorkspaceLabel =
    activeRepositoryWorkspace?.kind === "github"
      ? `${activeRepositoryWorkspace.owner}/${activeRepositoryWorkspace.repo}`
      : activeRepositoryWorkspace?.path ?? "";
  const activeRepositoryWorkspaceTarget =
    activeRepositoryWorkspace?.kind === "github"
      ? activeRepositoryWorkspace.url ?? `https://github.com/${activeRepositoryWorkspace.owner}/${activeRepositoryWorkspace.repo}`
      : activeRepositoryWorkspace?.path;
  const pagePilotRepositoryTarget =
    activeRepositoryWorkspace ??
    repositoryTargets.find((target) => target.id === primaryProject?.defaultRepositoryTargetId) ??
    (repositoryTargets.length === 1 ? repositoryTargets[0] : undefined);
  const pagePilotRepositoryLabel =
    pagePilotRepositoryTarget?.kind === "github"
      ? `${pagePilotRepositoryTarget.owner}/${pagePilotRepositoryTarget.repo}`
      : pagePilotRepositoryTarget?.path ?? "";
  const pipelinesByWorkItemId = useMemo(() => {
    const byWorkItem = new Map<string, PipelineRecordInfo>();
    for (const pipeline of pipelines) {
      byWorkItem.set(pipeline.workItemId, pipeline);
    }
    return byWorkItem;
  }, [pipelines]);
  const displayWorkItems = useMemo(
    () => workItems.map((item) => applyRuntimeWorkItemStatus(item, pipelinesByWorkItemId.get(item.id))),
    [pipelinesByWorkItemId, workItems]
  );
  const activeRepositoryWorkspaceKey =
    activeRepositoryWorkspace?.kind === "github"
      ? `${activeRepositoryWorkspace.owner}/${activeRepositoryWorkspace.repo}`
      : activeRepositoryWorkspace?.id ?? "";
  const activeRepositoryWorkspaceItems = activeRepositoryWorkspace
    ? displayWorkItems.filter((item) => item.repositoryTargetId === activeRepositoryWorkspace.id)
    : [];
  const activeRepositoryGitHubItems = activeRepositoryWorkspaceItems.filter((item) => item.source === "github_issue");
  const watcherByRepositoryTargetId = useMemo(
    () => new Map(orchestratorWatchers.map((watcher) => [watcher.repositoryTargetId, watcher])),
    [orchestratorWatchers]
  );
  const activeRepositoryWatcher = activeRepositoryWorkspace ? watcherByRepositoryTargetId.get(activeRepositoryWorkspace.id) : undefined;
  const activeRepositoryWatcherActive = activeRepositoryWatcher?.status === "active";
  const scopedWorkItems =
    activeNav === "Issues" && activeRepositoryWorkspace ? activeRepositoryWorkspaceItems : displayWorkItems;

  const workboardView = useMemo(
    () =>
      createWorkboardView(scopedWorkItems, {
        filters: {
          status: statusFilter === "All" ? undefined : [statusFilter],
          assignee: assigneeFilter === "All" ? undefined : [assigneeFilter]
        },
        sort: { field: "priority", direction: sortDirection },
        display: ["key", "title", "status", "priority", "assignee"]
      }),
    [assigneeFilter, scopedWorkItems, sortDirection, statusFilter]
  );

  const filteredItems = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) {
      return workboardView.items;
    }
    return workboardView.items.filter((item) =>
      `${item.key} ${item.title} ${item.description}`.toLowerCase().includes(query)
    );
  }, [searchQuery, workboardView.items]);

  const groupedItems = useMemo(() => groupWorkItemsByStatus(filteredItems), [filteredItems]);
  const selectedWorkItem = scopedWorkItems.find((item) => item.id === selectedWorkItemId) ?? scopedWorkItems[0] ?? displayWorkItems[0];
  const activeWorkItemDetail = activeNav === "Issues"
    ? displayWorkItems.find((item) => item.id === activeWorkItemDetailId)
    : undefined;
  const selectedRequirement = selectedWorkItem?.requirementId
    ? requirements.find((requirement) => requirement.id === selectedWorkItem.requirementId)
    : undefined;
  function repositoryLabelForItem(item: WorkItem | undefined): string {
    if (!item) return "";
    const repositoryTarget = item.repositoryTargetId
      ? repositoryTargets.find((target) => target.id === item.repositoryTargetId)
      : undefined;
    if (repositoryTarget?.kind === "github") return `${repositoryTarget.owner}/${repositoryTarget.repo}`;
    if (repositoryTarget?.kind === "local") return repositoryTarget.path;
    return activeRepositoryWorkspaceLabel;
  }
  const activeDetailRepositoryTarget = activeWorkItemDetail?.repositoryTargetId
    ? repositoryTargets.find((target) => target.id === activeWorkItemDetail.repositoryTargetId)
    : undefined;
  const activeDetailGitHubRepositoryTarget =
    activeDetailRepositoryTarget?.kind === "github" ? activeDetailRepositoryTarget : undefined;
  const activeDetailRepositoryLabel = repositoryLabelForItem(activeWorkItemDetail);
  const activityFeed = useMemo(
    () => createActivityFeed({ events: missionState.events, syncIntents: missionState.syncIntents }),
    [missionState]
  );
  const assigneeOptions = useMemo(() => ["All", ...new Set(workItems.map((item) => item.assignee))], [workItems]);
  const selectedLlmProvider = llmProviders.find((provider) => provider.id === llmSelection.providerId);
  const importedGitHubItems = useMemo(
    () => workItems.filter((item) => item.source === "github_issue"),
    [workItems]
  );
  const filteredGitHubRepositories = useMemo(() => {
    const query = githubRepositoryQuery.trim().toLowerCase();
    if (!query) return githubRepositories;
    return githubRepositories.filter((repository) => {
      const name = repository.nameWithOwner ?? `${repository.owner?.login ?? ""}/${repository.name}`;
      return `${name} ${repository.description ?? ""}`.toLowerCase().includes(query);
    });
  }, [githubRepositories, githubRepositoryQuery]);
  const selectedRepositoryNameWithOwner =
    githubRepoOwner && githubRepoName ? `${githubRepoOwner}/${githubRepoName}` : "";
  const selectedRepositoryTargetId = selectedRepositoryNameWithOwner
    ? `repo_${selectedRepositoryNameWithOwner.replace("/", "_")}`
    : "";
  const selectedRepositoryTarget = repositoryTargets.find((target) => target.id === selectedRepositoryTargetId);
  const selectedRepositoryBound = Boolean(selectedRepositoryTarget);
  const activeRepositoryWorkspacePipelines = activeRepositoryWorkspace
    ? pipelines.filter((pipeline) =>
        displayWorkItems.some((item) => item.id === pipeline.workItemId && item.repositoryTargetId === activeRepositoryWorkspace.id)
      )
    : [];
  const activeDetailPipeline = activeWorkItemDetail
    ? pipelinesByWorkItemId.get(activeWorkItemDetail.id)
    : undefined;
  const attemptsByWorkItemId = useMemo(() => {
    const byWorkItem = new Map<string, AttemptRecordInfo[]>();
    for (const attempt of attempts) {
      const list = byWorkItem.get(attempt.itemId) ?? [];
      list.push(attempt);
      byWorkItem.set(attempt.itemId, list);
    }
    for (const list of byWorkItem.values()) {
      list.sort((left, right) => (right.startedAt ?? right.createdAt ?? "").localeCompare(left.startedAt ?? left.createdAt ?? ""));
    }
    return byWorkItem;
  }, [attempts]);
  const activeDetailAttempts = activeWorkItemDetail
    ? attemptsByWorkItemId.get(activeWorkItemDetail.id) ?? []
    : [];
  const activeDetailAttempt = activeDetailAttempts[0];
  const activeDetailCompleted = activeWorkItemDetail
    ? isCompletedWork(activeWorkItemDetail, activeDetailPipeline)
    : false;
  useEffect(() => {
    if (!missionControlApiUrl || !activeDetailAttempt?.id) {
      setActiveAttemptTimeline(null);
      return;
    }
    let cancelled = false;
    void fetchAttemptTimeline(missionControlApiUrl, activeDetailAttempt.id)
      .then((timeline) => {
        if (!cancelled) {
          setActiveAttemptTimeline(timeline);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setActiveAttemptTimeline(null);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [activeDetailAttempt?.id, missionControlApiUrl]);
  useEffect(() => {
    if (!missionControlApiUrl || !activeDetailAttempt?.pullRequestUrl) {
      setActivePullRequestStatus(null);
      return;
    }
    let cancelled = false;
    void fetchGitHubPullRequestStatus(missionControlApiUrl, {
      url: activeDetailAttempt.pullRequestUrl,
      repositoryOwner: activeDetailGitHubRepositoryTarget?.owner,
      repositoryName: activeDetailGitHubRepositoryTarget?.repo,
      workspacePath: activeDetailAttempt.workspacePath,
      requiredChecks: activeDetailPipeline?.run?.workflow?.runtime?.requiredChecks
    })
      .then((status) => {
        if (!cancelled) {
          setActivePullRequestStatus(status);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setActivePullRequestStatus(null);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [
    activeDetailAttempt?.pullRequestUrl,
    activeDetailAttempt?.workspacePath,
    activeDetailGitHubRepositoryTarget?.owner,
    activeDetailGitHubRepositoryTarget?.repo,
    activeDetailPipeline?.run?.workflow?.runtime?.requiredChecks,
    missionControlApiUrl
  ]);
  const proofCountForItem = (item: WorkItem, pipeline?: PipelineRecordInfo): number => {
    const keys = new Set<string>();
    const pipelineId = pipeline?.id ?? "";
    for (const stage of pipeline?.run?.stages ?? []) {
      const evidence = "evidence" in stage && Array.isArray(stage.evidence) ? stage.evidence : [];
      for (const proof of evidence) {
        if (proof) keys.add(proof);
      }
    }
    const operationIds = new Set(
      operations
        .filter((operation) => {
          return (
            Boolean(pipelineId && (operation.id.includes(pipelineId) || operation.missionId?.includes(pipelineId))) ||
            Boolean(operation.missionId?.includes(item.id)) ||
            Boolean(operation.missionId?.includes(item.key)) ||
            Boolean(operation.prompt?.includes(item.key))
          );
        })
        .map((operation) => operation.id)
    );
    for (const record of proofRecords) {
      if (!record.sourcePath && !record.sourceUrl) continue;
      const belongsToPipeline = pipelineId && record.operationId?.includes(pipelineId);
      const belongsToOperation = record.operationId ? operationIds.has(record.operationId) : false;
      if (!belongsToPipeline && !belongsToOperation) continue;
      keys.add(record.sourcePath ?? record.sourceUrl ?? record.id);
    }
    return keys.size;
  };
  const agentTurnCountForItem = (item: WorkItem, pipeline?: PipelineRecordInfo): number => {
    const pipelineId = pipeline?.id ?? "";
    return operations.filter((operation) => {
      return (
        Boolean(pipelineId && (operation.id.includes(pipelineId) || operation.missionId?.includes(pipelineId))) ||
        Boolean(operation.missionId?.includes(item.id)) ||
        Boolean(operation.missionId?.includes(item.key)) ||
        Boolean(operation.prompt?.includes(item.key))
      );
    }).length;
  };
  const pendingCheckpoint = useMemo(
    () => checkpoints.find((checkpoint) => checkpoint.status === "pending"),
    [checkpoints]
  );
  const recentOperations = useMemo(
    () =>
      [...operations]
        .sort((left, right) => (right.updatedAt ?? right.createdAt ?? "").localeCompare(left.updatedAt ?? left.createdAt ?? ""))
        .slice(0, 6),
    [operations]
  );
  const recentRuntimeLogs = useMemo(
    () =>
      [...runtimeLogs]
        .sort((left, right) => (right.createdAt ?? "").localeCompare(left.createdAt ?? ""))
        .slice(0, 10),
    [runtimeLogs]
  );

  async function refreshControlPlane() {
    if (!missionControlApiUrl) return;
    const [
      nextObservability,
      nextProviders,
      nextSelection,
      nextTemplates,
      nextAgents,
      nextRequirements,
      nextPipelines,
      nextAttempts,
      nextProofRecords,
      nextRunWorkpads,
      nextCheckpoints,
      nextOperations,
      nextRuntimeLogs,
      nextExecutionLocks,
      nextOrchestratorWatchers,
      nextCapabilities,
      nextWorkspaceRoot,
      nextGitHubOAuthConfig,
      nextGitHubStatus
    ] = await Promise.all([
      fetchObservability(missionControlApiUrl),
      fetchLlmProviders(missionControlApiUrl),
      fetchLlmProviderSelection(missionControlApiUrl),
      fetchPipelineTemplates(missionControlApiUrl),
      fetchAgentDefinitions(missionControlApiUrl),
      fetchRequirements(missionControlApiUrl).catch(() => []),
      fetchPipelines(missionControlApiUrl),
      fetchAttempts(missionControlApiUrl).catch(() => []),
      fetchProofRecords(missionControlApiUrl).catch(() => []),
      fetchRunWorkpads(missionControlApiUrl).catch(() => []),
      fetchCheckpoints(missionControlApiUrl),
      fetchOperations(missionControlApiUrl).catch(() => []),
      fetchRuntimeLogs(missionControlApiUrl, { limit: 80 }).catch(() => []),
      fetchExecutionLocks(missionControlApiUrl).catch(() => []),
      fetchOrchestratorWatchers(missionControlApiUrl).catch(() => []),
      fetchLocalCapabilities(missionControlApiUrl),
      fetchLocalWorkspaceRoot(missionControlApiUrl).catch(() => ({ workspaceRoot: "" })),
      fetchGitHubOAuthConfig(missionControlApiUrl).catch(() => defaultGitHubOAuthConfig),
      fetchGitHubStatus(missionControlApiUrl).catch(() => null)
    ]);
    setObservability(nextObservability);
    setLlmProviders(nextProviders);
    setLlmSelection(nextSelection);
    setPipelineTemplates(nextTemplates);
    setAgentDefinitions(nextAgents);
    setRequirements(nextRequirements);
    setPipelines(nextPipelines);
    setAttempts(nextAttempts);
    setProofRecords(nextProofRecords);
    setRunWorkpads(nextRunWorkpads);
    setCheckpoints(nextCheckpoints);
    setOperations(nextOperations);
    setRuntimeLogs(nextRuntimeLogs);
    setExecutionLocks(nextExecutionLocks);
    setOrchestratorWatchers(nextOrchestratorWatchers);
    setLocalCapabilities(nextCapabilities);
    setLocalWorkspaceRoot(nextWorkspaceRoot.workspaceRoot);
    setLocalWorkspaceRootDraft((currentDraft) => currentDraft || nextWorkspaceRoot.workspaceRoot);
    setGitHubOAuthConfig(nextGitHubOAuthConfig);
    setGitHubStatus(nextGitHubStatus);
    if (nextGitHubStatus?.authenticated) {
      setConnections((current) => grantProviderConnection(current, "github", nextGitHubStatus.account ?? "gh-cli"));
    }
    setGitHubOAuthDraft((currentDraft) => ({
      clientId: nextGitHubOAuthConfig.clientId || currentDraft.clientId,
      clientSecret: "",
      redirectUri: nextGitHubOAuthConfig.redirectUri || currentDraft.redirectUri
    }));
    setLocalRunner((currentRunner) =>
      currentRunner === "local-proof" && nextCapabilities.some((capability) => capability.id === "codex" && capability.available)
        ? "codex"
        : currentRunner
    );
  }

  async function refreshWorkspaceState() {
    if (!missionControlApiUrl) return;
    const session = await fetchWorkspaceSession(missionControlApiUrl, run);
    if (!session) return;
    setProjects(session.projects);
    setRequirements(session.requirements);
    setWorkItems(session.workItems);
    setMissionState(session.missionState);
    setConnections(session.connections);
  }

  async function refreshExecutionState(options: { includeArtifacts?: boolean } = {}) {
    if (!missionControlApiUrl) return;
    const [
      session,
      nextPipelines,
      nextAttempts,
      nextRunWorkpads,
      nextCheckpoints
    ] = await Promise.all([
      fetchWorkspaceSession(missionControlApiUrl, run).catch(() => null),
      fetchPipelines(missionControlApiUrl),
      fetchAttempts(missionControlApiUrl).catch(() => []),
      fetchRunWorkpads(missionControlApiUrl).catch(() => []),
      fetchCheckpoints(missionControlApiUrl)
    ]);
    if (session) {
      setProjects(session.projects);
      setRequirements(session.requirements);
      setWorkItems(session.workItems);
      setMissionState(session.missionState);
      setConnections(session.connections);
    }
    setPipelines(nextPipelines);
    setAttempts(nextAttempts);
    setRunWorkpads(nextRunWorkpads);
    setCheckpoints(nextCheckpoints);
    if (options.includeArtifacts) {
      const [nextOperations, nextProofRecords] = await Promise.all([
        fetchOperations(missionControlApiUrl).catch(() => []),
        fetchProofRecords(missionControlApiUrl).catch(() => [])
      ]);
      setOperations(nextOperations);
      setProofRecords(nextProofRecords);
    }
  }

  async function updateRunWorkpadPatch(runWorkpadId: string, input: PatchRunWorkpadInput) {
    if (!missionControlApiUrl) {
      throw new Error("Omega control API is not connected.");
    }
    const patched = await patchRunWorkpad(missionControlApiUrl, runWorkpadId, input);
    setRunWorkpads((current) => current.map((record) => (record.id === patched.id ? patched : record)));
    await refreshExecutionState({ includeArtifacts: false });
  }

  const hasLiveExecution =
    Boolean(runningWorkItemId) ||
    workItems.some((item) => item.status === "Planning" || item.status === "In Review") ||
    pipelines.some((pipeline) => pipeline.status === "running" || pipeline.status === "waiting-human") ||
    attempts.some((attempt) => attempt.status === "running" || attempt.status === "waiting-human");

  useEffect(() => {
    if (!missionControlApiUrl || !hasLiveExecution) return;
    let cancelled = false;
    const pollExecutionState = async () => {
      try {
        await refreshExecutionState();
        if (cancelled) return;
      } catch (error) {
        console.warn("Live execution refresh failed", error);
      }
    };
    void pollExecutionState();
    const timer = window.setInterval(() => {
      void pollExecutionState();
    }, 2500);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [missionControlApiUrl, hasLiveExecution, run.id]);

  useEffect(() => {
    if (!missionControlApiUrl) return;

    let cancelled = false;
    fetchWorkspaceSession(missionControlApiUrl, run)
      .then(async (session) => {
        if (cancelled) return;
        if (session) {
          setProjects(session.projects);
          setRequirements(session.requirements);
          setWorkItems(session.workItems);
          setMissionState(session.missionState);
          setConnections(session.connections);
          setActiveNav(session.activeNav);
          setSelectedProviderId(session.selectedProviderId);
          setSelectedWorkItemId(session.selectedWorkItemId);
          setInspectorOpen(false);
          setActiveInspectorPanel(session.activeInspectorPanel);
          setRunnerPreset(session.runnerPreset);
          setStatusFilter(session.statusFilter);
          setAssigneeFilter(session.assigneeFilter);
          setSortDirection(session.sortDirection);
          setCollapsedGroups(session.collapsedGroups);
        } else {
          await saveWorkspaceSessionViaApi(missionControlApiUrl, run, {
            projects,
            requirements,
            workItems,
            missionState: { ...missionState, workItems },
            connections,
            activeNav: activeNav === "Settings" ? "Projects" : activeNav,
            selectedProviderId,
            selectedWorkItemId,
            inspectorOpen,
            activeInspectorPanel,
            runnerPreset,
            statusFilter,
            assigneeFilter,
            sortDirection,
            collapsedGroups
          });
        }
      })
      .catch((error) => {
        console.warn("Initial workspace load failed", error);
      })
      .finally(() => {
        if (!cancelled) setWorkspaceLoaded(true);
      });

    refreshControlPlane().catch((error) => {
      console.warn("Initial control plane refresh failed", error);
    });

    return () => {
      cancelled = true;
    };
  }, [run]);

  useEffect(() => {
    if (!workspaceLoaded) return;

    const session = {
      projects,
      requirements,
      workItems,
      missionState: { ...missionState, workItems },
      connections,
      activeNav: activeNav === "Settings" ? "Projects" : activeNav,
      selectedProviderId,
      selectedWorkItemId,
      inspectorOpen,
      activeInspectorPanel,
      runnerPreset,
      statusFilter,
      assigneeFilter,
      sortDirection,
      collapsedGroups
    };

    saveWorkspaceSession(run, session);
  }, [
    activeInspectorPanel,
    activeNav,
    assigneeFilter,
    collapsedGroups,
    connections,
    inspectorOpen,
    missionState,
    projects,
    requirements,
    runnerPreset,
    run,
    selectedProviderId,
    selectedWorkItemId,
    sortDirection,
    statusFilter,
    workspaceLoaded,
    workItems
  ]);

  useEffect(() => {
    if (!missionControlApiUrl || activeNav !== "Settings") return;
    let cancelled = false;
    fetchProjectAgentProfile(missionControlApiUrl, {
      projectId: primaryProject?.id,
      repositoryTargetId: activeRepositoryWorkspace?.id
    })
      .then((profile) => {
        if (cancelled) return;
        setAgentConfigDraft(normalizeAgentConfigurationDraft(profile));
      })
      .catch((error) => {
        console.warn("Agent profile load failed", error);
      });
    return () => {
      cancelled = true;
    };
  }, [activeNav, activeRepositoryWorkspace?.id, missionControlApiUrl, primaryProject?.id]);

  useEffect(() => {
    if (!missionControlApiUrl || !githubDeviceLoginUrl || connections.github.status === "connected") return;
    const timer = window.setInterval(() => {
      void refreshGitHubConnectionStatus();
    }, 4000);
    return () => window.clearInterval(timer);
  }, [connections.github.status, githubDeviceLoginUrl]);

  useEffect(() => {
    if (activeNav !== "Projects" || !missionControlApiUrl || githubRepositories.length > 0 || githubRepositoriesLoading) return;
    void loadGitHubRepositories();
  }, [activeNav, githubRepositories.length, githubRepositoriesLoading]);

  async function createItem() {
    if (creatingItemRef.current) return;
    const description = newItemDescription.trim();
    const title = newItemTitle.trim() || titleFromMarkdownDescription(description);
    if (!title) {
      setRunnerMessage("Add a title or description before creating a requirement.");
      setCreateComposerExpanded(true);
      return;
    }

    const target = activeRepositoryWorkspace
      ? activeRepositoryWorkspaceTarget ?? "No target"
      : newItemTarget.trim() || activeRepositoryWorkspaceTarget || "No target";
    const item = createManualWorkItem(
      workItems.length + 1,
      title,
      description || "No description provided.",
      newItemAssignee,
      target,
      activeRepositoryWorkspace?.id
    );
    try {
      creatingItemRef.current = true;
      setIsCreatingItem(true);
      setRunnerMessage("Creating requirement...");
      if (missionControlApiUrl) {
        const session = await createWorkItemViaApi(missionControlApiUrl, run, item);
        setProjects(session.projects);
        setRequirements(session.requirements);
        setWorkItems(session.workItems);
        setMissionState(session.missionState);
        await refreshControlPlane().catch((error) => {
          console.warn("Control plane refresh after work item create failed", error);
        });
      } else {
        setWorkItems((current) => [...current, item]);
      }

      setSelectedWorkItemId(item.id);
      setShowInlineCreate(false);
      setNewItemTitle("");
      setNewItemDescription("");
      setNewItemTarget("");
      setCreateComposerExpanded(false);
      setCreateDescriptionMode("write");
      setRunnerMessage(`Created requirement ${title}.`);
      setActiveNav("Issues");
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Create work item failed.");
    } finally {
      creatingItemRef.current = false;
      setIsCreatingItem(false);
    }
  }

  function selectWorkItem(item: WorkItem) {
    setSelectedWorkItemId(item.id);
    openWorkItemDetail(item.id);
    setActiveInspectorPanel("properties");
    setInspectorOpen(false);
    setShowInlineCreate(false);
    setCreateComposerExpanded(false);
    setCreateDescriptionMode("write");
  }

  function canDeleteWorkItem(item: WorkItem): boolean {
    return (item.status === "Ready" || item.status === "Backlog") && !pipelinesByWorkItemId.has(item.id) && runningWorkItemId !== item.id;
  }

  function applyDeletedWorkItemState(nextWorkItems: WorkItem[], deletedItem: WorkItem) {
    const nextSelectedId = selectedWorkItemId === deletedItem.id ? nextWorkItems[0]?.id ?? "" : selectedWorkItemId;
    const nextRequirementIds = new Set(nextWorkItems.map((item) => item.requirementId).filter(Boolean));
    setWorkItems(nextWorkItems);
    setRequirements((current) =>
      deletedItem.requirementId && !nextRequirementIds.has(deletedItem.requirementId)
        ? current.filter((requirement) => requirement.id !== deletedItem.requirementId)
        : current
    );
    setMissionState((current) => ({
      ...current,
      workItems: current.workItems.filter((candidate) => candidate.id !== deletedItem.id)
    }));
    setSelectedWorkItemId(nextSelectedId);
    if (activeWorkItemDetailId === deletedItem.id) {
      setActiveWorkItemDetailId("");
      window.history.replaceState(null, "", "#workboard");
    }
  }

  async function deleteWorkItem(item: WorkItem) {
    if (!canDeleteWorkItem(item)) {
      setRunnerMessage("Only not-started items without execution history can be deleted.");
      return;
    }
    const confirmed = window.confirm(`Delete "${item.title}" from Omega?\n\nThis removes the not-started item and its unshared Requirement record.`);
    if (!confirmed) return;
    try {
      setRunnerMessage(`Deleting ${item.key}...`);
      if (missionControlApiUrl) {
        const session = await deleteWorkItemViaApi(missionControlApiUrl, run, item.id);
        setProjects(session.projects);
        setRequirements(session.requirements);
        setWorkItems(session.workItems);
        setMissionState(session.missionState);
        setConnections(session.connections);
        setSelectedWorkItemId((current) => (current === item.id ? session.workItems[0]?.id ?? "" : current));
        if (activeWorkItemDetailId === item.id) {
          setActiveWorkItemDetailId("");
          window.history.replaceState(null, "", "#workboard");
        }
        setRunnerMessage(`Deleted ${item.key}.`);
        return;
      }
      applyDeletedWorkItemState(workItems.filter((candidate) => candidate.id !== item.id), item);
      setRunnerMessage(`Deleted ${item.key}.`);
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Delete work item failed.");
    }
  }

  function clearWorkspaceMessages() {
    setRunnerMessage("");
    setRepositorySyncMessage("");
  }

  async function runItem(item: WorkItem, options: { force?: boolean } = {}) {
    const existingPipeline = pipelinesByWorkItemId.get(item.id);
    if (isCompletedWork(item, existingPipeline) && !options.force) {
      setRunnerMessage(`${item.key} has already completed its pipeline. Open the detail view to inspect proof and delivery history.`);
      return;
    }
    if (activeRepositoryWorkspace && item.repositoryTargetId !== activeRepositoryWorkspace.id) {
      setRunnerMessage(
        `${item.key} is not linked to ${activeRepositoryWorkspaceLabel}. Open the right workspace or attach a repository target before running.`
      );
      return;
    }
    setSelectedWorkItemId(item.id);
    setActiveInspectorPanel("properties");
    setInspectorOpen(false);
    if (!missionControlApiUrl) {
      setWorkItems((current) => updateWorkItemStatus(current, item.id, "In Review"));
      setRunnerMessage("Mission Control API is not configured; using UI-only demo state.");
      return;
    }

    const hasCodeTarget = Boolean(item.repositoryTargetId || (item.target && item.target !== "No target"));
    if (hasCodeTarget && item.repositoryTargetId) {
      setRunningWorkItemId(item.id);
      setWorkItems((current) => updateWorkItemStatus(current, item.id, "Planning"));
      setRunnerMessage(`Planning pipeline and assigning agents for ${item.key}...`);
      try {
        const planningSession = await patchWorkItemViaApi(missionControlApiUrl, run, item.id, { status: "Planning" });
        setProjects(planningSession.projects);
        setRequirements(planningSession.requirements);
        setMissionState(planningSession.missionState);
        setWorkItems(planningSession.workItems);
        const pipeline = await createPipelineFromTemplate(missionControlApiUrl, "devflow-pr", item);
        setRunnerMessage(`Running DevFlow pipeline for ${item.key}: requirement -> implementation -> review -> human gate -> delivery.`);
        const runningSession = await patchWorkItemViaApi(missionControlApiUrl, run, item.id, { status: "In Review" });
        setProjects(runningSession.projects);
        setRequirements(runningSession.requirements);
        setMissionState(runningSession.missionState);
        setWorkItems(runningSession.workItems);
        const result = await runDevFlowCycle(missionControlApiUrl, pipeline.id);
        if (result.status === "accepted") {
          setRunnerMessage(`Pipeline started for ${item.key}. Agent progress will update in the item detail view.`);
          await refreshControlPlane().catch((error) => {
            console.warn("Control plane refresh after DevFlow start failed", error);
          });
          await refreshWorkspaceState().catch((error) => {
            console.warn("Workspace refresh after DevFlow start failed", error);
          });
          return;
        }
        const session = await fetchWorkspaceSession(missionControlApiUrl, run);
        if (session) {
          setProjects(session.projects);
          setRequirements(session.requirements);
          setMissionState(session.missionState);
          setWorkItems(session.workItems);
        }
        const pullRequest = result.pullRequestUrl ? ` PR: ${result.pullRequestUrl}.` : "";
        const branch = result.branchName ? ` Branch: ${result.branchName}.` : "";
        setRunnerMessage(`DevFlow cycle completed for ${item.key}.${branch}${pullRequest}`);
        await refreshControlPlane().catch((error) => {
          console.warn("Control plane refresh after DevFlow run failed", error);
        });
      } catch (error) {
        setRunnerMessage(error instanceof Error ? error.message : "DevFlow cycle failed.");
      } finally {
        setRunningWorkItemId("");
      }
      return;
    }

    const runner: MissionControlRunnerPreset =
      runnerPreset === "local-proof" && hasCodeTarget ? "demo-code" : runnerPreset;
    setRunningWorkItemId(item.id);
    setWorkItems((current) => updateWorkItemStatus(current, item.id, "Planning"));
    setRunnerMessage(`Preparing ${item.key} for ${runner}...`);
    try {
      const planningSession = await patchWorkItemViaApi(missionControlApiUrl, run, item.id, { status: "Planning" });
      setProjects(planningSession.projects);
      setRequirements(planningSession.requirements);
      setMissionState(planningSession.missionState);
      setWorkItems(planningSession.workItems);
      const mission = missionControlApiUrl
        ? await fetchMissionFromWorkItem(missionControlApiUrl, item)
        : createMissionFromRun(run, item);
      const runningSession = await patchWorkItemViaApi(missionControlApiUrl, run, item.id, { status: "In Review" });
      setProjects(runningSession.projects);
      setRequirements(runningSession.requirements);
      setMissionState(runningSession.missionState);
      setWorkItems(runningSession.workItems);
      setRunnerMessage(`Running ${item.key} with ${runner}...`);
      const response = await runOperationViaWorkspaceApi(
        missionControlApiUrl,
        mission,
        mission.operations[0].id,
        runner
      );
      const session = await fetchWorkspaceSession(missionControlApiUrl, run);
      if (session) {
        setProjects(session.projects);
        setRequirements(session.requirements);
        setMissionState(session.missionState);
        setWorkItems(session.workItems);
      } else {
        const nextState = applyMissionControlEvents(missionState, response.events);
        setMissionState(nextState);
        setWorkItems(nextState.workItems);
      }
      if (response.status === "passed") {
        const changed = response.changedFiles?.length ? ` Changed: ${response.changedFiles.join(", ")}.` : "";
        const branch = response.branchName ? ` Branch: ${response.branchName}.` : "";
        setRunnerMessage(`Run passed with ${response.proofFiles.length} proof file(s).${branch}${changed}`);
      } else {
        setRunnerMessage(response.stderr || "Run failed. Check the proof workspace for details.");
      }
      await refreshControlPlane().catch((error) => {
        console.warn("Control plane refresh after run failed", error);
      });
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Mission Control API failed.");
    } finally {
      setRunningWorkItemId("");
    }
  }

  function toggleGroup(status: WorkItemStatus) {
    setCollapsedGroups((current) =>
      current.includes(status) ? current.filter((candidate) => candidate !== status) : [...current, status]
    );
  }

  function canUseLocalGitHubOAuth(provider: ConnectionProvider): boolean {
    return provider.id === "github" && Boolean(missionControlApiUrl);
  }

  function oauthNeedsClientId(provider: ConnectionProvider): boolean {
    return provider.authMethod === "oauth" && !providerClientIds[provider.id] && !canUseLocalGitHubOAuth(provider);
  }

  async function connectProvider(provider: ConnectionProvider) {
    setSelectedProviderId(provider.id);
    openInspectorPanel("provider");
    if (provider.authMethod === "oauth" && provider.authorizeUrl) {
      if (canUseLocalGitHubOAuth(provider)) {
        try {
          setProviderFeedback("Starting GitHub sign-in...");
          setGitHubDeviceLoginUrl("");
          const start = await startGitHubOAuth(missionControlApiUrl);
          if (!start.configured || !start.authorizeUrl) {
            setGitHubOAuthSetupOpen(true);
            const cliLogin = await startGitHubCliLogin(missionControlApiUrl);
            setGitHubDeviceLoginUrl(cliLogin.verificationUrl ?? "https://github.com/login/device");
            if (cliLogin.started) {
              const message = cliLogin.message ?? "GitHub CLI sign-in opened. Complete the browser flow, then refresh provider status.";
              setProviderFeedback(message);
              setRunnerMessage(message);
              return;
            }
            const fallbackMessage = cliLogin.reason
              ? `${start.reason ?? "GitHub OAuth app setup is required."} ${cliLogin.reason}`
              : start.reason ?? "GitHub OAuth app setup is required before sign in.";
            setProviderFeedback(fallbackMessage);
            setRunnerMessage(fallbackMessage);
            return;
          }
          setGitHubOAuthSetupOpen(false);
          setGitHubDeviceLoginUrl("");
          setProviderFeedback("Opening GitHub authorization...");
          navigateToExternalUrl(start.authorizeUrl);
          setRunnerMessage("Redirecting to GitHub OAuth.");
        } catch (error) {
          const message = error instanceof Error ? error.message : "GitHub sign-in failed.";
          setProviderFeedback(message);
          setRunnerMessage(message);
        }
        return;
      }

      const clientId = providerClientIds[provider.id];
      if (!clientId) return;

      const authorizeUrl = buildAuthorizeUrl(provider.id, {
        clientId,
        redirectUri: "http://localhost:5173/auth/callback",
        state: run.id
      });
      navigateToExternalUrl(authorizeUrl);
      return;
    }
    setConnections((current) => grantProviderConnection(current, provider.id));
  }

  async function saveGitHubOAuthConfig() {
    if (!missionControlApiUrl) return;
    try {
      const nextConfig = await updateGitHubOAuthConfig(missionControlApiUrl, {
        clientId: githubOAuthDraft.clientId.trim(),
        clientSecret: githubOAuthDraft.clientSecret.trim(),
        redirectUri: githubOAuthDraft.redirectUri.trim(),
        tokenUrl: githubOAuthConfig.tokenUrl
      });
      setGitHubOAuthConfig(nextConfig);
      setGitHubOAuthSetupOpen(false);
      setProviderFeedback("OAuth app saved. Continue with GitHub can now open GitHub authorization directly.");
      setGitHubOAuthDraft((currentDraft) => ({
        clientId: nextConfig.clientId,
        clientSecret: "",
        redirectUri: nextConfig.redirectUri || currentDraft.redirectUri
      }));
      setRunnerMessage("GitHub OAuth config saved.");
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "GitHub OAuth config save failed.");
    }
  }

  async function refreshGitHubConnectionStatus() {
    if (!missionControlApiUrl) return;
    try {
      const status = await fetchGitHubStatus(missionControlApiUrl);
      setGitHubStatus(status);
      if (status.authenticated) {
        const account = status.account ?? "gh-cli";
        setConnections((current) => grantProviderConnection(current, "github", account));
        setProviderFeedback(`GitHub CLI is connected as ${account}.`);
        return;
      }
      setProviderFeedback("GitHub CLI is not connected yet. Complete the device page, then check again.");
    } catch (error) {
      setProviderFeedback(error instanceof Error ? error.message : "GitHub status check failed.");
    }
  }

  function disconnectProvider(providerId: ProviderId) {
    setConnections((current) => revokeProviderConnection(current, providerId));
  }

  function openInspectorPanel(panel: InspectorPanel) {
    setActiveInspectorPanel(panel);
    setInspectorOpen(true);
  }

  function openProviderAccess(providerId: ProviderId) {
    setSelectedProviderId(providerId);
    setActiveInspectorPanel("provider");
    setInspectorOpen(true);
  }

  function handleProviderRowClick(provider: ConnectionProvider) {
    openProviderAccess(provider.id);
  }

  async function chooseLlmProvider(providerId: string) {
    const provider = llmProviders.find((candidate) => candidate.id === providerId);
    if (!provider || !missionControlApiUrl) return;
    try {
      const nextSelection = await updateLlmProviderSelection(missionControlApiUrl, {
        providerId: provider.id,
        model: provider.defaultModel,
        reasoningEffort: llmSelection.reasoningEffort
      });
      setLlmSelection(nextSelection);
      const nextAgents = await fetchAgentDefinitions(missionControlApiUrl);
      setAgentDefinitions(nextAgents);
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Provider selection failed.");
    }
  }

  async function chooseLlmModel(model: string) {
    if (!missionControlApiUrl) return;
    try {
      const nextSelection = await updateLlmProviderSelection(missionControlApiUrl, {
        ...llmSelection,
        model
      });
      setLlmSelection(nextSelection);
      const nextAgents = await fetchAgentDefinitions(missionControlApiUrl);
      setAgentDefinitions(nextAgents);
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Model selection failed.");
    }
  }

  async function createPipelineForSelectedItem(templateId: string) {
    if (!missionControlApiUrl || !selectedWorkItem) return;
    try {
      const pipeline = await createPipelineFromTemplate(missionControlApiUrl, templateId, selectedWorkItem);
      await startPipeline(missionControlApiUrl, pipeline.id);
      setRunnerMessage(`Pipeline ${pipeline.id} started from template ${templateId}.`);
      await refreshControlPlane();
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Pipeline creation failed.");
    }
  }

  async function startOperatorPipeline(pipelineId: string) {
    if (!missionControlApiUrl) return;
    try {
      await startPipeline(missionControlApiUrl, pipelineId);
      setRunnerMessage(`Pipeline ${pipelineId} started.`);
      await refreshControlPlane();
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Pipeline start failed.");
    }
  }

  async function runOperatorPipelineStage(pipelineId: string) {
    if (!missionControlApiUrl) return;
    try {
      const result = await runCurrentPipelineStage(missionControlApiUrl, pipelineId, localRunner);
      setRunnerMessage(`Pipeline ${pipelineId} stage finished with ${result.operationResult.status}.`);
      await refreshControlPlane();
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Pipeline stage run failed.");
    }
  }

  async function runOperatorDevFlowCycle(pipelineId: string) {
    if (!missionControlApiUrl) return;
    try {
      const result = await runDevFlowCycle(missionControlApiUrl, pipelineId);
      const branch = result.branchName ? ` Branch: ${result.branchName}.` : "";
      const pullRequest = result.pullRequestUrl ? ` PR: ${result.pullRequestUrl}.` : "";
      setRunnerMessage(`DevFlow cycle completed with ${result.status}.${branch}${pullRequest}`);
      await refreshControlPlane();
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "DevFlow cycle run failed.");
    }
  }

  async function retryWorkItemAttempt(attemptId: string) {
    if (!missionControlApiUrl || !activeWorkItemDetail) return;
    const previousAttempt = attempts.find((attempt) => attempt.id === attemptId);
    setRunningWorkItemId(activeWorkItemDetail.id);
    setRunnerMessage(`Retrying ${activeWorkItemDetail.key} from attempt ${attemptId}...`);
    try {
      const result = await retryAttempt(missionControlApiUrl, attemptId, retryReasonForAttempt(previousAttempt));
      setRunnerMessage(`Retry attempt started for ${activeWorkItemDetail.key}: ${result.attempt.id}.`);
      await refreshControlPlane();
      await refreshWorkspaceState().catch((error) => {
        console.warn("Workspace refresh after attempt retry failed", error);
      });
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Attempt retry failed.");
    } finally {
      setRunningWorkItemId("");
    }
  }

  async function saveLocalWorkspaceRoot() {
    const nextRoot = localWorkspaceRootDraft.trim();
    if (!missionControlApiUrl || !nextRoot) return;
    try {
      const result = await updateLocalWorkspaceRoot(missionControlApiUrl, nextRoot);
      setLocalWorkspaceRoot(result.workspaceRoot);
      setLocalWorkspaceRootDraft(result.workspaceRoot);
      setRunnerMessage(`Local workspaces will be saved under ${result.workspaceRoot}.`);
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Workspace location save failed.");
    }
  }

  async function releaseOperatorExecutionLock(lockId: string) {
    if (!missionControlApiUrl) return;
    try {
      await releaseExecutionLock(missionControlApiUrl, lockId);
      setRunnerMessage(`Execution lock ${lockId} released.`);
      await refreshControlPlane();
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Execution lock release failed.");
    }
  }

  async function loadGitHubRepositories() {
    if (!missionControlApiUrl) return;
    setGitHubRepositoriesLoading(true);
    try {
      const repositories = await fetchGitHubRepositories(missionControlApiUrl);
      setGitHubRepositories(repositories);
      if (!githubRepoInfo && repositories.length > 0) {
        selectGitHubRepository(repositories[0]);
      }
      setRunnerMessage(`Loaded ${repositories.length} GitHub repository target(s).`);
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "GitHub repository list failed.");
    } finally {
      setGitHubRepositoriesLoading(false);
    }
  }

  function selectGitHubRepository(repository: GitHubRepositoryInfo) {
    const nameWithOwner = repository.nameWithOwner ?? `${repository.owner?.login ?? ""}/${repository.name}`;
    const [owner, repo] = nameWithOwner.split("/");
    if (!owner || !repo) return;
    setGitHubRepoOwner(owner);
    setGitHubRepoName(repo);
    setGitHubRepoInfo(repository);
  }

  async function openSelectedRepositoryWorkspace() {
    if (!missionControlApiUrl || !githubRepoOwner || !githubRepoName) return;
    if (selectedRepositoryBound) {
      setActiveRepositoryWorkspaceTargetId(selectedRepositoryTargetId);
      setActiveNav("Issues");
      setRunnerMessage(`${selectedRepositoryNameWithOwner} workspace is open.`);
      return;
    }
    try {
      const session = await bindGitHubRepositoryTargetViaApi(missionControlApiUrl, run, {
        owner: githubRepoOwner,
        repo: githubRepoName,
        nameWithOwner: selectedRepositoryNameWithOwner,
        defaultBranch: githubRepoInfo?.defaultBranchRef?.name,
        url: githubRepoInfo?.url
      });
      setProjects(session.projects);
      setRequirements(session.requirements);
      setWorkItems(session.workItems);
      setMissionState(session.missionState);
      setConnections(session.connections);
      setActiveRepositoryWorkspaceTargetId(selectedRepositoryTargetId);
      setActiveNav("Issues");
      setRunnerMessage(`${selectedRepositoryNameWithOwner} workspace was created under ${session.projects[0]?.name ?? "the current project"}.`);
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "GitHub repository workspace creation failed.");
    }
  }

  async function deleteRepositoryWorkspace(targetId: string) {
    if (!missionControlApiUrl) return;
    const target = repositoryTargets.find((candidate) => candidate.id === targetId);
    if (!target) return;
    const label = target.kind === "github" ? `${target.owner}/${target.repo}` : target.path;
    const itemCount = workItems.filter((item) => item.repositoryTargetId === targetId).length;
    const confirmed = window.confirm(
      `Delete ${label} from Omega?\n\nThis removes the repository workspace and ${itemCount} linked work item${itemCount === 1 ? "" : "s"} from this app. It does not delete the GitHub repository.`
    );
    if (!confirmed) return;
    try {
      const session = await deleteRepositoryTargetViaApi(missionControlApiUrl, run, targetId);
      setProjects(session.projects);
      setRequirements(session.requirements);
      setWorkItems(session.workItems);
      setMissionState(session.missionState);
      setConnections(session.connections);
      if (activeRepositoryWorkspaceTargetId === targetId) {
        setActiveRepositoryWorkspaceTargetId("");
        setActiveWorkItemDetailId("");
        window.history.replaceState(null, "", "#workboard");
        setSelectedWorkItemId(session.workItems[0]?.id ?? "");
        setActiveNav("Projects");
        setRunnerMessage(`${label} was removed from Omega.`);
      } else {
        setRunnerMessage("");
      }
      setRepositorySyncMessage("");
      await refreshControlPlane().catch((error) => {
        console.warn("Control plane refresh after workspace delete failed", error);
      });
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Repository workspace delete failed.");
    }
  }

  async function importGitHubIssues(ownerOverride?: string, repoOverride?: string) {
    if (!missionControlApiUrl) return;
    const owner = (ownerOverride ?? githubRepoOwner).trim();
    const repo = (repoOverride ?? githubRepoName).trim();
    if (!owner || !repo) return;
    const repositoryKey = `${owner}/${repo}`;
    const repositoryTargetId = `repo_${repositoryKey.replace("/", "_")}`;
    setSyncingRepositoryKey(repositoryKey);
    setRepositorySyncMessage(`Syncing issues from ${repositoryKey}...`);
    try {
      const beforeCount = workItems.filter((item) => item.repositoryTargetId === repositoryTargetId).length;
      const session = await importGitHubIssuesViaApi(missionControlApiUrl, run, owner, repo);
      const syncedItems = session.workItems.filter((item) => item.repositoryTargetId === repositoryTargetId);
      setProjects(session.projects);
      setRequirements(session.requirements);
      setWorkItems(session.workItems);
      setMissionState(session.missionState);
      setConnections(session.connections);
      setSelectedWorkItemId(syncedItems[0]?.id ?? session.workItems[0]?.id ?? "");
      setActiveRepositoryWorkspaceTargetId(repositoryTargetId);
      setActiveNav("Issues");
      const importedCount = Math.max(syncedItems.length - beforeCount, 0);
      const message =
        importedCount > 0
          ? `Synced ${importedCount} ${importedCount === 1 ? "issue" : "issues"} from ${repositoryKey}.`
          : `Already up to date: ${syncedItems.length} ${syncedItems.length === 1 ? "issue" : "issues"} linked from ${repositoryKey}.`;
      setRepositorySyncMessage(message);
      setRunnerMessage(message);
      await refreshControlPlane().catch((error) => {
        console.warn("Control plane refresh after GitHub sync failed", error);
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : "GitHub issue import failed.";
      setRepositorySyncMessage(message);
      setRunnerMessage(message);
    } finally {
      setSyncingRepositoryKey("");
    }
  }

  async function toggleRepositoryAutoProcessing(targetId: string) {
    if (!missionControlApiUrl) return;
    const target = repositoryTargets.find((candidate) => candidate.id === targetId);
    if (!target) return;
    const label = target.kind === "github" ? `${target.owner}/${target.repo}` : target.path;
    const current = watcherByRepositoryTargetId.get(targetId);
    const nextStatus = current?.status === "active" ? "paused" : "active";
    setSyncingRepositoryKey(label);
    setRepositorySyncMessage(
      nextStatus === "active"
        ? `Auto processing enabled for ${label}. Omega will scan ready GitHub issues every 60 seconds.`
        : `Auto processing paused for ${label}.`
    );
    try {
      const watcher = await updateOrchestratorWatcher(missionControlApiUrl, targetId, {
        status: nextStatus,
        intervalSeconds: current?.intervalSeconds || 60,
        limit: current?.limit || "20",
        autoRun: true,
        autoApproveHuman: false,
        autoMerge: false
      });
      setOrchestratorWatchers((currentWatchers) => {
        const rest = currentWatchers.filter((candidate) => candidate.repositoryTargetId !== targetId);
        return [...rest, watcher];
      });
      await refreshControlPlane().catch((error) => {
        console.warn("Control plane refresh after watcher update failed", error);
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : "Auto processing update failed.";
      setRepositorySyncMessage(message);
      setRunnerMessage(message);
    } finally {
      setSyncingRepositoryKey("");
    }
  }

  async function notifyFeishu() {
    if (!missionControlApiUrl) return;
    const chatId = feishuChatId.trim();
    if (!chatId) return;
    const text = pendingCheckpoint
      ? `Omega checkpoint waiting: ${pendingCheckpoint.title}. ${pendingCheckpoint.summary}`
      : `Omega pipeline status: ${observability.counts.pipelines} pipeline(s), ${observability.attention.waitingHuman} waiting for human review.`;
    try {
      const result = await sendFeishuNotification(missionControlApiUrl, chatId, text);
      setRunnerMessage(`Feishu notification ${result.status}${result.messageId ? `: ${result.messageId}` : ""}.`);
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Feishu notification failed.");
    }
  }

  async function approvePendingCheckpoint(checkpointId: string) {
    if (!missionControlApiUrl) return;
    try {
      setRunnerMessage(`Approving checkpoint ${checkpointId}...`);
      await approveCheckpoint(missionControlApiUrl, checkpointId, "human", true);
      setRunnerMessage(`Checkpoint ${checkpointId} approved. Delivery is continuing in the background.`);
      await refreshExecutionState();
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Checkpoint approval failed.");
    }
  }

  async function rejectPendingCheckpoint(checkpointId: string, note?: string) {
    if (!missionControlApiUrl) return;
    const reason =
      note ??
      window.prompt(
        "Tell the agent what needs to change before delivery.",
        "Please address the requested changes before delivery."
      );
    if (reason === null || reason === undefined) return;
    try {
      setRunnerMessage(`Sending checkpoint ${checkpointId} back for changes...`);
      await requestCheckpointChanges(missionControlApiUrl, checkpointId, reason.trim() || "Human requested changes.");
      setRunnerMessage(`Checkpoint ${checkpointId} sent back for changes. Rework is queued when the repository target is ready.`);
      await refreshExecutionState({ includeArtifacts: true });
      window.setTimeout(() => {
        void refreshExecutionState({ includeArtifacts: true });
      }, 1200);
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Checkpoint rejection failed.");
    }
  }

  const inspectorAvailable = true;
  const shellClassName = [
    "product-shell",
    `theme-${uiTheme}`,
    activeNav === "Page Pilot" ? "page-pilot-mode" : "",
    !inspectorAvailable ? "inspector-hidden" : "",
    inspectorAvailable && !inspectorOpen ? "inspector-collapsed" : ""
  ]
    .filter(Boolean)
    .join(" ");
  const selectedProvider =
    visibleConnectionProviders.find((provider) => provider.id === selectedProviderId) ?? visibleConnectionProviders[0];
  const selectedAgentProfile =
    agentConfigDraft.agentProfiles.find((profile) => profile.id === selectedAgentProfileId) ??
    agentConfigDraft.agentProfiles[0] ??
    defaultAgentProfiles[0];
  const selectedRunnerReady = capabilityAvailable(
    localCapabilities,
    runnerOptionFor(selectedAgentProfile.runner)?.capabilityId
  );
  const selectedRunnerAvailability = runnerAvailabilityLabel(selectedAgentProfile.runner, localCapabilities);
  const workflowStagePreview = [
    { id: "requirement", title: "Requirement", agents: "requirement", gate: "auto" },
    { id: "implementation", title: "Implementation", agents: "architect + coding + testing", gate: "auto" },
    { id: "code_review", title: "Code Review", agents: "review", gate: "changes requested -> rework" },
    { id: "rework", title: "Rework", agents: "coding + testing", gate: "loops to review" },
    { id: "human_review", title: "Human Review", agents: "human + review + delivery", gate: "manual gate" },
    { id: "delivery", title: "Delivery", agents: "delivery", gate: "after approval" }
  ];
  const runtimeConfigPreview =
    runtimeConfigTab === "omega"
      ? JSON.stringify(
          {
            project: primaryProject?.name ?? "Omega",
            repositoryTarget: activeRepositoryWorkspaceLabel || "project-default",
            workflow: agentConfigDraft.workflowTemplate,
            profileSource: agentConfigDraft.repositoryTargetId ? "repository" : "project",
            agent: selectedAgentProfile.id,
            runner: selectedAgentProfile.runner,
            model: selectedAgentProfile.model,
            skills: selectedAgentProfile.skills.split("\n").filter(Boolean),
            mcp: selectedAgentProfile.mcp.split("\n").filter(Boolean),
            sandbox: "repository-workspace"
          },
          null,
          2
        )
      : runtimeConfigTab === "codex"
        ? [
            `# .codex/OMEGA.md`,
            `agent: ${selectedAgentProfile.id}`,
            `runner: ${selectedAgentProfile.runner}`,
            `model: ${selectedAgentProfile.model}`,
            "",
            selectedAgentProfile.codexPolicy || agentConfigDraft.codexPolicy
          ].join("\n")
        : [
            `# .claude/CLAUDE.md`,
            `agent: ${selectedAgentProfile.id}`,
            `runner: ${selectedAgentProfile.runner}`,
            `model: ${selectedAgentProfile.model}`,
            "",
            selectedAgentProfile.claudePolicy || agentConfigDraft.claudePolicy
          ].join("\n");

  function openWorkboard() {
    setAppSurface("workboard");
    window.history.replaceState(null, "", "#workboard");
  }

  function openPagePilot() {
    setAppSurface("workboard");
    setActiveNav("Page Pilot");
    setActiveWorkItemDetailId("");
    if (!activeRepositoryWorkspaceTargetId) {
      const fallbackTargetId = primaryProject?.defaultRepositoryTargetId ?? (repositoryTargets.length === 1 ? repositoryTargets[0].id : "");
      if (fallbackTargetId) setActiveRepositoryWorkspaceTargetId(fallbackTargetId);
    }
    window.history.replaceState(null, "", "#page-pilot");
  }

  function openPagePilotForRepository(repositoryTargetId?: string) {
    if (repositoryTargetId) {
      setActiveRepositoryWorkspaceTargetId(repositoryTargetId);
    }
    openPagePilot();
  }

  async function reloadDesktopApp() {
    const desktopBridge = (window as Window & {
      omegaDesktop?: { reloadApp?: () => Promise<{ ok: boolean; error?: string }> };
    }).omegaDesktop;
    if (desktopBridge?.reloadApp) {
      const result = await desktopBridge.reloadApp();
      if (!result.ok) setRunnerMessage(result.error ?? "Electron reload failed.");
      return;
    }
    window.location.reload();
  }

  function openWorkItemDetail(itemId: string) {
    setAppSurface("workboard");
    setActiveNav("Issues");
    setActiveWorkItemDetailId(itemId);
    window.history.pushState(null, "", workItemDetailHash(itemId));
  }

  function openHome() {
    setAppSurface("home");
    window.history.replaceState(null, "", "#home");
  }

  function toggleUiTheme() {
    setUiTheme((current) => {
      const next = current === "light" ? "dark" : "light";
      window.localStorage.setItem("omega-ui-theme", next);
      return next;
    });
  }

  async function applyPagePilotChange(instruction: string, selection: PagePilotSelectionContext) {
    if (!missionControlApiUrl || !pagePilotRepositoryTarget?.id) {
      throw new Error("Page Pilot needs the local runtime and a repository workspace.");
    }
    return applyPagePilotInstruction(missionControlApiUrl, {
      projectId: primaryProject?.id ?? "project_omega",
      repositoryTargetId: pagePilotRepositoryTarget.id,
      instruction,
      selection,
      runner: "profile"
    });
  }

  async function deliverPagePilotConfirmedChange(instruction: string, selection: PagePilotSelectionContext, runId?: string) {
    if (!missionControlApiUrl || !pagePilotRepositoryTarget?.id) {
      throw new Error("Page Pilot needs the local runtime and a repository workspace.");
    }
    return deliverPagePilotChange(missionControlApiUrl, {
      runId,
      projectId: primaryProject?.id ?? "project_omega",
      repositoryTargetId: pagePilotRepositoryTarget.id,
      instruction,
      selection,
      draft: true
    });
  }

  async function discardPagePilotPendingChange(runId: string) {
    if (!missionControlApiUrl) {
      throw new Error("Page Pilot needs the local runtime.");
    }
    const result = await discardPagePilotRun(missionControlApiUrl, runId);
    await refreshControlPlane().catch(() => {
      setRunnerMessage("Page Pilot discarded, but the workboard refresh failed.");
    });
    return result;
  }

  async function loadPagePilotRuns() {
    if (!missionControlApiUrl) return [];
    const runs = await fetchPagePilotRuns(missionControlApiUrl);
    return pagePilotRepositoryTarget?.id
      ? runs.filter((run) => run.repositoryTargetId === pagePilotRepositoryTarget.id)
      : runs;
  }

  function updateAgentConfigDraft<Key extends keyof AgentConfigurationDraft>(key: Key, value: AgentConfigurationDraft[Key]) {
    setAgentConfigDraft((current) => ({ ...current, [key]: value }));
    setAgentConfigSavedMessage("");
  }

  function updateAgentProfileDraft<Key extends keyof AgentProfileDraft>(
    profileId: string,
    key: Key,
    value: AgentProfileDraft[Key]
  ) {
    setAgentConfigDraft((current) => ({
      ...current,
      agentProfiles: current.agentProfiles.map((profile) => (profile.id === profileId ? { ...profile, [key]: value } : profile))
    }));
    setAgentConfigSavedMessage("");
  }

  async function saveAgentConfigurationDraft() {
    const unavailableProfiles = unavailableAgentProfiles(agentConfigDraft, localCapabilities);
    if (unavailableProfiles.length > 0) {
      setAgentConfigOpen(true);
      setAgentConfigTab("agents");
      setSelectedAgentProfileId(unavailableProfiles[0].id);
      setAgentConfigSavedMessage(
        `Runner unavailable: ${unavailableProfiles.map((profile) => `${profile.label} uses ${profile.runner}`).join(", ")}.`
      );
      return;
    }
    const profileToSave: ProjectAgentProfileInfo = {
      ...agentConfigDraft,
      projectId: primaryProject?.id ?? agentConfigDraft.projectId ?? "project_omega",
      repositoryTargetId: activeRepositoryWorkspace?.id || agentConfigDraft.repositoryTargetId || undefined,
      agentProfiles: agentConfigDraft.agentProfiles
    };
    window.localStorage.setItem(agentConfigurationStorageKey, JSON.stringify(profileToSave));
    if (!missionControlApiUrl) {
      setAgentConfigSavedMessage("Saved as local project draft.");
      return;
    }
    try {
      const savedProfile = await updateProjectAgentProfile(missionControlApiUrl, profileToSave);
      setAgentConfigDraft(normalizeAgentConfigurationDraft(savedProfile));
      setAgentConfigSavedMessage("Saved to local runtime. New pipeline runs will use this profile.");
    } catch (error) {
      setAgentConfigSavedMessage(error instanceof Error ? error.message : "Agent profile save failed.");
    }
  }

  async function chooseLocalWorkspaceFolder() {
    setWorkspaceFolderPickerMessage("");
    const desktopBridge = (window as Window & {
      omegaDesktop?: { selectDirectory?: () => Promise<string | undefined> };
    }).omegaDesktop;
    if (desktopBridge?.selectDirectory) {
      const selectedPath = await desktopBridge.selectDirectory();
      if (selectedPath) {
        setLocalWorkspaceRootDraft(selectedPath);
        setWorkspaceFolderPickerMessage("Folder selected from desktop picker.");
      }
      return;
    }

    const browserPicker = (window as Window & {
      showDirectoryPicker?: (options?: { mode?: "read" | "readwrite" }) => Promise<{ name: string }>;
    }).showDirectoryPicker;
    if (browserPicker) {
      try {
        const handle = await browserPicker({ mode: "readwrite" });
        setWorkspaceFolderPickerMessage(
          `Selected "${handle.name}". Browser mode cannot expose the absolute local path yet, so keep or paste the path below before saving.`
        );
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError") return;
        setWorkspaceFolderPickerMessage(error instanceof Error ? error.message : "Folder picker failed.");
      }
      return;
    }

    setWorkspaceFolderPickerMessage("Folder picker is not available in this browser. Paste the absolute path or use the desktop shell picker.");
  }

  if (appSurface === "home") {
    return <PortalHome onOpenWorkboard={openWorkboard} onOpenPagePilot={openPagePilot} onToggleTheme={toggleUiTheme} uiTheme={uiTheme} />;
  }

  return (
    <main className={shellClassName}>
      <WorkspaceChrome
        activeNav={activeNav}
        activeWorkItemDetail={activeWorkItemDetail}
        activeDetailRepositoryLabel={activeDetailRepositoryLabel}
        activeDetailCompleted={activeDetailCompleted}
        detailRunDisabled={
          activeWorkItemDetail
            ? runningWorkItemId === activeWorkItemDetail.id ||
              activeWorkItemDetail.status === "Planning" ||
              activeWorkItemDetail.status === "In Review"
            : false
        }
        detailRunLabel={
          !activeWorkItemDetail
            ? "Run"
            : activeDetailCompleted
              ? "Rerun"
              : runningWorkItemId === activeWorkItemDetail.id
                ? "Running..."
                : activeWorkItemDetail.status === "Planning"
                  ? "Planning..."
                  : isFailedWork(activeWorkItemDetail, activeDetailPipeline)
                    ? "Retry"
                    : "Run"
        }
        runnerMessage={runnerMessage}
        searchQuery={searchQuery}
        uiTheme={uiTheme}
        repositoryTargets={repositoryTargets}
        workItems={workItems}
        activeRepositoryWorkspaceTargetId={activeRepositoryWorkspaceTargetId}
        workspaceSectionOpen={workspaceSectionOpen}
        connectionsSectionOpen={connectionsSectionOpen}
        visibleConnectionProviders={visibleConnectionProviders}
        selectedProviderId={selectedProviderId}
        connections={connections}
        onBackToWorkItems={() => {
          setActiveWorkItemDetailId("");
          setInspectorOpen(false);
          window.history.pushState(null, "", "#workboard");
        }}
        onHome={openHome}
        onNavigate={(item) => {
          setActiveNav(item);
          clearWorkspaceMessages();
          if (item === "Projects") {
            setActiveRepositoryWorkspaceTargetId("");
            setActiveWorkItemDetailId("");
            window.history.replaceState(null, "", "#workboard");
          }
          if (item === "Views" || item === "Page Pilot") {
            if (item === "Page Pilot" && !activeRepositoryWorkspaceTargetId) {
              const fallbackTargetId = primaryProject?.defaultRepositoryTargetId ?? (repositoryTargets.length === 1 ? repositoryTargets[0].id : "");
              if (fallbackTargetId) setActiveRepositoryWorkspaceTargetId(fallbackTargetId);
            }
            setActiveWorkItemDetailId("");
            window.history.replaceState(null, "", item === "Page Pilot" ? "#page-pilot" : "#workboard");
          }
        }}
        onRunDetail={() => {
          if (activeWorkItemDetail) {
            void runItem(activeWorkItemDetail, { force: activeDetailCompleted });
          }
        }}
        onSearchChange={setSearchQuery}
        onToggleTheme={toggleUiTheme}
        onToggleWorkspaceSection={setWorkspaceSectionOpen}
        onToggleConnectionsSection={setConnectionsSectionOpen}
        onSelectWorkspace={(target, targetItems) => {
          setActiveRepositoryWorkspaceTargetId(target.id);
          setSelectedWorkItemId(targetItems[0]?.id ?? "");
          setActiveWorkItemDetailId("");
          setActiveNav("Issues");
          window.history.replaceState(null, "", "#workboard");
          clearWorkspaceMessages();
        }}
        onConfigureWorkspace={(target) => {
          setActiveRepositoryWorkspaceTargetId(target.id);
          setActiveWorkItemDetailId("");
          setActiveNav("Settings");
          setAgentConfigOpen(true);
          window.history.replaceState(null, "", "#workboard");
          clearWorkspaceMessages();
        }}
        onProviderClick={handleProviderRowClick}
        onNewRequirement={() => {
          setShowInlineCreate((current) => !current);
          setCreateComposerExpanded(true);
          setCreateDescriptionMode("write");
        }}
      >

        {activeNav === "Projects" ? (
          <ProjectSurface
            primaryProject={primaryProject}
            repositoryTargets={repositoryTargets}
            repositoryTargetCount={repositoryTargetCount}
            workItems={workItems}
            pipelines={pipelines}
            activeRepositoryWorkspace={activeRepositoryWorkspace}
            activeRepositoryWorkspaceLabel={activeRepositoryWorkspaceLabel}
            activeRepositoryWorkspaceKey={activeRepositoryWorkspaceKey}
            activeRepositoryWorkspaceItems={activeRepositoryWorkspaceItems}
            activeRepositoryWorkspacePipelines={activeRepositoryWorkspacePipelines}
            repositorySyncMessage={repositorySyncMessage}
            syncingRepositoryKey={syncingRepositoryKey}
            githubRepositoriesLoading={githubRepositoriesLoading}
            githubRepositoryQuery={githubRepositoryQuery}
            githubRepoOwner={githubRepoOwner}
            githubRepoName={githubRepoName}
            selectedRepositoryBound={selectedRepositoryBound}
            filteredGitHubRepositories={filteredGitHubRepositories}
            githubRepoInfo={githubRepoInfo}
            onOpenProjectConfig={() => {
              setActiveNav("Settings");
              setAgentConfigOpen(true);
            }}
            onSyncActiveRepository={() => {
              if (activeRepositoryWorkspace?.kind === "github") {
                void importGitHubIssues(activeRepositoryWorkspace.owner, activeRepositoryWorkspace.repo);
              }
            }}
            onOpenWorkItems={() => setActiveNav("Issues")}
            onRefreshRepositories={loadGitHubRepositories}
            onRepositoryQueryChange={setGitHubRepositoryQuery}
            onCreateOrOpenWorkspace={openSelectedRepositoryWorkspace}
            onSelectGitHubRepository={selectGitHubRepository}
          />
        ) : null}

        {activeNav === "Views" ? (
          <section className="operator-surface">
            {runnerMessage && activeInspectorPanel !== "provider" ? (
              <p className="runner-status operator-runner-status" role="status">
                {runnerMessage}
              </p>
            ) : null}
            <section className="operator-section">
              <div className="operator-section-heading">
                <div>
                  <span className="section-label">Status</span>
                  <h2>Delivery overview</h2>
                </div>
                <button onClick={() => refreshControlPlane()}>Refresh</button>
              </div>
              <article className="metric-strip">
                <div>
                  <span>Work items</span>
                  <strong>{observability.counts.workItems}</strong>
                </div>
                <div>
                  <span>Pipelines</span>
                  <strong>{observability.counts.pipelines}</strong>
                </div>
                <div>
                  <span>Waiting</span>
                  <strong>{observability.attention.waitingHuman}</strong>
                </div>
                <div>
                  <span>Proof</span>
                  <strong>{observability.counts.proofRecords}</strong>
                </div>
              </article>
            </section>

            <section className="operator-section">
              <div className="operator-section-heading">
                <div>
                  <span className="section-label">Runtime</span>
                  <h2>Agent execution</h2>
                </div>
              </div>
              <div className="operator-grid runtime-grid">
                <article className="control-card">
                  <div className="control-card-header">
                    <div>
                      <span className="section-label">Runtime model</span>
                      <h2>{selectedLlmProvider?.name ?? llmSelection.providerId}</h2>
                    </div>
                  </div>
                  <div className="control-form">
                    <label>
                      <span>Provider</span>
                      <select value={llmSelection.providerId} onChange={(event) => chooseLlmProvider(event.currentTarget.value)}>
                        {llmProviders.map((provider) => (
                          <option key={provider.id} value={provider.id}>
                            {provider.name}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label>
                      <span>Model</span>
                      <select value={llmSelection.model} onChange={(event) => chooseLlmModel(event.currentTarget.value)}>
                        {(selectedLlmProvider?.models ?? [llmSelection.model]).map((model) => (
                          <option key={model} value={model}>
                            {model}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label>
                      <span>Local runner</span>
                      <select value={localRunner} onChange={(event) => setLocalRunner(event.currentTarget.value as MissionControlRunnerPreset)}>
                        <option value="local-proof">local-proof</option>
                        <option value="demo-code" disabled={!localCapabilities.some((capability) => capability.id === "git" && capability.available)}>
                          demo-code
                        </option>
                        <option value="codex" disabled={!localCapabilities.some((capability) => capability.id === "codex" && capability.available)}>
                          codex
                        </option>
                      </select>
                    </label>
                  </div>
                </article>

                <article className="control-card">
                  <div className="control-card-header">
                    <div>
                      <span className="section-label">Local tools</span>
                      <h2>{localCapabilities.filter((capability) => capability.available).length} available</h2>
                    </div>
                  </div>
                  <div className="capability-list compact-list">
                    {localCapabilities.map((capability) => (
                      <div key={capability.id}>
                        <span>
                          <strong>{capability.command}</strong>
                          <small>{capability.version || capability.category}</small>
                        </span>
                        <span className={capability.available ? "tool-status ready" : "tool-status missing"}>
                          {capability.available ? "ready" : "missing"}
                        </span>
                      </div>
                    ))}
                  </div>
                </article>
              </div>
            </section>

            <section className="operator-section">
              <div className="operator-section-heading">
                <div>
                  <span className="section-label">Pipeline</span>
                  <h2>Templates, runs, and gates</h2>
                </div>
              </div>
              <div className="operator-grid secondary">
                <article className="control-card">
                  <div className="control-card-header">
                    <div>
                      <span className="section-label">Pipeline templates</span>
                      <h2>{pipelineTemplates.length} templates</h2>
                    </div>
                  </div>
                  <div className="template-list">
                    {pipelineTemplates.map((template) => (
                      <div key={template.id}>
                        <span>
                          <strong>{template.name}</strong>
                          <small>{template.stages.length} stages</small>
                        </span>
                        <button disabled={!selectedWorkItem} onClick={() => createPipelineForSelectedItem(template.id)}>Use</button>
                      </div>
                    ))}
                  </div>
                </article>

                <article className="control-card">
                  <div className="control-card-header">
                    <div>
                      <span className="section-label">Agent contracts</span>
                      <h2>{agentDefinitions.length} agents</h2>
                    </div>
                  </div>
                  <div className="agent-list">
                    {agentDefinitions.slice(0, 6).map((agent) => (
                      <div key={agent.id}>
                        <strong>{agent.name}</strong>
                        <span>{agent.outputContract.length} outputs</span>
                      </div>
                    ))}
                  </div>
                </article>

                <article className="control-card">
                  <div className="control-card-header">
                    <div>
                      <span className="section-label">Pipelines</span>
                      <h2>{pipelines.length} active records</h2>
                    </div>
                  </div>
                  <div className="pipeline-list">
                    {pipelines.slice(0, 5).map((pipeline) => (
                      <div key={pipeline.id} className="pipeline-record">
                        <span>
                          <strong>{pipeline.id}</strong>
                          <small>{pipeline.status}</small>
                          {pipeline.run?.stages?.length ? (
                            <span className="stage-strip" aria-label={`${pipeline.id} stages`}>
                              {pipeline.run.stages.slice(0, 8).map((stage) => (
                                <span key={stage.id} className={`stage-pill ${stage.status}`}>
                                  {stage.title}
                                </span>
                              ))}
                            </span>
                          ) : null}
                        </span>
                        <span className="inline-actions">
                          {pipeline.status === "ready" || pipeline.status === "draft" ? (
                            <button onClick={() => startOperatorPipeline(pipeline.id)}>Start</button>
                          ) : null}
                          {pipeline.status === "ready" || pipeline.status === "draft" || pipeline.status === "running" ? (
                            <button onClick={() => runOperatorPipelineStage(pipeline.id)}>Run stage</button>
                          ) : null}
                          {pipeline.templateId === "devflow-pr" && pipeline.status !== "done" ? (
                            <button onClick={() => runOperatorDevFlowCycle(pipeline.id)}>Run DevFlow cycle</button>
                          ) : null}
                        </span>
                      </div>
                    ))}
                  </div>
                </article>

                <article className="control-card">
                  <div className="control-card-header">
                    <div>
                      <span className="section-label">Human checkpoints</span>
                      <h2>{checkpoints.filter((checkpoint) => checkpoint.status === "pending").length} pending</h2>
                    </div>
                  </div>
                  <div className="checkpoint-list">
                    {checkpoints.slice(0, 5).map((checkpoint) => (
                      <div key={checkpoint.id}>
                        <span>
                          <strong>{checkpoint.title}</strong>
                          <small>{checkpoint.summary}</small>
                        </span>
                        <span className="checkpoint-actions">
                          <button disabled={checkpoint.status !== "pending"} onClick={() => approvePendingCheckpoint(checkpoint.id)}>Approve</button>
                          <button disabled={checkpoint.status !== "pending"} onClick={() => rejectPendingCheckpoint(checkpoint.id)}>Request changes</button>
                        </span>
                      </div>
                    ))}
                  </div>
                </article>
              </div>
            </section>

            <section className="operator-section">
              <div className="operator-section-heading">
                <div>
                  <span className="section-label">Execution</span>
                  <h2>Locks and runner processes</h2>
                </div>
              </div>
              <div className="operator-grid execution-grid">
                <article className="control-card">
                  <div className="control-card-header">
                    <div>
                      <span className="section-label">Execution locks</span>
                      <h2>{executionLocks.filter((lock) => lock.status !== "released").length} active</h2>
                    </div>
                  </div>
                  <div className="execution-list">
                    {executionLocks.length === 0 ? (
                      <p className="muted-copy">No active execution locks.</p>
                    ) : (
                      executionLocks.slice(0, 8).map((lock) => (
                        <article key={lock.id} className="execution-lock-row">
                          <span>
                            <strong>{lock.scope}</strong>
                            <small>
                              {lock.status}
                              {lock.pipelineId ? ` · ${lock.pipelineId}` : ""}
                            </small>
                          </span>
                          <button
                            type="button"
                            disabled={lock.status === "released"}
                            onClick={() => releaseOperatorExecutionLock(lock.id)}
                          >
                            Release
                          </button>
                        </article>
                      ))
                    )}
                  </div>
                </article>

                <article className="control-card">
                  <div className="control-card-header">
                    <div>
                      <span className="section-label">Runner processes</span>
                      <h2>{recentOperations.length} recent</h2>
                    </div>
                  </div>
                  <div className="process-list">
                    {recentOperations.length === 0 ? (
                      <p className="muted-copy">No runner process has been recorded yet.</p>
                    ) : (
                      recentOperations.map((operation) => (
                        <article key={operation.id} className="process-row">
                          <header>
                            <strong>{operation.id}</strong>
                            <span className={`process-status ${operation.runnerProcess?.status ?? operation.status}`}>
                              {operation.runnerProcess?.status ?? operation.status}
                            </span>
                          </header>
                          <p className="telemetry-line">
                            <span>{operation.runnerProcess?.runner ?? operation.agentId ?? "runner"}</span>
                            {operation.runnerProcess?.command ? <span>{operation.runnerProcess.command}</span> : null}
                            {typeof operation.runnerProcess?.exitCode === "number" ? (
                              <span>exit {operation.runnerProcess.exitCode}</span>
                            ) : null}
                            {typeof operation.runnerProcess?.durationMs === "number" ? (
                              <span>{operation.runnerProcess.durationMs}ms</span>
                            ) : null}
                          </p>
                          {operation.runnerProcess?.stderr || operation.runnerProcess?.stdout ? (
                            <details className="process-output-details">
                              <summary>Runner output</summary>
                              <pre className="process-output">
                                {(operation.runnerProcess.stderr || operation.runnerProcess.stdout || "").trim()}
                              </pre>
                            </details>
                          ) : null}
                        </article>
                      ))
                    )}
                  </div>
                </article>

                <article className="control-card runtime-log-card">
                  <div className="control-card-header">
                    <div>
                      <span className="section-label">Runtime logs</span>
                      <h2>{observability.counts.runtimeLogs ?? runtimeLogs.length} entries</h2>
                    </div>
                  </div>
                  <div className="runtime-log-list">
                    {recentRuntimeLogs.length === 0 ? (
                      <p className="muted-copy">No runtime logs have been recorded yet.</p>
                    ) : (
                      recentRuntimeLogs.map((record) => (
                        <article key={record.id} className={`runtime-log-row ${record.level.toLowerCase()}`}>
                          <header>
                            <span className={`runtime-log-level ${record.level.toLowerCase()}`}>{record.level}</span>
                            <strong>{record.eventType}</strong>
                            <time>{formatShortTimestamp(record.createdAt)}</time>
                          </header>
                          <p>{record.message}</p>
                          <div className="runtime-log-meta">
                            {record.workItemId ? <span>{record.workItemId}</span> : null}
                            {record.pipelineId ? <span>{record.pipelineId}</span> : null}
                            {record.attemptId ? <span>{record.attemptId}</span> : null}
                            {record.requestId ? <span>{record.requestId}</span> : null}
                          </div>
                        </article>
                      ))
                    )}
                  </div>
                </article>
              </div>
            </section>

            <section className="operator-section">
              <div className="operator-section-heading">
                <div>
                  <span className="section-label">Collaboration</span>
                  <h2>Review notifications and activity</h2>
                </div>
              </div>
              <div className="operator-grid collaboration-grid">
                <article className="control-card">
                  <div className="control-card-header">
                    <div>
                      <span className="section-label">Feishu</span>
                      <h2>Notify reviewer</h2>
                    </div>
                  </div>
                  <div className="control-form">
                    <label>
                      <span>Feishu chat</span>
                      <input
                        value={feishuChatId}
                        onChange={(event) => {
                          const value = event.currentTarget.value;
                          setFeishuChatId(value);
                        }}
                        placeholder="oc_xxx"
                      />
                    </label>
                    <button onClick={notifyFeishu}>Notify Feishu</button>
                  </div>
                </article>

                <section className="activity-panel">
                  {activityFeed.length === 0 ? (
                    <p>No activity yet. Run an item and the activity feed will appear here.</p>
                  ) : (
                    activityFeed.map((item) => (
                      <article key={item.id} className={`activity-item ${item.kind}`}>
                        <strong>{item.title}</strong>
                        <span>{item.detail}</span>
                      </article>
                    ))
                  )}
                </section>
              </div>
            </section>
          </section>
        ) : null}

        {activeNav === "Settings" ? (
          <section className="settings-surface">
            <section className="operator-section">
              <div className="operator-section-heading">
                <div>
                  <span className="section-label">Workspace config</span>
                  <h2>{activeRepositoryWorkspaceLabel || primaryProject?.name || "Omega Project"}</h2>
                </div>
                {activeRepositoryWorkspace ? (
                  <button type="button" onClick={() => setActiveNav("Issues")}>
                    Open work items
                  </button>
                ) : null}
              </div>
              <div className="operator-grid settings-grid">
                <article className="control-card workspace-location-card">
                  <div className="control-card-header">
                    <div>
                      <span className="section-label">Local runtime</span>
                      <h2>Workspace folder</h2>
                    </div>
                    <details className="info-popover">
                      <summary aria-label="About workspace folder">
                        <InfoIcon />
                      </summary>
                      <p>Omega creates isolated runner workspaces under this folder before dispatching Agent stages.</p>
                    </details>
                  </div>
                  <div className="folder-picker">
                    <label>
                      <span>Folder path</span>
                      <input
                        value={localWorkspaceRootDraft}
                        onChange={(event) => {
                          setLocalWorkspaceRootDraft(event.currentTarget.value);
                          setWorkspaceFolderPickerMessage("");
                        }}
                        placeholder="~/Omega/workspaces"
                      />
                    </label>
                    <div className="folder-picker-actions">
                      <button type="button" onClick={chooseLocalWorkspaceFolder}>
                        Choose folder
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          setLocalWorkspaceRootDraft("~/Omega/workspaces");
                          setWorkspaceFolderPickerMessage("Default Omega workspace folder selected.");
                        }}
                      >
                        Use default
                      </button>
                      <button type="button" className="primary-action" onClick={saveLocalWorkspaceRoot}>
                        Save
                      </button>
                    </div>
                    {localWorkspaceRoot ? <small>Current: {localWorkspaceRoot}</small> : null}
                    {workspaceFolderPickerMessage ? <p role="status">{workspaceFolderPickerMessage}</p> : null}
                  </div>
                </article>

                <article className="control-card agent-config-map">
                  <div className="control-card-header">
                    <div>
                      <span className="section-label">Scope</span>
                      <h2>{activeRepositoryWorkspaceLabel ? "Repository" : "Project default"}</h2>
                    </div>
                    <details className="info-popover">
                      <summary aria-label="About config scope">
                        <InfoIcon />
                      </summary>
                      <p>These settings are resolved before a Pipeline starts and are written into each Agent runner's runtime spec.</p>
                    </details>
                  </div>
                  <div className="agent-config-chip-row">
                    <span>
                      <small>Template</small>
                      <strong>{agentConfigDraft.workflowTemplate}</strong>
                    </span>
                    <span>
                      <small>Runner</small>
                      <strong>{agentConfigDraft.runner}</strong>
                    </span>
                    <span>
                      <small>Contracts</small>
                      <strong>{agentDefinitions.length}</strong>
                    </span>
                  </div>
                  <div className="agent-config-map-list">
                    <button
                      type="button"
                      className="agent-config-map-entry"
                      onClick={() => {
                        setAgentConfigOpen(true);
                        setAgentConfigTab("workflow");
                      }}
                    >
                      <strong>Workflow</strong>
                      <small>Stage markdown</small>
                    </button>
                    <button
                      type="button"
                      className="agent-config-map-entry"
                      onClick={() => {
                        setAgentConfigOpen(true);
                        setAgentConfigTab("agents");
                      }}
                    >
                      <strong>Tools</strong>
                      <small>MCP / Skills</small>
                    </button>
                    <button
                      type="button"
                      className="agent-config-map-entry"
                      onClick={() => {
                        setAgentConfigOpen(true);
                        setAgentConfigTab("runtime");
                      }}
                    >
                      <strong>Runtime</strong>
                      <small>Policy files</small>
                    </button>
                  </div>
                </article>
              </div>

              {activeRepositoryWorkspace ? (
                <article className="control-card workspace-management-card">
                  <div className="control-card-header">
                    <div>
                      <span className="section-label">Operations</span>
                      <h2>Workspace controls</h2>
                    </div>
                    <details className="info-popover align-right">
                      <summary aria-label="About workspace controls">
                        <InfoIcon />
                      </summary>
                      <p>These controls apply only to {activeRepositoryWorkspaceLabel}. Deleting removes Omega's workspace target, not the GitHub repository.</p>
                    </details>
                  </div>
                  <div className="workspace-management-grid">
                    <div className="workspace-management-row">
                      <span>
                        <strong>Auto scan</strong>
                        <small>
                          {activeRepositoryWorkspace.kind === "github"
                            ? activeRepositoryWatcherActive
                              ? "On · ready issues can start"
                              : "Off · manual run only"
                            : "GitHub workspace only"}
                        </small>
                      </span>
                      <button
                        type="button"
                        role="switch"
                        aria-checked={activeRepositoryWatcherActive}
                        aria-label="Auto scan ready GitHub issues"
                        className={activeRepositoryWatcherActive ? "workspace-switch active" : "workspace-switch"}
                        disabled={activeRepositoryWorkspace.kind !== "github" || syncingRepositoryKey === activeRepositoryWorkspaceKey}
                        onClick={() => {
                          void toggleRepositoryAutoProcessing(activeRepositoryWorkspace.id);
                        }}
                      >
                        <span />
                      </button>
                    </div>
                    <div className="workspace-management-row danger-zone">
                      <span>
                        <strong>Delete workspace</strong>
                        <small>Remove from Omega only</small>
                      </span>
                      <button
                        type="button"
                        className="danger-action workspace-delete-action"
                        onClick={() => {
                          void deleteRepositoryWorkspace(activeRepositoryWorkspace.id);
                        }}
                      >
                        Delete workspace
                      </button>
                    </div>
                  </div>
                </article>
              ) : null}
            </section>

            <section className="operator-section agent-config-section">
              <div className="operator-section-heading">
                <div>
                  <span className="section-label">Agent profile</span>
                  <h2>Project Agent Profile</h2>
                </div>
                <button type="button" onClick={() => setAgentConfigOpen((current) => !current)}>
                  {agentConfigOpen ? "Collapse editor" : "Edit profile"}
                </button>
              </div>

              <article className="control-card agent-config-card">
                <div className="control-card-header">
                  <div>
                    <span className="section-label">DevFlow defaults</span>
                    <h2>{primaryProject?.name ?? "Omega"} Agent orchestration</h2>
                    <p>Draft workflow, per-Agent tools, and local runtime policy for this project.</p>
                  </div>
                  <button type="button" className="primary-action" onClick={saveAgentConfigurationDraft}>
                    Save draft
                  </button>
                </div>

                {agentConfigOpen ? (
                  <div className="agent-config-shell">
                    <div className="agent-config-summary-grid" aria-label="Agent profile summary">
                      <span>
                        <strong>{agentConfigDraft.workflowTemplate}</strong>
                        <small>workflow draft</small>
                      </span>
                      <span>
                        <strong>{agentConfigDraft.agentProfiles.length}</strong>
                        <small>agent profiles</small>
                      </span>
                      <span>
                        <strong>{agentConfigDraft.agentProfiles.reduce((count, profile) => count + profile.skills.split("\n").filter(Boolean).length, 0)}</strong>
                        <small>skill bindings</small>
                      </span>
                      <span>
                        <strong>.omega + .codex + .claude</strong>
                        <small>runtime files</small>
                      </span>
                    </div>

                    <div className="agent-config-tabs" role="tablist" aria-label="Project Agent Profile sections">
                      {(["workflow", "agents", "runtime"] as AgentConfigTab[]).map((tab) => (
                        <button
                          key={tab}
                          type="button"
                          className={agentConfigTab === tab ? "active" : ""}
                          onClick={() => setAgentConfigTab(tab)}
                        >
                          {tab === "workflow" ? "Workflow" : tab === "agents" ? "Agents" : "Runtime files"}
                        </button>
                      ))}
                    </div>

                    {agentConfigTab === "workflow" ? (
                      <div className="workflow-builder">
                        <div className="workflow-stage-flow" aria-label="Workflow parser draft">
                          {workflowStagePreview.map((stage, index) => (
                            <article key={stage.id} className="workflow-stage-card">
                              <span>{String(index + 1).padStart(2, "0")}</span>
                              <strong>{stage.title}</strong>
                              <small>{stage.agents}</small>
                              <em>{stage.gate}</em>
                            </article>
                          ))}
                        </div>
                        <div className="control-form workflow-markdown-editor">
                          <label>
                            <span>Template</span>
                            <select
                              value={agentConfigDraft.workflowTemplate}
                              onChange={(event) => updateAgentConfigDraft("workflowTemplate", event.currentTarget.value)}
                            >
                              <option value="devflow-pr">devflow-pr</option>
                              {pipelineTemplates
                                .filter((template) => template.id !== "devflow-pr")
                                .map((template) => (
                                  <option key={template.id} value={template.id}>
                                    {template.name}
                                  </option>
                                ))}
                            </select>
                          </label>
                          <label>
                            <span>Markdown</span>
                            <textarea
                              value={agentConfigDraft.workflowMarkdown}
                              onChange={(event) => updateAgentConfigDraft("workflowMarkdown", event.currentTarget.value)}
                            />
                          </label>
                          <label>
                            <span>Rules</span>
                            <textarea
                              value={agentConfigDraft.stagePolicy}
                              onChange={(event) => updateAgentConfigDraft("stagePolicy", event.currentTarget.value)}
                            />
                          </label>
                        </div>
                      </div>
                    ) : null}

                    {agentConfigTab === "agents" ? (
                      <div className="agent-profile-layout">
                        <div className="agent-roster" aria-label="Agent roster">
                          {agentConfigDraft.agentProfiles.map((profile) => (
                            <button
                              key={profile.id}
                              type="button"
                              className={[
                                profile.id === selectedAgentProfile.id ? "active" : "",
                                unavailableAgentProfiles({ ...agentConfigDraft, agentProfiles: [profile] }, localCapabilities).length > 0
                                  ? "runner-missing"
                                  : ""
                              ]
                                .filter(Boolean)
                                .join(" ")}
                              onClick={() => setSelectedAgentProfileId(profile.id)}
                            >
                              <strong>{profile.label}</strong>
                              <span>{profile.runner} · {profile.model}</span>
                            </button>
                          ))}
                        </div>
                        <div className="control-form agent-profile-editor">
                          <div className="agent-profile-editor-header">
                            <span className="section-label">Agent override</span>
                            <strong>{selectedAgentProfile.label}</strong>
                          </div>
                          <label>
                            <span>Runner</span>
                            <select
                              value={selectedAgentProfile.runner}
                              onChange={(event) =>
                                updateAgentProfileDraft(
                                  selectedAgentProfile.id,
                                  "runner",
                                  event.currentTarget.value as AgentProfileDraft["runner"]
                                )
                              }
                            >
                              {agentRunnerOptions.map((option) => (
                                <option
                                  key={option.value}
                                  value={option.value}
                                  disabled={!capabilityAvailable(localCapabilities, option.capabilityId)}
                                >
                                  {option.label}
                                  {!capabilityAvailable(localCapabilities, option.capabilityId) ? " (missing)" : ""}
                                </option>
                              ))}
                            </select>
                            <small className={selectedRunnerReady ? "runner-availability ready" : "runner-availability missing"}>
                              {selectedRunnerAvailability}
                            </small>
                          </label>
                          <label>
                            <span>Model</span>
                            <input
                              value={selectedAgentProfile.model}
                              onChange={(event) => updateAgentProfileDraft(selectedAgentProfile.id, "model", event.currentTarget.value)}
                            />
                          </label>
                          <label>
                            <span>Skills</span>
                            <textarea
                              value={selectedAgentProfile.skills}
                              onChange={(event) => updateAgentProfileDraft(selectedAgentProfile.id, "skills", event.currentTarget.value)}
                            />
                          </label>
                          <label>
                            <span>MCP</span>
                            <textarea
                              value={selectedAgentProfile.mcp}
                              onChange={(event) => updateAgentProfileDraft(selectedAgentProfile.id, "mcp", event.currentTarget.value)}
                            />
                          </label>
                          <label>
                            <span>Stage note</span>
                            <textarea
                              value={selectedAgentProfile.stageNotes}
                              onChange={(event) => updateAgentProfileDraft(selectedAgentProfile.id, "stageNotes", event.currentTarget.value)}
                            />
                          </label>
                        </div>
                      </div>
                    ) : null}

                    {agentConfigTab === "runtime" ? (
                      <div className="runtime-config-layout">
                        <div className="runtime-file-tabs" role="tablist" aria-label="Runtime file templates">
                          {(["omega", "codex", "claude"] as RuntimeConfigTab[]).map((tab) => (
                            <button
                              key={tab}
                              type="button"
                              className={runtimeConfigTab === tab ? "active" : ""}
                              onClick={() => setRuntimeConfigTab(tab)}
                            >
                              {tab === "omega" ? ".omega/agent-runtime.json" : tab === "codex" ? ".codex/OMEGA.md" : ".claude/CLAUDE.md"}
                            </button>
                          ))}
                        </div>
                        <div className="control-form runtime-policy-editor">
                          <label>
                            <span>.codex</span>
                            <textarea
                              value={selectedAgentProfile.codexPolicy}
                              onChange={(event) => updateAgentProfileDraft(selectedAgentProfile.id, "codexPolicy", event.currentTarget.value)}
                            />
                          </label>
                          <label>
                            <span>.claude</span>
                            <textarea
                              value={selectedAgentProfile.claudePolicy}
                              onChange={(event) => updateAgentProfileDraft(selectedAgentProfile.id, "claudePolicy", event.currentTarget.value)}
                            />
                          </label>
                        </div>
                        <div className="agent-config-preview">
                          <span className="section-label">Runtime file preview</span>
                          <pre>{runtimeConfigPreview}</pre>
                        </div>
                      </div>
                    ) : null}

                    {agentConfigSavedMessage ? <p className="agent-config-save-status" role="status">{agentConfigSavedMessage}</p> : null}
                  </div>
                ) : (
                  <div className="agent-profile-summary">
                    <span>Workflow: {agentConfigDraft.workflowTemplate}</span>
                    <span>Agents: {agentConfigDraft.agentProfiles.length}</span>
                    <span>Runtime: .omega / .codex / .claude</span>
                    <span>Draft: local only</span>
                  </div>
                )}
              </article>
            </section>
          </section>
        ) : null}

        {activeNav === "Page Pilot" ? (
          <PagePilotPreview
            projectId={primaryProject?.id ?? "project_omega"}
            repositoryTargets={repositoryTargets}
            repositoryTargetId={pagePilotRepositoryTarget?.id}
            repositoryLabel={pagePilotRepositoryLabel}
            apiAvailable={Boolean(missionControlApiUrl)}
            onReloadApp={() => void reloadDesktopApp()}
            onSelectRepositoryTarget={(targetId) => {
              setActiveRepositoryWorkspaceTargetId(targetId);
              clearWorkspaceMessages();
            }}
            onApply={applyPagePilotChange}
            onDeliver={deliverPagePilotConfirmedChange}
            onDiscard={discardPagePilotPendingChange}
            onFetchRuns={loadPagePilotRuns}
            onExit={() => setActiveNav("Issues")}
          />
        ) : null}

        {activeNav === "Issues" ? (
          <>
            {activeWorkItemDetail ? (
              <WorkItemDetailPage
                agentShortLabel={agentShortLabel}
                attemptStatusLabel={attemptStatusLabel}
                attemptTimeline={activeAttemptTimeline}
                attempts={activeDetailAttempts}
                checkpoints={checkpoints}
                operations={operations}
                operationStatusLabel={operationStatusLabel}
                pipeline={activeDetailPipeline}
                pipelineStageClassName={pipelineStageClassName}
                pipelineStageLabel={pipelineStageLabel}
                proofRecords={proofRecords}
                pullRequestStatus={activePullRequestStatus}
                repositoryLabel={activeDetailRepositoryLabel}
                repositoryTargets={repositoryTargets}
                requirements={requirements}
                runWorkpads={runWorkpads}
                sourceLabel={sourceLabel}
                statusClassName={statusClassName}
                workItem={activeWorkItemDetail}
                workItems={displayWorkItems}
                workItemStatusLabel={workItemStatusLabel}
                onOpenPagePilot={() => openPagePilotForRepository(activeWorkItemDetail.repositoryTargetId)}
                onApproveCheckpoint={(checkpointId) => void approvePendingCheckpoint(checkpointId)}
                onPatchRunWorkpad={updateRunWorkpadPatch}
                onRequestCheckpointChanges={(checkpointId, note) => void rejectPendingCheckpoint(checkpointId, note)}
                onRetryAttempt={(attemptId) => void retryWorkItemAttempt(attemptId)}
              />
            ) : (
            <>
            {showInlineCreate ? (
              <RequirementComposer
                variant="inline"
                title={newItemTitle}
                description={newItemDescription}
                assignee={newItemAssignee}
                target={newItemTarget}
                hasRepositoryWorkspace={Boolean(activeRepositoryWorkspace)}
                repositoryWorkspaceLabel={activeRepositoryWorkspaceLabel}
                isExpanded={createComposerExpanded}
                descriptionMode={createDescriptionMode}
                isCreating={isCreatingItem}
                descriptionPreview={
                  newItemDescription.trim() ? (
                    <div className="markdown-content">{renderMarkdown(newItemDescription)}</div>
                  ) : (
                    <p className="muted-copy">Nothing to preview yet.</p>
                  )
                }
                onTitleChange={setNewItemTitle}
                onDescriptionChange={setNewItemDescription}
                onAssigneeChange={setNewItemAssignee}
                onTargetChange={setNewItemTarget}
                onTitleFocus={() => setCreateComposerExpanded(true)}
                onDescriptionModeChange={setCreateDescriptionMode}
                onCreate={createItem}
              />
            ) : null}

            {activeRepositoryWorkspace ? (
              <>
                <p className="workspace-context" role="status">
                  <strong>{activeRepositoryWorkspaceLabel}</strong>
                  <span>{activeRepositoryWorkspaceItems.length} work items</span>
                  <span>Agent runs are locked to this repository target.</span>
                </p>
                {runnerMessage ? (
                  <p className="runner-status list-runner-status" role="status">
                    {runnerMessage}
                  </p>
                ) : null}
              </>
            ) : runnerMessage ? (
              <p className="runner-status list-runner-status" role="status">
                {runnerMessage}
              </p>
            ) : null}

            <section className="view-controls">
              <label>
                <span>Status</span>
                <select value={statusFilter} onChange={(event) => setStatusFilter(event.currentTarget.value as "All" | WorkItemStatus)}>
                  {["All", "Planning", "Ready", "In Review", "Backlog", "Done", "Blocked"].map((status) => (
                    <option key={status} value={status}>
                      {status === "All" ? "All" : workItemStatusLabel(status as WorkItemStatus)}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                <span>Assignee</span>
                <select value={assigneeFilter} onChange={(event) => setAssigneeFilter(event.currentTarget.value)}>
                  {assigneeOptions.map((assignee) => (
                    <option key={assignee}>{assignee}</option>
                  ))}
                </select>
              </label>
              <label>
                <span>Priority</span>
                <select value={sortDirection} onChange={(event) => setSortDirection(event.currentTarget.value as WorkboardViewSort["direction"])}>
                  <option value="desc">High first</option>
                  <option value="asc">Low first</option>
                </select>
              </label>
            </section>

            {activeRepositoryWorkspace?.kind === "github" ? (
              <section className="github-issues-panel" aria-label="GitHub issues">
                <div className="group-row active github-issues-header">
                  <button
                    type="button"
                    className="group-chevron"
                    aria-label={githubIssuesCollapsed ? "Expand GitHub issues" : "Collapse GitHub issues"}
                    onClick={() => setGithubIssuesCollapsed((current) => !current)}
                  >
                    {githubIssuesCollapsed ? "›" : "⌄"}
                  </button>
                  <span className="issue-state github-source-state" aria-hidden="true" />
                  <strong>GitHub Issues</strong>
                  <span>{activeRepositoryGitHubItems.length}</span>
                  <div className="github-issues-actions">
                    <small>{activeRepositoryGitHubItems.length} synced</small>
                    <button
                      type="button"
                      className="icon-action"
                      aria-label="Sync GitHub issues"
                      title="Sync GitHub issues"
                      disabled={syncingRepositoryKey === activeRepositoryWorkspaceKey}
                      onClick={() => {
                        void importGitHubIssues(activeRepositoryWorkspace.owner, activeRepositoryWorkspace.repo);
                      }}
                    >
                      {syncingRepositoryKey === activeRepositoryWorkspaceKey ? "..." : "↻"}
                    </button>
                  </div>
                </div>
                {githubIssuesCollapsed ? null : (
                  <div className="github-issue-list">
                    {activeRepositoryGitHubItems.length > 0 ? (
                      activeRepositoryGitHubItems.map((item) => (
                        <article
                          key={item.id}
                          className={`github-issue-row ${selectedWorkItem?.id === item.id ? "selected" : ""}`}
                          onClick={() => selectWorkItem(item)}
                        >
                          <div>
                            <span className="issue-key" title={`Internal key: ${item.key}`}>
                              {workItemDisplayLabel(item)}
                            </span>
                            <strong>{item.title}</strong>
                          </div>
                          <small>{item.sourceExternalRef ?? item.target}</small>
                          <span className={`status-pill ${statusClassName(item.status)}`}>
                            {workItemStatusLabel(item.status)}
                          </span>
                        </article>
                      ))
                    ) : (
                      <p className="github-issues-empty">No GitHub issues synced yet.</p>
                    )}
                  </div>
                )}
              </section>
            ) : null}

            {workItems.length === 0 ? (
              <section className="empty-view embedded-empty">
                <div className="empty-icon" aria-hidden="true">
                  <span />
                  <span />
                  <span />
                </div>
                <RequirementComposer
                  variant="empty"
                  title={newItemTitle}
                  description={newItemDescription}
                  assignee={newItemAssignee}
                  target={newItemTarget}
                  hasRepositoryWorkspace={Boolean(activeRepositoryWorkspace)}
                  repositoryWorkspaceLabel={activeRepositoryWorkspaceLabel}
                  isExpanded={createComposerExpanded}
                  descriptionMode={createDescriptionMode}
                  isCreating={isCreatingItem}
                  descriptionPreview={null}
                  onTitleChange={setNewItemTitle}
                  onDescriptionChange={setNewItemDescription}
                  onAssigneeChange={setNewItemAssignee}
                  onTargetChange={setNewItemTarget}
                  onTitleFocus={() => setCreateComposerExpanded(true)}
                  onDescriptionModeChange={setCreateDescriptionMode}
                  onCreate={createItem}
                />
              </section>
            ) : (
            <>
            <section className="issue-table" aria-label="Work items">
              {groupedItems.map((group) => (
                <div key={group.status} className="issue-group">
                  <div className="group-row active">
                    <button className="group-chevron" onClick={() => toggleGroup(group.status)}>
                      {collapsedGroups.includes(group.status) ? "›" : "⌄"}
                    </button>
                    <span className={`issue-state ${statusClassName(group.status)}`} aria-hidden="true" />
                    <strong>{workItemStatusLabel(group.status)}</strong>
                    <span>{group.items.length}</span>
                  </div>
                  {collapsedGroups.includes(group.status)
                    ? null
                    : group.items.map((item) => {
                        const repositoryTarget = item.repositoryTargetId
                          ? repositoryTargets.find((target) => target.id === item.repositoryTargetId)
                          : undefined;
                        const repositoryLabel =
                          repositoryTarget?.kind === "github"
                            ? `${repositoryTarget.owner}/${repositoryTarget.repo}`
                            : repositoryTarget?.path ?? activeRepositoryWorkspaceLabel;
                        const itemPipeline = pipelinesByWorkItemId.get(item.id);
                        const pipelineStages = itemPipeline?.run?.stages ?? [];
                        const itemPendingCheckpoint = itemPipeline
                          ? checkpoints.find((checkpoint) =>
                              checkpoint.pipelineId === itemPipeline.id && checkpoint.status === "pending"
                            )
                          : undefined;
                        const completed = isCompletedWork(item, itemPipeline);
                        const failed = isFailedWork(item, itemPipeline);
                        const runDisabled = completed || runningWorkItemId === item.id || item.status === "Planning" || item.status === "In Review";
                        const progress = summarizePipelineProgress(item, pipelineStages, runningWorkItemId === item.id);
                        const hasProgress = pipelineStages.length > 0 || item.status === "Planning" || runningWorkItemId === item.id;
                        const deleteAllowed = canDeleteWorkItem(item);
                        return (
                          <article
                            key={item.id}
                            className={`issue-row ${selectedWorkItem?.id === item.id ? "selected" : ""} ${pipelineStages.length ? "has-pipeline" : ""}`}
                            onClick={() => selectWorkItem(item)}
                          >
                            <div className="issue-leading">
                              {deleteAllowed ? (
                                <button
                                  type="button"
                                  className="issue-delete-button"
                                  aria-label={`Delete ${item.title}`}
                                  title="Delete not-started item"
                                  onClick={(event) => {
                                    event.stopPropagation();
                                    void deleteWorkItem(item);
                                  }}
                                >
                                  <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
                                    <path d="M9 3h6l1 2h4v2H4V5h4l1-2Z" />
                                    <path d="M6 9h12l-1 11H7L6 9Zm4 2v7h2v-7h-2Zm4 0v7h2v-7h-2Z" />
                                  </svg>
                                </button>
                              ) : (
                                <span className="issue-drag" aria-hidden="true">---</span>
                              )}
                              <span className={`issue-state ${statusClassName(item.status)}`} />
                            </div>
                            <div className="issue-main">
                              <div className="issue-title-line">
                                <span className="issue-key" title={`Internal key: ${item.key}`}>
                                  {workItemDisplayLabel(item)}
                                </span>
                                <strong>{item.title}</strong>
                              </div>
                              <div className="issue-meta-line">
                                {item.sourceExternalRef ? <span>{item.sourceExternalRef}</span> : null}
                                {item.requirementId ? <span>Req {item.requirementId.replace(/^req_/, "")}</span> : null}
                                {repositoryLabel ? <span>{repositoryLabel}</span> : null}
                                <span>{agentShortLabel(item.assignee)}</span>
                              </div>
                            </div>
                            <div className="issue-progress-slot">
                              {hasProgress ? (
                                <div
                                  className={`issue-progress-track ${pipelineStageClassName(progress.status)}`}
                                  aria-label={`${item.key} current progress ${progress.label}`}
                                >
                                  <div className="issue-progress-copy">
                                    <strong>{progress.label}</strong>
                                  </div>
                                  <div className="issue-progress-rail" aria-hidden="true">
                                    <span style={{ width: `${progress.percent}%` }} />
                                  </div>
                                </div>
                              ) : null}
                            </div>
                            <div className="issue-trailing">
                              {itemPendingCheckpoint ? (
                                <button
                                  type="button"
                                  className="review-inline"
                                  onClick={(event) => {
                                    event.stopPropagation();
                                    setSelectedWorkItemId(item.id);
                                    openWorkItemDetail(item.id);
                                    setInspectorOpen(false);
                                  }}
                                >
                                  Human review
                                </button>
                              ) : !completed && failed ? (
                                <button
                                  className="run-inline"
                                  onClick={(event) => {
                                    event.stopPropagation();
                                    void runItem(item);
                                  }}
                                >
                                  Retry
                                </button>
                              ) : !completed && !hasProgress && item.status !== "Planning" && item.status !== "In Review" ? (
                                <button
                                  className="run-inline"
                                  disabled={runDisabled}
                                  onClick={(event) => {
                                    event.stopPropagation();
                                    void runItem(item);
                                  }}
                                >
                                  {runningWorkItemId === item.id ? "Running..." : "Run"}
                                </button>
                              ) : completed ? (
                                <span className="status-pill status-done">Done</span>
                              ) : (
                                <span className={`status-pill ${statusClassName(item.status)}`}>{workItemStatusLabel(item.status)}</span>
                              )}
                            </div>
                          </article>
                        );
                      })}
                </div>
              ))}
            </section>
            </>
            )}
            </>
            )}
          </>
        ) : null}
      </WorkspaceChrome>

      {inspectorAvailable ? (
        <>
        <aside className="inspector-panel" aria-label="Properties">
          {selectedWorkItem ? (
          <details
            className="inspector-block"
            open={activeInspectorPanel === "properties"}
            onToggle={(event) => {
              if (event.currentTarget.open) setActiveInspectorPanel("properties");
            }}
          >
            <summary>
              <span className="section-label">Properties</span>
              <small>{selectedWorkItem.key}</small>
            </summary>
            <div className="properties-grid">
              <label>
                <span>Status</span>
                <select
                  value={selectedWorkItem.status}
                  onChange={async (event) => {
                    const nextStatus = event.currentTarget.value as WorkItemStatus;
                    if (!missionControlApiUrl) {
                      setWorkItems((current) => updateWorkItemStatus(current, selectedWorkItem.id, nextStatus));
                      return;
                    }
                    try {
                      const session = await patchWorkItemViaApi(missionControlApiUrl, run, selectedWorkItem.id, { status: nextStatus });
                      setProjects(session.projects);
                      setRequirements(session.requirements);
                      setWorkItems(session.workItems);
                      setMissionState(session.missionState);
                    } catch (error) {
                      setRunnerMessage(error instanceof Error ? error.message : "Update status failed.");
                    }
                  }}
                >
                  {["Planning", "Ready", "In Review", "Backlog", "Done", "Blocked"].map((status) => (
                    <option key={status} value={status}>
                      {workItemStatusLabel(status as WorkItemStatus)}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                <span>Priority</span>
                <select
                  value={selectedWorkItem.priority}
                  onChange={async (event) => {
                    const nextPriority = event.currentTarget.value as WorkItemPriority;
                    if (!missionControlApiUrl) {
                      setWorkItems((current) => updateWorkItemPriority(current, selectedWorkItem.id, nextPriority));
                      return;
                    }
                    try {
                      const session = await patchWorkItemViaApi(missionControlApiUrl, run, selectedWorkItem.id, { priority: nextPriority });
                      setProjects(session.projects);
                      setRequirements(session.requirements);
                      setWorkItems(session.workItems);
                      setMissionState(session.missionState);
                    } catch (error) {
                      setRunnerMessage(error instanceof Error ? error.message : "Update priority failed.");
                    }
                  }}
                >
                  {["No priority", "Low", "Medium", "High", "Urgent"].map((priority) => (
                    <option key={priority}>{priority}</option>
                  ))}
                </select>
              </label>
              <label>
                <span>Assignee</span>
                <input value={selectedWorkItem.assignee} readOnly />
              </label>
              <label>
                <span>Target</span>
                <input value={selectedWorkItem.target} readOnly />
              </label>
            </div>
            <div className="property-copy">
              <span>Requirement</span>
              <p>{selectedRequirement?.title ?? selectedWorkItem.title}</p>
              {selectedRequirement?.sourceExternalRef ? <small>{selectedRequirement.sourceExternalRef}</small> : null}
            </div>
            <div className="property-copy">
              <span>Item description</span>
              <p>{displayText(selectedWorkItem.description)}</p>
            </div>
            <div className="label-stack">
              {selectedWorkItem.labels.map((label) => (
                <span key={label}>{label}</span>
              ))}
            </div>
          </details>
          ) : null}

          <details
            className="inspector-block"
            open={activeInspectorPanel === "provider"}
            onToggle={(event) => {
              if (event.currentTarget.open) setActiveInspectorPanel("provider");
            }}
          >
            <summary>
              <span className="section-label">Provider access</span>
              <small>{selectedProvider?.name}</small>
            </summary>
            {visibleConnectionProviders
              .filter((provider) => provider.id === selectedProvider?.id)
              .map((provider) => (
                <div key={provider.id} className="provider-panel">
                  <h2>{provider.name}</h2>
                  <p>{provider.description}</p>
                  <div className="provider-status">
                    <span className={connections[provider.id].status === "connected" ? "dot online" : "dot"} />
                    <span>{connections[provider.id].status}</span>
                    <small>
                      {canUseLocalGitHubOAuth(provider)
                        ? githubOAuthConfig.configured
                          ? `oauth ready (${githubOAuthConfig.source})`
                          : "oauth setup"
                        : provider.authMethod === "oauth" && !providerClientIds[provider.id]
                        ? "client id required"
                        : provider.authMethod}
                    </small>
                  </div>
                  {providerFeedback && provider.id === selectedProviderId ? (
                    <div className="provider-feedback" role="status">
                      <span>{providerFeedback}</span>
                      {provider.id === "github" && githubDeviceLoginUrl ? (
                        <div className="provider-feedback-actions">
                          <button onClick={() => openExternalUrlInNewTab(githubDeviceLoginUrl)}>
                            Open device page
                          </button>
                          <button onClick={refreshGitHubConnectionStatus}>
                            Check GitHub status
                          </button>
                        </div>
                      ) : null}
                    </div>
                  ) : null}
                  {canUseLocalGitHubOAuth(provider) ? (
                    <>
                      {!githubOAuthConfig.configured ? (
                        <div className="provider-setup-note">
                          <strong>GitHub sign-in needs one local OAuth app setup.</strong>
                          <span>After that, this row will open GitHub authorization directly.</span>
                        </div>
                      ) : null}
                      <details
                        className="provider-advanced"
                        open={githubOAuthSetupOpen}
                        onToggle={(event) => setGitHubOAuthSetupOpen(event.currentTarget.open)}
                      >
                        <summary>OAuth app setup</summary>
                        <div className="provider-config-grid">
                          <label>
                            <span>Client ID</span>
                            <input
                              value={githubOAuthDraft.clientId}
                              onChange={(event) => {
                                const value = event.currentTarget.value;
                                setGitHubOAuthDraft((current) => ({ ...current, clientId: value }));
                              }}
                              placeholder="GitHub OAuth app client ID"
                            />
                          </label>
                          <label>
                            <span>Client secret</span>
                            <input
                              type="password"
                              value={githubOAuthDraft.clientSecret}
                              onChange={(event) => {
                                const value = event.currentTarget.value;
                                setGitHubOAuthDraft((current) => ({ ...current, clientSecret: value }));
                              }}
                              placeholder={githubOAuthConfig.secretConfigured ? "Saved secret" : "GitHub OAuth app secret"}
                            />
                          </label>
                          <label>
                            <span>Callback URL</span>
                            <input
                              value={githubOAuthDraft.redirectUri}
                              onChange={(event) => {
                                const value = event.currentTarget.value;
                                setGitHubOAuthDraft((current) => ({ ...current, redirectUri: value }));
                              }}
                            />
                          </label>
                          <button onClick={saveGitHubOAuthConfig}>Save OAuth app</button>
                        </div>
                      </details>
                    </>
                  ) : null}
                  <div className="scope-list">
                    {provider.scopes.map((scope) => (
                      <span key={scope}>{scope}</span>
                    ))}
                  </div>
                  <div className="permission-list">
                    {provider.permissions.map((permission) => (
                      <div key={permission.id}>
                        <span>{permission.label}</span>
                        <small>{permission.risk}</small>
                      </div>
                    ))}
                  </div>
                  <div className="provider-actions">
                    <button
                      disabled={oauthNeedsClientId(provider)}
                      onClick={() => connectProvider(provider)}
                    >
                      {provider.id === "github"
                        ? "Continue with GitHub"
                        : provider.authMethod === "oauth"
                        ? "Open OAuth"
                        : "Connect"}
                    </button>
                    <button
                      disabled={connections[provider.id].status !== "connected"}
                      onClick={() => disconnectProvider(provider.id)}
                    >
                      Disconnect
                    </button>
                  </div>
                </div>
              ))}
          </details>
        </aside>
        <aside className="inspector-rail" aria-label="Inspector panels">
          <button
            className="rail-button layout-button"
            aria-label={inspectorOpen ? "Collapse inspector" : "Expand inspector"}
            onClick={() => setInspectorOpen((current) => !current)}
            title={inspectorOpen ? "Collapse inspector" : "Expand inspector"}
          >
            <span className="layout-icon" aria-hidden="true">
              <span />
              <span />
            </span>
          </button>
          <button
            className={activeInspectorPanel === "properties" ? "rail-button active" : "rail-button"}
            aria-label="Properties"
            title="Properties"
            onClick={() => openInspectorPanel("properties")}
          >
            <span className="rail-icon info-icon" aria-hidden="true">i</span>
          </button>
          <button
            className={activeInspectorPanel === "provider" ? "rail-button active" : "rail-button"}
            aria-label="Provider access"
            title="Provider access"
            onClick={() => openInspectorPanel("provider")}
          >
            <span className="rail-icon link-icon" aria-hidden="true" />
          </button>
        </aside>
        </>
      ) : null}
    </main>
  );
}

export default App;
