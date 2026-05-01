import { useEffect, useMemo, useState } from "react";
import type { PagePilotApplyResult, PagePilotDeliverResult, PagePilotRunInfo, PagePilotSelectionContext } from "../omegaControlApiClient";

type PagePilotOverlayProps = {
  projectId?: string;
  repositoryTargetId?: string;
  repositoryLabel?: string;
  externalSelection?: PagePilotSelectionContext | null;
  targetDocument?: Document | null;
  targetFrameElement?: HTMLElement | null;
  targetUnavailableMessage?: string;
  apiAvailable: boolean;
  onApply: (instruction: string, selection: PagePilotSelectionContext) => Promise<PagePilotApplyResult>;
  onDeliver: (instruction: string, selection: PagePilotSelectionContext, runId?: string) => Promise<PagePilotDeliverResult>;
  onDiscard: (runId: string) => Promise<{ status: string; lineDiffSummary?: string }>;
  onFetchRuns: () => Promise<PagePilotRunInfo[]>;
};

function parseOmegaSource(value: string | null): PagePilotSelectionContext["sourceMapping"] {
  const source = value ?? "";
  const [filePart, symbolPart = ""] = source.split(":");
  return {
    source,
    file: filePart,
    symbol: symbolPart,
  };
}

function elementKind(element: Element): PagePilotSelectionContext["elementKind"] {
  const tagName = element.tagName.toLowerCase();
  if (tagName === "button" || element.getAttribute("role") === "button") return "button";
  if (/^h[1-6]$/.test(tagName)) return "title";
  if (element.closest(".portal-card, .overview-panel, .work-item-card, .requirement-source-card, .card, .hero, article, section")) return "card-copy";
  return "other";
}

function candidateFor(rawTarget: EventTarget | null): Element | null {
  if (!(rawTarget instanceof Element)) return null;
  return rawTarget.closest("[data-omega-source], button, [role='button'], h1, h2, h3, p, small, strong, article, .card, .hero");
}

function selectorFor(element: Element): string {
  const escapeSelector = (value: string) => (typeof CSS !== "undefined" && CSS.escape ? CSS.escape(value) : value.replace(/[^a-zA-Z0-9_-]/g, "\\$&"));
  const sourceElement = element.closest("[data-omega-source]");
  const source = sourceElement?.getAttribute("data-omega-source");
  if (source) {
    return `[data-omega-source="${source.replace(/"/g, '\\"')}"]`;
  }
  if (element.id) return `#${escapeSelector(element.id)}`;
  const parts: string[] = [];
  let cursor: Element | null = element;
  while (cursor && cursor !== document.body && parts.length < 4) {
    const tag = cursor.tagName.toLowerCase();
    const className = Array.from(cursor.classList).filter(Boolean).slice(0, 2).map((name) => `.${escapeSelector(name)}`).join("");
    const parentElement: Element | null = cursor.parentElement;
    const currentTagName = cursor.tagName;
    const siblings = parentElement ? Array.from(parentElement.children).filter((child: Element) => child.tagName === currentTagName) : [];
    const nth = siblings.length > 1 && parentElement ? `:nth-of-type(${siblings.indexOf(cursor) + 1})` : "";
    parts.unshift(`${tag}${className}${nth}`);
    cursor = parentElement;
  }
  return parts.join(" > ");
}

function collectSelection(element: Element, ownerDocument: Document = document): PagePilotSelectionContext {
  const sourceElement = element.closest("[data-omega-source]") ?? element;
  const rect = element.getBoundingClientRect();
  const view = ownerDocument.defaultView ?? window;
  const styles = view.getComputedStyle(element);
  const parent = element.parentElement;
  return {
    elementKind: elementKind(element),
    stableSelector: selectorFor(element),
    textSnapshot: (element.textContent ?? "").trim().replace(/\s+/g, " ").slice(0, 500),
    styleSnapshot: {
      color: styles.color,
      backgroundColor: styles.backgroundColor,
      fontSize: styles.fontSize,
      fontWeight: styles.fontWeight,
      fontFamily: styles.fontFamily,
      borderRadius: styles.borderRadius,
    },
    domContext: {
      tagName: element.tagName.toLowerCase(),
      role: element.getAttribute("role") ?? "",
      ariaLabel: element.getAttribute("aria-label") ?? "",
      className: element.getAttribute("class") ?? "",
      parentTagName: parent?.tagName.toLowerCase() ?? "",
      parentClassName: parent?.getAttribute("class") ?? "",
      route: `${view.location.pathname}${view.location.hash}`,
      viewport: { width: view.innerWidth, height: view.innerHeight },
      rect: { x: Math.round(rect.x), y: Math.round(rect.y), width: Math.round(rect.width), height: Math.round(rect.height) },
    },
    sourceMapping: parseOmegaSource(sourceElement.getAttribute("data-omega-source")),
  };
}

export function PagePilotOverlay({
  projectId,
  repositoryTargetId,
  repositoryLabel,
  externalSelection,
  targetDocument,
  targetFrameElement,
  targetUnavailableMessage,
  apiAvailable,
  onApply,
  onDeliver,
  onDiscard,
  onFetchRuns,
}: PagePilotOverlayProps) {
  const [open, setOpen] = useState(false);
  const [selecting, setSelecting] = useState(false);
  const [hovered, setHovered] = useState<Element | null>(null);
  const [selection, setSelection] = useState<PagePilotSelectionContext | null>(null);
  const [instruction, setInstruction] = useState("");
  const [status, setStatus] = useState("");
  const [applyResult, setApplyResult] = useState<PagePilotApplyResult | null>(null);
  const [deliverResult, setDeliverResult] = useState<PagePilotDeliverResult | null>(null);
  const [runs, setRuns] = useState<PagePilotRunInfo[]>([]);

  useEffect(() => {
    if (!externalSelection) return;
    setSelection(externalSelection);
    setSelecting(false);
    setOpen(true);
    setStatus("Element captured from the Electron preview. Add an instruction and apply it.");
  }, [externalSelection]);

  useEffect(() => {
    if (!selecting) {
      setHovered(null);
      return undefined;
    }
    if (!targetDocument) {
      setStatus("Open an inspectable target product preview before selecting.");
      setSelecting(false);
      return undefined;
    }
    const activeDocument = targetDocument;
    function onPointerOver(event: PointerEvent) {
      const target = candidateFor(event.target);
      if (target && !(target as Element).closest(".page-pilot-overlay")) {
        setHovered(target as Element);
      }
    }
    function onPointerMove(event: PointerEvent) {
      const target = candidateFor(event.target);
      if (target && !(target as Element).closest(".page-pilot-overlay")) {
        setHovered(target as Element);
      }
    }
    function onClick(event: MouseEvent) {
      const target = candidateFor(event.target);
      if (!target || target.closest(".page-pilot-overlay")) return;
      event.preventDefault();
      event.stopPropagation();
      setSelection(collectSelection(target, activeDocument));
      setSelecting(false);
      setOpen(true);
      setStatus("Element captured. Add an instruction and apply it to the mapped source.");
    }
    activeDocument.addEventListener("pointerover", onPointerOver, true);
    activeDocument.addEventListener("pointermove", onPointerMove, true);
    activeDocument.addEventListener("click", onClick, true);
    return () => {
      activeDocument.removeEventListener("pointerover", onPointerOver, true);
      activeDocument.removeEventListener("pointermove", onPointerMove, true);
      activeDocument.removeEventListener("click", onClick, true);
    };
  }, [selecting, targetDocument]);

  useEffect(() => {
    if (!open || !apiAvailable) return;
    let cancelled = false;
    onFetchRuns()
      .then((records) => {
        if (!cancelled) setRuns(records.slice(0, 5));
      })
      .catch(() => {
        if (!cancelled) setRuns([]);
      });
    return () => {
      cancelled = true;
    };
  }, [apiAvailable, open]);

  const hoverStyle = useMemo(() => {
    if (!hovered) return undefined;
    const rect = hovered.getBoundingClientRect();
    const frameRect = targetFrameElement?.getBoundingClientRect();
    return {
      left: rect.left + (frameRect?.left ?? 0),
      top: rect.top + (frameRect?.top ?? 0),
      width: rect.width,
      height: rect.height,
    };
  }, [hovered, targetFrameElement]);
  const hoveredSelection = useMemo(() => hovered ? collectSelection(hovered, targetDocument ?? document) : null, [hovered, targetDocument]);
  const canApply = Boolean(selection && instruction.trim());
  const canConfirm = Boolean(applyResult && applyResult.status === "applied");
  const canDiscard = Boolean(applyResult?.id && applyResult.status === "applied");

  async function applyInstruction() {
    if (!selection) {
      setStatus("Select a page element first.");
      return;
    }
    if (!repositoryTargetId || !apiAvailable) {
      setStatus("Page Pilot needs an active repository workspace and the local runtime.");
      return;
    }
    setStatus("Applying real source change through the local runtime...");
    setDeliverResult(null);
    try {
      const result = await onApply(instruction, selection);
      setApplyResult(result);
      setRuns((current) => [result, ...current.filter((run) => run.id !== result.id)].slice(0, 5));
      setStatus("Applied. Vite HMR or dev server reload should now reflect the source change.");
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Page Pilot apply failed.");
    }
  }

  async function deliverChange() {
    if (!selection) return;
    setStatus("Creating branch, commit, and PR-ready delivery...");
    try {
      const result = await onDeliver(instruction, selection, applyResult?.id);
      setDeliverResult(result);
      setRuns((current) => current.map((run) => (run.id === result.id ? { ...run, ...result } : run)));
      setStatus(result.pullRequestUrl ? "Delivered with a pull request." : "Delivered as a local branch and commit.");
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Page Pilot delivery failed.");
    }
  }

  async function discardChange() {
    if (!applyResult?.id) return;
    setStatus("Discarding local Page Pilot source changes...");
    try {
      const result = await onDiscard(applyResult.id);
      setApplyResult((current) => current ? { ...current, status: result.status, lineDiffSummary: result.lineDiffSummary ?? current.lineDiffSummary } : current);
      setRuns((current) => current.map((run) => (run.id === applyResult.id ? { ...run, ...result } : run)));
      setStatus("Discarded local Page Pilot changes.");
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Page Pilot discard failed.");
    }
  }

  return (
    <>
      {selecting && hoverStyle ? <div className="page-pilot-highlight" style={hoverStyle} aria-hidden="true" /> : null}
      <section className={`${open ? "page-pilot-overlay open" : "page-pilot-overlay"} ${selecting ? "selecting" : ""}`} aria-label="Omega Page Pilot">
        <button type="button" className="page-pilot-fab" onClick={() => setOpen((current) => !current)}>
          AI
        </button>
        {open ? (
          <div className="page-pilot-panel">
            <div className="page-pilot-header">
              <span>Page Pilot</span>
              <small>{repositoryLabel || "No repository workspace"}</small>
            </div>
            <div className="page-pilot-actions">
              <button
                type="button"
                disabled={Boolean(targetUnavailableMessage)}
                onClick={() => {
                  setSelecting((current) => !current);
                  setStatus("Move over the preview to inspect elements, then click one to capture it.");
                }}
              >
                {selecting ? "Cancel" : "Select"}
              </button>
              <button type="button" onClick={() => setSelection(null)}>Clear</button>
            </div>
            {selecting ? (
              <>
                <div className="page-pilot-inspector">
                  <span>{hoveredSelection?.elementKind ?? "Inspecting"}</span>
                  <strong>{hoveredSelection?.textSnapshot || hoveredSelection?.stableSelector || "Move over a visible element."}</strong>
                  <small>{hoveredSelection?.sourceMapping.source || "No data-omega-source on current element"}</small>
                </div>
                <p className="page-pilot-status">Click the highlighted element to capture it. The panel stays compact while selecting.</p>
              </>
            ) : (
              <>
            {selection ? (
              <div className="page-pilot-selection">
                <span>{selection.elementKind}</span>
                <strong>{selection.textSnapshot || selection.stableSelector}</strong>
                <small>{selection.sourceMapping.source || "missing data-omega-source"}</small>
              </div>
            ) : (
              <p className="page-pilot-empty">Select a visible element to collect selector, DOM context, style snapshot, and source mapping.</p>
            )}
            <textarea
              value={instruction}
              onChange={(event) => setInstruction(event.currentTarget.value)}
              placeholder="Tell Page Pilot what to change in the mapped source..."
            />
            <div className="page-pilot-footer">
              <button
                type="button"
                disabled={!canApply}
                title={canApply ? "Apply the requested source edit" : "Select an element and enter an instruction first"}
                onClick={() => void applyInstruction()}
              >
                Apply
              </button>
              <button
                type="button"
                disabled={!canConfirm}
                title={canConfirm ? "Create branch, commit, and PR-ready delivery" : "Apply a change before confirming"}
                onClick={() => void deliverChange()}
              >
                Confirm
              </button>
              <button
                type="button"
                disabled={!canDiscard}
                title={canDiscard ? "Discard the applied Page Pilot change" : "Only an applied run can be discarded"}
                onClick={() => void discardChange()}
              >
                Discard
              </button>
            </div>
            {!canApply || !canConfirm || !canDiscard ? (
              <div className="page-pilot-button-help">
                <span>Apply: select + instruction</span>
                <span>Confirm / Discard: after Apply</span>
              </div>
            ) : null}
            {status ? <p className="page-pilot-status">{status}</p> : null}
            {targetUnavailableMessage ? <p className="page-pilot-status">{targetUnavailableMessage}</p> : null}
            {applyResult ? (
              <div className="page-pilot-result">
                <small>Changed</small>
                <strong>{applyResult.changedFiles.join(", ") || "pending diff"}</strong>
                {applyResult.id ? <small>Run {applyResult.id}</small> : null}
                {applyResult.lineDiffSummary ? <pre>{applyResult.lineDiffSummary}</pre> : null}
              </div>
            ) : null}
            {deliverResult?.pullRequestUrl ? (
              <a className="page-pilot-pr" href={deliverResult.pullRequestUrl} target="_blank" rel="noreferrer">
                Open PR
              </a>
            ) : null}
            {runs.length > 0 ? (
              <div className="page-pilot-history">
                <small>Recent runs</small>
                {runs.map((run) => (
                  <button
                    key={run.id ?? `${run.updatedAt}-${run.status}`}
                    type="button"
                    onClick={() => {
                      if (run.status === "applied") setApplyResult(run);
                    }}
                  >
                    <span>{run.status}</span>
                    <strong>{run.changedFiles?.[0] ?? run.repositoryTarget ?? "Page Pilot run"}</strong>
                  </button>
                ))}
              </div>
            ) : null}
            {projectId ? <span className="page-pilot-scope">Project {projectId}</span> : null}
              </>
            )}
          </div>
        ) : null}
      </section>
    </>
  );
}
