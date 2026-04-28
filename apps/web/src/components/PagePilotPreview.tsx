import { useEffect, useRef, useState } from "react";
import { PagePilotOverlay } from "./PagePilotOverlay";
import type { PagePilotApplyResult, PagePilotDeliverResult, PagePilotRunInfo, PagePilotSelectionContext } from "../omegaControlApiClient";

type PagePilotPreviewProps = {
  projectId?: string;
  repositoryTargetId?: string;
  repositoryLabel?: string;
  apiAvailable: boolean;
  onApply: (instruction: string, selection: PagePilotSelectionContext) => Promise<PagePilotApplyResult>;
  onDeliver: (instruction: string, selection: PagePilotSelectionContext, runId?: string) => Promise<PagePilotDeliverResult>;
  onDiscard: (runId: string) => Promise<{ status: string; lineDiffSummary?: string }>;
  onFetchRuns: () => Promise<PagePilotRunInfo[]>;
  onExit?: () => void;
};

const previewUrlStorageKey = "omega-page-pilot-preview-url";

function initialPreviewUrl() {
  if (typeof window === "undefined") return "";
  return normalizePreviewUrl(window.localStorage.getItem(previewUrlStorageKey) ?? "");
}

function normalizePreviewUrl(value: string) {
  const trimmed = value.trim();
  if (!trimmed || typeof window === "undefined") return trimmed;
  try {
    const url = new URL(trimmed, window.location.href);
    const current = new URL(window.location.href);
    const localHosts = new Set(["127.0.0.1", "localhost", "::1"]);
    if (
      url.pathname.startsWith("/page-pilot-target") &&
      url.port === current.port &&
      localHosts.has(url.hostname) &&
      localHosts.has(current.hostname)
    ) {
      return `${url.pathname}${url.search}${url.hash}`;
    }
    return url.href;
  } catch {
    return trimmed;
  }
}

export function PagePilotPreview({
  projectId,
  repositoryTargetId,
  repositoryLabel,
  apiAvailable,
  onApply,
  onDeliver,
  onDiscard,
  onFetchRuns,
  onExit,
}: PagePilotPreviewProps) {
  const iframeRef = useRef<HTMLIFrameElement | null>(null);
  const [previewUrl, setPreviewUrl] = useState(initialPreviewUrl);
  const [draftUrl, setDraftUrl] = useState(initialPreviewUrl);
  const [targetDocument, setTargetDocument] = useState<Document | null>(null);
  const [targetMessage, setTargetMessage] = useState("");

  useEffect(() => {
    if (!previewUrl) return;
    window.localStorage.setItem(previewUrlStorageKey, previewUrl);
  }, [previewUrl]);

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
      setTargetMessage("Browser mode cannot inspect a cross-origin iframe. Use an Electron preview webview or serve the target preview through the Omega origin.");
    }
  }

  return (
    <section className="page-pilot-surface">
      <header className="page-pilot-preview-header">
        <div>
          <span className="section-label">Page Pilot</span>
          <h2>Target project preview</h2>
          <p>Open the software you are building here, then select its page elements for real source edits.</p>
        </div>
        <form
          className="page-pilot-url-form"
          onSubmit={(event) => {
            event.preventDefault();
            setTargetDocument(null);
            const nextUrl = normalizePreviewUrl(draftUrl);
            setDraftUrl(nextUrl);
            setPreviewUrl(nextUrl);
          }}
        >
          <input
            value={draftUrl}
            onChange={(event) => setDraftUrl(event.currentTarget.value)}
            placeholder="http://127.0.0.1:3000"
          />
          <button type="submit">Open preview</button>
          {onExit ? <button type="button" className="page-pilot-exit" onClick={onExit}>Workboard</button> : null}
        </form>
      </header>

      <div className="page-pilot-preview-meta">
        <span>{repositoryLabel || "No repository workspace"}</span>
        <span>{targetDocument ? "Inspectable" : "Needs preview bridge"}</span>
      </div>

      <div className="page-pilot-frame-wrap">
        {previewUrl ? (
          <iframe
            ref={iframeRef}
            title="Page Pilot target project preview"
            src={previewUrl}
            onLoad={syncFrameDocument}
          />
        ) : (
          <div className="page-pilot-frame-empty">
            <strong>Open a target app preview URL</strong>
            <p>For browser mode, same-origin previews are inspectable. Electron webview support will make local cross-origin previews inspectable without changing the target app.</p>
          </div>
        )}
      </div>

      <PagePilotOverlay
        projectId={projectId}
        repositoryTargetId={repositoryTargetId}
        repositoryLabel={repositoryLabel}
        targetDocument={targetDocument}
        targetFrameElement={iframeRef.current}
        targetUnavailableMessage={previewUrl && !targetDocument ? targetMessage || "Preview is not inspectable yet. Use the same-origin Page Pilot target URL or Electron preview bridge." : ""}
        apiAvailable={apiAvailable}
        onApply={onApply}
        onDeliver={onDeliver}
        onDiscard={onDiscard}
        onFetchRuns={onFetchRuns}
      />
    </section>
  );
}
