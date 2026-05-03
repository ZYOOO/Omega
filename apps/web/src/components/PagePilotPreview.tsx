import { useEffect, useState } from "react";
import type { RepositoryTarget } from "../core";
import type { PagePilotApplyResult, PagePilotDeliverResult, PagePilotRunInfo, PagePilotSelectionContext } from "../omegaControlApiClient";
import { workItemDetailHash } from "../workItemRoutes";

type PagePilotPreviewProps = {
  projectId?: string;
  repositoryTargets: RepositoryTarget[];
  repositoryTargetId?: string;
  repositoryLabel?: string;
  apiAvailable: boolean;
  onReloadApp?: () => void;
  onSelectRepositoryTarget: (repositoryTargetId: string) => void;
  onApply: (instruction: string, selection: PagePilotSelectionContext) => Promise<PagePilotApplyResult>;
  onDeliver: (instruction: string, selection: PagePilotSelectionContext, runId?: string) => Promise<PagePilotDeliverResult>;
  onDiscard: (runId: string) => Promise<{ status: string; lineDiffSummary?: string }>;
  onFetchRuns: () => Promise<PagePilotRunInfo[]>;
  onExit?: () => void;
};

const previewUrlStorageKey = "omega-page-pilot-preview-url";
const previewModeStorageKey = "omega-page-pilot-preview-mode";

type PreviewMode = "repo-source" | "dev-server" | "html-file";

type PreviewRuntimeProfile = {
  agentId?: string;
  stageId?: string;
  repositoryTargetId?: string;
  workingDirectory?: string;
  devCommand?: string;
  previewUrl?: string;
  reloadStrategy?: string;
  source?: string;
  evidence?: string[];
  healthCheck?: Record<string, unknown>;
  responsibilities?: string[];
  createdAt?: string;
};

type PreviewLaunchTarget = {
  url: string;
  profile?: PreviewRuntimeProfile | null;
};

type OmegaDesktopBridge = {
  resolvePreviewTarget?: (target: RepositoryTarget) => Promise<{
    ok: boolean;
    error?: string;
    repoPath?: string;
    htmlFile?: string;
    previewUrl?: string;
    hasPackageJson?: boolean;
  }>;
  startPreviewDevServer?: (input: {
    target: RepositoryTarget;
    projectId?: string;
    repositoryTargetId?: string;
    intent?: string;
  }) => Promise<{
    ok: boolean;
    error?: string;
    status?: string;
    repoPath?: string;
    previewUrl?: string;
    profile?: PreviewRuntimeProfile;
  }>;
  openPreview?: (input: string | {
    url: string;
    projectId?: string;
    repositoryTargetId?: string;
    repositoryLabel?: string;
    returnUrl?: string;
    previewRuntimeProfile?: PreviewRuntimeProfile;
  }) => Promise<{ ok: boolean; error?: string; url?: string }>;
};

function initialPreviewUrl() {
  if (typeof window === "undefined") return "";
  return window.localStorage.getItem(previewUrlStorageKey) ?? "";
}

function initialPreviewMode(): PreviewMode {
  if (typeof window === "undefined") return "repo-source";
  const saved = window.localStorage.getItem(previewModeStorageKey);
  return saved === "dev-server" || saved === "html-file" ? saved : "repo-source";
}

function omegaDesktopBridge(): OmegaDesktopBridge | undefined {
  if (typeof window === "undefined") return undefined;
  return (window as Window & { omegaDesktop?: OmegaDesktopBridge }).omegaDesktop;
}

function normalizePreviewUrl(value: string, mode: PreviewMode) {
  let trimmed = value.trim();
  if (!trimmed || typeof window === "undefined") return trimmed;
  if (mode === "html-file") return fileUrlFor(trimmed);
  if (/^(localhost|127\.0\.0\.1|\[::1\])(?::\d+)?(?:\/.*)?$/i.test(trimmed)) {
    trimmed = `http://${trimmed}`;
  }
  try {
    return new URL(trimmed, window.location.href).href;
  } catch {
    return trimmed;
  }
}

function fileUrlFor(value: string) {
  if (value.startsWith("file://")) return value;
  if (value.startsWith("/")) return `file://${value}`;
  return value;
}

function withPreviewIntent(previewUrl: string, intent: string) {
  const trimmed = intent.trim();
  if (!trimmed || !trimmed.startsWith("/")) return previewUrl;
  try {
    const url = new URL(previewUrl);
    url.pathname = trimmed;
    return url.href;
  } catch {
    return previewUrl;
  }
}

export function PagePilotPreview({
  projectId,
  repositoryTargets,
  repositoryTargetId,
  repositoryLabel,
  apiAvailable,
  onSelectRepositoryTarget,
  onFetchRuns,
}: PagePilotPreviewProps) {
  const [previewMode, setPreviewMode] = useState<PreviewMode>(initialPreviewMode);
  const [draftUrl, setDraftUrl] = useState(initialPreviewUrl);
  const [status, setStatus] = useState("");
  const [runs, setRuns] = useState<PagePilotRunInfo[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsError, setRunsError] = useState("");
  const [selectedRun, setSelectedRun] = useState<PagePilotRunInfo | null>(null);
  const [activePreviewRuntimeProfile, setActivePreviewRuntimeProfile] = useState<PreviewRuntimeProfile | null>(null);
  const desktopBridge = omegaDesktopBridge();
  const selectedRepositoryTarget = repositoryTargetId
    ? repositoryTargets.find((target) => target.id === repositoryTargetId)
    : undefined;
  const selectedRepositoryLabel = selectedRepositoryTarget
    ? repositoryTargetLabel(selectedRepositoryTarget)
    : repositoryLabel || "";

  useEffect(() => {
    window.localStorage.setItem(previewModeStorageKey, previewMode);
  }, [previewMode]);

  useEffect(() => {
    if (draftUrl.trim()) window.localStorage.setItem(previewUrlStorageKey, draftUrl);
  }, [draftUrl]);

  useEffect(() => {
    let canceled = false;
    if (!apiAvailable || !repositoryTargetId) {
      setRuns([]);
      return () => {
        canceled = true;
      };
    }
    setRunsLoading(true);
    setRunsError("");
    onFetchRuns()
      .then((nextRuns) => {
        if (!canceled) setRuns(filterRunsForRepository(nextRuns, repositoryTargetId).slice(0, 8));
      })
      .catch((error) => {
        if (!canceled) setRunsError(error instanceof Error ? error.message : String(error));
      })
      .finally(() => {
        if (!canceled) setRunsLoading(false);
      });
    return () => {
      canceled = true;
    };
  }, [apiAvailable, repositoryTargetId]);

  async function resolveSelectedRepoPreview() {
    if (!selectedRepositoryTarget) {
      setStatus("Choose a Repository Workspace first.");
      return { url: "" };
    }
    if (!desktopBridge?.resolvePreviewTarget) {
      setStatus("Electron is required to prepare an isolated preview workspace.");
      return { url: "" };
    }
    setStatus("Preparing isolated preview workspace...");
    const result = await desktopBridge.resolvePreviewTarget(selectedRepositoryTarget);
    if (!result.ok) {
      setStatus(result.error ?? "Could not prepare the target repository preview.");
      return { url: "" };
    }
    if (result.htmlFile) {
      const fileUrl = fileUrlFor(result.htmlFile);
      const profile: PreviewRuntimeProfile = {
        agentId: "preview-runtime-agent",
        stageId: "preview_runtime",
        repositoryTargetId,
        workingDirectory: result.repoPath,
        previewUrl: fileUrl,
        source: "repository-html",
        reloadStrategy: "browser-reload",
        evidence: result.htmlFile ? ["index.html"] : [],
        createdAt: new Date().toISOString(),
      };
      setDraftUrl(result.htmlFile);
      setActivePreviewRuntimeProfile(profile);
      setStatus(`HTML preview ready: ${result.repoPath ?? result.htmlFile}`);
      return { url: fileUrl, profile };
    }
    if (result.previewUrl) {
      const profile: PreviewRuntimeProfile = {
        agentId: "preview-runtime-agent",
        stageId: "preview_runtime",
        repositoryTargetId,
        workingDirectory: result.repoPath,
        previewUrl: result.previewUrl,
        source: result.hasPackageJson ? "repository-dev-server" : "repository-preview-url",
        reloadStrategy: result.hasPackageJson ? "hmr-wait" : "browser-reload",
        createdAt: new Date().toISOString(),
      };
      setDraftUrl(result.previewUrl);
      setActivePreviewRuntimeProfile(profile);
      setStatus(`Dev server preview ready: ${result.previewUrl}`);
      return { url: result.previewUrl, profile };
    }
    setActivePreviewRuntimeProfile(null);
    setStatus(`Repository ready: ${result.repoPath ?? ""}. Enter a preview URL or HTML file.`);
    return { url: "" };
  }

  async function startDevServerPreview() {
    if (!selectedRepositoryTarget) {
      setStatus("Choose a Repository Workspace first.");
      return { url: "" };
    }
    if (!desktopBridge?.startPreviewDevServer) {
      setStatus("This desktop shell does not support the Preview Runtime Agent yet.");
      return { url: "" };
    }
    const intent = draftUrl.trim();
    setStatus("Preview Runtime Agent is starting the dev server...");
    const result = await desktopBridge.startPreviewDevServer({
      target: selectedRepositoryTarget,
      projectId,
      repositoryTargetId,
      intent,
    });
    if (!result.ok || !result.previewUrl) {
      setStatus(result.error ?? "Preview Runtime Agent could not start the dev server.");
      return { url: "" };
    }
    const nextUrl = withPreviewIntent(result.previewUrl, intent);
    setDraftUrl(result.previewUrl);
    setActivePreviewRuntimeProfile(result.profile ?? null);
    const source = result.profile?.source ? ` (${result.profile.source})` : "";
    setStatus(`Preview Runtime Agent started${source}: ${result.previewUrl}`);
    return { url: nextUrl, profile: result.profile ?? null };
  }

  async function openDirectPilot() {
    if (!selectedRepositoryTarget || !repositoryTargetId) {
      setStatus("Choose a Repository Workspace first. Page Pilot changes must be locked to one target repository.");
      return;
    }
    if (!desktopBridge?.openPreview) {
      setStatus("Page editing requires the Electron desktop shell.");
      return;
    }
    let launchTarget: PreviewLaunchTarget = { url: "", profile: activePreviewRuntimeProfile };
    try {
      if (previewMode === "repo-source") {
        launchTarget = await resolveSelectedRepoPreview();
      } else if (previewMode === "dev-server") {
        launchTarget = await startDevServerPreview();
      } else {
        const profile: PreviewRuntimeProfile = {
          agentId: "preview-runtime-agent",
          stageId: "preview_runtime",
          repositoryTargetId,
          previewUrl: normalizePreviewUrl(draftUrl, previewMode),
          source: "html-file",
          reloadStrategy: "browser-reload",
          createdAt: new Date().toISOString(),
        };
        launchTarget = { url: profile.previewUrl ?? "", profile };
        setActivePreviewRuntimeProfile(profile);
      }
      if (!launchTarget.url) {
        if (previewMode !== "dev-server") {
          setStatus(previewMode === "html-file" ? "Enter a local HTML file path." : "Preview Runtime Agent must start the target project first.");
        }
        return;
      }
      setStatus("Opening target page...");
      const result = await desktopBridge.openPreview({
        url: launchTarget.url,
        projectId,
        repositoryTargetId,
        repositoryLabel: selectedRepositoryLabel,
        returnUrl: "#page-pilot",
        previewRuntimeProfile: launchTarget.profile ?? undefined,
      });
      setStatus(result.ok ? "Target page opened. Select elements, add notes, and apply changes there." : result.error ?? "Could not open Page Pilot.");
    } catch (error) {
      setStatus(error instanceof Error ? error.message : String(error));
    }
  }

  return (
    <section className="page-pilot-surface page-pilot-launcher">
      <section className="page-pilot-repository-picker" aria-label="Page Pilot repository workspace">
        <div>
          <span className="section-label">Repository Workspace</span>
          <strong>{selectedRepositoryLabel || "Choose a target repository"}</strong>
          <small>Page Pilot will edit the preview workspace for this repository only.</small>
        </div>
        <label>
          <span>Target repo</span>
          <select
            value={repositoryTargetId ?? ""}
            onChange={(event) => {
              onSelectRepositoryTarget(event.currentTarget.value);
              setStatus("");
            }}
          >
            <option value="">Select repository...</option>
            {repositoryTargets.map((target) => (
              <option key={target.id} value={target.id}>
                {repositoryTargetLabel(target)}
              </option>
            ))}
          </select>
        </label>
      </section>

      <section className="page-pilot-launch-panel">
        <span className="section-label">Preview source</span>
        <form
          className="page-pilot-url-form"
          onSubmit={(event) => {
            event.preventDefault();
            void openDirectPilot();
          }}
        >
          <select
            className="page-pilot-preview-mode"
            value={previewMode}
            onChange={(event) => {
              const nextMode = event.currentTarget.value as PreviewMode;
              setPreviewMode(nextMode);
              setStatus("");
            }}
            aria-label="Preview source"
          >
            <option value="repo-source">Repository source</option>
            <option value="dev-server">Dev server by Agent</option>
            <option value="html-file">HTML file</option>
          </select>
          <input
            value={draftUrl}
            onChange={(event) => setDraftUrl(event.currentTarget.value)}
            placeholder={previewMode === "html-file" ? "/Users/demo/app/index.html" : previewMode === "dev-server" ? "Optional: /login or launch note" : "Resolved from selected repository"}
            disabled={previewMode === "repo-source"}
          />
          <button type="submit" disabled={!apiAvailable}>Open page editor</button>
        </form>
        {status ? <p className="page-pilot-launch-status">{status}</p> : null}
      </section>

      <section className="page-pilot-runs-panel" aria-label="Recent Page Pilot runs">
        <header>
          <div>
            <span className="section-label">Recent runs</span>
            <h3>Traceable Page Pilot sessions</h3>
          </div>
          <button
            type="button"
            onClick={() => {
              setRunsLoading(true);
              setRunsError("");
              onFetchRuns()
                .then((nextRuns) => setRuns(filterRunsForRepository(nextRuns, repositoryTargetId).slice(0, 8)))
                .catch((error) => setRunsError(error instanceof Error ? error.message : String(error)))
                .finally(() => setRunsLoading(false));
            }}
            disabled={!apiAvailable || !repositoryTargetId || runsLoading}
          >
            Refresh
          </button>
        </header>
        {runsError ? <p className="page-pilot-launch-status">{runsError}</p> : null}
        {runsLoading ? <p className="page-pilot-run-empty">Loading Page Pilot runs...</p> : null}
        {!runsLoading && runs.length === 0 ? (
          <p className="page-pilot-run-empty">No Page Pilot runs recorded for this repository yet.</p>
        ) : (
          <div className="page-pilot-run-list">
            {runs.map((run) => (
              <article key={run.id} className="page-pilot-run-card">
                <div>
                  <span className={`page-pilot-run-status status-${run.status || "unknown"}`}>{run.status || "unknown"}</span>
                  <strong>{pagePilotRunTitle(run)}</strong>
                  <small>{pagePilotRunSubtitle(run)}</small>
                </div>
                <div className="page-pilot-run-actions">
                  <button type="button" onClick={() => setSelectedRun(run)}>Details</button>
                  {run.pullRequestUrl ? <a href={run.pullRequestUrl} target="_blank" rel="noreferrer">PR</a> : null}
                  {run.workItemId ? <a href={workItemDetailHash(run.workItemId)}>Work Item</a> : null}
                  {run.pipelineId ? <span>{run.pipelineId}</span> : null}
                </div>
              </article>
            ))}
          </div>
        )}
      </section>
      {selectedRun ? (
        <div className="page-pilot-run-modal" role="dialog" aria-modal="true" aria-label="Page Pilot run details">
          <div className="page-pilot-run-modal-panel">
            <header>
              <div>
                <span className={`page-pilot-run-status status-${selectedRun.status || "unknown"}`}>{selectedRun.status || "unknown"}</span>
                <h3>{pagePilotRunTitle(selectedRun)}</h3>
                <p>{pagePilotRunSubtitle(selectedRun)}</p>
              </div>
              <button type="button" onClick={() => setSelectedRun(null)}>Close</button>
            </header>
            <div className="page-pilot-run-detail-grid">
              <RunDetailBlock title="PR preview" value={selectedRun.prPreview?.title || selectedRun.pullRequestUrl || "Not created yet"} body={selectedRun.prPreview?.body} />
              <RunDetailBlock title="Diff summary" value={`${selectedRun.changedFiles?.length ?? 0} changed file(s)`} body={[selectedRun.diffSummary, selectedRun.lineDiffSummary].filter(Boolean).join("\n\n")} />
              <RunDetailBlock title="Source mapping" value={String(selectedRun.sourceMappingReport?.status ?? "unknown")} body={formatRecord(selectedRun.sourceMappingReport)} />
              <RunDetailBlock title="Visual proof" value={String(selectedRun.visualProof?.kind ?? "not captured")} body={formatRecord(selectedRun.visualProof)} />
              <RunDetailBlock title="Preview runtime" value={String(selectedRun.previewRuntimeProfile?.source ?? "unknown")} body={formatRecord(selectedRun.previewRuntimeProfile)} />
              <RunDetailBlock title="Conversation" value={`${selectedRun.submittedAnnotations?.length ?? 0} annotations · round ${selectedRun.roundNumber ?? 1}`} body={formatRecord(selectedRun.conversationBatch)} />
            </div>
          </div>
        </div>
      ) : null}
    </section>
  );
}

function RunDetailBlock({ title, value, body }: { title: string; value: string; body?: string }) {
  return (
    <section className="page-pilot-run-detail-block">
      <span className="section-label">{title}</span>
      <strong>{value}</strong>
      {body ? <pre>{body}</pre> : <p>No detail recorded yet.</p>}
    </section>
  );
}

function repositoryTargetLabel(target: RepositoryTarget) {
  return target.kind === "github" ? `${target.owner}/${target.repo}` : target.path;
}

function pagePilotRunTitle(run: PagePilotRunInfo) {
  if (run.pullRequestUrl) return "Pull request ready";
  if (run.status === "discarded") return "Discarded local changes";
  if (run.status === "delivered") return "Delivered";
  if (run.status === "applied") return "Waiting for confirmation";
  return "Page Pilot run";
}

function pagePilotRunSubtitle(run: PagePilotRunInfo) {
  const files = run.changedFiles?.length ? `${run.changedFiles.length} changed file${run.changedFiles.length === 1 ? "" : "s"}` : "No changed files";
  const updated = run.updatedAt ? ` · ${formatRunTime(run.updatedAt)}` : "";
  const item = run.workItemId ? ` · ${run.workItemId}` : "";
  return `${files}${item}${updated}`;
}

function filterRunsForRepository(runs: PagePilotRunInfo[], repositoryTargetId?: string) {
  if (!repositoryTargetId) return runs;
  return runs.filter((run) => !run.repositoryTargetId || run.repositoryTargetId === repositoryTargetId);
}

function formatRecord(value: unknown) {
  if (!value) return "";
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function formatRunTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);
}
