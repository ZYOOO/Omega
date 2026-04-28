import { describe, expect, it, vi } from "vitest";
import {
  applyPagePilotInstruction,
  approveCheckpoint,
  createPipelineFromTemplate,
  createGitHubPullRequest,
  deliverPagePilotChange,
  decomposeRequirement,
  discardPagePilotRun,
  fetchCheckpoints,
  fetchAgentDefinitions,
  fetchGitHubRepoInfo,
  fetchGitHubPullRequestStatus,
  fetchGitHubOAuthConfig,
  fetchGitHubRepositories,
  fetchGitHubStatus,
  fetchLocalCapabilities,
  fetchLlmProviderSelection,
  fetchLlmProviders,
  fetchObservability,
  fetchPagePilotRuns,
  fetchPipelines,
  fetchPipelineTemplates,
  requestCheckpointChanges,
  runCurrentPipelineStage,
  sendFeishuNotification,
  startGitHubCliLogin,
  startGitHubOAuth,
  startPipeline,
  updateGitHubOAuthConfig,
  updateLlmProviderSelection
} from "../omegaControlApiClient";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" }
  });
}

describe("omegaControlApiClient", () => {
  it("loads control-plane summaries and configuration", async () => {
    const fetchImpl = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/observability")) {
        return Promise.resolve(jsonResponse({ counts: { pipelines: 1 }, attention: { waitingHuman: 1 } }));
      }
      if (url.endsWith("/llm-providers")) {
        return Promise.resolve(jsonResponse([{ id: "openai", models: ["gpt-5.4-mini"] }]));
      }
      if (url.endsWith("/llm-provider-selection")) {
        return Promise.resolve(jsonResponse({ providerId: "openai", model: "gpt-5.4-mini", reasoningEffort: "medium" }));
      }
      if (url.endsWith("/pipeline-templates")) {
        return Promise.resolve(jsonResponse([{ id: "feature", stages: [] }]));
      }
      if (url.endsWith("/agent-definitions")) {
        return Promise.resolve(jsonResponse([{ id: "requirement", inputContract: [], outputContract: [] }]));
      }
      if (url.endsWith("/local-capabilities")) {
        return Promise.resolve(jsonResponse([{ id: "git", available: true }]));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    await expect(fetchObservability("http://omega.local", fetchImpl)).resolves.toMatchObject({ counts: { pipelines: 1 } });
    await expect(fetchLlmProviders("http://omega.local", fetchImpl)).resolves.toHaveLength(1);
    await expect(fetchLlmProviderSelection("http://omega.local", fetchImpl)).resolves.toMatchObject({ providerId: "openai" });
    await expect(fetchPipelineTemplates("http://omega.local", fetchImpl)).resolves.toHaveLength(1);
    await expect(fetchAgentDefinitions("http://omega.local", fetchImpl)).resolves.toHaveLength(1);
    await expect(fetchLocalCapabilities("http://omega.local", fetchImpl)).resolves.toMatchObject([{ id: "git", available: true }]);
  });

  it("persists provider selection", async () => {
    const fetchImpl = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      expect(init?.method).toBe("PUT");
      expect(JSON.parse(String(init?.body))).toMatchObject({ providerId: "openai-compatible" });
      return Promise.resolve(jsonResponse({ providerId: "openai-compatible", model: "qwen-plus", reasoningEffort: "medium" }));
    }) as unknown as typeof fetch;

    await expect(
      updateLlmProviderSelection(
        "http://omega.local",
        { providerId: "openai-compatible", model: "qwen-plus", reasoningEffort: "medium" },
        fetchImpl
      )
    ).resolves.toMatchObject({ model: "qwen-plus" });
  });

  it("sends Feishu notifications through the local service", async () => {
    const fetchImpl = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      expect(init?.method).toBe("POST");
      expect(JSON.parse(String(init?.body))).toMatchObject({
        chatId: "oc_demo",
        text: "Pipeline waiting for review"
      });
      return Promise.resolve(jsonResponse({ status: "sent", messageId: "om_123" }));
    }) as unknown as typeof fetch;

    await expect(
      sendFeishuNotification("http://omega.local", "oc_demo", "Pipeline waiting for review", fetchImpl)
    ).resolves.toMatchObject({ status: "sent", messageId: "om_123" });
  });

  it("sends Page Pilot apply and delivery requests to the local runtime", async () => {
    const selection = {
      elementKind: "title",
      stableSelector: `[data-omega-source="apps/web/src/components/PortalHome.tsx:headline"]`,
      textSnapshot: "Old headline",
      styleSnapshot: { fontSize: "32px" },
      domContext: { tagName: "h1" },
      sourceMapping: {
        source: "apps/web/src/components/PortalHome.tsx:headline",
        file: "apps/web/src/components/PortalHome.tsx",
        symbol: "headline"
      }
    };
    const fetchImpl = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).endsWith("/page-pilot/apply")) {
        expect(init?.method).toBe("POST");
        const body = JSON.parse(String(init?.body));
        expect(body.repositoryTargetId).toBe("repo_ZYOOO_Omega");
        expect(body.selection.sourceMapping.symbol).toBe("headline");
        return Promise.resolve(jsonResponse({ id: "page_pilot_1", status: "applied", changedFiles: ["apps/web/src/components/PortalHome.tsx"] }));
      }
      if (String(input).endsWith("/page-pilot/runs")) {
        expect(init).toBeUndefined();
        return Promise.resolve(jsonResponse([{ id: "page_pilot_1", status: "applied", changedFiles: ["apps/web/src/components/PortalHome.tsx"] }]));
      }
      if (String(input).endsWith("/page-pilot/runs/page_pilot_1/discard")) {
        expect(init?.method).toBe("POST");
        return Promise.resolve(jsonResponse({ id: "page_pilot_1", status: "discarded", changedFiles: ["apps/web/src/components/PortalHome.tsx"] }));
      }
      expect(init?.method).toBe("POST");
      const body = JSON.parse(String(init?.body));
      expect(body.repositoryTargetId).toBe("repo_ZYOOO_Omega");
      expect(body.selection.sourceMapping.symbol).toBe("headline");
      return Promise.resolve(jsonResponse({ status: "delivered", branchName: "omega/page-pilot-headline", changedFiles: ["apps/web/src/components/PortalHome.tsx"] }));
    }) as unknown as typeof fetch;

    await expect(
      applyPagePilotInstruction("http://omega.local", {
        projectId: "project_omega",
        repositoryTargetId: "repo_ZYOOO_Omega",
        instruction: "Make the headline shorter",
        selection
      }, fetchImpl)
    ).resolves.toMatchObject({ id: "page_pilot_1", status: "applied" });

    await expect(
      deliverPagePilotChange("http://omega.local", {
        runId: "page_pilot_1",
        projectId: "project_omega",
        repositoryTargetId: "repo_ZYOOO_Omega",
        instruction: "Make the headline shorter",
        selection
      }, fetchImpl)
    ).resolves.toMatchObject({ status: "delivered" });

    await expect(fetchPagePilotRuns("http://omega.local", fetchImpl)).resolves.toMatchObject([
      { id: "page_pilot_1", status: "applied" }
    ]);
    await expect(discardPagePilotRun("http://omega.local", "page_pilot_1", fetchImpl)).resolves.toMatchObject({
      status: "discarded"
    });
  });

  it("starts GitHub OAuth through the local control plane", async () => {
    const fetchImpl = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      expect(String(input)).toBe("http://omega.local/github/oauth/start");
      expect(init?.method).toBe("POST");
      return Promise.resolve(jsonResponse({
        configured: true,
        authorizeUrl: "https://github.com/login/oauth/authorize?client_id=omega",
        state: "state_123",
        redirectUri: "http://127.0.0.1:3888/auth/github/callback",
        scopes: ["repo", "read:org", "workflow"]
      }));
    }) as unknown as typeof fetch;

    await expect(startGitHubOAuth("http://omega.local", fetchImpl)).resolves.toMatchObject({
      configured: true,
      state: "state_123"
    });
  });

  it("starts GitHub CLI sign-in through the local control plane", async () => {
    const fetchImpl = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      expect(String(input)).toBe("http://omega.local/github/cli-login/start");
      expect(init?.method).toBe("POST");
      return Promise.resolve(jsonResponse({
        started: true,
        method: "gh-cli",
        message: "GitHub CLI sign-in opened.",
        command: "gh auth login --web",
        verificationUrl: "https://github.com/login/device"
      }));
    }) as unknown as typeof fetch;

    await expect(startGitHubCliLogin("http://omega.local", fetchImpl)).resolves.toMatchObject({
      started: true,
      method: "gh-cli",
      verificationUrl: "https://github.com/login/device"
    });
  });

  it("reads GitHub CLI authentication status through the local control plane", async () => {
    const fetchImpl = vi.fn((input: RequestInfo | URL) => {
      expect(String(input)).toBe("http://omega.local/github/status");
      return Promise.resolve(jsonResponse({
        available: true,
        authenticated: true,
        account: "ZYOOO",
        output: "logged in",
        oauthConfigured: false,
        oauthAuthenticated: false
      }));
    }) as unknown as typeof fetch;

    await expect(fetchGitHubStatus("http://omega.local", fetchImpl)).resolves.toMatchObject({
      authenticated: true,
      account: "ZYOOO"
    });
  });

  it("reads and saves GitHub OAuth config through the local control plane", async () => {
    const fetchImpl = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      expect(url).toBe("http://omega.local/github/oauth/config");
      if (!init) {
        return Promise.resolve(jsonResponse({
          configured: false,
          clientId: "",
          redirectUri: "http://127.0.0.1:3888/auth/github/callback",
          tokenUrl: "https://github.com/login/oauth/access_token",
          secretConfigured: false,
          source: "empty"
        }));
      }
      expect(init.method).toBe("PUT");
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
    }) as unknown as typeof fetch;

    await expect(fetchGitHubOAuthConfig("http://omega.local", fetchImpl)).resolves.toMatchObject({
      configured: false,
      secretConfigured: false
    });
    await expect(
      updateGitHubOAuthConfig(
        "http://omega.local",
        {
          clientId: "omega-client",
          clientSecret: "omega-secret",
          redirectUri: "http://127.0.0.1:3888/auth/github/callback"
        },
        fetchImpl
      )
    ).resolves.toMatchObject({ configured: true, clientId: "omega-client", secretConfigured: true });
  });

  it("reads GitHub repository metadata through the local control plane", async () => {
    const fetchImpl = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      expect(String(input)).toBe("http://omega.local/github/repo-info");
      expect(init?.method).toBe("POST");
      expect(JSON.parse(String(init?.body))).toMatchObject({ owner: "acme", repo: "demo" });
      return Promise.resolve(jsonResponse({
        name: "demo",
        owner: { login: "acme" },
        description: "Demo repo",
        url: "https://github.com/acme/demo",
        isPrivate: false,
        defaultBranchRef: { name: "main" }
      }));
    }) as unknown as typeof fetch;

    await expect(fetchGitHubRepoInfo("http://omega.local", "acme", "demo", fetchImpl)).resolves.toMatchObject({
      name: "demo",
      defaultBranchRef: { name: "main" }
    });
  });

  it("lists GitHub repositories through the local control plane", async () => {
    const fetchImpl = vi.fn((input: RequestInfo | URL) => {
      expect(String(input)).toBe("http://omega.local/github/repositories");
      return Promise.resolve(jsonResponse([
        {
          name: "demo",
          nameWithOwner: "acme/demo",
          owner: { login: "acme" },
          description: "Demo repo",
          isPrivate: false,
          defaultBranchRef: { name: "main" }
        }
      ]));
    }) as unknown as typeof fetch;

    await expect(fetchGitHubRepositories("http://omega.local", fetchImpl)).resolves.toMatchObject([
      { nameWithOwner: "acme/demo", defaultBranchRef: { name: "main" } }
    ]);
  });

  it("creates GitHub pull requests through the local control plane", async () => {
    const fetchImpl = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      expect(String(input)).toBe("http://omega.local/github/create-pr");
      expect(init?.method).toBe("POST");
      expect(JSON.parse(String(init?.body))).toMatchObject({
        workspacePath: "/tmp/omega/OMG-1-coding",
        title: "Omega delivery for OMG-1",
        branchName: "omega/OMG-1-coding",
        draft: true
      });
      return Promise.resolve(jsonResponse({
        status: "created",
        url: "https://github.com/acme/demo/pull/12",
        branchName: "omega/OMG-1-coding"
      }));
    }) as unknown as typeof fetch;

    await expect(
      createGitHubPullRequest(
        "http://omega.local",
        {
          workspacePath: "/tmp/omega/OMG-1-coding",
          title: "Omega delivery for OMG-1",
          branchName: "omega/OMG-1-coding",
          draft: true
        },
        fetchImpl
      )
    ).resolves.toMatchObject({ status: "created", url: "https://github.com/acme/demo/pull/12" });
  });

  it("reads GitHub pull request status and checks through the local control plane", async () => {
    const fetchImpl = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      expect(String(input)).toBe("http://omega.local/github/pr-status");
      expect(init?.method).toBe("POST");
      expect(JSON.parse(String(init?.body))).toMatchObject({
        repositoryOwner: "acme",
        repositoryName: "demo",
        number: 12
      });
      return Promise.resolve(jsonResponse({
        number: 12,
        state: "OPEN",
        reviewDecision: "APPROVED",
        deliveryGate: "pending",
        checks: [{ name: "lint", state: "SUCCESS" }, { name: "test", state: "PENDING" }],
        proofRecords: [{ label: "pull-request" }, { label: "check" }, { label: "check" }]
      }));
    }) as unknown as typeof fetch;

    await expect(
      fetchGitHubPullRequestStatus(
        "http://omega.local",
        { repositoryOwner: "acme", repositoryName: "demo", number: 12 },
        fetchImpl
      )
    ).resolves.toMatchObject({ deliveryGate: "pending", checks: [{ name: "lint" }, { name: "test" }] });
  });

  it("decomposes raw requirements through the local control plane", async () => {
    const fetchImpl = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      expect(String(input)).toBe("http://omega.local/requirements/decompose");
      expect(init?.method).toBe("POST");
      expect(JSON.parse(String(init?.body))).toMatchObject({
        title: "Add GitHub PR delivery",
        repositoryTarget: "/Users/demo/Omega"
      });
      return Promise.resolve(jsonResponse({
        summary: "Add GitHub PR delivery",
        repositoryTarget: "/Users/demo/Omega",
        acceptanceCriteria: ["PR is created", "Checks are visible"],
        risks: ["GitHub permissions may block delivery"],
        suggestedWorkItems: [{ stageId: "intake" }, { stageId: "solution" }]
      }));
    }) as unknown as typeof fetch;

    await expect(
      decomposeRequirement(
        "http://omega.local",
        {
          title: "Add GitHub PR delivery",
          description: "Create a PR and show checks.",
          repositoryTarget: "/Users/demo/Omega"
        },
        fetchImpl
      )
    ).resolves.toMatchObject({ summary: "Add GitHub PR delivery", suggestedWorkItems: [{ stageId: "intake" }, { stageId: "solution" }] });
  });

  it("drives pipeline and checkpoint operations", async () => {
    const fetchImpl = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/pipelines")) {
        return Promise.resolve(jsonResponse([{ id: "pipeline_item_1", status: "ready" }]));
      }
      if (url.endsWith("/checkpoints")) {
        return Promise.resolve(jsonResponse([{ id: "pipeline_item_1:intake", status: "pending" }]));
      }
      if (url.endsWith("/pipelines/from-template")) {
        expect(init?.method).toBe("POST");
        expect(JSON.parse(String(init?.body))).toMatchObject({ templateId: "feature" });
        return Promise.resolve(jsonResponse({ id: "pipeline_item_1", templateId: "feature" }));
      }
      if (url.endsWith("/pipelines/pipeline_item_1/start")) {
        expect(init?.method).toBe("POST");
        return Promise.resolve(jsonResponse({ id: "pipeline_item_1", status: "running" }));
      }
      if (url.endsWith("/pipelines/pipeline_item_1/run-current-stage")) {
        expect(init?.method).toBe("POST");
        expect(JSON.parse(String(init?.body))).toMatchObject({ runner: "local-proof" });
        return Promise.resolve(jsonResponse({ pipeline: { id: "pipeline_item_1", status: "waiting-human" }, operationResult: { status: "passed" } }));
      }
      if (url.endsWith("/checkpoints/pipeline_item_1:intake/approve")) {
        expect(JSON.parse(String(init?.body))).toMatchObject({ reviewer: "human" });
        return Promise.resolve(jsonResponse({ id: "pipeline_item_1:intake", status: "approved" }));
      }
      if (url.endsWith("/checkpoints/pipeline_item_1:intake/request-changes")) {
        expect(JSON.parse(String(init?.body))).toMatchObject({ reason: "Needs clearer acceptance criteria" });
        return Promise.resolve(jsonResponse({ id: "pipeline_item_1:intake", status: "rejected" }));
      }
      return Promise.resolve(jsonResponse({}, 404));
    }) as unknown as typeof fetch;

    await expect(fetchPipelines("http://omega.local", fetchImpl)).resolves.toHaveLength(1);
    await expect(fetchCheckpoints("http://omega.local", fetchImpl)).resolves.toHaveLength(1);
    await expect(createPipelineFromTemplate("http://omega.local", "feature", { id: "item_1" }, fetchImpl)).resolves.toMatchObject({ templateId: "feature" });
    await expect(startPipeline("http://omega.local", "pipeline_item_1", fetchImpl)).resolves.toMatchObject({ status: "running" });
    await expect(runCurrentPipelineStage("http://omega.local", "pipeline_item_1", "local-proof", fetchImpl)).resolves.toMatchObject({
      pipeline: { status: "waiting-human" },
      operationResult: { status: "passed" }
    });
    await expect(approveCheckpoint("http://omega.local", "pipeline_item_1:intake", "human", fetchImpl)).resolves.toMatchObject({ status: "approved" });
    await expect(
      requestCheckpointChanges("http://omega.local", "pipeline_item_1:intake", "Needs clearer acceptance criteria", fetchImpl)
    ).resolves.toMatchObject({ status: "rejected" });
  });
});
