import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkspaceAgentStudio, type AgentConfigurationDraft, type AgentRunnerOption } from "../WorkspaceAgentStudio";

const runnerOptions: AgentRunnerOption[] = [
  { value: "codex", label: "Codex", capabilityId: "codex" },
  { value: "opencode", label: "opencode", capabilityId: "opencode" },
  { value: "claude-code", label: "Claude Code", capabilityId: "claude-code" },
  { value: "trae-agent", label: "Trae Agent", capabilityId: "trae-agent" }
];

const draft: AgentConfigurationDraft = {
  projectId: "project_omega",
  repositoryTargetId: "repo_1",
  runner: "codex",
  workflowTemplate: "devflow-pr",
  workflowMarkdown: "## Stage: coding\n\nAgent: testing\nGate: no\nArtifacts: proof\n",
  stagePolicy: "",
  skillAllowlist: "",
  mcpAllowlist: "",
  codexPolicy: "",
  claudePolicy: "",
  agentProfiles: [
    {
      id: "testing",
      label: "Testing",
      runner: "opencode",
      model: "gpt-5.4-mini",
      skills: "playwright",
      mcp: "omega-filesystem",
      stageNotes: "",
      codexPolicy: "",
      claudePolicy: ""
    }
  ]
};

function renderStudio(overrides: Partial<Parameters<typeof WorkspaceAgentStudio>[0]> = {}) {
  return render(
    <WorkspaceAgentStudio
      activeRepositoryWorkspaceLabel="ZYOOO/TestRepo"
      agentConfigDraft={draft}
      agentConfigOpen
      agentConfigSavedMessage=""
      agentConfigTab="agents"
      agentRunnerOptions={runnerOptions}
      agentPreflightResults={{}}
      testingAgentProfileId=""
      localCapabilities={[
        { id: "codex", command: "codex", category: "ai-runner", available: true, required: false },
        { id: "opencode", command: "opencode", category: "ai-runner", available: true, required: false },
        { id: "claude-code", command: "claude", category: "ai-runner", available: true, required: false },
        { id: "trae-agent", command: "trae-cli", category: "ai-runner", available: true, required: false }
      ]}
      pipelineTemplates={[]}
      workflowTemplates={[]}
      primaryProjectName="Omega"
      runnerCredentials={[
        {
          id: "opencode-kimi",
          runner: "opencode",
          provider: "kimi",
          label: "opencode Kimi",
          model: "kimi/kimi-for-coding",
          baseUrl: "https://kimi.a7m.com.cn",
          secretConfigured: true
        }
      ]}
      runtimeConfigTab="omega"
      selectedAgentProfileId="testing"
      onSave={vi.fn()}
      onImportTemplate={vi.fn()}
      onSelectAgentProfile={vi.fn()}
      onTestAgentProfile={vi.fn()}
      onSetAgentConfigOpen={vi.fn()}
      onSetAgentConfigTab={vi.fn()}
      onSetRuntimeConfigTab={vi.fn()}
      onUpdateAgentProfile={vi.fn()}
      onUpdateDraft={vi.fn()}
      {...overrides}
    />
  );
}

describe("WorkspaceAgentStudio", () => {
  afterEach(() => cleanup());

  it("uses global runner defaults instead of static model presets", () => {
    renderStudio();

    expect(screen.queryByText("qwen-plus")).not.toBeInTheDocument();
    expect(screen.queryByText("deepseek-reasoner")).not.toBeInTheDocument();
    expect(screen.getByText("opencode · kimi/kimi-for-coding")).toBeInTheDocument();
    expect(screen.queryByText("opencode · gpt-5.4-mini")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Model")).toHaveValue("");
    expect(screen.getByPlaceholderText("Inherit: kimi/kimi-for-coding")).toBeInTheDocument();
    expect(screen.getByText("Inheriting kimi/kimi-for-coding.")).toBeInTheDocument();
  });

  it("keeps runner changes independent from the model override", () => {
    const onUpdateAgentProfile = vi.fn();
    renderStudio({ onUpdateAgentProfile });

    fireEvent.change(screen.getByLabelText("Runner"), { target: { value: "codex" } });

    expect(onUpdateAgentProfile).toHaveBeenCalledWith("testing", { runner: "codex" });
  });

  it("uses a themed secondary action for agent preflight testing", () => {
    renderStudio();

    expect(screen.getByRole("button", { name: "Test connection" })).toHaveClass("agent-test-action");
  });

  it("updates workflow markdown when the selected built-in template changes", () => {
    const onUpdateDraft = vi.fn();
    renderStudio({
      agentConfigTab: "workflow",
      pipelineTemplates: [
        {
          id: "devflow-pr",
          name: "DevFlow PR cycle",
          description: "",
          workflowMarkdown: "workflow: devflow-pr\nstages:\n  - todo",
          stages: []
        },
        {
          id: "saas-launch",
          name: "SaaS Launch Flow",
          description: "",
          workflowMarkdown: "---\nid: saas-launch\n---\n\nworkflow: saas-launch",
          stages: []
        }
      ],
      onUpdateDraft
    });

    fireEvent.change(screen.getByLabelText("Template"), { target: { value: "saas-launch" } });

    expect(onUpdateDraft).toHaveBeenCalledWith({
      workflowTemplate: "saas-launch",
      workflowMarkdown: "---\nid: saas-launch\n---\n\nworkflow: saas-launch"
    });
  });

  it("prefers the repository workflow template record when a template is selected", () => {
    const onUpdateDraft = vi.fn();
    renderStudio({
      agentConfigTab: "workflow",
      pipelineTemplates: [
        {
          id: "saas-launch",
          name: "SaaS Launch Flow",
          description: "",
          workflowMarkdown: "---\nid: saas-launch\n---\n\nworkflow: saas-launch",
          stages: []
        }
      ],
      workflowTemplates: [
        {
          id: "workflow_template_project_saas",
          templateId: "saas-launch",
          name: "SaaS Launch Flow",
          scope: "project",
          projectId: "project_omega",
          markdown: "workflow: saas-launch\nstages:\n  - project-default"
        },
        {
          id: "workflow_template_repo_saas",
          templateId: "saas-launch",
          name: "SaaS Launch Flow",
          scope: "repository",
          projectId: "project_omega",
          repositoryTargetId: "repo_1",
          workflowMarkdown: "workflow: saas-launch\nstages:\n  - repo-override"
        }
      ],
      onUpdateDraft
    });

    fireEvent.change(screen.getByLabelText("Template"), { target: { value: "saas-launch" } });

    expect(onUpdateDraft).toHaveBeenCalledWith({
      workflowTemplate: "saas-launch",
      workflowMarkdown: "workflow: saas-launch\nstages:\n  - repo-override"
    });
  });
});
