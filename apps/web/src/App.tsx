import { type ReactNode, useEffect, useMemo, useState } from "react";
import {
  buildAuthorizeUrl,
  connectionProviders,
  createActivityFeed,
  createMissionFromRun,
  createSampleRun,
  createWorkboardView,
  applyMissionControlEvents,
  groupWorkItemsByStatus,
  grantProviderConnection,
  loadWorkspaceSession,
  revokeProviderConnection,
  saveWorkspaceSession,
  updateWorkItemPriority,
  updateWorkItemStatus
} from "./core";
import { runOperationViaMissionControlApi } from "./missionControlApiClient";
import type { MissionControlRunnerPreset } from "./missionControlApiClient";
import { navigateToExternalUrl } from "./browserNavigation";
import { openExternalUrlInNewTab } from "./browserNavigation";
import { PortalHome } from "./components/PortalHome";
import {
  AgentTraceList,
  ArtifactGrid,
  AttemptHistory,
  WorkItemAttemptPanel
} from "./components/WorkItemDetailPanels";
import {
  approveCheckpoint,
  createPipelineFromTemplate,
  fetchAttempts,
  fetchExecutionLocks,
  fetchCheckpoints,
  fetchAgentDefinitions,
  fetchGitHubOAuthConfig,
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
  fetchProofRecords,
  fetchProjectAgentProfile,
  fetchRequirements,
  releaseExecutionLock,
  requestCheckpointChanges,
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
  type CheckpointRecordInfo,
  type ExecutionLockInfo,
  type GitHubOAuthConfigInfo,
  type GitHubStatusInfo,
  type GitHubRepositoryInfo,
  type LocalCapabilityInfo,
  type LlmProviderInfo,
  type LlmProviderSelection,
  type ObservabilitySummary,
  type OperationRecordInfo,
  type OrchestratorWatcherInfo,
  type PipelineRecordInfo,
  type PipelineTemplateInfo,
  type ProjectAgentProfileInfo,
  type ProofRecordInfo,
  type RequirementRecordInfo
} from "./omegaControlApiClient";
import {
  bindGitHubRepositoryTargetViaApi,
  createWorkItemViaApi,
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

type PrimaryNav = "Projects" | "Views" | "Issues" | "Settings";
type InspectorPanel = "properties" | "provider";
type AppSurface = "home" | "workboard";
type UiTheme = "light" | "dark";
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

function primaryNavLabel(nav: PrimaryNav) {
  return nav === "Issues" ? "Work items" : nav;
}

function InfoIcon() {
  return (
    <svg className="info-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <circle cx="12" cy="12" r="9" />
      <path d="M12 10.8v5.2" />
      <path d="M12 7.5h.01" />
    </svg>
  );
}

function topbarSearchPlaceholder(nav: PrimaryNav) {
  if (nav === "Issues") return "Search work items...";
  if (nav === "Settings") return "Search settings...";
  return "Search...";
}

function initialAppSurface(): AppSurface {
  if (typeof window === "undefined") return "home";
  if (window.location.hash === "#home") return "home";
  if (window.location.hash === "#workboard") return "workboard";
  return import.meta.env.MODE === "test" ? "workboard" : "home";
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

function createManualWorkItem(
  index: number,
  title: string,
  description: string,
  assignee: string,
  target: string,
  repositoryTargetId?: string
): WorkItem {
  return {
    id: `item_manual_${index}`,
    key: `OMG-${index}`,
    title,
    description,
    status: "Ready",
    priority: "High",
    assignee,
    labels: ["manual", "ai-delivery"],
    team: "Omega",
    stageId: "intake",
    target,
    source: "manual",
    repositoryTargetId,
    acceptanceCriteria: ["The requested change can be verified by a human reviewer."],
    blockedByItemIds: []
  };
}

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

function runnerMessageSummary(message: string): string {
  const lower = message.toLowerCase();
  if (lower.includes("failed")) return "Run needs attention";
  if (lower.includes("completed") || lower.includes("passed")) return "Run updated";
  if (lower.includes("pipeline started") || lower.includes("running")) return "Pipeline running";
  if (lower.includes("sync")) return "Sync updated";
  return "Status updated";
}

function displayText(value: string): string {
  return value.replace(/\\n/g, "\n");
}

function isCompletedWork(item: WorkItem, pipeline?: PipelineRecordInfo): boolean {
  return item.status === "Done" || pipeline?.status === "done";
}

function isFailedWork(item: WorkItem, pipeline?: PipelineRecordInfo): boolean {
  return item.status === "Blocked" || pipeline?.status === "failed";
}

function fileNameFromPath(value: string): string {
  return value.split(/[\\/]/).pop() ?? value;
}

function proofKindLabel(value: string): string {
  const lower = value.toLowerCase();
  if (lower.includes("requirement")) return "Requirement";
  if (lower.includes("solution") || lower.includes("plan")) return "Solution";
  if (lower.includes("diff") || lower.includes("implementation") || lower.includes("change")) return "Diff";
  if (lower.includes("test") || lower.includes("check")) return "Test";
  if (lower.includes("review")) return "Review";
  if (lower.includes("pr") || lower.includes("pull")) return "PR";
  if (lower.includes("merge") || lower.includes("delivery")) return "Merge";
  if (lower.includes("handoff")) return "Handoff";
  return "Artifact";
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

interface ProofCard {
  id: string;
  kind: string;
  label: string;
  stage?: string;
  path?: string;
  url?: string;
}

interface ReviewEventCard {
  id: string;
  type: string;
  message: string;
  stageId?: string;
  createdAt?: string;
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
      events: 0
    },
    pipelineStatus: {},
    checkpointStatus: {},
    operationStatus: {},
    workItemStatus: {},
    attention: { waitingHuman: 0, failed: 0, blocked: 0 }
  };
}

function App() {
  const run = useMemo(() => createSampleRun(), []);
  const persistedSession = useMemo(() => loadWorkspaceSession(run), [run.id]);
  const [appSurface, setAppSurface] = useState<AppSurface>(() => initialAppSurface());
  const [uiTheme, setUiTheme] = useState<UiTheme>(() => initialUiTheme());
  const [activeNav, setActiveNav] = useState<PrimaryNav>(persistedSession.activeNav);
  const [connections, setConnections] = useState(persistedSession.connections);
  const [selectedProviderId, setSelectedProviderId] = useState<ProviderId>(persistedSession.selectedProviderId);
  const [inspectorOpen, setInspectorOpen] = useState(false);
  const [activeInspectorPanel, setActiveInspectorPanel] = useState<InspectorPanel>(persistedSession.activeInspectorPanel);
  const [projects, setProjects] = useState<ProjectRecord[]>(persistedSession.projects);
  const [requirements, setRequirements] = useState<RequirementRecordInfo[]>(persistedSession.requirements);
  const [workItems, setWorkItems] = useState<WorkItem[]>(persistedSession.workItems);
  const [selectedWorkItemId, setSelectedWorkItemId] = useState(persistedSession.selectedWorkItemId);
  const [activeWorkItemDetailId, setActiveWorkItemDetailId] = useState("");
  const [newItemTitle, setNewItemTitle] = useState("");
  const [newItemDescription, setNewItemDescription] = useState("");
  const [newItemAssignee, setNewItemAssignee] = useState("requirement");
  const [newItemTarget, setNewItemTarget] = useState("");
  const [showInlineCreate, setShowInlineCreate] = useState(false);
  const [createComposerExpanded, setCreateComposerExpanded] = useState(false);
  const [createDescriptionMode, setCreateDescriptionMode] = useState<"write" | "preview">("write");
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
  const [proofRecords, setProofRecords] = useState<ProofRecordInfo[]>([]);
  const [checkpoints, setCheckpoints] = useState<CheckpointRecordInfo[]>([]);
  const [operations, setOperations] = useState<OperationRecordInfo[]>([]);
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

  const primaryProject = projects[0];
  const repositoryTargets = primaryProject?.repositoryTargets ?? [];
  const repositoryTargetCount = repositoryTargets.length;
  const effectiveRepositoryWorkspaceTargetId =
    activeRepositoryWorkspaceTargetId ||
    (activeNav === "Issues" ? primaryProject?.defaultRepositoryTargetId ?? (repositoryTargets.length === 1 ? repositoryTargets[0].id : "") : "");
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
  const activeRepositoryWorkspaceKey =
    activeRepositoryWorkspace?.kind === "github"
      ? `${activeRepositoryWorkspace.owner}/${activeRepositoryWorkspace.repo}`
      : activeRepositoryWorkspace?.id ?? "";
  const activeRepositoryWorkspaceItems = activeRepositoryWorkspace
    ? workItems.filter((item) => item.repositoryTargetId === activeRepositoryWorkspace.id)
    : [];
  const activeRepositoryGitHubItems = activeRepositoryWorkspaceItems.filter((item) => item.source === "github_issue");
  const watcherByRepositoryTargetId = useMemo(
    () => new Map(orchestratorWatchers.map((watcher) => [watcher.repositoryTargetId, watcher])),
    [orchestratorWatchers]
  );
  const activeRepositoryWatcher = activeRepositoryWorkspace ? watcherByRepositoryTargetId.get(activeRepositoryWorkspace.id) : undefined;
  const activeRepositoryWatcherActive = activeRepositoryWatcher?.status === "active";
  const scopedWorkItems =
    activeNav === "Issues" && activeRepositoryWorkspace ? activeRepositoryWorkspaceItems : workItems;

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
  const selectedWorkItem = scopedWorkItems.find((item) => item.id === selectedWorkItemId) ?? scopedWorkItems[0] ?? workItems[0];
  const activeWorkItemDetail = activeNav === "Issues"
    ? workItems.find((item) => item.id === activeWorkItemDetailId)
    : undefined;
  const selectedRequirement = selectedWorkItem?.requirementId
    ? requirements.find((requirement) => requirement.id === selectedWorkItem.requirementId)
    : undefined;
  const activeDetailRequirement = activeWorkItemDetail?.requirementId
    ? requirements.find((requirement) => requirement.id === activeWorkItemDetail.requirementId)
    : undefined;
  const activeDetailSiblingItems = activeDetailRequirement
    ? workItems.filter((item) => item.requirementId === activeDetailRequirement.id)
    : [];
  function repositoryLabelForItem(item: WorkItem | undefined): string {
    if (!item) return "";
    const repositoryTarget = item.repositoryTargetId
      ? repositoryTargets.find((target) => target.id === item.repositoryTargetId)
      : undefined;
    if (repositoryTarget?.kind === "github") return `${repositoryTarget.owner}/${repositoryTarget.repo}`;
    if (repositoryTarget?.kind === "local") return repositoryTarget.path;
    return activeRepositoryWorkspaceLabel;
  }
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
        workItems.some((item) => item.id === pipeline.workItemId && item.repositoryTargetId === activeRepositoryWorkspace.id)
      )
    : [];
  const pipelinesByWorkItemId = useMemo(() => {
    const byWorkItem = new Map<string, PipelineRecordInfo>();
    for (const pipeline of pipelines) {
      byWorkItem.set(pipeline.workItemId, pipeline);
    }
    return byWorkItem;
  }, [pipelines]);
  const activeDetailPipeline = activeWorkItemDetail
    ? pipelinesByWorkItemId.get(activeWorkItemDetail.id)
    : undefined;
  const activeDetailCheckpoint = useMemo(
    () =>
      activeDetailPipeline
        ? checkpoints.find((checkpoint) =>
            checkpoint.pipelineId === activeDetailPipeline.id && checkpoint.status === "pending"
          )
        : undefined,
    [activeDetailPipeline, checkpoints]
  );
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
  const activeDetailProofCards = useMemo<ProofCard[]>(() => {
    if (!activeWorkItemDetail) return [];
    const cards: ProofCard[] = [];
    const seen = new Set<string>();
    const addCard = (input: Omit<ProofCard, "id" | "kind"> & { id?: string; kind?: string }) => {
      const label = input.label || (input.path ? fileNameFromPath(input.path) : input.url ?? "proof");
      const key = input.path || input.url || `${input.stage ?? ""}:${label}`;
      if (seen.has(key)) return;
      seen.add(key);
      cards.push({
        id: input.id ?? key,
        kind: input.kind ?? proofKindLabel(`${label} ${input.path ?? ""} ${input.url ?? ""}`),
        label,
        stage: input.stage,
        path: input.path,
        url: input.url
      });
    };

    const activeOperationIds = new Set(
      operations
        .filter((operation) => {
          const pipelineId = activeDetailPipeline?.id ?? activeDetailAttempt?.pipelineId ?? "";
          const itemId = activeWorkItemDetail.id;
          const itemKey = activeWorkItemDetail.key;
          return (
            Boolean(pipelineId && (operation.id.includes(pipelineId) || operation.missionId?.includes(pipelineId))) ||
            Boolean(operation.missionId?.includes(itemId)) ||
            Boolean(operation.missionId?.includes(itemKey)) ||
            Boolean(operation.prompt?.includes(itemKey))
          );
        })
        .map((operation) => operation.id)
    );
    const stageSnapshots = activeDetailAttempt?.stages?.length
      ? activeDetailAttempt.stages
      : activeDetailPipeline?.run?.stages ?? [];
    for (const stage of stageSnapshots) {
      const evidence = "evidence" in stage && Array.isArray(stage.evidence) ? stage.evidence : [];
      for (const proof of evidence) {
        addCard({ label: fileNameFromPath(proof), path: proof, stage: stage.title ?? stage.id });
      }
    }

    const pipelineId = activeDetailPipeline?.id ?? activeDetailAttempt?.pipelineId ?? "";
    for (const record of proofRecords) {
      const belongsToPipeline = pipelineId && record.operationId?.includes(pipelineId);
      const belongsToAttempt = activeDetailAttempt?.id && record.operationId?.includes(activeDetailAttempt.id);
      const belongsToOperation = record.operationId ? activeOperationIds.has(record.operationId) : false;
      if (!belongsToPipeline && !belongsToAttempt && !belongsToOperation) continue;
      if (!record.sourcePath && !record.sourceUrl) continue;
      addCard({
        id: record.id,
        label: record.value || record.label,
        path: record.sourcePath,
        url: record.sourceUrl,
        stage: record.label
      });
    }
    return cards;
  }, [activeDetailAttempt, activeDetailPipeline, activeWorkItemDetail, operations, proofRecords]);
  const activeDetailOperations = useMemo(() => {
    if (!activeWorkItemDetail) return [];
    const pipelineId = activeDetailPipeline?.id ?? activeDetailAttempt?.pipelineId ?? "";
    const itemId = activeWorkItemDetail.id;
    const itemKey = activeWorkItemDetail.key;
    return [...operations]
      .filter((operation) => {
        return (
          Boolean(pipelineId && (operation.id.includes(pipelineId) || operation.missionId?.includes(pipelineId))) ||
          Boolean(operation.missionId?.includes(itemId)) ||
          Boolean(operation.missionId?.includes(itemKey)) ||
          Boolean(operation.prompt?.includes(itemKey))
        );
      })
      .sort((left, right) => (left.createdAt ?? left.updatedAt ?? "").localeCompare(right.createdAt ?? right.updatedAt ?? ""));
  }, [activeDetailAttempt, activeDetailPipeline, activeWorkItemDetail, operations]);
  const activeFailureOperations = useMemo(
    () => activeDetailOperations.filter((operation) => operation.status === "failed" || operation.runnerProcess?.status === "failed"),
    [activeDetailOperations]
  );
  const activeFailedStages = useMemo(() => {
    const stages = activeDetailAttempt?.stages?.length
      ? activeDetailAttempt.stages
      : activeDetailPipeline?.run?.stages ?? [];
    return stages.filter((stage) => stage.status === "failed" || stage.status === "blocked");
  }, [activeDetailAttempt, activeDetailPipeline]);
  const activeFailureProofCards = useMemo(() => {
    if (!activeFailedStages.length) return activeDetailProofCards.filter((proof) => proof.kind === "REVIEW").slice(0, 3);
    const failedStageNames = new Set(activeFailedStages.flatMap((stage) => [stage.id, stage.title].filter(Boolean) as string[]));
    return activeDetailProofCards
      .filter((proof) => proof.kind === "REVIEW" || (proof.stage ? failedStageNames.has(proof.stage) : false))
      .slice(0, 4);
  }, [activeDetailProofCards, activeFailedStages]);
  const activeHumanReviewArtifacts = useMemo(() => {
    const preferred = /human-review|code-review|review|git-diff|diff|test-report|implementation-summary|rework-summary|solution-plan|changed-files/i;
    const ranked = activeDetailProofCards
      .filter((proof) => preferred.test(`${proof.label} ${proof.path ?? ""} ${proof.stage ?? ""} ${proof.kind}`))
      .sort((left, right) => {
        const leftReview = /review|human/i.test(`${left.label} ${left.kind} ${left.stage ?? ""}`) ? 0 : 1;
        const rightReview = /review|human/i.test(`${right.label} ${right.kind} ${right.stage ?? ""}`) ? 0 : 1;
        return leftReview - rightReview;
      });
    return ranked.slice(0, 8);
  }, [activeDetailProofCards]);
  const activeHumanReviewEvents = useMemo<ReviewEventCard[]>(() => {
    const attemptEvents =
      activeDetailAttempt?.events?.map((event, index) => ({
        id: `attempt:${activeDetailAttempt.id}:${index}`,
        type: event.type ?? "event",
        message: event.message ?? "",
        stageId: event.stageId,
        createdAt: event.createdAt
      })) ?? [];
    const runEvents =
      activeDetailPipeline?.run?.events?.map((event, index) => ({
        id: `run:${activeDetailPipeline.id}:${index}`,
        type: event.type,
        message: event.message,
        stageId: event.stageId,
        createdAt: event.timestamp
      })) ?? [];
    const seen = new Set<string>();
    return [...attemptEvents, ...runEvents]
      .filter((event) => {
        const text = `${event.type} ${event.stageId ?? ""} ${event.message}`.toLowerCase();
        if (!/(review|rework|coding|test|delivery|human|changes|approve|blocked|passed)/.test(text)) return false;
        const key = `${event.type}:${event.stageId ?? ""}:${event.message}:${event.createdAt ?? ""}`;
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      })
      .slice(-8)
      .reverse();
  }, [activeDetailAttempt, activeDetailPipeline]);
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
      nextCheckpoints,
      nextOperations,
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
      fetchCheckpoints(missionControlApiUrl),
      fetchOperations(missionControlApiUrl).catch(() => []),
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
    setCheckpoints(nextCheckpoints);
    setOperations(nextOperations);
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
        await refreshControlPlane();
        if (cancelled) return;
        await refreshWorkspaceState();
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
    const title = newItemTitle.trim();
    if (!title) return;

    const target = activeRepositoryWorkspace
      ? activeRepositoryWorkspaceTarget ?? "No target"
      : newItemTarget.trim() || activeRepositoryWorkspaceTarget || "No target";
    const item = createManualWorkItem(
      workItems.length + 1,
      title,
      newItemDescription.trim() || "No description provided.",
      newItemAssignee,
      target,
      activeRepositoryWorkspace?.id
    );
    try {
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
      setActiveNav("Issues");
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Create work item failed.");
    }
  }

  function selectWorkItem(item: WorkItem) {
    setSelectedWorkItemId(item.id);
    setActiveWorkItemDetailId(item.id);
    setActiveInspectorPanel("properties");
    setInspectorOpen(false);
    setShowInlineCreate(false);
    setCreateComposerExpanded(false);
    setCreateDescriptionMode("write");
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
      await approveCheckpoint(missionControlApiUrl, checkpointId, "human");
      setRunnerMessage(`Checkpoint ${checkpointId} approved.`);
      await refreshControlPlane();
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
      setRunnerMessage(`Checkpoint ${checkpointId} sent back for changes.`);
      await refreshControlPlane();
    } catch (error) {
      setRunnerMessage(error instanceof Error ? error.message : "Checkpoint rejection failed.");
    }
  }

  const inspectorAvailable = true;
  const shellClassName = [
    "product-shell",
    `theme-${uiTheme}`,
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
    return <PortalHome onOpenWorkboard={openWorkboard} onToggleTheme={toggleUiTheme} uiTheme={uiTheme} />;
  }

  return (
    <main className={shellClassName}>
      <aside className="sidebar" aria-label="Workspace navigation">
        <div className="brand-lockup">
          <img className="brand-logo" src="/omega-logo.png" alt="Omega AI DevFlow Engine" />
          <button type="button" className="sidebar-home-button" onClick={openHome}>
            Home
          </button>
        </div>

        <nav className="nav-stack">
          {(["Projects", "Views", "Issues"] as const).map((item) => (
            <button
              key={item}
              className={item === activeNav ? "nav-item active" : "nav-item"}
              onClick={() => {
                setActiveNav(item);
                clearWorkspaceMessages();
                if (item === "Projects") {
                  setActiveRepositoryWorkspaceTargetId("");
                  setActiveWorkItemDetailId("");
                }
                if (item === "Views") {
                  setActiveWorkItemDetailId("");
                }
              }}
            >
              <span>{primaryNavLabel(item)}</span>
            </button>
          ))}
        </nav>

        {repositoryTargets.length > 0 ? (
          <details
            className="sidebar-section workspace-section"
            open={workspaceSectionOpen}
            onToggle={(event) => setWorkspaceSectionOpen(event.currentTarget.open)}
          >
            <summary>
              <span className="section-label">Workspaces</span>
            </summary>
            <nav className="workspace-stack" aria-label="Project workspaces">
              {repositoryTargets.map((target) => {
                const label = target.kind === "github" ? `${target.owner}/${target.repo}` : target.path;
                const targetItems = workItems.filter((item) => item.repositoryTargetId === target.id);
                const count = targetItems.length;
                const selected = target.id === activeRepositoryWorkspaceTargetId;
                return (
                  <div key={target.id} className={selected ? "workspace-entry selected" : "workspace-entry"}>
                    <button
                      className="workspace-row"
                      aria-label={`${label} ${count}`}
                      onClick={() => {
                        setActiveRepositoryWorkspaceTargetId(target.id);
                        setSelectedWorkItemId(targetItems[0]?.id ?? "");
                        setActiveWorkItemDetailId("");
                        setActiveNav("Issues");
                        clearWorkspaceMessages();
                      }}
                    >
                      <span className="dot online" aria-hidden="true" />
                      <span>
                        <strong>{label}</strong>
                        <small>{count} items</small>
                      </span>
                    </button>
                    <button
                      type="button"
                      className="workspace-config-button"
                      aria-label={`Configure ${label}`}
                      title="Workspace config"
                      onClick={() => {
                        setActiveRepositoryWorkspaceTargetId(target.id);
                        setActiveWorkItemDetailId("");
                        setActiveNav("Settings");
                        setAgentConfigOpen(true);
                        clearWorkspaceMessages();
                      }}
                    >
                      <span aria-hidden="true">⚙</span>
                    </button>
                  </div>
                );
              })}
            </nav>
          </details>
        ) : null}

        <details
          className="sidebar-section"
          open={connectionsSectionOpen}
          onToggle={(event) => setConnectionsSectionOpen(event.currentTarget.open)}
        >
          <summary>
            <span className="section-label">Connections</span>
          </summary>
          <div className="connection-stack">
            {visibleConnectionProviders.map((provider) => (
              <button
                key={provider.id}
                className={`connection-row ${selectedProviderId === provider.id ? "selected" : ""}`}
                onClick={() => handleProviderRowClick(provider)}
              >
                <span className={connections[provider.id].status === "connected" ? "dot online" : "dot"} />
                <span>{provider.name}</span>
                <small>{connections[provider.id].status === "connected" ? "on" : "off"}</small>
              </button>
            ))}
          </div>
        </details>
      </aside>

      <section className="workbench">
        <header className={activeWorkItemDetail ? "topbar detail-mode" : "topbar"}>
          {activeWorkItemDetail ? (
            <>
              <nav className="detail-breadcrumb" aria-label="Issue detail breadcrumb">
                <button
                  type="button"
                  onClick={() => {
                    setActiveWorkItemDetailId("");
                    setInspectorOpen(false);
                  }}
                >
                  Work items
                </button>
                <span>›</span>
                {activeDetailRepositoryLabel ? <span>{activeDetailRepositoryLabel}</span> : null}
                {activeDetailRepositoryLabel ? <span>›</span> : null}
                <strong>{activeWorkItemDetail.key}</strong>
                <span>{activeWorkItemDetail.title}</span>
              </nav>
              <div className="detail-toolbar">
                {runnerMessage ? (
                  <span className="detail-runner-chip" role="status" title={runnerMessage}>
                    {runnerMessageSummary(runnerMessage)}
                  </span>
                ) : null}
                <button type="button" className="theme-toggle" onClick={toggleUiTheme} aria-label={`Switch to ${uiTheme === "light" ? "night" : "day"} mode`}>
                  <span aria-hidden="true">{uiTheme === "light" ? "☾" : "☼"}</span>
                  {uiTheme === "light" ? "Night" : "Day"}
                </button>
                <button type="button" onClick={() => navigator.clipboard?.writeText(activeWorkItemDetail.target)}>
                  Copy target
                </button>
                <button
                  type="button"
                  className="primary-action"
                  disabled={
                    runningWorkItemId === activeWorkItemDetail.id ||
                    activeWorkItemDetail.status === "Planning" ||
                    activeWorkItemDetail.status === "In Review"
                  }
                  onClick={() => void runItem(activeWorkItemDetail, { force: activeDetailCompleted })}
                >
                  {activeDetailCompleted
                    ? "Rerun"
                    : runningWorkItemId === activeWorkItemDetail.id
                      ? "Running..."
                      : activeWorkItemDetail.status === "Planning"
                        ? "Planning..."
                        : isFailedWork(activeWorkItemDetail, activeDetailPipeline)
                          ? "Retry"
                        : "Run"}
                </button>
              </div>
            </>
          ) : (
            <>
              <div>
                <p className="section-label">Omega</p>
                <h1>{primaryNavLabel(activeNav)}</h1>
              </div>
              <div className="search-control">
                <input
                  className="command-input"
                  value={searchQuery}
                  onChange={(event) => setSearchQuery(event.currentTarget.value)}
                  placeholder={topbarSearchPlaceholder(activeNav)}
                />
                <button type="button">Search</button>
              </div>
              <div className="topbar-actions">
                {activeNav === "Issues" ? (
                  <button
                    type="button"
                    className="topbar-create"
                    onClick={() => {
                      setShowInlineCreate((current) => !current);
                      setCreateComposerExpanded(true);
                      setCreateDescriptionMode("write");
                    }}
                  >
                    <span className="topbar-create-label">New requirement</span>
                  </button>
                ) : null}
                <button type="button" className="theme-toggle" onClick={toggleUiTheme} aria-label={`Switch to ${uiTheme === "light" ? "night" : "day"} mode`}>
                  <span aria-hidden="true">{uiTheme === "light" ? "☾" : "☼"}</span>
                  {uiTheme === "light" ? "Night" : "Day"}
                </button>
              </div>
            </>
          )}
        </header>

        {activeNav === "Projects" ? (
          <section className="project-surface">
            <div className="overview-panel project-hero-panel">
              <div className="project-hero-copy">
                <span className="section-label">Project</span>
                <h2>{primaryProject?.name ?? "Omega Project"}</h2>
                <p>
                  {primaryProject?.description ||
                    "A delivery space that groups requirements, repository workspaces, agent pipelines, human review, and delivery proof."}
                </p>
                {repositoryTargets.length > 0 ? (
                  <div className="target-chip-list" aria-label="Project repository targets">
                    {repositoryTargets.map((target) => (
                      <span key={target.id}>
                        {target.kind === "github" ? `${target.owner}/${target.repo}` : target.path}
                      </span>
                    ))}
                  </div>
                ) : null}
                <button
                  type="button"
                  className="project-config-link"
                  onClick={() => {
                    setActiveNav("Settings");
                    setAgentConfigOpen(true);
                  }}
                >
                  Project config
                </button>
              </div>
              <div className="project-stat-grid" aria-label="Project delivery summary">
                <span>
                  <small>Work items</small>
                  <strong>{workItems.length}</strong>
                </span>
                <span>
                  <small>Repository workspaces</small>
                  <strong>{repositoryTargetCount}</strong>
                </span>
                <span>
                  <small>Pipeline runs</small>
                  <strong>{pipelines.length}</strong>
                </span>
              </div>
              <div className="project-flow-strip" aria-label="Project delivery flow">
                <span>Requirements</span>
                <span>Repository Workspace</span>
                <span>Agent Pipeline</span>
                <span>Human Review</span>
                <span>Delivery</span>
              </div>
            </div>

            {activeRepositoryWorkspace ? (
              <div className="overview-panel repository-workspace-panel repository-detail-panel">
                <div className="control-card-header">
                  <div>
                    <span className="section-label">Repository workspace</span>
                    <h2>{activeRepositoryWorkspaceLabel}</h2>
                  </div>
                  <div className="repository-actions">
                    <button
                      className="primary-action"
                      disabled={syncingRepositoryKey === activeRepositoryWorkspaceKey}
                      onClick={() => {
                        if (activeRepositoryWorkspace.kind === "github") {
                          void importGitHubIssues(activeRepositoryWorkspace.owner, activeRepositoryWorkspace.repo);
                        }
                      }}
                    >
                      {syncingRepositoryKey === activeRepositoryWorkspaceKey ? "Syncing..." : "Sync GitHub issues"}
                    </button>
                    <button onClick={() => setActiveNav("Issues")} disabled={activeRepositoryWorkspaceItems.length === 0}>
                      View work items
                    </button>
                  </div>
                </div>
                {repositorySyncMessage ? (
                  <p className="sync-feedback" role="status">
                    {repositorySyncMessage}
                  </p>
                ) : null}
                <div className="workspace-metrics">
                  <span>{activeRepositoryWorkspaceItems.length} work items</span>
                  <span>{activeRepositoryWorkspacePipelines.length} pipelines</span>
                  <span>0 pull requests</span>
                </div>
                <div className="repository-workspace-grid">
                  <section>
                    <span className="section-label">Work items</span>
                    {activeRepositoryWorkspaceItems.length > 0 ? (
                      <div className="imported-issue-list">
                        {activeRepositoryWorkspaceItems.slice(0, 8).map((item) => (
                          <div key={item.id}>
                            <span>{item.title}</span>
                            <small>{item.sourceExternalRef ?? item.key}</small>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <p>No work items synced yet.</p>
                    )}
                  </section>
                  <section>
                    <span className="section-label">Pull requests</span>
                    <p>No pull requests linked yet.</p>
                  </section>
                </div>
              </div>
            ) : null}

            <div className="overview-panel repository-panel">
              <div className="control-card-header">
                <div>
                  <span className="section-label">Repository workspace</span>
                  <h2>Attach GitHub repositories</h2>
                  <p>Choose the repository targets this Project is allowed to run agents inside.</p>
                </div>
                <button onClick={loadGitHubRepositories} disabled={githubRepositoriesLoading}>
                  {githubRepositoriesLoading ? "Loading..." : "Refresh repositories"}
                </button>
              </div>
              <div className="repository-picker">
                <label>
                  <span>Search repositories</span>
                  <input
                    value={githubRepositoryQuery}
                    onChange={(event) => setGitHubRepositoryQuery(event.currentTarget.value)}
                    placeholder="Search by repo name or description"
                  />
                </label>
                <div className="repository-actions">
                  <button disabled={!githubRepoOwner || !githubRepoName} className="primary-action" onClick={openSelectedRepositoryWorkspace}>
                    {selectedRepositoryBound ? "Open workspace" : "Create workspace"}
                  </button>
                </div>
              </div>
              <div className="repository-list" aria-label="GitHub repositories">
                {filteredGitHubRepositories.length === 0 ? (
                  <p>{githubRepositoriesLoading ? "Loading repositories..." : "No repositories loaded. Refresh repositories after connecting GitHub."}</p>
                ) : (
                  filteredGitHubRepositories.slice(0, 20).map((repository) => {
                    const nameWithOwner = repository.nameWithOwner ?? `${repository.owner?.login ?? ""}/${repository.name}`;
                    const selected = nameWithOwner === `${githubRepoOwner}/${githubRepoName}`;
                    return (
                      <button
                        key={nameWithOwner}
                        className={selected ? "repository-option selected" : "repository-option"}
                        onClick={() => selectGitHubRepository(repository)}
                      >
                        <span>
                          <strong>{nameWithOwner}</strong>
                          <small>{repository.description || "No description"}</small>
                        </span>
                        <small>{repository.isPrivate ? "private" : "public"} · {repository.defaultBranchRef?.name || "branch unknown"}</small>
                      </button>
                    );
                  })
                )}
              </div>
              {githubRepoInfo ? (
                <div className="repository-summary">
                  <strong>{githubRepoInfo.nameWithOwner ?? `${githubRepoInfo.owner?.login ?? githubRepoOwner}/${githubRepoInfo.name}`}</strong>
                  <span>{githubRepoInfo.description || "No repository description"}</span>
                  <small>
                    {selectedRepositoryBound ? "Attached to project" : "Not attached yet"} · {githubRepoInfo.defaultBranchRef?.name ?? "default branch unknown"}
                  </small>
                </div>
              ) : null}
            </div>
          </section>
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

        {activeNav === "Issues" ? (
          <>
            {activeWorkItemDetail ? (
              <section className="issue-detail-view" aria-label="Work item detail">
                <article className="issue-detail-document">
                  <nav className="detail-breadcrumb" aria-label="Requirement hierarchy">
                    <span>{activeDetailRepositoryLabel || "Workspace"}</span>
                    <span>Requirement</span>
                    <strong>{activeWorkItemDetail.key}</strong>
                  </nav>
                  <header className="issue-detail-title">
                    <div className="issue-detail-state">
                      <span className={`issue-state ${statusClassName(activeWorkItemDetail.status)}`} aria-hidden="true" />
                      <span>{workItemStatusLabel(activeWorkItemDetail.status)}</span>
                    </div>
                    <h2>{activeWorkItemDetail.title}</h2>
                    <div className="issue-detail-meta">
                      <span>{activeWorkItemDetail.key}</span>
                      <span>{sourceLabel(activeWorkItemDetail)}</span>
                      {activeWorkItemDetail.sourceExternalRef ? <span>{activeWorkItemDetail.sourceExternalRef}</span> : null}
                      {activeDetailRepositoryLabel ? <span>{activeDetailRepositoryLabel}</span> : null}
                      <span>{agentShortLabel(activeWorkItemDetail.assignee)}</span>
                    </div>
                  </header>

                  <section className="issue-detail-section">
                    <h3>Requirement source</h3>
                    <div className="requirement-source-card">
                      <div>
                        <span>{activeDetailRequirement?.source === "github_issue" ? "GitHub issue" : "Manual requirement"}</span>
                        <strong>{activeDetailRequirement?.title ?? activeWorkItemDetail.title}</strong>
                      </div>
                      <div className="requirement-source-meta">
                        {activeDetailRequirement?.sourceExternalRef ? <span>{activeDetailRequirement.sourceExternalRef}</span> : null}
                        {activeDetailRequirement?.status ? <span>{activeDetailRequirement.status}</span> : null}
                        <span>{activeDetailSiblingItems.length || 1} item{(activeDetailSiblingItems.length || 1) === 1 ? "" : "s"}</span>
                      </div>
                    </div>
                    {(activeDetailRequirement?.rawText || activeWorkItemDetail.description) && activeWorkItemDetail.description !== "No description provided." ? (
                      <div className="issue-detail-copy markdown-content">
                        {renderMarkdown(activeDetailRequirement?.rawText ?? activeWorkItemDetail.description)}
                      </div>
                    ) : (
                      <p className="muted-copy">No description provided yet.</p>
                    )}
                  </section>

                  <section className="issue-detail-section">
                    <h3>Acceptance criteria</h3>
                    <ul className="criteria-list">
                      {activeWorkItemDetail.acceptanceCriteria.map((criterion) => (
                        <li key={criterion}>{criterion}</li>
                      ))}
                    </ul>
                  </section>

                  <section className="issue-detail-section">
                    <h3>Current attempt</h3>
                    <WorkItemAttemptPanel
                      agentShortLabel={agentShortLabel}
                      attempt={activeDetailAttempt}
                      attemptStatusLabel={attemptStatusLabel}
                      checkpoint={activeDetailCheckpoint}
                      displayText={displayText}
                      failedStages={activeFailedStages}
                      failureOperations={activeFailureOperations}
                      failureProofCards={activeFailureProofCards}
                      humanReviewArtifacts={activeHumanReviewArtifacts}
                      humanReviewEvents={activeHumanReviewEvents}
                      onApproveCheckpoint={(checkpointId) => void approvePendingCheckpoint(checkpointId)}
                      onRequestCheckpointChanges={(checkpointId, note) => void rejectPendingCheckpoint(checkpointId, note)}
                      operationStatusLabel={operationStatusLabel}
                      pipeline={activeDetailPipeline}
                      pipelineStageClassName={pipelineStageClassName}
                      pipelineStageLabel={pipelineStageLabel}
                    />
                  </section>

                  <section className="issue-detail-section">
                    <h3>Delivery flow</h3>
                    <div className="detail-stage-list">
                      {activeDetailPipeline?.run?.stages?.length
                        ? activeDetailPipeline.run.stages.map((stage, index) => {
                            const agentIds = stage.agentIds ?? (stage.agentId ? [stage.agentId] : []);
                            return (
                              <article key={stage.id}>
                                <span>{index + 1}</span>
                                <div>
                                  <strong>{stage.title ?? stage.id}</strong>
                                  <p>{agentIds.length ? `Agents: ${agentIds.map(agentShortLabel).join(", ")}` : "Agents are assigned by the pipeline template."}</p>
                                </div>
                                <small>{pipelineStageLabel(stage.status)}</small>
                              </article>
                            );
                          })
                        : createMissionFromRun(run, activeWorkItemDetail).operations.map((operation, index) => (
                            <article key={operation.id}>
                              <span>{index + 1}</span>
                              <div>
                                <strong>{operation.stageId}</strong>
                                <p>{operation.prompt}</p>
                              </div>
                              <small>{operation.status}</small>
                            </article>
                          ))}
                    </div>
                  </section>

                  <section className="issue-detail-section">
                    <h3>Agent orchestration</h3>
                    <AgentTraceList
                      agentShortLabel={agentShortLabel}
                      operations={activeDetailOperations}
                      operationStatusLabel={operationStatusLabel}
                      pipelineStageClassName={pipelineStageClassName}
                    />
                  </section>

                  <section className="issue-detail-section">
                    <h3>Artifacts</h3>
                    <ArtifactGrid proofs={activeDetailProofCards} />
                  </section>

                  <section className="issue-detail-section">
                    <h3>Target</h3>
                    <div className="detail-target-box">
                      <span>{activeWorkItemDetail.target}</span>
                    </div>
                  </section>

                  <section className="issue-detail-section">
                    <h3>Attempt history</h3>
                    <AttemptHistory attempts={activeDetailAttempts} attemptStatusLabel={attemptStatusLabel} />
                  </section>
                </article>
              </section>
            ) : (
            <>
            {showInlineCreate ? (
              <section className="inline-create">
                <div className="inline-create-form">
                  <label className="inline-title-field">
                    <span>Add a title *</span>
                    <input
                      value={newItemTitle}
                      onFocus={() => setCreateComposerExpanded(true)}
                      onChange={(event) => setNewItemTitle(event.currentTarget.value)}
                      placeholder="Title"
                    />
                  </label>
                  <select value={newItemAssignee} onChange={(event) => setNewItemAssignee(event.currentTarget.value)}>
                    {["requirement", "architect", "coding", "testing", "review", "delivery"].map((agent) => (
                      <option key={agent}>{agent}</option>
                    ))}
                  </select>
                  <button className="primary-action" onClick={createItem}>Create</button>
                  {createComposerExpanded ? (
                    <div className="description-composer">
                      <span>Add a description</span>
                      <div className="composer-tabs">
                        <button
                          type="button"
                          className={createDescriptionMode === "write" ? "active" : ""}
                          onClick={() => setCreateDescriptionMode("write")}
                        >
                          Write
                        </button>
                        <button
                          type="button"
                          className={createDescriptionMode === "preview" ? "active" : ""}
                          onClick={() => setCreateDescriptionMode("preview")}
                        >
                          Preview
                        </button>
                      </div>
                      {createDescriptionMode === "write" ? (
                        <textarea
                          value={newItemDescription}
                          onChange={(event) => setNewItemDescription(event.currentTarget.value)}
                          placeholder="Type your description here..."
                        />
                      ) : (
                        <div className="description-preview" aria-label="Description preview">
                          {newItemDescription.trim() ? (
                            <div className="markdown-content">{renderMarkdown(newItemDescription)}</div>
                          ) : (
                            <p className="muted-copy">Nothing to preview yet.</p>
                          )}
                        </div>
                      )}
                    </div>
                  ) : null}
                </div>
                <aside className="inline-create-note">
                  <span>Requirement to item</span>
                  <p>
                    {activeRepositoryWorkspace
                      ? `A requirement will be stored under ${activeRepositoryWorkspaceLabel}, then converted into its first executable item.`
                      : "No repository workspace selected."}
                  </p>
                </aside>
              </section>
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
                <div className="empty-create">
                  <div className="empty-copy">
                    <h2>Create your first work item</h2>
                    <p>Start the Workboard with a concrete requirement Mission Control can turn into an operation.</p>
                  </div>
                  <input
                    value={newItemTitle}
                    onChange={(event) => setNewItemTitle(event.currentTarget.value)}
                    placeholder="Work item title"
                  />
                  <textarea
                    value={newItemDescription}
                    onChange={(event) => setNewItemDescription(event.currentTarget.value)}
                    placeholder="Optional description"
                  />
                  {activeRepositoryWorkspace ? null : (
                    <input
                      value={newItemTarget}
                      onChange={(event) => setNewItemTarget(event.currentTarget.value)}
                      placeholder="Local repository path or GitHub repo URL"
                    />
                  )}
                  <div className="empty-create-footer">
                    <select value={newItemAssignee} onChange={(event) => setNewItemAssignee(event.currentTarget.value)}>
                      {["requirement", "architect", "coding", "testing", "review", "delivery"].map((agent) => (
                        <option key={agent}>{agent}</option>
                      ))}
                    </select>
                    <button className="primary-action" onClick={createItem}>Create item</button>
                  </div>
                </div>
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
                        const artifactCount = proofCountForItem(item, itemPipeline);
                        const turnCount = agentTurnCountForItem(item, itemPipeline);
                        const progress = summarizePipelineProgress(item, pipelineStages, runningWorkItemId === item.id);
                        return (
                          <article
                            key={item.id}
                            className={`issue-row ${selectedWorkItem?.id === item.id ? "selected" : ""} ${pipelineStages.length ? "has-pipeline" : ""}`}
                            onClick={() => selectWorkItem(item)}
                          >
                            <div className="issue-leading" aria-hidden="true">
                              <span className="issue-drag">---</span>
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
                                <span>{sourceLabel(item)}</span>
                                {item.sourceExternalRef ? <span>{item.sourceExternalRef}</span> : null}
                                {item.requirementId ? <span>Req {item.requirementId.replace(/^req_/, "")}</span> : null}
                                {repositoryLabel ? <span>{repositoryLabel}</span> : null}
                                <span>{agentShortLabel(item.assignee)}</span>
                              </div>
                              {pipelineStages.length || item.status === "Planning" || runningWorkItemId === item.id ? (
                                <div
                                  className={`issue-progress-track ${pipelineStageClassName(progress.status)}`}
                                  aria-label={`${item.key} current progress ${progress.label}`}
                                >
                                  <div className="issue-progress-copy">
                                    <span>{pipelineStageLabel(progress.status)}</span>
                                    <strong>{progress.label}</strong>
                                  </div>
                                  <div className="issue-progress-rail" aria-hidden="true">
                                    <span style={{ width: `${progress.percent}%` }} />
                                  </div>
                                </div>
                              ) : null}
                            </div>
                            <div className="issue-trailing">
                              {!pipelineStages.length ? (
                                <span className="issue-chip">
                                  {turnCount > 0 ? `Turns ${turnCount}` : artifactCount > 0 ? `Artifacts ${artifactCount}` : "Trace"}
                                </span>
                              ) : null}
                              {itemPendingCheckpoint ? (
                                <button
                                  type="button"
                                  className="review-inline"
                                  onClick={(event) => {
                                    event.stopPropagation();
                                    setSelectedWorkItemId(item.id);
                                    setActiveWorkItemDetailId(item.id);
                                    setInspectorOpen(false);
                                  }}
                                >
                                  Human review
                                </button>
                              ) : null}
                              {completed ? (
                                <span className="status-pill status-done">Done</span>
                              ) : !itemPendingCheckpoint && item.status !== "In Review" ? (
                                <span className={`status-pill ${statusClassName(item.status)}`}>{workItemStatusLabel(item.status)}</span>
                              ) : null}
                              {!completed && item.status !== "In Review" && item.status !== "Planning" ? (
                                <button
                                  className="run-inline"
                                  disabled={runDisabled}
                                  onClick={(event) => {
                                    event.stopPropagation();
                                    void runItem(item);
                                  }}
                                >
                                  {runningWorkItemId === item.id ? "Running..." : failed ? "Retry" : "Run"}
                                </button>
                              ) : null}
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
      </section>

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
