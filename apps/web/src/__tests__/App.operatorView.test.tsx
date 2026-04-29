import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" }
  });
}

describe("App operator view", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllEnvs();
    vi.restoreAllMocks();
    vi.resetModules();
    localStorage.clear();
    window.location.hash = "";
  });

  it("renders the portal homepage", async () => {
    window.location.hash = "#home";
    const { default: App } = await import("../App");
    render(<App />);

    expect(await screen.findByText("张涌，欢迎回到 Omega")).toBeInTheDocument();
    expect(screen.getByText("AI DevFlow 工作台")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "进入 Workboard" })).toBeInTheDocument();
  });

  it("creates manual work items with a local repository target path", async () => {
    const { default: App } = await import("../App");
    render(<App />);

    fireEvent.change(await screen.findByPlaceholderText("Work item title"), { target: { value: "Implement demo writing" } });
    fireEvent.change(screen.getByPlaceholderText("Local repository path or GitHub repo URL"), { target: { value: "/Users/demo/Omega" } });
    fireEvent.click(screen.getByRole("button", { name: "Create item" }));

    await waitFor(() => expect(screen.getByDisplayValue("/Users/demo/Omega")).toBeInTheDocument());
  });

  it("deletes not-started work items from the workboard", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const workspaceDatabase = (workItems: unknown[]) => ({
      schemaVersion: 1,
      savedAt: new Date().toISOString(),
      tables: {
        projects: [{ id: "project_omega", name: "Omega", description: "", team: "Omega", status: "Active", labels: [], createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() }],
        requirements: workItems.length ? [{ id: "req_item_manual_1", projectId: "project_omega", title: "Remove stale requirement", rawText: "Remove stale requirement", status: "converted" }] : [],
        workItems,
        missionControlStates: [{ runId: "run_req_omega_001", projectId: "project_omega", workItems, events: [], syncIntents: [], updatedAt: new Date().toISOString() }],
        missionEvents: [],
        syncIntents: [],
        connections: [],
        uiPreferences: [],
        pipelines: [],
        attempts: [],
        checkpoints: [],
        missions: [],
        operations: [],
        proofRecords: []
      }
    });
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) {
        return Promise.resolve(jsonResponse({ error: "workspace not found" }, 404));
      }
      if (url.endsWith("/workspace") && init?.method === "PUT") {
        return Promise.resolve(jsonResponse({ ok: true }));
      }
      if (url.endsWith("/work-items") && init?.method === "POST") {
        const item = (JSON.parse(String(init.body)) as { item: unknown }).item;
        return Promise.resolve(jsonResponse(workspaceDatabase([{ ...(item as object), requirementId: "req_item_manual_1" }])));
      }
      if (url.endsWith("/work-items/item_manual_1") && init?.method === "DELETE") {
        return Promise.resolve(jsonResponse(workspaceDatabase([])));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;
    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    fireEvent.change(await screen.findByPlaceholderText("Work item title"), { target: { value: "Remove stale requirement" } });
    fireEvent.click(screen.getByRole("button", { name: "Create item" }));
    expect(await screen.findByRole("button", { name: "Delete Remove stale requirement" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Delete Remove stale requirement" }));

    await waitFor(() => expect(screen.queryByRole("button", { name: "Delete Remove stale requirement" })).not.toBeInTheDocument());
  });

  it("renders Go control-plane observability, provider, templates, and agent contracts", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) {
        return Promise.resolve(jsonResponse({ error: "workspace not found" }, 404));
      }
      if (url.endsWith("/workspace") && init?.method === "PUT") {
        return Promise.resolve(jsonResponse({ ok: true }));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 2, pipelines: 1, checkpoints: 1, missions: 1, operations: 1, proofRecords: 3, events: 4 },
          pipelineStatus: { "waiting-human": 1 },
          checkpointStatus: { pending: 1 },
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 2, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/llm-providers")) {
        return Promise.resolve(jsonResponse([
          { id: "openai", name: "OpenAI", models: ["gpt-5.4-mini"], defaultModel: "gpt-5.4-mini" },
          { id: "openai-compatible", name: "OpenAI-compatible", models: ["qwen-plus"], defaultModel: "qwen-plus" }
        ]));
      }
      if (url.endsWith("/llm-provider-selection")) {
        return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      }
      if (url.endsWith("/pipeline-templates")) {
        return Promise.resolve(jsonResponse([{ id: "feature", name: "Feature delivery", description: "", stages: [{ id: "intake" }] }]));
      }
      if (url.endsWith("/pipelines")) {
        return Promise.resolve(jsonResponse([{ id: "pipeline_item_1", workItemId: "item_1", status: "waiting-human", run: { stages: [] } }]));
      }
      if (url.endsWith("/checkpoints")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/agent-definitions")) {
        return Promise.resolve(jsonResponse([
          { id: "requirement", name: "Requirement Agent", outputContract: ["requirements"], defaultModel: { providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" } }
        ]));
      }
      if (url.includes("/agent-profile") && !init) {
        return Promise.resolve(jsonResponse({
          projectId: "project_omega",
          workflowTemplate: "devflow-pr",
          workflowMarkdown: "workflow: devflow-pr",
          stagePolicy: "Human Review blocks delivery.",
          skillAllowlist: "browser-use",
          mcpAllowlist: "github",
          codexPolicy: "workspace-write",
          claudePolicy: "repository only",
          agentProfiles: [
            { id: "requirement", label: "Requirement", runner: "codex", model: "gpt-5.4-mini", skills: "browser-use", mcp: "github", stageNotes: "Clarify requirement.", codexPolicy: "write artifact", claudePolicy: "summarize" }
          ],
          source: "project"
        }));
      }
      if (url.includes("/agent-profile") && init?.method === "PUT") {
        return Promise.resolve(jsonResponse({ ...JSON.parse(String(init.body)), source: "project", updatedAt: new Date().toISOString() }));
      }
      if (url.endsWith("/local-capabilities")) {
        return Promise.resolve(jsonResponse([
          { id: "git", command: "git", category: "source-control", available: true, version: "git version 2.45.0", required: true },
          { id: "codex", command: "codex", category: "ai-runner", available: true, version: "codex 0.98.0", required: false },
          { id: "lark-cli", command: "lark-cli", category: "feishu", available: false, required: false }
        ]));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    fireEvent.click(screen.getByRole("button", { name: /Views/ }));

    await waitFor(() => expect(screen.getByText("Runtime model")).toBeInTheDocument());
    expect(screen.getAllByText("OpenAI").length).toBeGreaterThan(0);
    expect(screen.getByText("Feature delivery")).toBeInTheDocument();
    expect(screen.getByText("Requirement Agent")).toBeInTheDocument();
    expect(screen.getByText("Proof")).toBeInTheDocument();
    expect(screen.getAllByText("3").length).toBeGreaterThan(0);
    expect(screen.getByText("Local tools")).toBeInTheDocument();
    expect(screen.getByText("git")).toBeInTheDocument();
    expect(screen.getByText("lark-cli")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Projects/ }));
    fireEvent.click(screen.getByRole("button", { name: "Project config" }));
    expect(screen.getByText("Workspace folder")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Choose folder" })).toBeInTheDocument();
    expect(screen.getByText("Project Agent Profile")).toBeInTheDocument();
    expect(screen.getByText(/Agent orchestration/)).toBeInTheDocument();
    const editProfileButton = screen.queryByRole("button", { name: "Edit profile" });
    if (editProfileButton) fireEvent.click(editProfileButton);
    expect(screen.getByRole("button", { name: "Workflow" })).toBeInTheDocument();
    expect(screen.getByLabelText("Workflow parser draft")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Agents" }));
    expect(screen.getByLabelText("Agent roster")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Runtime files" }));
    expect(screen.getByRole("button", { name: ".codex/OMEGA.md" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: ".claude/CLAUDE.md" })).toBeInTheDocument();
    expect(screen.getByText("Runtime file preview")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Save draft" }));
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Saved to local runtime"));
    expect(localStorage.getItem("omega-agent-configuration-draft")).toContain("devflow-pr");
  });

  it("renders execution locks, runner process telemetry, and pipeline stages", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");

    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/workspace")) {
        return Promise.resolve(jsonResponse({
          schemaVersion: 1,
          savedAt: new Date().toISOString(),
          tables: {
            projects: [{ id: "project_omega", name: "Omega", description: "", team: "Omega", status: "Active", labels: [], createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() }],
            workItems: [],
            missionControlStates: [{ runId: "run_req_omega_001", projectId: "project_omega", workItems: [], events: [], syncIntents: [], updatedAt: new Date().toISOString() }],
            missionEvents: [],
            syncIntents: [],
            connections: [],
            uiPreferences: [],
            pipelines: [],
            checkpoints: [],
            missions: [],
            operations: [],
            proofRecords: []
          }
        }));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 1, pipelines: 1, checkpoints: 0, missions: 1, operations: 1, proofRecords: 2, events: 3, runtimeLogs: 2 },
          pipelineStatus: { running: 1 },
          checkpointStatus: {},
          operationStatus: { failed: 1 },
          workItemStatus: {},
          attention: { waitingHuman: 0, failed: 1, blocked: 0 },
          recentErrors: []
        }));
      }
      if (url.endsWith("/pipelines")) {
        return Promise.resolve(jsonResponse([{
          id: "pipeline_gh_42",
          workItemId: "github_acme-demo_42",
          runId: "run_gh_42",
          status: "running",
          templateId: "devflow-pr",
          run: {
            stages: [
              { id: "todo", title: "Requirement", status: "passed", agentId: "requirement" },
              { id: "in_progress", title: "Coding", status: "running", agentId: "coding" },
              { id: "review", title: "Review", status: "waiting", agentId: "review" }
            ]
          }
        }]));
      }
      if (url.endsWith("/operations")) {
        return Promise.resolve(jsonResponse([{
          id: "operation_coding",
          missionId: "mission_gh_42",
          stageId: "coding",
          agentId: "coding",
          status: "failed",
          prompt: "Implement issue",
          requiredProof: ["diff"],
          runnerProcess: {
            runner: "codex",
            command: "codex",
            status: "failed",
            exitCode: 7,
            durationMs: 1260,
            stdout: "codex stdout before failure",
            stderr: "codex stderr before failure"
          },
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString()
        }]));
      }
      if (url.endsWith("/execution-locks")) {
        return Promise.resolve(jsonResponse([{
          id: "execution-lock:github-issue_acme_demo_42",
          scope: "github-issue:acme/demo#42",
          repositoryTargetId: "repo_acme_demo",
          workItemId: "github_acme-demo_42",
          pipelineId: "pipeline_gh_42",
          status: "claimed",
          owner: "local-app",
          runnerProcessState: "not-started",
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString()
        }]));
      }
      if (url.includes("/runtime-logs")) {
        return Promise.resolve(jsonResponse([
          {
            id: "log_error_1",
            level: "ERROR",
            eventType: "checkpoint.approve.missing_attempt",
            message: "Missing attempt was detected.",
            pipelineId: "pipeline_gh_42",
            attemptId: "attempt_missing",
            requestId: "req_42",
            createdAt: new Date().toISOString()
          },
          {
            id: "log_info_1",
            level: "INFO",
            eventType: "devflow.job.started",
            message: "DevFlow background job started.",
            pipelineId: "pipeline_gh_42",
            createdAt: new Date().toISOString()
          }
        ]));
      }
      if (url.endsWith("/llm-provider-selection")) {
        return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      }
      if (url.endsWith("/llm-providers") || url.endsWith("/pipeline-templates") || url.endsWith("/agent-definitions") || url.endsWith("/checkpoints") || url.endsWith("/local-capabilities")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/local-workspace-root")) {
        return Promise.resolve(jsonResponse({ workspaceRoot: "/Users/demo/Omega/workspaces" }));
      }
      if (url.endsWith("/github/oauth/config")) {
        return Promise.resolve(jsonResponse({
          configured: false,
          clientId: "",
          redirectUri: "http://127.0.0.1:3888/auth/github/callback",
          tokenUrl: "https://github.com/login/oauth/access_token",
          secretConfigured: false,
          source: "empty"
        }));
      }
      if (url.endsWith("/github/status")) {
        return Promise.resolve(jsonResponse({ available: true, authenticated: true, output: "", account: "ZYOOO", oauthConfigured: false, oauthAuthenticated: false }));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    await screen.findByText("Create your first work item");
    fireEvent.click(screen.getByRole("button", { name: /Views/ }));

    await waitFor(() => expect(screen.getByText("Execution locks")).toBeInTheDocument());
    expect(screen.getByText("github-issue:acme/demo#42")).toBeInTheDocument();
    expect(screen.getByText("Runner processes")).toBeInTheDocument();
    expect(screen.getByText("operation_coding")).toBeInTheDocument();
    expect(screen.getByText("exit 7")).toBeInTheDocument();
    expect(screen.getByText("Runtime logs")).toBeInTheDocument();
    expect(screen.getByText("checkpoint.approve.missing_attempt")).toBeInTheDocument();
    expect(screen.getByText("Missing attempt was detected.")).toBeInTheDocument();
    expect(screen.getByText("Requirement")).toBeInTheDocument();
    expect(screen.getByText("Coding")).toBeInTheDocument();
  });

  it("navigates the current window to GitHub OAuth when using the Go local service", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");
    const navigateToExternalUrl = vi.fn();
    vi.doMock("../browserNavigation", () => ({ navigateToExternalUrl, openExternalUrlInNewTab: vi.fn() }));

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) {
        return Promise.resolve(jsonResponse({
          schemaVersion: 1,
          savedAt: new Date().toISOString(),
          tables: {
            projects: [{ id: "project_omega", name: "Omega", description: "", team: "Omega", status: "Active", labels: [], repositoryTargets: [], createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() }],
            workItems: [{
              projectId: "project_omega",
              createdAt: new Date().toISOString(),
              updatedAt: new Date().toISOString(),
              id: "item_manual_1",
              key: "OMG-1",
              title: "OAuth test item",
              description: "Keep inspector visible.",
              status: "Ready",
              priority: "High",
              assignee: "requirement",
              labels: ["manual"],
              team: "Omega",
              stageId: "intake",
              target: "No target",
              source: "manual",
              acceptanceCriteria: ["OAuth can start."],
              blockedByItemIds: []
            }],
            missionControlStates: [{ runId: "run_req_omega_001", projectId: "project_omega", workItems: [], events: [], syncIntents: [], updatedAt: new Date().toISOString() }],
            missionEvents: [],
            syncIntents: [],
            connections: [],
            uiPreferences: [{ id: "default", activeNav: "Issues", selectedProviderId: "github", selectedWorkItemId: "item_manual_1", inspectorOpen: true, activeInspectorPanel: "provider", runnerPreset: "local-proof", statusFilter: "All", assigneeFilter: "All", sortDirection: "desc", collapsedGroups: [] }],
            pipelines: [],
            checkpoints: [],
            missions: [],
            operations: [],
            proofRecords: []
          }
        }));
      }
      if (url.endsWith("/workspace") && init?.method === "PUT") {
        return Promise.resolve(jsonResponse({ ok: true }));
      }
      if (url.endsWith("/github/oauth/start")) {
        expect(init?.method).toBe("POST");
        return Promise.resolve(jsonResponse({
          configured: true,
          authorizeUrl: "https://github.com/login/oauth/authorize?client_id=omega",
          state: "omega_state",
          redirectUri: "http://127.0.0.1:3888/auth/github/callback",
          scopes: ["repo", "read:org", "workflow"]
        }));
      }
      if (url.endsWith("/github/oauth/config") && !init) {
        return Promise.resolve(jsonResponse({
          configured: false,
          clientId: "",
          redirectUri: "http://127.0.0.1:3888/auth/github/callback",
          tokenUrl: "https://github.com/login/oauth/access_token",
          secretConfigured: false,
          source: "empty"
        }));
      }
      if (url.endsWith("/github/status")) {
        return Promise.resolve(jsonResponse({
          available: true,
          authenticated: false,
          output: "",
          oauthConfigured: false,
          oauthAuthenticated: false
        }));
      }
      if (url.endsWith("/github/oauth/config") && init?.method === "PUT") {
        expect(JSON.parse(String(init.body))).toMatchObject({
          clientId: "omega-client",
          clientSecret: "omega-secret",
          redirectUri: "http://127.0.0.1:3888/auth/github/callback"
        });
        return Promise.resolve(jsonResponse({
          configured: true,
          clientId: "omega-client",
          redirectUri: "http://127.0.0.1:3888/auth/github/callback",
          tokenUrl: "https://github.com/login/oauth/access_token",
          secretConfigured: true,
          source: "app"
        }));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 0, pipelines: 0, checkpoints: 0, missions: 0, operations: 0, proofRecords: 0, events: 0 },
          pipelineStatus: {},
          checkpointStatus: {},
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 0, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/llm-providers")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/llm-provider-selection")) {
        return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      }
      if (url.endsWith("/pipeline-templates") || url.endsWith("/agent-definitions") || url.endsWith("/pipelines") || url.endsWith("/checkpoints") || url.endsWith("/local-capabilities")) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    fireEvent.click(await screen.findByText("Provider access"));
    fireEvent.click(await screen.findByText("OAuth app setup"));
    fireEvent.change(await screen.findByLabelText("Client ID"), { target: { value: "omega-client" } });
    fireEvent.change(screen.getByLabelText("Client secret"), { target: { value: "omega-secret" } });
    fireEvent.change(screen.getByLabelText("Callback URL"), { target: { value: "http://127.0.0.1:3888/auth/github/callback" } });
    fireEvent.click(screen.getByRole("button", { name: "Save OAuth app" }));

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "http://127.0.0.1:3888/github/oauth/config",
        expect.objectContaining({ method: "PUT" })
      )
    );

    fireEvent.click(await screen.findByRole("button", { name: "Continue with GitHub" }));

    await waitFor(() =>
      expect(navigateToExternalUrl).toHaveBeenCalledWith("https://github.com/login/oauth/authorize?client_id=omega")
    );
  });

  it("opens GitHub provider access from the sidebar even without work items", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");
    const openExternalUrlInNewTab = vi.fn();
    vi.doMock("../browserNavigation", () => ({ navigateToExternalUrl: vi.fn(), openExternalUrlInNewTab }));

    const now = new Date().toISOString();
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/workspace")) {
        return Promise.resolve(jsonResponse({
          schemaVersion: 1,
          savedAt: now,
          tables: {
            projects: [{ id: "project_omega", name: "Omega", description: "", team: "Omega", status: "Active", labels: [], repositoryTargets: [], createdAt: now, updatedAt: now }],
            workItems: [],
            missionControlStates: [{ runId: "run_req_omega_001", projectId: "project_omega", workItems: [], events: [], syncIntents: [], updatedAt: now }],
            missionEvents: [],
            syncIntents: [],
            connections: [],
            uiPreferences: [{ id: "default", activeNav: "Views", selectedProviderId: "google", selectedWorkItemId: "", inspectorOpen: false, activeInspectorPanel: "properties", runnerPreset: "local-proof", statusFilter: "All", assigneeFilter: "All", sortDirection: "desc", collapsedGroups: [] }],
            pipelines: [],
            checkpoints: [],
            missions: [],
            operations: [],
            proofRecords: []
          }
        }));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 0, pipelines: 0, checkpoints: 0, missions: 0, operations: 0, proofRecords: 0, events: 0 },
          pipelineStatus: {},
          checkpointStatus: {},
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 0, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/github/oauth/config")) {
        return Promise.resolve(jsonResponse({
          configured: false,
          clientId: "",
          redirectUri: "http://127.0.0.1:3888/auth/github/callback",
          tokenUrl: "https://github.com/login/oauth/access_token",
          secretConfigured: false,
          source: "empty"
        }));
      }
      if (url.endsWith("/github/oauth/start")) {
        return Promise.resolve(jsonResponse({
          configured: false,
          reason: "GitHub OAuth app is not configured."
        }));
      }
      if (url.endsWith("/github/cli-login/start")) {
        return Promise.resolve(jsonResponse({
          started: true,
          method: "gh-cli",
          message: "GitHub CLI sign-in opened. Paste the copied one-time code on GitHub's device page.",
          verificationUrl: "https://github.com/login/device"
        }));
      }
      if (url.endsWith("/github/status")) {
        return Promise.resolve(jsonResponse({
          available: true,
          authenticated: true,
          account: "ZYOOO",
          output: "",
          oauthConfigured: false,
          oauthAuthenticated: false
        }));
      }
      if (url.endsWith("/llm-provider-selection")) {
        return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      }
      return Promise.resolve(jsonResponse([]));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    fireEvent.click(await screen.findByText("GitHub"));

    await waitFor(() => expect(screen.getByText("Provider access")).toBeInTheDocument());
    const oauthSetupSummary = screen.getByText("OAuth app setup");
    expect(oauthSetupSummary.closest("details")).not.toHaveAttribute("open");
    fireEvent.click(oauthSetupSummary);
    expect(screen.getByLabelText("Client ID")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Continue with GitHub" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Continue with GitHub" }));
    await waitFor(() =>
      expect(screen.getByText("GitHub CLI sign-in opened. Paste the copied one-time code on GitHub's device page.")).toBeInTheDocument()
    );
    fireEvent.click(screen.getByRole("button", { name: "Open device page" }));
    expect(openExternalUrlInNewTab).toHaveBeenCalledWith("https://github.com/login/device");
    fireEvent.click(screen.getByRole("button", { name: "Check GitHub status" }));
    await waitFor(() => expect(screen.getByText("GitHub CLI is connected as ZYOOO.")).toBeInTheDocument());
  });

  it("imports GitHub issues from the Projects view", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");
    vi.spyOn(window, "confirm").mockReturnValue(true);

    const now = new Date().toISOString();
    const emptyWorkspace = {
      schemaVersion: 1,
      savedAt: now,
      tables: {
        projects: [{ id: "project_omega", name: "Omega", description: "", team: "Omega", status: "Active", labels: [], repositoryTargets: [], createdAt: now, updatedAt: now }],
        workItems: [],
        missionControlStates: [{ runId: "run_req_omega_001", projectId: "project_omega", workItems: [], events: [], syncIntents: [], updatedAt: now }],
        missionEvents: [],
        syncIntents: [],
        connections: [],
        uiPreferences: [{ id: "default", activeNav: "Projects", selectedProviderId: "github", selectedWorkItemId: "", inspectorOpen: true, activeInspectorPanel: "provider", runnerPreset: "local-proof", statusFilter: "All", assigneeFilter: "All", sortDirection: "desc", collapsedGroups: [] }],
        pipelines: [],
        checkpoints: [],
        missions: [],
        operations: [],
        proofRecords: [
          {
            id: "proof_impl",
            operationId: "pipeline_item_done:devflow-cycle",
            label: "devflow-cycle-proof",
            value: "implementation-summary.md",
            sourcePath: "/Users/zyong/Omega/workspaces/OMG-2/.omega/proof/implementation-summary.md",
            createdAt: now
          }
        ]
      }
    };
    const importedWorkspace = {
      ...emptyWorkspace,
      tables: {
        ...emptyWorkspace.tables,
        projects: [{
          ...emptyWorkspace.tables.projects[0],
          repositoryTargets: [{
            id: "repo_acme_demo",
            kind: "github",
            owner: "acme",
            repo: "demo",
            defaultBranch: "main",
            url: "https://github.com/acme/demo"
          }],
          defaultRepositoryTargetId: "repo_acme_demo"
        }],
        requirements: [{
          id: "req_acme_demo_7",
          projectId: "project_omega",
          repositoryTargetId: "repo_acme_demo",
          source: "github_issue",
          sourceExternalRef: "acme/demo#7",
          title: "Imported issue",
          rawText: "From GitHub",
          structured: { summary: "Imported issue" },
          acceptanceCriteria: ["Imported issue is understood"],
          risks: [],
          status: "converted",
          createdAt: now,
          updatedAt: now
        }],
        workItems: [{
          projectId: "project_omega",
          createdAt: now,
          updatedAt: now,
          id: "github_acme_demo_7",
          requirementId: "req_acme_demo_7",
          key: "GH-7",
          title: "Imported issue",
          description: "From GitHub",
          status: "Ready",
          priority: "Medium",
          assignee: "alice",
          labels: ["github", "bug"],
          team: "Omega",
          stageId: "intake",
          target: "https://github.com/acme/demo/issues/7",
          source: "github_issue",
          sourceExternalRef: "acme/demo#7",
          repositoryTargetId: "repo_acme_demo",
          acceptanceCriteria: ["Imported issue is understood"],
          blockedByItemIds: []
        }]
      }
    };

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) {
        return Promise.resolve(jsonResponse(emptyWorkspace));
      }
      if (url.endsWith("/workspace") && init?.method === "PUT") {
        return Promise.resolve(jsonResponse({ ok: true }));
      }
      if (url.endsWith("/github/import-issues")) {
        expect(init).toMatchObject({ method: "POST" });
        expect(JSON.parse(String(init?.body))).toMatchObject({ owner: "acme", repo: "demo" });
        return Promise.resolve(jsonResponse(importedWorkspace));
      }
      if (url.endsWith("/github/bind-repository-target")) {
        expect(init).toMatchObject({ method: "POST" });
        expect(JSON.parse(String(init?.body))).toMatchObject({ owner: "acme", repo: "demo", nameWithOwner: "acme/demo" });
        return Promise.resolve(jsonResponse({
          ...emptyWorkspace,
          tables: {
            ...emptyWorkspace.tables,
            projects: [{
              ...emptyWorkspace.tables.projects[0],
              repositoryTargets: [{
                id: "repo_acme_demo",
                kind: "github",
                owner: "acme",
                repo: "demo",
                defaultBranch: "main",
                url: "https://github.com/acme/demo"
              }],
              defaultRepositoryTargetId: "repo_acme_demo"
            }]
          }
        }));
      }
      if (url.includes("/github/repository-targets/")) {
        expect(init).toMatchObject({ method: "DELETE" });
        expect(url).toContain("repo_acme_demo");
        return Promise.resolve(jsonResponse(emptyWorkspace));
      }
      if (url.endsWith("/github/repositories")) {
        return Promise.resolve(jsonResponse([
          {
            name: "demo",
            nameWithOwner: "acme/demo",
            owner: { login: "acme" },
            description: "Demo repository",
            url: "https://github.com/acme/demo",
            isPrivate: false,
            defaultBranchRef: { name: "main" }
          }
        ]));
      }
      if (url.endsWith("/requirements")) {
        return Promise.resolve(jsonResponse(importedWorkspace.tables.requirements));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 0, pipelines: 0, checkpoints: 0, missions: 0, operations: 0, proofRecords: 0, events: 0 },
          pipelineStatus: {},
          checkpointStatus: {},
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 0, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/llm-provider-selection")) {
        return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      }
      if (url.endsWith("/github/oauth/config")) {
        return Promise.resolve(jsonResponse({
          configured: false,
          clientId: "",
          redirectUri: "http://127.0.0.1:3888/auth/github/callback",
          tokenUrl: "https://github.com/login/oauth/access_token",
          secretConfigured: false,
          source: "empty"
        }));
      }
      if (url.endsWith("/llm-providers") || url.endsWith("/pipeline-templates") || url.endsWith("/agent-definitions") || url.endsWith("/pipelines") || url.endsWith("/checkpoints") || url.endsWith("/local-capabilities")) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    await waitFor(() => expect(screen.getAllByText("acme/demo").length).toBeGreaterThan(0));
    expect(screen.queryByRole("button", { name: "Sync issues" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Run ready issue now" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Create workspace" }));
    await waitFor(() => expect(screen.getByRole("heading", { name: "Work items" })).toBeInTheDocument());
    const workspaceNavigation = screen.getByRole("navigation", { name: "Project workspaces" });
    expect(within(workspaceNavigation).getByRole("button", { name: "acme/demo 0" })).toBeInTheDocument();
    expect(within(workspaceNavigation).queryByRole("button", { name: "Sync issues" })).not.toBeInTheDocument();
    expect(within(workspaceNavigation).queryByRole("button", { name: "Run ready issue now" })).not.toBeInTheDocument();
    expect(screen.queryByText("Repository workspace")).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "GitHub repositories" })).not.toBeInTheDocument();
    const githubIssuesPanel = screen.getByRole("region", { name: "GitHub issues" });
    expect(within(githubIssuesPanel).getByText("GitHub Issues")).toBeInTheDocument();
    expect(within(githubIssuesPanel).getByText("0 synced")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Sync GitHub issues" }));

    await waitFor(() => expect(screen.getAllByText("Synced 1 issue from acme/demo.").length).toBeGreaterThan(0));
    await waitFor(() => expect(screen.getAllByText("Imported issue").length).toBeGreaterThan(0));
    const syncedGitHubIssuesPanel = screen.getByRole("region", { name: "GitHub issues" });
    expect(within(syncedGitHubIssuesPanel).getByText("1 synced")).toBeInTheDocument();
    expect(within(syncedGitHubIssuesPanel).getByText("Imported issue")).toBeInTheDocument();
    expect(screen.getAllByText("acme/demo#7").length).toBeGreaterThan(0);
    fireEvent.click(within(syncedGitHubIssuesPanel).getByRole("button", { name: "Collapse GitHub issues" }));
    expect(within(syncedGitHubIssuesPanel).queryByText("Imported issue")).not.toBeInTheDocument();
    fireEvent.click(within(syncedGitHubIssuesPanel).getByRole("button", { name: "Expand GitHub issues" }));
    fireEvent.click(screen.getAllByText("Imported issue")[0]);
    await waitFor(() => expect(screen.getByText("Requirement source")).toBeInTheDocument());
    expect(screen.getByText("GitHub issue")).toBeInTheDocument();
    expect(screen.getByText("1 item")).toBeInTheDocument();

    fireEvent.click(within(workspaceNavigation).getByRole("button", { name: "Configure acme/demo" }));
    fireEvent.click(await screen.findByRole("button", { name: "Delete workspace" }));
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "http://127.0.0.1:3888/github/repository-targets/repo_acme_demo",
        expect.objectContaining({ method: "DELETE" })
      )
    );
    await waitFor(() => expect(screen.queryByText("Imported issue")).not.toBeInTheDocument());
  });

  it("defaults work items to the only repository workspace and keeps legacy unscoped items out of that view", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");
    const now = new Date().toISOString();
    const workspace = {
      schemaVersion: 1,
      savedAt: now,
      tables: {
        projects: [{
          id: "project_omega",
          name: "Omega",
          description: "",
          team: "Omega",
          status: "Active",
          labels: [],
          repositoryTargets: [{
            id: "repo_acme_demo",
            kind: "github",
            owner: "acme",
            repo: "demo",
            defaultBranch: "main",
            url: "https://github.com/acme/demo"
          }],
          defaultRepositoryTargetId: "repo_acme_demo",
          createdAt: now,
          updatedAt: now
        }],
        workItems: [
          {
            projectId: "project_omega",
            createdAt: now,
            updatedAt: now,
            id: "item_legacy",
            key: "OMG-1",
            title: "Legacy demo writing",
            description: "",
            status: "Ready",
            priority: "High",
            assignee: "requirement",
            labels: ["manual"],
            team: "Omega",
            stageId: "intake",
            target: "/Users/demo/Omega",
            source: "manual",
            acceptanceCriteria: [],
            blockedByItemIds: []
          },
          {
            projectId: "project_omega",
            createdAt: now,
            updatedAt: now,
            id: "item_repo",
            key: "GH-1",
            title: "Repo scoped issue",
            description: "",
            status: "Ready",
            priority: "Medium",
            assignee: "requirement",
            labels: ["github"],
            team: "Omega",
            stageId: "intake",
            target: "https://github.com/acme/demo/issues/1",
            source: "github_issue",
            sourceExternalRef: "acme/demo#1",
            repositoryTargetId: "repo_acme_demo",
            acceptanceCriteria: [],
            blockedByItemIds: []
          }
        ],
        missionControlStates: [{ runId: "run_req_omega_001", projectId: "project_omega", workItems: [], events: [], syncIntents: [], updatedAt: now }],
        missionEvents: [],
        syncIntents: [],
        connections: [],
        uiPreferences: [{ id: "default", activeNav: "Issues", selectedProviderId: "github", selectedWorkItemId: "", inspectorOpen: false, activeInspectorPanel: "properties", runnerPreset: "local-proof", statusFilter: "All", assigneeFilter: "All", sortDirection: "desc", collapsedGroups: [] }],
        pipelines: [],
        checkpoints: [],
        missions: [],
        operations: [],
        proofRecords: []
      }
    };
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) {
        return Promise.resolve(jsonResponse(workspace));
      }
      if (url.endsWith("/workspace") && init?.method === "PUT") {
        return Promise.resolve(jsonResponse({ ok: true }));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 2, pipelines: 0, checkpoints: 0, missions: 0, operations: 0, proofRecords: 0, events: 0 },
          pipelineStatus: {},
          checkpointStatus: {},
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 0, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/llm-provider-selection")) {
        return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      }
      if (url.endsWith("/github/oauth/config")) {
        return Promise.resolve(jsonResponse({
          configured: false,
          clientId: "",
          redirectUri: "http://127.0.0.1:3888/auth/github/callback",
          tokenUrl: "https://github.com/login/oauth/access_token",
          secretConfigured: false,
          source: "empty"
        }));
      }
      if (url.endsWith("/github/status")) {
        return Promise.resolve(jsonResponse({ available: true, authenticated: true, output: "", account: "acme", oauthConfigured: false, oauthAuthenticated: false }));
      }
      if (url.endsWith("/llm-providers") || url.endsWith("/pipeline-templates") || url.endsWith("/agent-definitions") || url.endsWith("/pipelines") || url.endsWith("/checkpoints") || url.endsWith("/local-capabilities")) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    await waitFor(() => expect(screen.getAllByText("Repo scoped issue").length).toBeGreaterThan(0));
    expect(screen.queryByText("Legacy demo writing")).not.toBeInTheDocument();
    expect(screen.getAllByText("acme/demo").length).toBeGreaterThan(0);
  });

  it("creates app requirements inside the active repository workspace and runs them against that repo", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");

    const now = new Date().toISOString();
    const baseWorkspace = {
      schemaVersion: 1,
      savedAt: now,
      tables: {
        projects: [{
          id: "project_omega",
          name: "Omega",
          description: "",
          team: "Omega",
          status: "Active",
          labels: [],
          repositoryTargets: [{
            id: "repo_acme_demo",
            kind: "github",
            owner: "acme",
            repo: "demo",
            defaultBranch: "main",
            url: "https://github.com/acme/demo"
          }],
          defaultRepositoryTargetId: "repo_acme_demo",
          createdAt: now,
          updatedAt: now
        }],
        workItems: [{
          projectId: "project_omega",
          createdAt: now,
          updatedAt: now,
          id: "github_acme_demo_7",
          key: "GH-7",
          title: "Imported issue",
          description: "From GitHub",
          status: "Ready",
          priority: "Medium",
          assignee: "requirement",
          labels: ["github"],
          team: "Omega",
          stageId: "intake",
          target: "https://github.com/acme/demo/issues/7",
          source: "github_issue",
          sourceExternalRef: "acme/demo#7",
          repositoryTargetId: "repo_acme_demo",
          acceptanceCriteria: ["Imported issue is understood"],
          blockedByItemIds: []
        }],
        missionControlStates: [{ runId: "run_req_omega_001", projectId: "project_omega", workItems: [], events: [], syncIntents: [], updatedAt: now }],
        missionEvents: [],
        syncIntents: [],
        connections: [],
        uiPreferences: [{ id: "default", activeNav: "Issues", selectedProviderId: "github", selectedWorkItemId: "", inspectorOpen: false, activeInspectorPanel: "properties", runnerPreset: "local-proof", statusFilter: "All", assigneeFilter: "All", sortDirection: "desc", collapsedGroups: [] }],
        pipelines: [],
        checkpoints: [],
        missions: [],
        operations: [],
        proofRecords: []
      }
    };
    let workspaceSnapshot: any = baseWorkspace;
    let workItemPostCount = 0;

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) {
        return Promise.resolve(jsonResponse(workspaceSnapshot));
      }
      if (url.endsWith("/workspace") && init?.method === "PUT") {
        return Promise.resolve(jsonResponse({ ok: true }));
      }
      if (url.endsWith("/work-items")) {
        workItemPostCount += 1;
        expect(init).toMatchObject({ method: "POST" });
        const { item } = JSON.parse(String(init?.body));
        expect(item).toMatchObject({
          title: "Create an empty docs file",
          source: "manual",
          target: "https://github.com/acme/demo",
          repositoryTargetId: "repo_acme_demo"
        });
        workspaceSnapshot = {
          ...baseWorkspace,
          tables: {
            ...baseWorkspace.tables,
            workItems: [...baseWorkspace.tables.workItems, { ...item, projectId: "project_omega", createdAt: now, updatedAt: now }]
          }
        };
        return Promise.resolve(jsonResponse(workspaceSnapshot));
      }
      if (/\/work-items\/[^/]+$/.test(url) && init?.method === "PATCH") {
        const patch = JSON.parse(String(init.body));
        workspaceSnapshot = {
          ...workspaceSnapshot,
          tables: {
            ...workspaceSnapshot.tables,
            workItems: workspaceSnapshot.tables.workItems.map((item: any) =>
              url.endsWith(`/work-items/${item.id}`)
                ? { ...item, ...patch, updatedAt: now }
                : item
            )
          }
        };
        return Promise.resolve(jsonResponse(workspaceSnapshot));
      }
      if (url.endsWith("/missions/from-work-item")) {
        expect(init).toMatchObject({ method: "POST" });
        const { item } = JSON.parse(String(init?.body));
        expect(item).toMatchObject({
          source: "manual",
          target: "https://github.com/acme/demo",
          repositoryTargetId: "repo_acme_demo"
        });
        return Promise.resolve(jsonResponse({
          id: "mission_OMG-1_intake",
          sourceIssueKey: "OMG-1",
          sourceWorkItemId: item.id,
          title: item.title,
          target: item.target,
          repositoryTargetId: item.repositoryTargetId,
          status: "ready",
          checkpointRequired: true,
          operations: [{ id: "operation_intake", stageId: "intake", agentId: "requirement", status: "ready", prompt: "Create an empty docs file", requiredProof: ["proof"] }],
          links: []
        }));
      }
      if (url.endsWith("/operations/run")) {
        expect(init).toMatchObject({ method: "POST" });
        expect(JSON.parse(String(init?.body))).toMatchObject({ runner: "local-proof" });
        return Promise.resolve(jsonResponse({
          operationId: "operation_intake",
          status: "passed",
          workspacePath: "/tmp/omega",
          proofFiles: ["/tmp/omega/.omega/proof/git-diff.patch"],
          stdout: "",
          stderr: "",
          branchName: "omega/OMG-1-intake",
          changedFiles: ["omega-empty.md"],
          events: []
        }));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 0, pipelines: 0, checkpoints: 0, missions: 0, operations: 0, proofRecords: 0, events: 0 },
          pipelineStatus: {},
          checkpointStatus: {},
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 0, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/llm-provider-selection")) {
        return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      }
      if (url.endsWith("/github/oauth/config")) {
        return Promise.resolve(jsonResponse({
          configured: false,
          clientId: "",
          redirectUri: "http://127.0.0.1:3888/auth/github/callback",
          tokenUrl: "https://github.com/login/oauth/access_token",
          secretConfigured: false,
          source: "empty"
        }));
      }
      if (url.endsWith("/llm-providers") || url.endsWith("/pipeline-templates") || url.endsWith("/agent-definitions") || url.endsWith("/pipelines") || url.endsWith("/checkpoints") || url.endsWith("/local-capabilities")) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    const workspaceNavigation = await screen.findByRole("navigation", { name: "Project workspaces" });
    fireEvent.click(within(workspaceNavigation).getByRole("button", { name: "acme/demo 1" }));

    expect(screen.queryByRole("button", { name: "New item in Ready" })).not.toBeInTheDocument();
    fireEvent.click(await screen.findByRole("button", { name: "New requirement" }));
    fireEvent.focus(await screen.findByPlaceholderText("Title"));
    fireEvent.change(screen.getByPlaceholderText("Type your description here..."), {
      target: { value: "## Create an empty docs file\n\n- Use this app-created requirement as the pipeline entry.\n- Touch `omega-empty.md`." }
    });
    fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    expect(screen.getByRole("heading", { name: "Create an empty docs file" })).toBeInTheDocument();
    expect(screen.getByText("Use this app-created requirement as the pipeline entry.")).toBeInTheDocument();
    expect(screen.getByText("omega-empty.md")).toBeInTheDocument();
    const createButton = screen.getByRole("button", { name: "Create" });
    fireEvent.click(createButton);
    fireEvent.click(createButton);

    await waitFor(() => expect(screen.queryByRole("button", { name: "Creating..." })).not.toBeInTheDocument());
    expect(workItemPostCount).toBe(1);
    const shell = document.querySelector("main.product-shell");
    expect(shell?.className).toContain("inspector-collapsed");

    fireEvent.click(within(screen.getByRole("region", { name: "Work items" })).getByText("Create an empty docs file"));
    expect(screen.getByRole("region", { name: "Work item detail" })).toBeInTheDocument();
    expect(screen.getByText("Acceptance criteria")).toBeInTheDocument();
    expect(shell?.className).toContain("inspector-collapsed");
    fireEvent.click(screen.getByRole("button", { name: "Properties" }));
    expect(shell?.className).not.toContain("inspector-collapsed");
    fireEvent.click(screen.getByRole("button", { name: "Run" }));

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "http://127.0.0.1:3888/pipelines/from-template",
        expect.objectContaining({ method: "POST" })
      )
    );
  });

  it("shows pending checkpoints and can approve one", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) {
        return Promise.resolve(jsonResponse({ error: "workspace not found" }, 404));
      }
      if (url.endsWith("/workspace") && init?.method === "PUT") {
        return Promise.resolve(jsonResponse({ ok: true }));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 1, pipelines: 1, checkpoints: 1, missions: 0, operations: 0, proofRecords: 0, events: 0 },
          pipelineStatus: { "waiting-human": 1 },
          checkpointStatus: { pending: 1 },
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 2, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/llm-providers")) {
        return Promise.resolve(jsonResponse([{ id: "openai", name: "OpenAI", models: ["gpt-5.4-mini"], defaultModel: "gpt-5.4-mini" }]));
      }
      if (url.endsWith("/llm-provider-selection")) {
        return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      }
      if (url.endsWith("/pipeline-templates")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/agent-definitions")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/local-capabilities")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/pipelines")) {
        return Promise.resolve(jsonResponse([{ id: "pipeline_item_1", workItemId: "item_1", status: "waiting-human", run: { stages: [] } }]));
      }
      if (url.endsWith("/checkpoints")) {
        return Promise.resolve(jsonResponse([
          {
            id: "pipeline_item_1:intake",
            pipelineId: "pipeline_item_1",
            stageId: "intake",
            status: "pending",
            title: "Intake approval",
            summary: "Structured requirements need confirmation"
          }
        ]));
      }
      if (url.endsWith("/checkpoints/pipeline_item_1:intake/approve")) {
        expect(init?.method).toBe("POST");
        expect(JSON.parse(String(init?.body))).toMatchObject({ reviewer: "human" });
        return Promise.resolve(jsonResponse({ id: "pipeline_item_1:intake", status: "approved" }));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    fireEvent.click(screen.getByRole("button", { name: /Views/ }));

    await waitFor(() => expect(screen.getByText("Human checkpoints")).toBeInTheDocument());
    expect(screen.getByText("Intake approval")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "http://127.0.0.1:3888/checkpoints/pipeline_item_1:intake/approve",
        expect.objectContaining({ method: "POST" })
      )
    );
  });

  it("sends a Feishu checkpoint notification from the operator view", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) {
        return Promise.resolve(jsonResponse({ error: "workspace not found" }, 404));
      }
      if (url.endsWith("/workspace") && init?.method === "PUT") {
        return Promise.resolve(jsonResponse({ ok: true }));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 1, pipelines: 1, checkpoints: 1, missions: 0, operations: 0, proofRecords: 0, events: 0 },
          pipelineStatus: { "waiting-human": 1 },
          checkpointStatus: { pending: 1 },
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 1, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/checkpoints")) {
        return Promise.resolve(jsonResponse([
          {
            id: "pipeline_item_1:intake",
            pipelineId: "pipeline_item_1",
            stageId: "intake",
            status: "pending",
            title: "Intake approval",
            summary: "Structured requirements need confirmation"
          }
        ]));
      }
      if (url.endsWith("/feishu/notify")) {
        expect(init).toMatchObject({ method: "POST" });
        expect(JSON.parse(String(init?.body))).toMatchObject({
          chatId: "oc_demo"
        });
        expect(String(init?.body)).toContain("Intake approval");
        return Promise.resolve(jsonResponse({ status: "sent", provider: "feishu", tool: "lark-cli", chatId: "oc_demo", messageId: "om_123" }));
      }
      if (url.endsWith("/llm-provider-selection")) {
        return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      }
      if (url.endsWith("/github/oauth/config")) {
        return Promise.resolve(jsonResponse({
          configured: false,
          clientId: "",
          redirectUri: "http://127.0.0.1:3888/auth/github/callback",
          tokenUrl: "https://github.com/login/oauth/access_token",
          secretConfigured: false,
          source: "empty"
        }));
      }
      if (url.endsWith("/llm-providers") || url.endsWith("/pipeline-templates") || url.endsWith("/agent-definitions") || url.endsWith("/pipelines") || url.endsWith("/local-capabilities")) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    fireEvent.click(screen.getByRole("button", { name: /Views/ }));
    fireEvent.change(await screen.findByLabelText("Feishu chat"), { target: { value: "oc_demo" } });
    fireEvent.click(screen.getByRole("button", { name: "Notify Feishu" }));

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "http://127.0.0.1:3888/feishu/notify",
        expect.objectContaining({ method: "POST" })
      )
    );
  });

  it("can run the current pipeline stage from the operator view", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) {
        return Promise.resolve(jsonResponse({ error: "workspace not found" }, 404));
      }
      if (url.endsWith("/workspace") && init?.method === "PUT") {
        return Promise.resolve(jsonResponse({ ok: true }));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 1, pipelines: 1, checkpoints: 0, missions: 0, operations: 0, proofRecords: 0, events: 0 },
          pipelineStatus: { running: 1 },
          checkpointStatus: {},
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 0, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/llm-providers")) {
        return Promise.resolve(jsonResponse([{ id: "openai", name: "OpenAI", models: ["gpt-5.4-mini"], defaultModel: "gpt-5.4-mini" }]));
      }
      if (url.endsWith("/llm-provider-selection")) {
        return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      }
      if (url.endsWith("/pipeline-templates")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/agent-definitions")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/local-capabilities")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/pipelines")) {
        return Promise.resolve(jsonResponse([{ id: "pipeline_item_1", workItemId: "item_1", status: "running", run: { stages: [] } }]));
      }
      if (url.endsWith("/checkpoints")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/pipelines/pipeline_item_1/run-current-stage")) {
        expect(init?.method).toBe("POST");
        expect(JSON.parse(String(init?.body))).toMatchObject({ runner: "local-proof" });
        return Promise.resolve(jsonResponse({ pipeline: { id: "pipeline_item_1", status: "waiting-human" }, operationResult: { status: "passed" } }));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    fireEvent.click(screen.getByRole("button", { name: /Views/ }));

    await waitFor(() => expect(screen.getByText("pipeline_item_1")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Run stage" }));

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "http://127.0.0.1:3888/pipelines/pipeline_item_1/run-current-stage",
        expect.objectContaining({ method: "POST" })
      )
    );
  });

  it("can run a DevFlow PR cycle from the operator pipeline list", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) {
        return Promise.resolve(jsonResponse({ error: "workspace not found" }, 404));
      }
      if (url.endsWith("/workspace") && init?.method === "PUT") {
        return Promise.resolve(jsonResponse({ ok: true }));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 1, pipelines: 1, checkpoints: 0, missions: 0, operations: 0, proofRecords: 0, events: 0 },
          pipelineStatus: { running: 1 },
          checkpointStatus: {},
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 0, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/llm-provider-selection")) {
        return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      }
      if (url.endsWith("/pipeline-templates")) {
        return Promise.resolve(jsonResponse([{ id: "devflow-pr", name: "DevFlow PR cycle", description: "", stages: [{ id: "todo" }, { id: "done" }] }]));
      }
      if (url.endsWith("/pipelines")) {
        return Promise.resolve(jsonResponse([{ id: "pipeline_item_1", workItemId: "item_1", status: "running", templateId: "devflow-pr", run: { stages: [] } }]));
      }
      if (url.endsWith("/pipelines/pipeline_item_1/run-devflow-cycle")) {
        expect(init).toMatchObject({ method: "POST" });
        expect(JSON.parse(String(init?.body))).toMatchObject({ autoApproveHuman: false, autoMerge: false });
        return Promise.resolve(jsonResponse({
          status: "done",
          branchName: "omega/OMG-1-devflow-cycle",
          pullRequestUrl: "https://github.com/acme/demo/pull/123",
          proofFiles: ["/tmp/proof.md"],
          pipeline: { id: "pipeline_item_1", workItemId: "item_1", status: "done", templateId: "devflow-pr", run: { stages: [] } }
        }));
      }
      if (url.endsWith("/github/oauth/config")) {
        return Promise.resolve(jsonResponse({
          configured: false,
          clientId: "",
          redirectUri: "http://127.0.0.1:3888/auth/github/callback",
          tokenUrl: "https://github.com/login/oauth/access_token",
          secretConfigured: false,
          source: "empty"
        }));
      }
      if (url.endsWith("/github/status")) {
        return Promise.resolve(jsonResponse({ available: true, authenticated: true, output: "", account: "acme", oauthConfigured: false, oauthAuthenticated: false }));
      }
      if (url.endsWith("/local-workspace-root")) {
        return Promise.resolve(jsonResponse({ workspaceRoot: "/Users/zyong/Omega/workspaces" }));
      }
      if (url.endsWith("/llm-providers") || url.endsWith("/agent-definitions") || url.endsWith("/checkpoints") || url.endsWith("/local-capabilities")) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    fireEvent.click(screen.getByRole("button", { name: /Views/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Run DevFlow cycle" }));

    await waitFor(() => expect(screen.getByText(/DevFlow cycle completed/)).toBeInTheDocument());
    expect(fetchMock).toHaveBeenCalledWith(
      "http://127.0.0.1:3888/pipelines/pipeline_item_1/run-devflow-cycle",
      expect.objectContaining({ method: "POST" })
    );
  });

  it("runs repository work item buttons through the DevFlow PR cycle", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");

    const now = new Date().toISOString();
    const workspace = {
      schemaVersion: 1,
      savedAt: now,
      tables: {
        projects: [{
          id: "project_req_omega_001",
          name: "OMEGA-1",
          description: "",
          team: "Omega",
          status: "Active",
          labels: [],
          repositoryTargets: [{ id: "repo_ZYOOO_TestRepo", kind: "github", owner: "ZYOOO", repo: "TestRepo", url: "https://github.com/ZYOOO/TestRepo", defaultBranch: "main" }],
          defaultRepositoryTargetId: "repo_ZYOOO_TestRepo",
          createdAt: now,
          updatedAt: now
        }],
        requirements: [{
          id: "req_item_manual_52",
          projectId: "project_req_omega_001",
          repositoryTargetId: "repo_ZYOOO_TestRepo",
          source: "manual",
          sourceExternalRef: "",
          title: "添加一个md文档",
          rawText: "添加一个md文档，里面附上99乘法表",
          structured: {},
          acceptanceCriteria: [],
          risks: [],
          status: "converted",
          createdAt: now,
          updatedAt: now
        }],
        workItems: [{
          id: "item_manual_52",
          key: "OMG-52",
          requirementId: "req_item_manual_52",
          projectId: "project_req_omega_001",
          title: "添加一个md文档",
          description: "添加一个md文档，里面附上99乘法表",
          status: "Ready",
          priority: "High",
          assignee: "requirement",
          labels: ["manual"],
          team: "Omega",
          stageId: "intake",
          target: "https://github.com/ZYOOO/TestRepo",
          source: "manual",
          repositoryTargetId: "repo_ZYOOO_TestRepo",
          acceptanceCriteria: [],
          blockedByItemIds: [],
          createdAt: now,
          updatedAt: now
        }],
        missionControlStates: [],
        missionEvents: [],
        syncIntents: [],
        connections: [],
        uiPreferences: [{ id: "default", activeNav: "Issues", selectedProjectId: "project_req_omega_001" }],
        pipelines: [],
        checkpoints: [],
        missions: [],
        operations: [],
        proofRecords: []
      }
    };

    let workspaceSnapshot: any = workspace;
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) return Promise.resolve(jsonResponse(workspaceSnapshot));
      if (url.endsWith("/workspace") && init?.method === "PUT") return Promise.resolve(jsonResponse({ ok: true }));
      if (url.endsWith("/requirements")) return Promise.resolve(jsonResponse(workspaceSnapshot.tables.requirements));
      if (url.endsWith("/work-items/item_manual_52") && init?.method === "PATCH") {
        const patch = JSON.parse(String(init.body));
        workspaceSnapshot = {
          ...workspaceSnapshot,
          tables: {
            ...workspaceSnapshot.tables,
            workItems: workspaceSnapshot.tables.workItems.map((item: any) =>
              item.id === "item_manual_52" ? { ...item, ...patch } : item
            )
          }
        };
        return Promise.resolve(jsonResponse(workspaceSnapshot));
      }
      if (url.endsWith("/pipelines/from-template")) {
        expect(JSON.parse(String(init?.body))).toMatchObject({ templateId: "devflow-pr", item: { id: "item_manual_52", repositoryTargetId: "repo_ZYOOO_TestRepo" } });
        return Promise.resolve(jsonResponse({ id: "pipeline_item_manual_52", workItemId: "item_manual_52", status: "draft", templateId: "devflow-pr", run: { stages: [] } }));
      }
      if (url.endsWith("/pipelines/pipeline_item_manual_52/run-devflow-cycle")) {
        expect(JSON.parse(String(init?.body))).toMatchObject({ autoApproveHuman: false, autoMerge: false });
        return Promise.resolve(jsonResponse({
          status: "accepted",
          pipeline: { id: "pipeline_item_manual_52", workItemId: "item_manual_52", status: "running", templateId: "devflow-pr", run: { stages: [] } },
          attempt: { id: "attempt_52", itemId: "item_manual_52", pipelineId: "pipeline_item_manual_52", status: "running" }
        }));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 1, pipelines: 0, checkpoints: 0, missions: 0, operations: 0, proofRecords: 0, events: 0 },
          pipelineStatus: {},
          checkpointStatus: {},
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 0, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/llm-provider-selection")) return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      if (url.endsWith("/github/status")) return Promise.resolve(jsonResponse({ available: true, authenticated: true, output: "", account: "ZYOOO", oauthConfigured: false, oauthAuthenticated: false }));
      if (url.endsWith("/local-workspace-root")) return Promise.resolve(jsonResponse({ workspaceRoot: "/Users/zyong/Omega/workspaces" }));
      if (url.endsWith("/llm-providers") || url.endsWith("/agent-definitions") || url.endsWith("/local-capabilities") || url.endsWith("/pipeline-templates") || url.endsWith("/pipelines") || url.endsWith("/checkpoints") || url.endsWith("/operations") || url.endsWith("/proof-records") || url.endsWith("/execution-locks")) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    const title = (await screen.findAllByText("添加一个md文档"))[0];
    const row = title.closest("article");
    expect(row).not.toBeNull();
    fireEvent.click(within(row as HTMLElement).getByRole("button", { name: "Run" }));

    await waitFor(() => expect(screen.getByText(/Pipeline started for OMG-52/)).toBeInTheDocument());
    expect(fetchMock).toHaveBeenCalledWith(
      "http://127.0.0.1:3888/pipelines/from-template",
      expect.objectContaining({ method: "POST" })
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "http://127.0.0.1:3888/pipelines/pipeline_item_manual_52/run-devflow-cycle",
      expect.objectContaining({ method: "POST" })
    );
  });

  it("marks not-started work clearly, disables completed runs, and shows pipeline agents inline", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");

    const now = new Date().toISOString();
    const workspace = {
      schemaVersion: 1,
      savedAt: now,
      tables: {
        projects: [{
          id: "project_req_omega_001",
          name: "OMEGA-1",
          description: "",
          team: "Omega",
          status: "Active",
          labels: [],
          repositoryTargets: [{ id: "repo_ZYOOO_TestRepo", kind: "github", owner: "ZYOOO", repo: "TestRepo", url: "https://github.com/ZYOOO/TestRepo", defaultBranch: "main" }],
          defaultRepositoryTargetId: "repo_ZYOOO_TestRepo",
          createdAt: now,
          updatedAt: now
        }],
        requirements: [],
        workItems: [
          {
            id: "item_ready",
            key: "OMG-1",
            projectId: "project_req_omega_001",
            title: "Fresh requirement",
            description: "Ready to plan",
            status: "Ready",
            priority: "High",
            assignee: "requirement",
            labels: ["manual"],
            team: "Omega",
            stageId: "intake",
            target: "https://github.com/ZYOOO/TestRepo",
            source: "manual",
            repositoryTargetId: "repo_ZYOOO_TestRepo",
            acceptanceCriteria: [],
            blockedByItemIds: [],
            createdAt: now,
            updatedAt: now
          },
          {
            id: "item_done",
            key: "OMG-2",
            projectId: "project_req_omega_001",
            title: "Delivered requirement",
            description: "Already delivered",
            status: "Done",
            priority: "High",
            assignee: "delivery",
            labels: ["manual"],
            team: "Omega",
            stageId: "delivery",
            target: "https://github.com/ZYOOO/TestRepo",
            source: "manual",
            repositoryTargetId: "repo_ZYOOO_TestRepo",
            acceptanceCriteria: [],
            blockedByItemIds: [],
            createdAt: now,
            updatedAt: now
          },
          {
            id: "item_page_pilot_waiting",
            key: "PP-3",
            projectId: "project_req_omega_001",
            title: "Page Pilot: kept for review",
            description: "Page Pilot edit awaiting delivery confirmation",
            status: "Done",
            priority: "High",
            assignee: "page-pilot",
            labels: ["page-pilot", "live-preview"],
            team: "Omega",
            stageId: "page_pilot",
            target: "https://github.com/ZYOOO/TestRepo",
            source: "manual",
            sourceExternalRef: "page-pilot:item_page_pilot_waiting",
            repositoryTargetId: "repo_ZYOOO_TestRepo",
            acceptanceCriteria: [],
            blockedByItemIds: [],
            createdAt: now,
            updatedAt: now
          }
        ],
        missionControlStates: [],
        missionEvents: [],
        syncIntents: [],
        connections: [],
        uiPreferences: [{ id: "default", activeNav: "Issues", selectedProjectId: "project_req_omega_001" }],
        pipelines: [],
        checkpoints: [],
        missions: [],
        operations: [],
        proofRecords: []
      }
    };

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) return Promise.resolve(jsonResponse(workspace));
      if (url.endsWith("/workspace") && init?.method === "PUT") return Promise.resolve(jsonResponse({ ok: true }));
      if (url.endsWith("/requirements")) return Promise.resolve(jsonResponse([]));
      if (url.endsWith("/pipelines")) {
        return Promise.resolve(jsonResponse([
          {
            id: "pipeline_item_done",
            workItemId: "item_done",
            runId: "run_item_done",
            status: "done",
            templateId: "devflow-pr",
            run: {
              stages: [
                { id: "requirement", title: "Requirement intake", status: "passed", agentIds: ["master", "requirement"] },
                { id: "implementation", title: "Implementation and PR", status: "passed", agentIds: ["coding", "testing"] },
                { id: "review", title: "Review gate", status: "passed", agentIds: ["review", "delivery"] }
              ]
            }
          },
          {
            id: "pipeline_item_page_pilot_waiting",
            workItemId: "item_page_pilot_waiting",
            runId: "run_item_page_pilot_waiting",
            status: "waiting-human",
            templateId: "page-pilot",
            run: {
              stages: [
                { id: "preview_runtime", title: "Preview runtime", status: "passed", agentIds: ["page-pilot"] },
                { id: "page_editing", title: "Page editing", status: "passed", agentIds: ["page-pilot"] },
                { id: "delivery", title: "Delivery", status: "waiting-human", agentIds: ["page-pilot"] }
              ]
            }
          }
        ]));
      }
      if (url.endsWith("/attempts")) {
        return Promise.resolve(jsonResponse([{
          id: "pipeline_item_done:attempt:1",
          itemId: "item_done",
          pipelineId: "pipeline_item_done",
          repositoryTargetId: "repo_ZYOOO_TestRepo",
          status: "done",
          runner: "devflow-pr",
          currentStageId: "delivery",
          workspacePath: "/Users/zyong/Omega/workspaces/OMG-2-devflow-pr",
          branchName: "omega/OMG-2-devflow-cycle",
          pullRequestUrl: "https://github.com/ZYOOO/TestRepo/pull/8",
          durationMs: 1200,
          startedAt: now,
          finishedAt: now,
          stages: [
            { id: "requirement", title: "Requirement intake", status: "passed", agentIds: ["master", "requirement"], outputArtifacts: ["requirement-artifact.json"] },
            { id: "implementation", title: "Implementation and PR", status: "passed", agentIds: ["coding", "testing"], outputArtifacts: ["implementation-summary.md"] },
            { id: "review", title: "Review gate", status: "passed", agentIds: ["review", "delivery"], outputArtifacts: ["code-review-round-1.md"] }
          ],
          events: [{ type: "attempt.completed", message: "Pipeline attempt finished with done.", createdAt: now }]
        }]));
      }
      if (url.endsWith("/orchestrator/tick") && init?.method === "POST") {
        return Promise.resolve(jsonResponse({
          status: "completed",
          repositoryTargetId: "repo_ZYOOO_TestRepo",
          workItem: { id: "item_auto", key: "GH-9", title: "AutoRun issue" },
          pipeline: { id: "pipeline_item_auto", status: "done" },
          runResult: { pullRequestUrl: "https://github.com/ZYOOO/TestRepo/pull/9" }
        }));
      }
      if (url.endsWith("/orchestrator/watchers") && !init) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/orchestrator/watchers/repo_ZYOOO_TestRepo") && init?.method === "PUT") {
        expect(JSON.parse(String(init.body))).toMatchObject({
          status: "active",
          intervalSeconds: 60,
          autoRun: true,
          autoApproveHuman: false,
          autoMerge: false
        });
        return Promise.resolve(jsonResponse({
          id: "orchestrator-watcher:repo_ZYOOO_TestRepo",
          repositoryTargetId: "repo_ZYOOO_TestRepo",
          status: "active",
          intervalSeconds: 60,
          limit: "20",
          autoRun: true,
          autoApproveHuman: false,
          autoMerge: false,
          lastTickStatus: "accepted"
        }));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 3, pipelines: 2, checkpoints: 0, missions: 0, operations: 0, proofRecords: 0, events: 0 },
          pipelineStatus: { done: 1, "waiting-human": 1 },
          checkpointStatus: {},
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 0, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/llm-provider-selection")) return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      if (url.endsWith("/github/status")) return Promise.resolve(jsonResponse({ available: true, authenticated: true, output: "", account: "ZYOOO", oauthConfigured: false, oauthAuthenticated: false }));
      if (url.endsWith("/local-workspace-root")) return Promise.resolve(jsonResponse({ workspaceRoot: "/Users/zyong/Omega/workspaces" }));
      if (url.endsWith("/proof-records")) return Promise.resolve(jsonResponse(workspace.tables.proofRecords));
      if (url.endsWith("/llm-providers") || url.endsWith("/agent-definitions") || url.endsWith("/local-capabilities") || url.endsWith("/pipeline-templates") || url.endsWith("/checkpoints") || url.endsWith("/operations") || url.endsWith("/execution-locks")) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    await screen.findAllByText("Fresh requirement");
    expect(screen.getAllByText("Not started").length).toBeGreaterThan(1);
    expect(screen.getByLabelText(/current progress Rev \+ Ship/)).toBeInTheDocument();
    expect(screen.getAllByText("Rev + Ship")).toHaveLength(1);
    expect(screen.getByText("Page Pilot: kept for review")).toBeInTheDocument();
    expect(screen.getByLabelText(/PP-3 current progress Delivery/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Completed" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByText("Delivered requirement"));
    expect(await screen.findByText("Current attempt")).toBeInTheDocument();
    expect(screen.getByText("devflow-pr · 1200ms")).toBeInTheDocument();
    expect(screen.getByText("implementation-summary.md")).toBeInTheDocument();
    expect(screen.getByText("Attempt history")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /ZYOOO\/TestRepo 3/ }));
    expect(screen.queryByRole("button", { name: "Run ready issue now" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Configure ZYOOO/TestRepo" }));
    fireEvent.click(await screen.findByRole("switch", { name: "Auto scan ready GitHub issues" }));
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "http://127.0.0.1:3888/orchestrator/watchers/repo_ZYOOO_TestRepo",
        expect.objectContaining({
          method: "PUT",
          body: expect.stringContaining("\"status\":\"active\"")
        })
      );
    });
  });

  it("shows review packet details when a work item is waiting for human review", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");

    const now = new Date().toISOString();
    const workspace = {
      schemaVersion: 1,
      savedAt: now,
      tables: {
        projects: [{
          id: "project_req_omega_001",
          name: "OMEGA-1",
          description: "",
          team: "Omega",
          status: "Active",
          labels: [],
          repositoryTargets: [{ id: "repo_ZYOOO_TestRepo", kind: "github", owner: "ZYOOO", repo: "TestRepo", url: "https://github.com/ZYOOO/TestRepo", defaultBranch: "main" }],
          defaultRepositoryTargetId: "repo_ZYOOO_TestRepo",
          createdAt: now,
          updatedAt: now
        }],
        requirements: [],
        workItems: [{
          id: "item_review",
          key: "OMG-8",
          projectId: "project_req_omega_001",
          title: "Add registration page",
          description: "Needs human approval",
          status: "In Review",
          priority: "High",
          assignee: "review",
          labels: ["manual"],
          team: "Omega",
          stageId: "human_review",
          target: "https://github.com/ZYOOO/TestRepo",
          source: "manual",
          repositoryTargetId: "repo_ZYOOO_TestRepo",
          acceptanceCriteria: [],
          blockedByItemIds: [],
          createdAt: now,
          updatedAt: now
        }],
        missionControlStates: [],
        missionEvents: [],
        syncIntents: [],
        connections: [],
        uiPreferences: [{ id: "default", activeNav: "Issues", selectedProjectId: "project_req_omega_001" }],
        pipelines: [],
        checkpoints: [],
        missions: [],
        operations: [],
        proofRecords: []
      }
    };

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) return Promise.resolve(jsonResponse(workspace));
      if (url.endsWith("/workspace") && init?.method === "PUT") return Promise.resolve(jsonResponse({ ok: true }));
      if (url.endsWith("/requirements")) return Promise.resolve(jsonResponse([]));
      if (url.endsWith("/pipelines")) {
        return Promise.resolve(jsonResponse([{
          id: "pipeline_item_review",
          workItemId: "item_review",
          runId: "run_item_review",
          status: "waiting-human",
          templateId: "devflow-pr",
          run: {
            stages: [
              { id: "implementation", title: "Implementation and PR", status: "passed", agentIds: ["coding", "testing"] },
              { id: "human_review", title: "Human Review", status: "needs-human", agentIds: ["review", "delivery"], outputArtifacts: ["human-review-request.md"] }
            ],
            events: [{ type: "agent.passed", stageId: "code_review_round_2", message: "Review agent approved the rework.", timestamp: now }]
          }
        }]));
      }
      if (url.endsWith("/attempts")) {
        return Promise.resolve(jsonResponse([{
          id: "pipeline_item_review:attempt:1",
          itemId: "item_review",
          pipelineId: "pipeline_item_review",
          repositoryTargetId: "repo_ZYOOO_TestRepo",
          status: "waiting-human",
          runner: "devflow-pr",
          currentStageId: "human_review",
          workspacePath: "/Users/zyong/Omega/workspaces/OMG-8-devflow-pr",
          branchName: "omega/OMG-8-devflow",
          pullRequestUrl: "https://github.com/ZYOOO/TestRepo/pull/18",
          durationMs: 871128,
          startedAt: now,
          stages: [
            { id: "implementation", title: "Implementation and PR", status: "passed", agentIds: ["coding", "testing"], evidence: ["implementation-summary.md", "git-diff.patch", "test-report.md"] },
            { id: "code_review_round_2", title: "Code Review Round 2", status: "passed", agentIds: ["review"], evidence: ["code-review-round-2-cycle-3.md"] },
            { id: "human_review", title: "Human Review", status: "needs-human", agentIds: ["review", "delivery"], evidence: ["human-review-request.md"] }
          ],
          events: [
            { type: "agent.changes-requested", stageId: "code_review_round_1", message: "Review requested changes.", createdAt: now },
            { type: "agent.passed", stageId: "code_review_round_2", message: "Review approved the current diff.", createdAt: now }
          ]
        }]));
      }
      if (url.endsWith("/attempts/pipeline_item_review%3Aattempt%3A1/timeline")) {
        return Promise.resolve(jsonResponse({
          attempt: { id: "pipeline_item_review:attempt:1", itemId: "item_review", pipelineId: "pipeline_item_review", status: "waiting-human" },
          pipeline: { id: "pipeline_item_review", workItemId: "item_review", status: "waiting-human" },
          items: [
            { id: "attempt-event:1", time: now, source: "attempt", level: "INFO", eventType: "attempt.started", message: "Attempt started.", stageId: "implementation" },
            { id: "runtime-log:1", time: now, source: "runtime-log", level: "INFO", eventType: "checkpoint.pending", message: "Checkpoint pending.", stageId: "human_review" }
          ],
          generatedAt: now
        }));
      }
      if (url.endsWith("/github/pr-status") && init?.method === "POST") {
        expect(JSON.parse(String(init.body))).toMatchObject({
          url: "https://github.com/ZYOOO/TestRepo/pull/18",
          repositoryOwner: "ZYOOO",
          repositoryName: "TestRepo"
        });
        return Promise.resolve(jsonResponse({
          number: 18,
          title: "Add registration page",
          state: "OPEN",
          mergeable: "MERGEABLE",
          reviewDecision: "APPROVED",
          headRefName: "omega/OMG-8-devflow",
          baseRefName: "main",
          url: "https://github.com/ZYOOO/TestRepo/pull/18",
          deliveryGate: "ready",
          checks: [{ name: "lint", state: "SUCCESS", link: "https://github.com/ZYOOO/TestRepo/actions/runs/1" }],
          proofRecords: []
        }));
      }
      if (url.endsWith("/checkpoints")) {
        return Promise.resolve(jsonResponse([{
          id: "pipeline_item_review:human_review",
          pipelineId: "pipeline_item_review",
          stageId: "human_review",
          status: "pending",
          title: "Human Review approval",
          summary: "Review the PR, diff, tests, and agent decisions before delivery."
        }]));
      }
      if (url.endsWith("/proof-records")) return Promise.resolve(jsonResponse([]));
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 1, pipelines: 1, checkpoints: 1, missions: 0, operations: 0, proofRecords: 0, events: 0 },
          pipelineStatus: { "waiting-human": 1 },
          checkpointStatus: { pending: 1 },
          operationStatus: {},
          workItemStatus: { "In Review": 1 },
          attention: { waitingHuman: 1, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/llm-provider-selection")) return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      if (url.endsWith("/github/status")) return Promise.resolve(jsonResponse({ available: true, authenticated: true, output: "", account: "ZYOOO", oauthConfigured: false, oauthAuthenticated: false }));
      if (url.endsWith("/local-workspace-root")) return Promise.resolve(jsonResponse({ workspaceRoot: "/Users/zyong/Omega/workspaces" }));
      if (url.endsWith("/llm-providers") || url.endsWith("/agent-definitions") || url.endsWith("/local-capabilities") || url.endsWith("/pipeline-templates") || url.endsWith("/operations") || url.endsWith("/execution-locks")) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    fireEvent.click((await screen.findAllByText("Add registration page"))[0]);

    expect(await screen.findByRole("button", { name: "Approve delivery" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "https://github.com/ZYOOO/TestRepo/pull/18" })).toHaveAttribute("href", "https://github.com/ZYOOO/TestRepo/pull/18");
    expect(screen.getByText("Changed")).toBeInTheDocument();
    expect(screen.getAllByText("code-review-round-2-cycle-3.md").length).toBeGreaterThan(0);
    expect(screen.getByText("Validation")).toBeInTheDocument();
    expect(screen.getByText("Review approved the current diff.")).toBeInTheDocument();
    expect(await screen.findByText("Run timeline")).toBeInTheDocument();
    expect(screen.getByText("checkpoint.pending")).toBeInTheDocument();
    expect(await screen.findByText("PR lifecycle")).toBeInTheDocument();
    expect(screen.getByText("APPROVED")).toBeInTheDocument();
    expect(screen.getByText("SUCCESS")).toBeInTheDocument();
    expect(screen.getByLabelText("Human review comment")).toBeInTheDocument();
  });

  it("does not overwrite the Go workspace snapshot on initial load", async () => {
    vi.stubEnv("VITE_MISSION_CONTROL_API_URL", "http://127.0.0.1:3888");

    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/workspace") && !init) {
        return Promise.resolve(jsonResponse({
          schemaVersion: 1,
          tables: {
            workItems: [],
            missionControlStates: [],
            pipelines: [{ id: "pipeline_existing", status: "waiting-human" }],
            checkpoints: [{ id: "pipeline_existing:intake", status: "pending" }]
          }
        }));
      }
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({
          counts: { workItems: 0, pipelines: 1, checkpoints: 1, missions: 0, operations: 0, proofRecords: 0, events: 0 },
          pipelineStatus: { "waiting-human": 1 },
          checkpointStatus: { pending: 1 },
          operationStatus: {},
          workItemStatus: {},
          attention: { waitingHuman: 1, failed: 0, blocked: 0 }
        }));
      }
      if (url.endsWith("/llm-providers")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/llm-provider-selection")) {
        return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      }
      if (url.endsWith("/pipeline-templates")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/agent-definitions")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/local-capabilities")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (url.endsWith("/pipelines")) {
        return Promise.resolve(jsonResponse([{ id: "pipeline_existing", status: "waiting-human" }]));
      }
      if (url.endsWith("/checkpoints")) {
        return Promise.resolve(jsonResponse([{ id: "pipeline_existing:intake", status: "pending" }]));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    vi.stubGlobal("fetch", fetchMock);
    const { default: App } = await import("../App");
    render(<App />);

    fireEvent.click(screen.getByRole("button", { name: /Views/ }));
    await waitFor(() => expect(screen.getByText("Runtime model")).toBeInTheDocument());
    expect(fetchMock).not.toHaveBeenCalledWith(
      "http://127.0.0.1:3888/workspace",
      expect.objectContaining({ method: "PUT" })
    );
  });
});
