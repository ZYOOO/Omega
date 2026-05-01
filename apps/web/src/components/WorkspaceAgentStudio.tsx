import { useMemo, useState } from "react";
import type { AgentProfileDraftInfo, LocalCapabilityInfo, PipelineTemplateInfo, RunnerCredentialInfo } from "../omegaControlApiClient";

export type AgentConfigTab = "workflow" | "prompts" | "agents" | "runtime";
export type RuntimeConfigTab = "omega" | "codex" | "opencode" | "claude" | "trae";

export type AgentProfileDraft = AgentProfileDraftInfo;

export type AgentConfigurationDraft = {
  projectId?: string;
  repositoryTargetId?: string;
  runner: string;
  workflowTemplate: string;
  workflowMarkdown: string;
  stagePolicy: string;
  skillAllowlist: string;
  mcpAllowlist: string;
  codexPolicy: string;
  claudePolicy: string;
  agentProfiles: AgentProfileDraft[];
};

export type AgentRunnerOption = {
  value: string;
  label: string;
  capabilityId?: string;
  setupHint?: string;
};

type WorkflowStageModel = {
  id: string;
  title: string;
  agents: string;
  gate: string;
  artifacts: string[];
};

type WorkflowPromptSection = {
  id: string;
  title: string;
  body: string;
};

type StagePolicyEntry = {
  label: string;
  body: string;
};

type WorkflowEditorSelection = "template" | "contract" | `stage:${string}`;

const defaultStagePolicyHints: Record<string, string> = {
  requirement: "clarify acceptance criteria, repository target, open questions, and acceptance risks before planning.",
  architecture: "list affected files, integration boundaries, risky assumptions, and validation strategy before coding.",
  coding: "edit only inside the bound repository workspace and keep the diff reviewable for a single Work Item.",
  testing: "run focused validation first, then broader checks when shared contracts, delivery, or UI behavior changed.",
  review: "changes_requested must route to Rework with a checklist; review feedback should not be treated as an infrastructure failure.",
  rework: "reuse the existing implementation workspace, apply the checklist, update PR notes when the behavior changed, and return to review.",
  humanreview: "stop delivery until explicit approval; request changes becomes first-class feedback for the next rework attempt.",
  delivery: "after approval, run merge/check actions separately and record PR/check/proof output in the Run Workpad."
};

const modelPresetsByRunner: Record<string, string[]> = {
  codex: ["gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5.3-codex-spark"],
  opencode: ["gpt-5.4-mini", "qwen-plus", "qwen-coder-plus", "deepseek-chat", "deepseek-reasoner"],
  "claude-code": ["claude-sonnet-4.5", "claude-opus-4.1", "claude-haiku-4.5"],
  "trae-agent": ["trae-default", "doubao-seed-1.6", "deepseek-v3", "deepseek-r1"]
};

const skillOptions = [
  { value: "browser-use", label: "Browser" },
  { value: "github:github", label: "GitHub" },
  { value: "github:gh-fix-ci", label: "Fix CI" },
  { value: "github:gh-address-comments", label: "PR comments" },
  { value: "github:yeet", label: "Publish PR" },
  { value: "openai-docs", label: "OpenAI docs" }
];

const mcpOptions = [
  { value: "github", label: "GitHub" },
  { value: "filesystem:repository-workspace", label: "Repo files" },
  { value: "browser:localhost-preview", label: "Preview browser" },
  { value: "runtime-logs", label: "Runtime logs" },
  { value: "feishu", label: "Feishu" }
];

type RunnerCredentialDraft = {
  runner: string;
  provider: string;
  label: string;
  model: string;
  baseUrl: string;
  secret: string;
};

const defaultCredentialDrafts: Record<string, RunnerCredentialDraft> = {
  "trae-agent": {
    runner: "trae-agent",
    provider: "doubao",
    label: "Trae Doubao",
    model: "",
    baseUrl: "",
    secret: ""
  },
  opencode: {
    runner: "opencode",
    provider: "openai",
    label: "opencode OpenAI",
    model: "",
    baseUrl: "",
    secret: ""
  }
};

const providerOptionsByRunner: Record<string, string[]> = {
  "trae-agent": ["doubao", "openai", "anthropic", "google"],
  opencode: ["openai", "openrouter", "deepseek", "qwen"]
};

function EyeIcon({ open }: { open: boolean }) {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path d="M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6Z" />
      {open ? <circle cx="12" cy="12" r="3" /> : <path d="M4 4l16 16" />}
    </svg>
  );
}

type WorkspaceAgentStudioProps = {
  activeRepositoryWorkspaceLabel: string;
  agentConfigDraft: AgentConfigurationDraft;
  agentConfigOpen: boolean;
  agentConfigSavedMessage: string;
  agentConfigTab: AgentConfigTab;
  agentRunnerOptions: AgentRunnerOption[];
  localCapabilities: LocalCapabilityInfo[];
  pipelineTemplates: PipelineTemplateInfo[];
  primaryProjectName: string;
  runnerCredentials: RunnerCredentialInfo[];
  runtimeConfigTab: RuntimeConfigTab;
  selectedAgentProfileId: string;
  onSave: () => void;
  onSelectAgentProfile: (profileId: string) => void;
  onSaveRunnerCredential: (input: {
    id?: string;
    runner: string;
    provider: string;
    label?: string;
    model?: string;
    baseUrl?: string;
    secret?: string;
  }) => void;
  onSetAgentConfigOpen: (open: boolean) => void;
  onSetAgentConfigTab: (tab: AgentConfigTab) => void;
  onSetRuntimeConfigTab: (tab: RuntimeConfigTab) => void;
  onUpdateAgentProfile: (profileId: string, patch: Partial<AgentProfileDraft>) => void;
  onUpdateDraft: (patch: Partial<AgentConfigurationDraft>) => void;
};

const workflowTitles: Record<string, string> = {
  requirement: "Requirement",
  todo: "Todo intake",
  implementation: "Implementation",
  devflow_pr: "Implementation and PR",
  code_review: "Code Review",
  code_review_round_1: "Code Review Round 1",
  code_review_round_2: "Code Review Round 2",
  rework: "Rework",
  human_review: "Human Review",
  merging: "Merging",
  delivery: "Delivery",
  done: "Done"
};

const defaultWorkflowStages: WorkflowStageModel[] = [
  { id: "requirement", title: "Requirement", agents: "requirement", gate: "auto", artifacts: ["requirement"] },
  { id: "implementation", title: "Implementation", agents: "architect + coding + testing", gate: "auto", artifacts: ["plan", "diff", "test"] },
  { id: "code_review", title: "Code Review", agents: "review", gate: "changes -> rework", artifacts: ["review"] },
  { id: "rework", title: "Rework", agents: "coding + testing", gate: "loops to review", artifacts: ["checklist", "diff"] },
  { id: "human_review", title: "Human Review", agents: "human + review + delivery", gate: "manual gate", artifacts: ["checkpoint"] },
  { id: "delivery", title: "Delivery", agents: "delivery", gate: "after approval", artifacts: ["pr", "proof"] }
];

function titleFromStageId(id: string) {
  return workflowTitles[id] ?? id.replace(/_/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function parseListValue(value: string) {
  const trimmed = value.trim().replace(/^\[/, "").replace(/\]$/, "");
  if (!trimmed) return [];
  return trimmed
    .split(",")
    .map((item) => item.trim().replace(/^["']|["']$/g, ""))
    .filter(Boolean);
}

function parseWorkflowStages(markdown: string): WorkflowStageModel[] {
  const stages: WorkflowStageModel[] = [];
  let inStages = false;
  let current: WorkflowStageModel | null = null;
  for (const line of markdown.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    if (/^[a-zA-Z_][\w-]*:/.test(line) && !trimmed.startsWith("-") && !trimmed.startsWith("stages:")) {
      inStages = false;
      continue;
    }
    if (trimmed === "stages:" || trimmed.startsWith("stages:")) {
      inStages = true;
      continue;
    }
    if (!inStages) continue;

    const compactMatch = trimmed.match(/^-\s*([^:]+):\s*(.+)$/);
    if (compactMatch && compactMatch[1] !== "id") {
      const id = compactMatch[1].trim();
      stages.push({
        id,
        title: titleFromStageId(id),
        agents: compactMatch[2].trim(),
        gate: id.includes("human") ? "manual gate" : id.includes("review") ? "review route" : "auto",
        artifacts: []
      });
      current = stages[stages.length - 1];
      continue;
    }

    const idMatch = trimmed.match(/^-\s*id:\s*(.+)$/);
    if (idMatch) {
      const id = idMatch[1].trim();
      current = { id, title: titleFromStageId(id), agents: "agent", gate: "auto", artifacts: [] };
      stages.push(current);
      continue;
    }
    if (!current) continue;
    const titleMatch = trimmed.match(/^title:\s*(.+)$/);
    if (titleMatch) current.title = titleMatch[1].trim().replace(/^["']|["']$/g, "");
    const agentMatch = trimmed.match(/^agentId:\s*(.+)$/);
    if (agentMatch) current.agents = agentMatch[1].trim();
    const agentsMatch = trimmed.match(/^agents:\s*(.+)$/);
    if (agentsMatch) current.agents = parseListValue(agentsMatch[1]).join(" + ") || agentsMatch[1].trim();
    const gateMatch = trimmed.match(/^humanGate:\s*(true|false)$/);
    if (gateMatch) current.gate = gateMatch[1] === "true" ? "manual gate" : "auto";
    const artifactsMatch = trimmed.match(/^outputArtifacts:\s*(.+)$/);
    if (artifactsMatch) current.artifacts = parseListValue(artifactsMatch[1]);
  }
  return stages.length > 0 ? stages : defaultWorkflowStages;
}

function parseWorkflowPromptSections(markdown: string): WorkflowPromptSection[] {
  const matches = [...markdown.matchAll(/^##\s*Prompt:\s*(.+?)\s*$([\s\S]*?)(?=^##\s*Prompt:|(?![\s\S]))/gm)];
  return matches.map((match) => ({
    id: match[1].trim(),
    title: titleFromStageId(match[1].trim()),
    body: match[2].trim()
  }));
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function replaceWorkflowPromptSection(markdown: string, id: string, body: string) {
  const pattern = new RegExp(`(^##\\s*Prompt:\\s*${escapeRegExp(id)}\\s*$)([\\s\\S]*?)(?=^##\\s*Prompt:|(?![\\s\\S]))`, "m");
  if (pattern.test(markdown)) {
    return markdown.replace(pattern, `$1\n\n${body.trimEnd()}\n\n`);
  }
  return `${markdown.trimEnd()}\n\n## Prompt: ${id}\n\n${body.trimEnd()}\n`;
}

function capabilityAvailable(capabilities: LocalCapabilityInfo[], capabilityId?: string) {
  if (!capabilityId || capabilities.length === 0) return true;
  return capabilities.some((capability) => capability.id === capabilityId && capability.available);
}

function runnerAvailabilityLabel(runner: string, options: AgentRunnerOption[], capabilities: LocalCapabilityInfo[]) {
  const option = options.find((candidate) => candidate.value === runner);
  if (!option) return `Unsupported runner: ${runner}`;
  if (capabilityAvailable(capabilities, option.capabilityId)) return `${option.label} ready`;
  return option.setupHint ?? `${option.label} is not available.`;
}

function lines(value: string) {
  return value.split("\n").map((line) => line.trim()).filter(Boolean);
}

function hasLine(value: string, line: string) {
  return lines(value).includes(line);
}

function toggleLine(value: string, line: string) {
  const current = lines(value);
  if (current.includes(line)) {
    return current.filter((item) => item !== line).join("\n");
  }
  return [...current, line].join("\n");
}

function parseStagePolicyEntries(value: string): StagePolicyEntry[] {
  const entries = lines(value).map((line) => {
    const separatorIndex = line.indexOf(":");
    if (separatorIndex === -1) {
      return { label: "General", body: line };
    }
    return {
      label: line.slice(0, separatorIndex).trim() || "Policy",
      body: line.slice(separatorIndex + 1).trim()
    };
  });
  return entries.length > 0 ? entries : [{ label: "General", body: "No workflow policy has been configured yet." }];
}

function serializeStagePolicyEntries(entries: StagePolicyEntry[]) {
  return entries
    .map((entry) => `${entry.label.trim() || "Policy"}: ${entry.body.trim()}`)
    .join("\n");
}

function stagePolicyKey(value: string) {
  return value.toLowerCase().replace(/[^a-z0-9]/g, "");
}

function stagePolicyIndexForStage(entries: StagePolicyEntry[], stage: WorkflowStageModel) {
  const candidateKeys = new Set([stagePolicyKey(stage.title), stagePolicyKey(stage.id)]);
  return entries.findIndex((entry) => candidateKeys.has(stagePolicyKey(entry.label)));
}

function defaultStagePolicyHintForStage(stage: WorkflowStageModel) {
  return defaultStagePolicyHints[stagePolicyKey(stage.title)] ?? defaultStagePolicyHints[stagePolicyKey(stage.id)] ?? "";
}

export function WorkspaceAgentStudio({
  activeRepositoryWorkspaceLabel,
  agentConfigDraft,
  agentConfigOpen,
  agentConfigSavedMessage,
  agentConfigTab,
  agentRunnerOptions,
  localCapabilities,
  pipelineTemplates,
  primaryProjectName,
  runnerCredentials,
  runtimeConfigTab,
  selectedAgentProfileId,
  onSave,
  onSelectAgentProfile,
  onSaveRunnerCredential,
  onSetAgentConfigOpen,
  onSetAgentConfigTab,
  onSetRuntimeConfigTab,
  onUpdateAgentProfile,
  onUpdateDraft
}: WorkspaceAgentStudioProps) {
  const workflowStages = useMemo(() => parseWorkflowStages(agentConfigDraft.workflowMarkdown), [agentConfigDraft.workflowMarkdown]);
  const promptSections = useMemo(
    () => parseWorkflowPromptSections(agentConfigDraft.workflowMarkdown),
    [agentConfigDraft.workflowMarkdown]
  );
  const stagePolicyEntries = useMemo(
    () => parseStagePolicyEntries(agentConfigDraft.stagePolicy),
    [agentConfigDraft.stagePolicy]
  );
  const [selectedWorkflowItem, setSelectedWorkflowItem] = useState<WorkflowEditorSelection>("template");
  const [selectedWorkflowPromptId, setSelectedWorkflowPromptId] = useState(promptSections[0]?.id ?? "");
  const [credentialRunner, setCredentialRunner] = useState<"trae-agent" | "opencode">("trae-agent");
  const [credentialDraft, setCredentialDraft] = useState<RunnerCredentialDraft>(defaultCredentialDrafts["trae-agent"]);
  const [credentialSecretVisible, setCredentialSecretVisible] = useState(false);
  const selectedWorkflowStageId = selectedWorkflowItem.startsWith("stage:")
    ? selectedWorkflowItem.slice("stage:".length)
    : "";
  const selectedWorkflowStage = workflowStages.find((stage) => stage.id === selectedWorkflowStageId);
  const selectedAgentProfile =
    agentConfigDraft.agentProfiles.find((profile) => profile.id === selectedAgentProfileId) ??
    agentConfigDraft.agentProfiles[0];
  const selectedPromptSection =
    promptSections.find((section) => section.id === selectedWorkflowPromptId) ??
    promptSections[0];
  const readyRunnerCount = agentConfigDraft.agentProfiles.filter((profile) =>
    capabilityAvailable(localCapabilities, agentRunnerOptions.find((option) => option.value === profile.runner)?.capabilityId)
  ).length;
  const skillCount = agentConfigDraft.agentProfiles.reduce((count, profile) => count + lines(profile.skills).length, 0);
  const runtimeConfigPreview =
    runtimeConfigTab === "omega"
      ? JSON.stringify(
          {
            project: primaryProjectName,
            repositoryTarget: activeRepositoryWorkspaceLabel || "project-default",
            workflow: agentConfigDraft.workflowTemplate,
            profileSource: agentConfigDraft.repositoryTargetId ? "repository" : "project",
            agent: selectedAgentProfile?.id ?? "agent",
            runner: selectedAgentProfile?.runner ?? agentConfigDraft.runner,
            model: selectedAgentProfile?.model ?? "",
            skills: lines(selectedAgentProfile?.skills ?? ""),
            mcp: lines(selectedAgentProfile?.mcp ?? ""),
            sandbox: "repository-workspace"
          },
          null,
          2
        )
      : runtimeConfigTab === "codex"
        ? [
            "# .codex/OMEGA.md",
            `agent: ${selectedAgentProfile?.id ?? "agent"}`,
            `runner: ${selectedAgentProfile?.runner ?? agentConfigDraft.runner}`,
            `model: ${selectedAgentProfile?.model ?? ""}`,
            "credential: local Codex CLI login",
            "",
            selectedAgentProfile?.codexPolicy || agentConfigDraft.codexPolicy
          ].join("\n")
        : runtimeConfigTab === "opencode"
          ? [
              "# opencode runner profile",
              `agent: ${selectedAgentProfile?.id ?? "agent"}`,
              `runner: ${selectedAgentProfile?.runner ?? agentConfigDraft.runner}`,
              `model: ${selectedAgentProfile?.model ?? ""}`,
              "credential: workspace account key",
              "",
              selectedAgentProfile?.codexPolicy || agentConfigDraft.codexPolicy
            ].join("\n")
          : runtimeConfigTab === "claude"
            ? [
                "# .claude/CLAUDE.md",
                `agent: ${selectedAgentProfile?.id ?? "agent"}`,
                `runner: ${selectedAgentProfile?.runner ?? agentConfigDraft.runner}`,
                `model: ${selectedAgentProfile?.model ?? ""}`,
                "credential: local Claude Code CLI login",
                "",
                selectedAgentProfile?.claudePolicy || agentConfigDraft.claudePolicy
              ].join("\n")
            : [
                "# Trae Agent runner profile",
                `agent: ${selectedAgentProfile?.id ?? "agent"}`,
                "runner: trae-agent",
                "command: trae-cli run <prompt> --working-dir <repo>",
                "credential: workspace account key or env",
                `model: ${selectedAgentProfile?.model ?? ""}`,
                "",
                selectedAgentProfile?.codexPolicy || agentConfigDraft.codexPolicy
            ].join("\n");

  const canEditSelectedAgent = Boolean(selectedAgentProfile);
  const selectedStagePolicyIndex = selectedWorkflowStage
    ? stagePolicyIndexForStage(stagePolicyEntries, selectedWorkflowStage)
    : -1;
  const selectedStagePolicyRawBody = selectedStagePolicyIndex >= 0 ? stagePolicyEntries[selectedStagePolicyIndex].body : "";
  const selectedStagePolicyDefaultHint = selectedWorkflowStage
    ? defaultStagePolicyHintForStage(selectedWorkflowStage)
    : "";
  const selectedStagePolicyBody =
    selectedStagePolicyDefaultHint && selectedStagePolicyRawBody === selectedStagePolicyDefaultHint
      ? ""
      : selectedStagePolicyRawBody;
  const selectedModelPresets = selectedAgentProfile
    ? modelPresetsByRunner[selectedAgentProfile.runner] ?? modelPresetsByRunner.codex
    : modelPresetsByRunner.codex;
  const selectedModelOptions = selectedAgentProfile && !selectedModelPresets.includes(selectedAgentProfile.model)
    ? [selectedAgentProfile.model, ...selectedModelPresets].filter(Boolean)
    : selectedModelPresets;
  const selectedRunnerCredential = runnerCredentials.find(
    (credential) => credential.runner === credentialRunner && credential.provider === credentialDraft.provider
  );
  const updateCredentialRunner = (runner: "trae-agent" | "opencode") => {
    const stored = runnerCredentials.find((credential) => credential.runner === runner);
    setCredentialRunner(runner);
    setCredentialDraft({
      ...defaultCredentialDrafts[runner],
      ...(stored
        ? {
            provider: stored.provider,
            label: stored.label,
            model: stored.model,
            baseUrl: stored.baseUrl,
            secret: ""
          }
        : {})
    });
    setCredentialSecretVisible(false);
  };
  const updateCredentialDraft = (patch: Partial<RunnerCredentialDraft>) => {
    setCredentialDraft((current) => ({ ...current, ...patch }));
  };
  const updateSelectedStagePolicy = (body: string) => {
    if (!selectedWorkflowStage) return;
    const bodyToSave = body.trim() ? body : selectedStagePolicyDefaultHint;
    if (!bodyToSave && selectedStagePolicyIndex === -1) return;
    const nextEntries = [...stagePolicyEntries];
    if (selectedStagePolicyIndex >= 0) {
      nextEntries[selectedStagePolicyIndex] = {
        ...nextEntries[selectedStagePolicyIndex],
        label: selectedWorkflowStage.title,
        body: bodyToSave
      };
    } else {
      nextEntries.push({ label: selectedWorkflowStage.title, body: bodyToSave });
    }
    onUpdateDraft({ stagePolicy: serializeStagePolicyEntries(nextEntries) });
  };

  return (
    <section className="operator-section agent-config-section">
      <div className="operator-section-heading">
        <div>
          <span className="section-label">Workspace defaults</span>
          <h2>Workspace Agent Studio</h2>
        </div>
        <span className="inline-actions">
          <button type="button" onClick={() => onSetAgentConfigOpen(!agentConfigOpen)}>
            {agentConfigOpen ? "Hide editor" : "Edit workspace defaults"}
          </button>
          <button type="button" className="primary-action" onClick={onSave}>
            Save draft
          </button>
        </span>
      </div>

      <article className="control-card agent-config-card workspace-agent-studio">
        <div className="studio-hero">
          <div>
            <span className="section-label">Agent orchestration</span>
            <h3>{primaryProjectName} shared workflow</h3>
          </div>
          <div className="studio-status-strip" aria-label="Workspace Agent Studio summary">
            <span>
              <small>Template</small>
              <strong>{agentConfigDraft.workflowTemplate}</strong>
            </span>
            <span>
              <small>Stages</small>
              <strong>{workflowStages.length}</strong>
            </span>
            <span>
              <small>Agents</small>
              <strong>{agentConfigDraft.agentProfiles.length}</strong>
            </span>
            <span>
              <small>Runners</small>
              <strong>{readyRunnerCount}/{agentConfigDraft.agentProfiles.length}</strong>
            </span>
          </div>
        </div>

        {agentConfigOpen ? (
          <div className="agent-config-shell">
            <div className="agent-config-summary-grid" aria-label="Agent profile summary">
              <span>
                <strong>{activeRepositoryWorkspaceLabel || "Project default"}</strong>
                <small>configuration scope</small>
              </span>
              <span>
                <strong>{workflowStages.length}</strong>
                <small>workflow stages</small>
              </span>
              <span>
                <strong>{skillCount}</strong>
                <small>skill bindings</small>
              </span>
              <span>
                <strong>.omega + .codex + .claude + account profiles</strong>
                <small>runtime preview</small>
              </span>
            </div>

            <div className="agent-config-tabs" role="tablist" aria-label="Workspace Agent Studio sections">
              {(["workflow", "prompts", "agents", "runtime"] as AgentConfigTab[]).map((tab) => (
                <button
                  key={tab}
                  type="button"
                  className={agentConfigTab === tab ? "active" : ""}
                  onClick={() => onSetAgentConfigTab(tab)}
                >
                  {tab === "workflow" ? "Workflow" : tab === "prompts" ? "Prompts" : tab === "agents" ? "Agents" : "Runtime files"}
                </button>
              ))}
            </div>

            {agentConfigTab === "workflow" ? (
              <div className="workflow-studio-grid">
                <nav className="workflow-graph" aria-label="Workflow graph">
                  <button
                    type="button"
                    className={selectedWorkflowItem === "template" ? "workflow-node-card active" : "workflow-node-card"}
                    onClick={() => setSelectedWorkflowItem("template")}
                  >
                    <span className="workflow-node-index">T</span>
                    <span className="workflow-node-main">
                      <strong>Template</strong>
                      <small>{agentConfigDraft.workflowTemplate}</small>
                    </span>
                  </button>
                  {workflowStages.map((stage, index) => (
                    <button
                      key={`${stage.id}-${index}`}
                      type="button"
                      className={selectedWorkflowItem === `stage:${stage.id}` ? "workflow-node-card active" : "workflow-node-card"}
                      onClick={() => setSelectedWorkflowItem(`stage:${stage.id}` as WorkflowEditorSelection)}
                    >
                      <span className="workflow-node-index">{String(index + 1).padStart(2, "0")}</span>
                      <span className="workflow-node-main">
                        <strong>{stage.title}</strong>
                        <small>{stage.agents} · {stage.gate}</small>
                      </span>
                    </button>
                  ))}
                  <button
                    type="button"
                    className={selectedWorkflowItem === "contract" ? "workflow-node-card active" : "workflow-node-card"}
                    onClick={() => setSelectedWorkflowItem("contract")}
                  >
                    <span className="workflow-node-index">MD</span>
                    <span className="workflow-node-main">
                      <strong>Markdown contract</strong>
                      <small>Raw workflow template</small>
                    </span>
                  </button>
                </nav>
                <div className="workflow-details-panel">
                  <div className="control-form workflow-contract-editor">
                    {selectedWorkflowItem === "template" ? (
                      <div className="workflow-editor-section workflow-template-section">
                        <div className="workflow-details-heading">
                          <span className="section-label">Template</span>
                          <strong>{agentConfigDraft.workflowTemplate}</strong>
                        </div>
                        <label className="workflow-template-field">
                          <span>Template</span>
                          <select
                            value={agentConfigDraft.workflowTemplate}
                            onChange={(event) => onUpdateDraft({ workflowTemplate: event.currentTarget.value })}
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
                        <p className="workflow-template-help">
                          Default contract content is edited below. Stage rules are edited one stage at a time.
                        </p>
                        <textarea
                          className="workflow-contract-textarea"
                          aria-label="Current template content"
                          value={agentConfigDraft.workflowMarkdown}
                          onChange={(event) => onUpdateDraft({ workflowMarkdown: event.currentTarget.value })}
                        />
                      </div>
                    ) : null}

                    {selectedWorkflowStage ? (
                      <div className="workflow-editor-section">
                        <div className="workflow-details-heading">
                          <span className="section-label">Stage rule</span>
                          <strong>{selectedWorkflowStage.title}</strong>
                        </div>
                        <div className="workflow-rule-summary">
                          <div>
                            <span>Stage</span>
                            <strong>{selectedWorkflowStage.title}</strong>
                            <small>{selectedWorkflowStage.agents} · {selectedWorkflowStage.gate}</small>
                          </div>
                          <div>
                            <span>Default guidance</span>
                            <strong>{selectedWorkflowStage.artifacts.length > 0
                              ? selectedWorkflowStage.artifacts.join(", ")
                              : "contract driven"}</strong>
                            <small className="workflow-default-rule-note">
                              {selectedStagePolicyDefaultHint || "No default guidance is defined for this stage."}
                            </small>
                          </div>
                        </div>
                        <div className="workflow-policy-editor" aria-label={`${selectedWorkflowStage.title} workflow rule`}>
                          <label className="workflow-policy-single">
                            <textarea
                              aria-label={`${selectedWorkflowStage.title} rule content`}
                              value={selectedStagePolicyBody}
                              placeholder="Add a custom override for this stage. Leave empty to use the default guidance above."
                              onChange={(event) => updateSelectedStagePolicy(event.currentTarget.value)}
                            />
                          </label>
                        </div>
                      </div>
                    ) : null}

                    {selectedWorkflowItem === "contract" ? (
                      <div className="workflow-editor-section">
                        <div className="workflow-details-heading">
                          <span className="section-label">Template content</span>
                          <strong>Markdown contract</strong>
                        </div>
                        <div className="workflow-context-note">
                          <strong>{agentConfigDraft.workflowTemplate}</strong>
                          <span>This contract defines stages, artifacts, prompts, runtime policy, and routing defaults.</span>
                        </div>
                        <textarea
                          className="workflow-contract-textarea"
                          aria-label="Workflow markdown contract"
                          value={agentConfigDraft.workflowMarkdown}
                          onChange={(event) => onUpdateDraft({ workflowMarkdown: event.currentTarget.value })}
                        />
                      </div>
                    ) : null}
                  </div>
                </div>
              </div>
            ) : null}

            {agentConfigTab === "prompts" && selectedAgentProfile ? (
              <div className="prompt-studio-grid">
                <div className="agent-side-list" aria-label="Prompt sections">
                  {agentConfigDraft.agentProfiles.map((profile) => (
                    <button
                      key={profile.id}
                      type="button"
                      className={profile.id === selectedAgentProfile.id ? "agent-side-item active" : "agent-side-item"}
                      onClick={() => onSelectAgentProfile(profile.id)}
                    >
                      <strong>{profile.label}</strong>
                      <small>{profile.runner} · {profile.model}</small>
                    </button>
                  ))}
                </div>
                <div className="control-form prompt-editor prompt-workspace">
                  <div className="agent-profile-editor-header">
                    <span className="section-label">Agent prompts</span>
                    <strong>{selectedAgentProfile.label}</strong>
                  </div>
                  <div className="prompt-field-grid">
                    <label className="prompt-field">
                      <span>Role instruction</span>
                      <small>What this agent should accomplish in this stage.</small>
                      <textarea
                        value={selectedAgentProfile.stageNotes}
                        onChange={(event) => onUpdateAgentProfile(selectedAgentProfile.id, { stageNotes: event.currentTarget.value })}
                      />
                    </label>
                    <label className="prompt-field">
                      <span>Execution rules</span>
                      <small>Runtime constraints, write scope, artifacts, and validation expectations.</small>
                      <textarea
                        value={selectedAgentProfile.codexPolicy}
                        onChange={(event) => onUpdateAgentProfile(selectedAgentProfile.id, { codexPolicy: event.currentTarget.value })}
                      />
                    </label>
                    <label className="prompt-field">
                      <span>Review notes</span>
                      <small>Secondary guidance for summarizing risk, assumptions, and handoff.</small>
                      <textarea
                        value={selectedAgentProfile.claudePolicy}
                        onChange={(event) => onUpdateAgentProfile(selectedAgentProfile.id, { claudePolicy: event.currentTarget.value })}
                      />
                    </label>
                  </div>
                  <details className="advanced-config-details">
                    <summary>Workflow prompt sections</summary>
                    <div className="workflow-prompt-section-grid">
                      <div className="runtime-file-tabs" role="tablist" aria-label="Workflow prompt tabs">
                        {promptSections.map((section) => (
                          <button
                            key={section.id}
                            type="button"
                            className={section.id === selectedPromptSection?.id ? "active" : ""}
                            onClick={() => setSelectedWorkflowPromptId(section.id)}
                          >
                            {section.title}
                          </button>
                        ))}
                        {promptSections.length === 0 ? (
                          <button
                            type="button"
                            onClick={() => {
                              const nextMarkdown = replaceWorkflowPromptSection(
                                agentConfigDraft.workflowMarkdown,
                                selectedAgentProfile.id,
                                selectedAgentProfile.stageNotes
                              );
                              onUpdateDraft({ workflowMarkdown: nextMarkdown });
                              setSelectedWorkflowPromptId(selectedAgentProfile.id);
                            }}
                          >
                            Add section
                          </button>
                        ) : null}
                      </div>
                      {selectedPromptSection ? (
                        <textarea
                          value={selectedPromptSection.body}
                          onChange={(event) =>
                            onUpdateDraft({
                              workflowMarkdown: replaceWorkflowPromptSection(
                                agentConfigDraft.workflowMarkdown,
                                selectedPromptSection.id,
                                event.currentTarget.value
                              )
                            })
                          }
                        />
                      ) : null}
                    </div>
                  </details>
                </div>
              </div>
            ) : null}

            {agentConfigTab === "agents" && selectedAgentProfile ? (
              <div className="agent-profile-layout">
                <div className="agent-side-list" aria-label="Agent roster">
                  {agentConfigDraft.agentProfiles.map((profile) => {
                    const runnerReady = capabilityAvailable(
                      localCapabilities,
                      agentRunnerOptions.find((option) => option.value === profile.runner)?.capabilityId
                    );
                    return (
                      <button
                        key={profile.id}
                        type="button"
                        className={[
                          "agent-side-item",
                          profile.id === selectedAgentProfile.id ? "active" : "",
                          !runnerReady ? "runner-missing" : ""
                        ]
                          .filter(Boolean)
                          .join(" ")}
                        onClick={() => onSelectAgentProfile(profile.id)}
                      >
                        <strong>{profile.label}</strong>
                        <span>{profile.runner} · {profile.model}</span>
                      </button>
                    );
                  })}
                </div>
                <div className="control-form agent-profile-editor">
                  <div className="agent-profile-editor-header">
                    <span className="section-label">Agent override</span>
                    <strong>{selectedAgentProfile.label}</strong>
                  </div>
                  <div className="agent-settings-grid">
                    <label className="agent-setting-field">
                      <span>Runner</span>
                      <select
                        value={selectedAgentProfile.runner}
                        onChange={(event) => {
                          const runner = event.currentTarget.value;
                          const nextModel = modelPresetsByRunner[runner]?.[0] ?? selectedAgentProfile.model;
                          onUpdateAgentProfile(selectedAgentProfile.id, { runner, model: nextModel });
                        }}
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
                      <small
                        className={
                          capabilityAvailable(
                            localCapabilities,
                            agentRunnerOptions.find((option) => option.value === selectedAgentProfile.runner)?.capabilityId
                          )
                            ? "runner-availability ready"
                            : "runner-availability missing"
                        }
                      >
                        {runnerAvailabilityLabel(selectedAgentProfile.runner, agentRunnerOptions, localCapabilities)}
                      </small>
                    </label>
                    <label className="agent-setting-field">
                      <span>Model</span>
                      <select
                        value={selectedAgentProfile.model}
                        onChange={(event) => onUpdateAgentProfile(selectedAgentProfile.id, { model: event.currentTarget.value })}
                      >
                        {selectedModelOptions.map((model) => (
                          <option key={model} value={model}>{model}</option>
                        ))}
                      </select>
                    </label>
                  </div>
                  <div className="agent-picker-section">
                    <span className="section-label">Skills</span>
                    <div className="chip-picker" aria-label="Skill selector">
                      {skillOptions.map((option) => (
                        <button
                          key={option.value}
                          type="button"
                          className={hasLine(selectedAgentProfile.skills, option.value) ? "active" : ""}
                          onClick={() =>
                            onUpdateAgentProfile(selectedAgentProfile.id, {
                              skills: toggleLine(selectedAgentProfile.skills, option.value)
                            })
                          }
                        >
                          {option.label}
                        </button>
                      ))}
                    </div>
                  </div>
                  <div className="agent-picker-section">
                    <span className="section-label">MCP</span>
                    <div className="chip-picker" aria-label="MCP selector">
                      {mcpOptions.map((option) => (
                        <button
                          key={option.value}
                          type="button"
                          className={hasLine(selectedAgentProfile.mcp, option.value) ? "active" : ""}
                          onClick={() =>
                            onUpdateAgentProfile(selectedAgentProfile.id, {
                              mcp: toggleLine(selectedAgentProfile.mcp, option.value)
                            })
                          }
                        >
                          {option.label}
                        </button>
                      ))}
                    </div>
                  </div>
                  <details className="advanced-config-details">
                    <summary>Raw bindings</summary>
                    <label>
                      <span>Skills</span>
                      <textarea
                        value={selectedAgentProfile.skills}
                        onChange={(event) => onUpdateAgentProfile(selectedAgentProfile.id, { skills: event.currentTarget.value })}
                      />
                    </label>
                    <label>
                      <span>MCP</span>
                      <textarea
                        value={selectedAgentProfile.mcp}
                        onChange={(event) => onUpdateAgentProfile(selectedAgentProfile.id, { mcp: event.currentTarget.value })}
                      />
                    </label>
                  </details>
                </div>
              </div>
            ) : null}

            {agentConfigTab === "runtime" ? (
              <div className="runtime-config-layout">
                <div className="runtime-file-tabs" role="tablist" aria-label="Runtime file templates">
                  {(["omega", "codex", "opencode", "claude", "trae"] as RuntimeConfigTab[]).map((tab) => (
                    <button
                      key={tab}
                      type="button"
                      className={runtimeConfigTab === tab ? "active" : ""}
                      onClick={() => {
                        onSetRuntimeConfigTab(tab);
                        if (tab === "trae") updateCredentialRunner("trae-agent");
                        if (tab === "opencode") updateCredentialRunner("opencode");
                      }}
                    >
                      {tab === "omega"
                        ? ".omega/agent-runtime.json"
                        : tab === "codex"
                          ? ".codex/OMEGA.md"
                          : tab === "opencode"
                            ? "opencode profile"
                            : tab === "claude"
                              ? ".claude/CLAUDE.md"
                              : "Trae Agent profile"}
                    </button>
                  ))}
                </div>
                {runtimeConfigTab === "trae" || runtimeConfigTab === "opencode" ? (
                  <div className="runner-account-panel">
                    <div className="runner-account-heading">
                      <div>
                        <span className="section-label">Runner account</span>
                        <strong>{credentialRunner === "trae-agent" ? "Trae Agent" : "opencode"}</strong>
                      </div>
                      <span className={selectedRunnerCredential?.secretConfigured ? "runner-availability ready" : "runner-availability missing"}>
                        {selectedRunnerCredential?.secretConfigured ? "Key saved" : "Key not configured"}
                      </span>
                    </div>
                    <div className="runner-account-grid">
                      <label>
                        <span>Provider</span>
                        <select
                          value={credentialDraft.provider}
                          onChange={(event) => {
                            const provider = event.currentTarget.value;
                            const stored = runnerCredentials.find(
                              (credential) => credential.runner === credentialRunner && credential.provider === provider
                            );
                            updateCredentialDraft({
                              provider,
                              label: stored?.label ?? `${credentialRunner === "trae-agent" ? "Trae" : "opencode"} ${provider}`,
                              model: stored?.model ?? "",
                              baseUrl: stored?.baseUrl ?? "",
                              secret: ""
                            });
                          }}
                        >
                          {providerOptionsByRunner[credentialRunner].map((provider) => (
                            <option key={provider} value={provider}>{provider}</option>
                          ))}
                        </select>
                      </label>
                      <label>
                        <span>{credentialRunner === "trae-agent" ? "EP ID / model" : "Model"}</span>
                        <input
                          value={credentialDraft.model}
                          placeholder={credentialRunner === "trae-agent" ? "ep-..." : "model name"}
                          onChange={(event) => updateCredentialDraft({ model: event.currentTarget.value })}
                        />
                      </label>
                      <label>
                        <span>Base URL</span>
                        <input
                          value={credentialDraft.baseUrl}
                          placeholder="Optional provider base URL"
                          onChange={(event) => updateCredentialDraft({ baseUrl: event.currentTarget.value })}
                        />
                      </label>
                      <label className="secret-input-field">
                        <span>API key</span>
                        <span className="secret-input-shell">
                          <input
                            type={credentialSecretVisible ? "text" : "password"}
                            value={credentialDraft.secret}
                            placeholder={selectedRunnerCredential?.secretConfigured ? selectedRunnerCredential.secretMasked ?? "********" : "Paste API key"}
                            onChange={(event) => updateCredentialDraft({ secret: event.currentTarget.value })}
                            autoComplete="off"
                          />
                          <button
                            type="button"
                            className="secret-toggle-button"
                            aria-label={credentialSecretVisible ? "Hide API key" : "Show API key"}
                            onClick={() => setCredentialSecretVisible((open) => !open)}
                          >
                            <EyeIcon open={credentialSecretVisible} />
                          </button>
                        </span>
                      </label>
                    </div>
                    <div className="runner-account-actions">
                      <button
                        type="button"
                        className="primary-action"
                        onClick={() => {
                          onSaveRunnerCredential({
                            id: selectedRunnerCredential?.id,
                            runner: credentialRunner,
                            provider: credentialDraft.provider,
                            label: credentialDraft.label,
                            model: credentialDraft.model,
                            baseUrl: credentialDraft.baseUrl,
                            secret: credentialDraft.secret
                          });
                          setCredentialDraft((current) => ({ ...current, secret: "" }));
                          setCredentialSecretVisible(false);
                        }}
                      >
                        Save account
                      </button>
                      <small>Stored locally as encrypted ciphertext. The key is decrypted only when the runner process starts.</small>
                    </div>
                  </div>
                ) : null}
                <div className="control-form runtime-policy-editor">
                  <label>
                    <span>Runner policy</span>
                    <textarea
                      value={agentConfigDraft.codexPolicy}
                      onChange={(event) => onUpdateDraft({ codexPolicy: event.currentTarget.value })}
                    />
                  </label>
                  <label>
                    <span>Secondary notes</span>
                    <textarea
                      value={agentConfigDraft.claudePolicy}
                      onChange={(event) => onUpdateDraft({ claudePolicy: event.currentTarget.value })}
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
            <span>Stages: {workflowStages.length}</span>
            <span>Agents: {agentConfigDraft.agentProfiles.length}</span>
            <span>Runners: {readyRunnerCount}/{agentConfigDraft.agentProfiles.length}</span>
          </div>
        )}

        {!canEditSelectedAgent ? <p className="agent-config-save-status" role="status">No agent profile is available.</p> : null}
      </article>
    </section>
  );
}
