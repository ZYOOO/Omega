import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PagePilotPreview } from "../PagePilotPreview";

describe("PagePilotPreview", () => {
  afterEach(() => {
    cleanup();
    window.localStorage.clear();
    delete (window as Window & { omegaDesktop?: unknown }).omegaDesktop;
    vi.unstubAllGlobals();
  });

  it("asks the user to choose a repository workspace before editing", () => {
    const onSelectRepositoryTarget = vi.fn();
    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" },
          { id: "repo_local", kind: "local", path: "/Users/demo/App", defaultBranch: "main" }
        ]}
        repositoryTargetId=""
        repositoryLabel=""
        apiAvailable={true}
        onSelectRepositoryTarget={onSelectRepositoryTarget}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={vi.fn().mockResolvedValue([])}
      />
    );

    expect(screen.getByText("Choose a target repository")).toBeInTheDocument();
    const select = screen.getByLabelText("Target repo");
    fireEvent.change(select, { target: { value: "repo_local" } });

    expect(onSelectRepositoryTarget).toHaveBeenCalledWith("repo_local");
  });

  it("shows the selected repository workspace", () => {
    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" }
        ]}
        repositoryTargetId="repo_test"
        repositoryLabel="ZYOOO/TestRepo"
        apiAvailable={true}
        onSelectRepositoryTarget={vi.fn()}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={vi.fn().mockResolvedValue([])}
      />
    );

    expect(screen.getAllByText("ZYOOO/TestRepo").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("Target repo")).toHaveValue("repo_test");
    expect(screen.queryByRole("button", { name: "AI" })).not.toBeInTheDocument();
  });

  it("shows recent Page Pilot runs with Work Item links", async () => {
    const onFetchRuns = vi.fn().mockResolvedValue([
      {
        id: "page_pilot_1",
        status: "applied",
        repositoryTargetId: "repo_test",
        workItemId: "item_page_pilot_1",
        pipelineId: "pipeline_item_page_pilot_1",
        changedFiles: ["src/Page.tsx"],
        prPreview: { title: "Page change", body: "## Page Pilot change\n\nChanged src/Page.tsx" },
        visualProof: { kind: "dom-snapshot", annotationCount: 1 },
        updatedAt: "2026-04-30T12:20:00Z",
      }
    ]);

    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" }
        ]}
        repositoryTargetId="repo_test"
        repositoryLabel="ZYOOO/TestRepo"
        apiAvailable={true}
        onSelectRepositoryTarget={vi.fn()}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={onFetchRuns}
      />
    );

    expect(await screen.findByText("Waiting for confirmation")).toBeInTheDocument();
    expect(screen.getByText("pipeline_item_page_pilot_1")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Work Item" })).toHaveAttribute("href", "#/work-items/item_page_pilot_1");
    fireEvent.click(screen.getByRole("button", { name: "Details" }));
    expect(await screen.findByRole("dialog", { name: "Page Pilot run details" })).toBeInTheDocument();
    expect(screen.getByText("PR preview")).toBeInTheDocument();
    expect(screen.getByText("Visual proof")).toBeInTheDocument();
  });

  it("starts a dev server through the Preview Runtime Agent before opening", async () => {
    const startPreviewDevServer = vi.fn().mockResolvedValue({
      ok: true,
      previewUrl: "http://127.0.0.1:3009/",
      profile: { source: "npm:dev", devCommand: "npm run dev -- --host 127.0.0.1 --port 3009" },
    });
    const openPreview = vi.fn().mockResolvedValue({ ok: true, url: "http://127.0.0.1:3009/" });
    (window as Window & { omegaDesktop?: unknown }).omegaDesktop = {
      startPreviewDevServer,
      openPreview,
    };

    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" }
        ]}
        repositoryTargetId="repo_test"
        repositoryLabel="ZYOOO/TestRepo"
        apiAvailable={true}
        onSelectRepositoryTarget={vi.fn()}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={vi.fn().mockResolvedValue([])}
      />
    );

    fireEvent.change(screen.getByLabelText("Preview source"), { target: { value: "dev-server" } });
    fireEvent.change(screen.getByPlaceholderText("Optional: /login or launch note"), {
      target: { value: "/dashboard" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Open page editor" }));

    await waitFor(() => expect(startPreviewDevServer).toHaveBeenCalledWith({
      target: { id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" },
      projectId: "project_omega",
      repositoryTargetId: "repo_test",
      intent: "/dashboard",
    }));
    await waitFor(() => expect(openPreview).toHaveBeenCalledWith({
      url: "http://127.0.0.1:3009/dashboard",
      projectId: "project_omega",
      repositoryTargetId: "repo_test",
      repositoryLabel: "ZYOOO/TestRepo",
      returnUrl: "#page-pilot",
      previewRuntimeProfile: { source: "npm:dev", devCommand: "npm run dev -- --host 127.0.0.1 --port 3009" },
    }));
    expect(await screen.findByText("Target page opened. Select elements, add notes, and apply changes there.")).toBeInTheDocument();
  });

  it("surfaces Preview Runtime Agent failures instead of appearing idle", async () => {
    const startPreviewDevServer = vi.fn().mockResolvedValue({ ok: false, error: "no preview command could be detected" });
    const openPreview = vi.fn().mockResolvedValue({ ok: false, error: "ERR_CONNECTION_REFUSED" });
    (window as Window & { omegaDesktop?: unknown }).omegaDesktop = {
      startPreviewDevServer,
      openPreview,
    };

    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" }
        ]}
        repositoryTargetId="repo_test"
        repositoryLabel="ZYOOO/TestRepo"
        apiAvailable={true}
        onSelectRepositoryTarget={vi.fn()}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={vi.fn().mockResolvedValue([])}
      />
    );

    fireEvent.change(screen.getByLabelText("Preview source"), { target: { value: "dev-server" } });
    fireEvent.click(screen.getByRole("button", { name: "Open page editor" }));

    await waitFor(() => expect(startPreviewDevServer).toHaveBeenCalled());
    expect(openPreview).not.toHaveBeenCalled();
    expect(await screen.findByText("no preview command could be detected")).toBeInTheDocument();
  });

  it("opens a browser fallback preview when Electron is not available", async () => {
    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" }
        ]}
        repositoryTargetId="repo_test"
        repositoryLabel="ZYOOO/TestRepo"
        apiAvailable={true}
        onSelectRepositoryTarget={vi.fn()}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={vi.fn().mockResolvedValue([])}
      />
    );

    fireEvent.change(screen.getByLabelText("Preview source"), { target: { value: "dev-server" } });
    fireEvent.change(screen.getByPlaceholderText("Optional: /login or launch note"), {
      target: { value: "http://127.0.0.1:3009/dashboard?tab=customers#health" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Open page editor" }));

    const frame = await screen.findByTitle("Page Pilot target project preview");
    expect(frame).toHaveAttribute("src", "/page-pilot-target/dashboard?tab=customers#health");
    expect(screen.getByRole("button", { name: "AI" })).toBeInTheDocument();
    expect(await screen.findByText("Browser preview opened. Select elements in the embedded preview when it is inspectable.")).toBeInTheDocument();
  });

  it("uses the local runtime API for browser-mode HTML workspace previews", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      ok: true,
      status: "running",
      agentId: "preview-runtime-agent",
      stageId: "preview_runtime",
      repositoryTargetId: "repo_local",
      repositoryPath: "/Users/demo/App",
      previewUrl: "http://127.0.0.1:3009/",
      profile: {
        source: "static-index",
        previewUrl: "http://127.0.0.1:3009/",
        workingDirectory: "/Users/demo/App",
      },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { id: "repo_local", kind: "local", path: "/Users/demo/App", defaultBranch: "main" }
        ]}
        repositoryTargetId="repo_local"
        repositoryLabel="/Users/demo/App"
        apiAvailable={true}
        apiUrl="/api"
        onSelectRepositoryTarget={vi.fn()}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={vi.fn().mockResolvedValue([])}
      />
    );

    fireEvent.change(screen.getByLabelText("Preview source"), { target: { value: "html-file" } });
    fireEvent.change(screen.getByPlaceholderText("/Users/demo/app/index.html"), {
      target: { value: "http://127.0.0.1:3009/" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Open page editor" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("/api/page-pilot/preview-runtime/start", expect.objectContaining({
      method: "POST",
      body: JSON.stringify({
        projectId: "project_omega",
        repositoryTargetId: "repo_local",
        intent: "",
      }),
    })));
    const frame = await screen.findByTitle("Page Pilot target project preview");
    expect(frame).toHaveAttribute("src", "/page-pilot-target/");
  });

  it("derives a repository target id for older saved workspace sessions", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      ok: true,
      status: "running",
      repositoryTargetId: "repo_ZYOOO_TestRepo",
      previewUrl: "http://127.0.0.1:3009/",
      profile: { source: "static-index", previewUrl: "http://127.0.0.1:3009/" },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" } as never
        ]}
        repositoryTargetId=""
        repositoryLabel="ZYOOO/TestRepo"
        apiAvailable={true}
        apiUrl="/api"
        onSelectRepositoryTarget={vi.fn()}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={vi.fn().mockResolvedValue([])}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Open page editor" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("/api/page-pilot/preview-runtime/start", expect.objectContaining({
      body: JSON.stringify({
        projectId: "project_omega",
        repositoryTargetId: "repo_ZYOOO_TestRepo",
        intent: "",
      }),
    })));
  });

  it("treats a full dev-server URL as the Preview Runtime target URL", async () => {
    const startPreviewDevServer = vi.fn().mockResolvedValue({
      ok: true,
      previewUrl: "http://127.0.0.1:3009/",
      profile: { source: "external", previewUrl: "http://127.0.0.1:3009/" },
    });
    const openPreview = vi.fn().mockResolvedValue({ ok: true, url: "http://127.0.0.1:3009/dashboard?tab=customers#health" });
    (window as Window & { omegaDesktop?: unknown }).omegaDesktop = {
      startPreviewDevServer,
      openPreview,
    };

    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" }
        ]}
        repositoryTargetId="repo_test"
        repositoryLabel="ZYOOO/TestRepo"
        apiAvailable={true}
        onSelectRepositoryTarget={vi.fn()}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={vi.fn().mockResolvedValue([])}
      />
    );

    fireEvent.change(screen.getByLabelText("Preview source"), { target: { value: "dev-server" } });
    fireEvent.change(screen.getByPlaceholderText("Optional: /login or launch note"), {
      target: { value: "http://127.0.0.1:3009/dashboard?tab=customers#health" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Open page editor" }));

    await waitFor(() => expect(startPreviewDevServer).toHaveBeenCalledWith({
      target: { id: "repo_test", kind: "github", owner: "ZYOOO", repo: "TestRepo", defaultBranch: "main" },
      projectId: "project_omega",
      repositoryTargetId: "repo_test",
      intent: "/dashboard?tab=customers#health",
      previewUrl: "http://127.0.0.1:3009/",
    }));
    await waitFor(() => expect(openPreview).toHaveBeenCalledWith(expect.objectContaining({
      url: "http://127.0.0.1:3009/dashboard?tab=customers#health",
      repositoryTargetId: "repo_test",
    })));
  });

  it("can open a local HTML file through the Electron preview bridge", async () => {
    const openPreview = vi.fn().mockResolvedValue({ ok: true, url: "file:///Users/demo/App/index.html" });
    (window as Window & { omegaDesktop?: unknown }).omegaDesktop = {
      openPreview,
    };

    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { id: "repo_local", kind: "local", path: "/Users/demo/App", defaultBranch: "main" }
        ]}
        repositoryTargetId="repo_local"
        repositoryLabel="/Users/demo/App"
        apiAvailable={true}
        onSelectRepositoryTarget={vi.fn()}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={vi.fn().mockResolvedValue([])}
      />
    );

    fireEvent.change(screen.getByLabelText("Preview source"), { target: { value: "html-file" } });
    fireEvent.change(screen.getByPlaceholderText("/Users/demo/app/index.html"), {
      target: { value: "/Users/demo/App/index.html" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Open page editor" }));

    await waitFor(() => expect(openPreview).toHaveBeenCalledWith(expect.objectContaining({
      url: "file:///Users/demo/App/index.html",
      repositoryTargetId: "repo_local",
    })));
  });

  it("finds the selected workspace index.html when HTML file mode has no path", async () => {
    const resolvePreviewTarget = vi.fn().mockResolvedValue({
      ok: true,
      repoPath: "/Users/demo/App",
      htmlFile: "/Users/demo/App/index.html",
      hasPackageJson: false,
    });
    const openPreview = vi.fn().mockResolvedValue({ ok: true, url: "file:///Users/demo/App/index.html" });
    (window as Window & { omegaDesktop?: unknown }).omegaDesktop = {
      resolvePreviewTarget,
      openPreview,
    };

    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { id: "repo_local", kind: "local", path: "/Users/demo/App", defaultBranch: "main" }
        ]}
        repositoryTargetId="repo_local"
        repositoryLabel="/Users/demo/App"
        apiAvailable={true}
        onSelectRepositoryTarget={vi.fn()}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={vi.fn().mockResolvedValue([])}
      />
    );

    fireEvent.change(screen.getByLabelText("Preview source"), { target: { value: "html-file" } });
    fireEvent.click(screen.getByRole("button", { name: "Open page editor" }));

    await waitFor(() => expect(resolvePreviewTarget).toHaveBeenCalledWith(
      { id: "repo_local", kind: "local", path: "/Users/demo/App", defaultBranch: "main" }
    ));
    await waitFor(() => expect(openPreview).toHaveBeenCalledWith(expect.objectContaining({
      url: "file:///Users/demo/App/index.html",
      repositoryTargetId: "repo_local",
      previewRuntimeProfile: expect.objectContaining({
        workingDirectory: "/Users/demo/App",
        source: "html-file",
      }),
    })));
  });

  it("ignores a stale HTTP URL in HTML file mode and uses the workspace index.html", async () => {
    const resolvePreviewTarget = vi.fn().mockResolvedValue({
      ok: true,
      repoPath: "/Users/demo/App",
      htmlFile: "/Users/demo/App/index.html",
      hasPackageJson: false,
    });
    const openPreview = vi.fn().mockResolvedValue({ ok: true, url: "file:///Users/demo/App/index.html" });
    (window as Window & { omegaDesktop?: unknown }).omegaDesktop = {
      resolvePreviewTarget,
      openPreview,
    };

    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { id: "repo_local", kind: "local", path: "/Users/demo/App", defaultBranch: "main" }
        ]}
        repositoryTargetId="repo_local"
        repositoryLabel="/Users/demo/App"
        apiAvailable={true}
        onSelectRepositoryTarget={vi.fn()}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={vi.fn().mockResolvedValue([])}
      />
    );

    fireEvent.change(screen.getByLabelText("Preview source"), { target: { value: "html-file" } });
    fireEvent.change(screen.getByPlaceholderText("/Users/demo/app/index.html"), {
      target: { value: "http://127.0.0.1:3009/" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Open page editor" }));

    await waitFor(() => expect(resolvePreviewTarget).toHaveBeenCalledWith(
      { id: "repo_local", kind: "local", path: "/Users/demo/App", defaultBranch: "main" }
    ));
    await waitFor(() => expect(openPreview).toHaveBeenCalledWith(expect.objectContaining({
      url: "file:///Users/demo/App/index.html",
      repositoryTargetId: "repo_local",
    })));
  });

  it("resolves the selected repository source and opens page editing", async () => {
    const resolvePreviewTarget = vi.fn().mockResolvedValue({
      ok: true,
      repoPath: "/Users/demo/App",
      htmlFile: "/Users/demo/App/index.html",
    });
    const openPreview = vi.fn().mockResolvedValue({ ok: true, url: "file:///Users/demo/App/index.html" });
    (window as Window & { omegaDesktop?: unknown }).omegaDesktop = {
      resolvePreviewTarget,
      openPreview,
    };

    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { id: "repo_local", kind: "local", path: "/Users/demo/App", defaultBranch: "main" }
        ]}
        repositoryTargetId="repo_local"
        repositoryLabel="/Users/demo/App"
        apiAvailable={true}
        onSelectRepositoryTarget={vi.fn()}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={vi.fn().mockResolvedValue([])}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Open page editor" }));

    await waitFor(() => expect(resolvePreviewTarget).toHaveBeenCalledWith(
      { id: "repo_local", kind: "local", path: "/Users/demo/App", defaultBranch: "main" }
    ));
    await waitFor(() => expect(openPreview).toHaveBeenCalledWith(expect.objectContaining({
      url: "file:///Users/demo/App/index.html",
      repositoryTargetId: "repo_local",
    })));
  });

  it("starts the Preview Runtime Agent for repository source projects with package.json", async () => {
    const resolvePreviewTarget = vi.fn().mockResolvedValue({
      ok: true,
      repoPath: "/Users/demo/App",
      htmlFile: "/Users/demo/App/index.html",
      hasPackageJson: true,
    });
    const startPreviewDevServer = vi.fn().mockResolvedValue({
      ok: true,
      previewUrl: "http://127.0.0.1:3010/",
      profile: { source: "npm:dev", previewUrl: "http://127.0.0.1:3010/" },
    });
    const openPreview = vi.fn().mockResolvedValue({ ok: true, url: "http://127.0.0.1:3010/" });
    (window as Window & { omegaDesktop?: unknown }).omegaDesktop = {
      resolvePreviewTarget,
      startPreviewDevServer,
      openPreview,
    };

    render(
      <PagePilotPreview
        projectId="project_omega"
        repositoryTargets={[
          { id: "repo_local", kind: "local", path: "/Users/demo/App", defaultBranch: "main" }
        ]}
        repositoryTargetId="repo_local"
        repositoryLabel="/Users/demo/App"
        apiAvailable={true}
        onSelectRepositoryTarget={vi.fn()}
        onApply={vi.fn()}
        onDeliver={vi.fn()}
        onDiscard={vi.fn()}
        onFetchRuns={vi.fn().mockResolvedValue([])}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Open page editor" }));

    await waitFor(() => expect(resolvePreviewTarget).toHaveBeenCalledWith(
      { id: "repo_local", kind: "local", path: "/Users/demo/App", defaultBranch: "main" }
    ));
    await waitFor(() => expect(startPreviewDevServer).toHaveBeenCalledWith({
      target: { id: "repo_local", kind: "local", path: "/Users/demo/App", defaultBranch: "main" },
      projectId: "project_omega",
      repositoryTargetId: "repo_local",
      intent: "",
    }));
    await waitFor(() => expect(openPreview).toHaveBeenCalledWith(expect.objectContaining({
      url: "http://127.0.0.1:3010/",
      repositoryTargetId: "repo_local",
      previewRuntimeProfile: { source: "npm:dev", previewUrl: "http://127.0.0.1:3010/" },
    })));
  });
});
