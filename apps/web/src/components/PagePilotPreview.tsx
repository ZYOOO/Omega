import { useEffect, useRef, useState } from "react";
import type { RepositoryTarget } from "../core";
import {
  startPagePilotPreviewRuntime,
  type PagePilotApplyResult,
  type PagePilotDeliverResult,
  type PagePilotRunInfo,
  type PagePilotSelectionContext,
} from "../omegaControlApiClient";
import { workItemDetailHash } from "../workItemRoutes";
import { PagePilotOverlay } from "./PagePilotOverlay";

type PagePilotPreviewProps = {
  projectId?: string;
  repositoryTargets: RepositoryTarget[];
  repositoryTargetId?: string;
  repositoryLabel?: string;
  apiAvailable: boolean;
  apiUrl?: string;
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

const previewResolveTimeoutMs = 20000;
const previewStartTimeoutMs = 65000;
const previewOpenTimeoutMs = 20000;

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
    previewUrl?: string;
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

function looksLikeHtmlFilePath(value: string) {
  const trimmed = value.trim();
  if (!trimmed) return false;
  if (/^https?:\/\//i.test(trimmed)) return false;
  return trimmed.startsWith("file://") || trimmed.startsWith("/") || /\.html?(?:[?#].*)?$/i.test(trimmed);
}

function withPreviewIntent(previewUrl: string, intent: string) {
  const trimmed = intent.trim();
  if (!trimmed || !trimmed.startsWith("/")) return previewUrl;
  try {
    return new URL(trimmed, previewUrl).href;
  } catch {
    return previewUrl;
  }
}

function devServerLaunchInput(value: string) {
  const trimmed = value.trim();
  if (!trimmed) return { intent: "", previewUrl: undefined };
  if (trimmed.startsWith("/")) return { intent: trimmed, previewUrl: undefined };
  const normalized = normalizePreviewUrl(trimmed, "dev-server");
  try {
    const url = new URL(normalized);
    if (url.protocol === "http:" || url.protocol === "https:") {
      const base = `${url.origin}/`;
      const pathIntent = `${url.pathname}${url.search}${url.hash}`;
      return { intent: pathIntent === "/" ? "" : pathIntent, previewUrl: base };
    }
  } catch {
    // Non-URL text is treated as a launch note for the Preview Runtime Agent.
  }
  return { intent: trimmed, previewUrl: undefined };
}

function localHostName(value: string) {
  return value === "localhost" || value === "127.0.0.1" || value === "::1";
}

function browserFrameUrlFor(value: string) {
  if (!value.trim() || typeof window === "undefined") return value;
  try {
    const target = new URL(value, window.location.href);
    const current = new URL(window.location.href);
    if (target.origin === current.origin) return `${target.pathname}${target.search}${target.hash}`;
    if (localHostName(target.hostname) && localHostName(current.hostname) && (target.port || defaultPortFor(target.protocol)) === "3009") {
      return `/page-pilot-target${target.pathname}${target.search}${target.hash}`;
    }
    return target.href;
  } catch {
    return value;
  }
}

function defaultPortFor(protocol: string) {
  if (protocol === "https:") return "443";
  if (protocol === "http:") return "80";
  return "";
}

function withTimeout<T>(promise: Promise<T>, label: string, timeoutMs: number): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = window.setTimeout(() => {
      reject(new Error(`${label} timed out after ${Math.round(timeoutMs / 1000)}s.`));
    }, timeoutMs);
    promise.then(
      (value) => {
        window.clearTimeout(timer);
        resolve(value);
      },
      (error) => {
        window.clearTimeout(timer);
        reject(error);
      },
    );
  });
}

export function PagePilotPreview({
  projectId,
  repositoryTargets,
  repositoryTargetId,
  repositoryLabel,
  apiAvailable,
  apiUrl,
  onSelectRepositoryTarget,
  onApply,
  onDeliver,
  onDiscard,
  onFetchRuns,
}: PagePilotPreviewProps) {
  const iframeRef = useRef<HTMLIFrameElement | null>(null);
  const [previewMode, setPreviewMode] = useState<PreviewMode>(initialPreviewMode);
  const [draftUrl, setDraftUrl] = useState(initialPreviewUrl);
  const [browserPreviewUrl, setBrowserPreviewUrl] = useState("");
  const [targetDocument, setTargetDocument] = useState<Document | null>(null);
  const [targetMessage, setTargetMessage] = useState("");
  const [status, setStatus] = useState("");
  const [runs, setRuns] = useState<PagePilotRunInfo[]>([]);
  const [runsLoading, setRunsLoading] = useState(false);
  const [runsError, setRunsError] = useState("");
  const [selectedRun, setSelectedRun] = useState<PagePilotRunInfo | null>(null);
  const [activePreviewRuntimeProfile, setActivePreviewRuntimeProfile] = useState<PreviewRuntimeProfile | null>(null);
  const [launching, setLaunching] = useState(false);
  const desktopBridge = omegaDesktopBridge();
  const selectedRepositoryTarget = repositoryTargetId
    ? repositoryTargets.find((target) => repositoryTargetStableId(target) === repositoryTargetId)
    : repositoryTargets.find((target) => repositoryTargetLabel(target) === repositoryLabel) ?? (repositoryTargets.length === 1 ? repositoryTargets[0] : undefined);
  const effectiveRepositoryTargetId = selectedRepositoryTarget ? repositoryTargetStableId(selectedRepositoryTarget) : repositoryTargetId ?? "";
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
    setTargetDocument(null);
    setTargetMessage("");
  }, [browserPreviewUrl]);

  useEffect(() => {
    let canceled = false;
    if (!apiAvailable || !effectiveRepositoryTargetId) {
      setRuns([]);
      return () => {
        canceled = true;
      };
    }
    setRunsLoading(true);
    setRunsError("");
    onFetchRuns()
      .then((nextRuns) => {
        if (!canceled) setRuns(filterRunsForRepository(nextRuns, effectiveRepositoryTargetId).slice(0, 8));
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
  }, [apiAvailable, effectiveRepositoryTargetId]);

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
    const result = await withTimeout(desktopBridge.resolvePreviewTarget(selectedRepositoryTarget), "Preparing repository preview", previewResolveTimeoutMs);
    if (!result.ok) {
      setStatus(result.error ?? "Could not prepare the target repository preview.");
      return { url: "" };
    }
    if (result.hasPackageJson) {
      if (!desktopBridge?.startPreviewDevServer) {
        setActivePreviewRuntimeProfile(null);
        setStatus("Preview Runtime Agent is required to start this repository.");
        return { url: "" };
      }
      setStatus("Preview Runtime Agent is starting the repository dev server...");
      const runtime = await withTimeout(desktopBridge.startPreviewDevServer({
        target: selectedRepositoryTarget,
        projectId,
        repositoryTargetId: effectiveRepositoryTargetId,
        intent: "",
      }), "Starting Preview Runtime Agent", previewStartTimeoutMs);
      if (!runtime.ok || !runtime.previewUrl) {
        setActivePreviewRuntimeProfile(runtime.profile ?? null);
        setStatus(runtime.error ?? "Preview Runtime Agent could not start the repository dev server.");
        return { url: "" };
      }
      setDraftUrl(runtime.previewUrl);
      setActivePreviewRuntimeProfile(runtime.profile ?? null);
      const source = runtime.profile?.source ? ` (${runtime.profile.source})` : "";
      setStatus(`Preview Runtime Agent started${source}: ${runtime.previewUrl}`);
      return { url: runtime.previewUrl, profile: runtime.profile ?? null };
    }
    if (result.htmlFile) {
      const fileUrl = fileUrlFor(result.htmlFile);
      const profile: PreviewRuntimeProfile = {
        agentId: "preview-runtime-agent",
        stageId: "preview_runtime",
        repositoryTargetId: effectiveRepositoryTargetId,
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
        repositoryTargetId: effectiveRepositoryTargetId,
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
    const launchInput = devServerLaunchInput(draftUrl);
    setStatus("Preview Runtime Agent is starting the dev server...");
    const result = await withTimeout(desktopBridge.startPreviewDevServer({
      target: selectedRepositoryTarget,
      projectId,
      repositoryTargetId: effectiveRepositoryTargetId,
      intent: launchInput.intent,
      previewUrl: launchInput.previewUrl,
    }), "Starting Preview Runtime Agent", previewStartTimeoutMs);
    if (!result.ok || !result.previewUrl) {
      setStatus(result.error ?? "Preview Runtime Agent could not start the dev server.");
      return { url: "" };
    }
    const nextUrl = withPreviewIntent(result.previewUrl, launchInput.intent);
    setDraftUrl(result.previewUrl);
    setActivePreviewRuntimeProfile(result.profile ?? null);
    const source = result.profile?.source ? ` (${result.profile.source})` : "";
    setStatus(`Preview Runtime Agent started${source}: ${result.previewUrl}`);
    return { url: nextUrl, profile: result.profile ?? null };
  }

  async function startApiRuntimePreview(intent = "", previewUrl?: string) {
    if (!effectiveRepositoryTargetId) {
      setStatus("Choose a Repository Workspace first.");
      return { url: "" };
    }
    if (!apiUrl) {
      setStatus("Browser mode needs the local runtime API to prepare a repository preview.");
      return { url: "" };
    }
    setStatus("Preview Runtime Agent is starting the preview server...");
    const result = await withTimeout(startPagePilotPreviewRuntime(apiUrl, {
      projectId,
      repositoryTargetId: effectiveRepositoryTargetId,
      intent,
      previewUrl,
    }), "Starting Preview Runtime Agent", previewStartTimeoutMs);
    if (!result.ok || !result.previewUrl) {
      setActivePreviewRuntimeProfile((result.profile as PreviewRuntimeProfile | undefined) ?? null);
      setStatus(result.error ?? "Preview Runtime Agent could not start the preview server.");
      return { url: "" };
    }
    const profile = (result.profile as PreviewRuntimeProfile | undefined) ?? {
      agentId: result.agentId,
      stageId: result.stageId,
      repositoryTargetId: result.repositoryTargetId,
      workingDirectory: result.repositoryPath,
      previewUrl: result.previewUrl,
      createdAt: new Date().toISOString(),
    };
    setDraftUrl(result.previewUrl);
    setActivePreviewRuntimeProfile(profile);
    const source = profile.source ? ` (${profile.source})` : "";
    setStatus(`Preview Runtime Agent started${source}: ${result.previewUrl}`);
    return { url: withPreviewIntent(result.previewUrl, intent), profile };
  }

  async function resolveHtmlFilePreview() {
    const explicitPath = draftUrl.trim();
    if (explicitPath) {
      if (/^https?:\/\//i.test(explicitPath)) {
        setStatus("Finding HTML entry in the selected workspace...");
      } else if (!looksLikeHtmlFilePath(explicitPath)) {
        setStatus("HTML file mode needs a local .html file path. Clear the field to use the selected workspace index.html.");
        return { url: "" };
      } else {
        const previewUrl = normalizePreviewUrl(explicitPath, "html-file");
        const profile: PreviewRuntimeProfile = {
          agentId: "preview-runtime-agent",
          stageId: "preview_runtime",
          repositoryTargetId: effectiveRepositoryTargetId,
          previewUrl,
          source: "html-file",
          reloadStrategy: "browser-reload",
          createdAt: new Date().toISOString(),
        };
        setActivePreviewRuntimeProfile(profile);
        return { url: profile.previewUrl ?? "", profile };
      }
    }
    if (!selectedRepositoryTarget) {
      setStatus("Choose a Repository Workspace first.");
      return { url: "" };
    }
    if (!desktopBridge?.resolvePreviewTarget) {
      setStatus("Electron is required to find an HTML file in this workspace.");
      return { url: "" };
    }
    if (!explicitPath) {
      setStatus("Finding HTML entry in the selected workspace...");
    }
    const result = await withTimeout(desktopBridge.resolvePreviewTarget(selectedRepositoryTarget), "Finding HTML entry", previewResolveTimeoutMs);
    if (!result.ok) {
      setStatus(result.error ?? "Could not prepare the selected repository workspace.");
      return { url: "" };
    }
    if (!result.htmlFile) {
      setStatus(`No root index.html was found in ${result.repoPath ?? "the selected workspace"}. Enter an HTML file path.`);
      return { url: "" };
    }
    const fileUrl = fileUrlFor(result.htmlFile);
    const profile: PreviewRuntimeProfile = {
      agentId: "preview-runtime-agent",
      stageId: "preview_runtime",
      repositoryTargetId: effectiveRepositoryTargetId,
      workingDirectory: result.repoPath,
      previewUrl: fileUrl,
      source: "html-file",
      reloadStrategy: "browser-reload",
      evidence: ["index.html"],
      createdAt: new Date().toISOString(),
    };
    setDraftUrl(result.htmlFile);
    setActivePreviewRuntimeProfile(profile);
    setStatus(`HTML preview ready: ${result.htmlFile}`);
    return { url: fileUrl, profile };
  }

  async function openDirectPilot() {
    if (!selectedRepositoryTarget || !effectiveRepositoryTargetId) {
      setStatus("Choose a Repository Workspace first. Page Pilot changes must be locked to one target repository.");
      return;
    }
    if (launching) return;
    let launchTarget: PreviewLaunchTarget = { url: "", profile: activePreviewRuntimeProfile };
    setLaunching(true);
    try {
      if (desktopBridge?.openPreview) {
        if (previewMode === "repo-source") {
          launchTarget = await resolveSelectedRepoPreview();
        } else if (previewMode === "dev-server") {
          launchTarget = await startDevServerPreview();
        } else {
          launchTarget = await resolveHtmlFilePreview();
        }
        if (!launchTarget.url) {
          if (previewMode === "repo-source") {
            setStatus("Preview Runtime Agent must start the target project first.");
          }
          return;
        }
        setStatus("Opening target page...");
        const result = await withTimeout(desktopBridge.openPreview({
          url: launchTarget.url,
          projectId,
          repositoryTargetId: effectiveRepositoryTargetId,
          repositoryLabel: selectedRepositoryLabel,
          returnUrl: "#page-pilot",
          previewRuntimeProfile: launchTarget.profile ?? undefined,
        }), "Opening Page Pilot", previewOpenTimeoutMs);
        setStatus(result.ok ? "Target page opened. Select elements, add notes, and apply changes there." : result.error ?? "Could not open Page Pilot.");
      } else {
        if (previewMode === "dev-server") {
          const launchInput = devServerLaunchInput(draftUrl);
          if (apiUrl) {
            launchTarget = await startApiRuntimePreview(launchInput.intent, launchInput.previewUrl);
          } else if (launchInput.previewUrl) {
            launchTarget = { url: withPreviewIntent(launchInput.previewUrl, launchInput.intent) };
          } else {
            setStatus("Browser mode needs a full preview URL, such as http://127.0.0.1:3009/.");
            return;
          }
        } else {
          launchTarget = await startApiRuntimePreview("", undefined);
        }
        if (!launchTarget.url) return;
        setBrowserPreviewUrl(browserFrameUrlFor(launchTarget.url));
        setStatus("Browser preview opened. Select elements in the embedded preview when it is inspectable.");
      }
    } catch (error) {
      setStatus(error instanceof Error ? error.message : String(error));
    } finally {
      setLaunching(false);
    }
  }

  function syncFrameDocument() {
    const frame = iframeRef.current;
    if (!frame) return;
    try {
      const doc = frame.contentDocument;
      if (!doc?.body) {
        setTargetDocument(null);
        setTargetMessage("Preview loaded, but the document is not ready yet.");
        return;
      }
      setTargetDocument(doc);
      setTargetMessage("");
    } catch {
      setTargetDocument(null);
      setTargetMessage("Browser mode cannot inspect a cross-origin iframe. Use the local Page Pilot target proxy or the Electron preview bridge.");
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
            value={effectiveRepositoryTargetId}
            onChange={(event) => {
              onSelectRepositoryTarget(event.currentTarget.value);
              setStatus("");
            }}
          >
            <option value="">Select repository...</option>
            {repositoryTargets.map((target) => (
              <option key={repositoryTargetStableId(target)} value={repositoryTargetStableId(target)}>
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
          <button type="submit" disabled={!apiAvailable || launching}>{launching ? "Opening..." : "Open page editor"}</button>
        </form>
        {status ? <p className="page-pilot-launch-status">{status}</p> : null}
      </section>

      {browserPreviewUrl ? (
        <div className="page-pilot-frame-wrap">
          <iframe
            ref={iframeRef}
            title="Page Pilot target project preview"
            src={browserPreviewUrl}
            onLoad={syncFrameDocument}
          />
        </div>
      ) : null}

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
                .then((nextRuns) => setRuns(filterRunsForRepository(nextRuns, effectiveRepositoryTargetId).slice(0, 8)))
                .catch((error) => setRunsError(error instanceof Error ? error.message : String(error)))
                .finally(() => setRunsLoading(false));
            }}
            disabled={!apiAvailable || !effectiveRepositoryTargetId || runsLoading}
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
      {browserPreviewUrl ? (
        <PagePilotOverlay
          projectId={projectId}
          repositoryTargetId={effectiveRepositoryTargetId}
          repositoryLabel={selectedRepositoryLabel}
          targetDocument={targetDocument}
          targetFrameElement={iframeRef.current}
          targetUnavailableMessage={!targetDocument ? targetMessage || "Preview is not inspectable yet. Use the local Page Pilot target proxy or Electron preview bridge." : ""}
          apiAvailable={apiAvailable}
          onApply={onApply}
          onDeliver={onDeliver}
          onDiscard={onDiscard}
          onFetchRuns={onFetchRuns}
        />
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

function repositoryTargetStableId(target: RepositoryTarget) {
  if (target.id) return target.id;
  if (target.kind === "github") return `repo_${target.owner}_${target.repo}`;
  return `local_${target.path}`;
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
