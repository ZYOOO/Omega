const runtimeUrl = process.env.OMEGA_RUNTIME_URL || "http://127.0.0.1:3888";
const projectId = process.env.OMEGA_PAGE_PILOT_PROJECT_ID || "project_req_omega_001";
const repositoryTargetId = process.env.OMEGA_PAGE_PILOT_REPOSITORY_TARGET_ID || "repo_ZYOOO_TestRepo";
const repositoryLabel = process.env.OMEGA_PAGE_PILOT_REPOSITORY_LABEL || "ZYOOO/TestRepo";
const persistedRunKey = "omega-page-pilot-last-run";
const conversationHistoryKey = "omega-page-pilot-conversation-history";
const statusBarDockKey = "omega-page-pilot-status-dock";
const statusBarPositionKey = "omega-page-pilot-status-position";

function sourceElementFor(element) {
  const sourceElement = element.closest("[data-omega-source]");
  if (sourceElement) return sourceElement;
  return element.querySelector?.("[data-omega-source]") || null;
}

function sourceFor(element) {
  const sourceElement = sourceElementFor(element);
  return sourceElement ? sourceElement.getAttribute("data-omega-source") || "" : "";
}

function sourceParts(source) {
  const [file = "", symbol = ""] = source.split(":");
  return { file, symbol };
}

function kindFor(element) {
  const tag = element.tagName.toLowerCase();
  if (element.getAttribute("role") === "status" || element.hasAttribute("aria-live")) return "status";
  if (tag === "button" || element.getAttribute("role") === "button") return "button";
  if (tag === "a") return "link";
  if (["input", "textarea", "select"].includes(tag)) return "field";
  if (tag === "label") return "label";
  if (/^h[1-6]$/.test(tag)) return "title";
  if (element.closest("article, section, .card, .hero, [class*='card'], [class*='stat']")) return "card-copy";
  return "other";
}

function candidateRank(element) {
  const tag = element.tagName.toLowerCase();
  if (tag === "button" || tag === "a" || ["input", "textarea", "select"].includes(tag) || element.getAttribute("role") === "button") return 100;
  if (element.getAttribute("role") === "status" || element.hasAttribute("aria-live") || element.matches(".message, .alert, [class*='message'], [class*='toast'], [class*='notice'], [class*='error'], [class*='success']")) return 96;
  if (tag === "label") return 92;
  if (element.hasAttribute("data-omega-source")) return 84;
  if (/^h[1-6]$/.test(tag)) return 76;
  if (["p", "small", "strong", "span"].includes(tag)) return 62;
  if (element.matches("article, section, .card, .hero, [class*='card'], [class*='stat']")) return 48;
  if (element.matches("div, li, ul, ol, form, main, header, footer, nav, aside")) return 18;
  return 1;
}

function visibleCandidate(element) {
  if (!(element instanceof Element)) return false;
  if (["HTML", "BODY", "SCRIPT", "STYLE"].includes(element.tagName)) return false;
  const rect = element.getBoundingClientRect();
  return rect.width > 1 && rect.height > 1;
}

function candidateForEvent(event, ignoredRoot) {
  const elements = document.elementsFromPoint(event.clientX, event.clientY)
    .filter((element) => !ignoredRoot?.contains(element));
  const candidates = [];
  elements.forEach((element, depth) => {
    if (!visibleCandidate(element)) return;
    candidates.push({ element, depth, rank: candidateRank(element) });
    const nearest = element.closest("button, a, input, textarea, select, label, [role='button'], [role='status'], [aria-live], .message, .alert, [class*='message'], [class*='toast'], [class*='notice'], [class*='error'], [class*='success'], [data-omega-source], h1, h2, h3, h4, h5, h6, p, small, strong, span, article, section, .card, .hero, [class*='card'], [class*='stat']");
    if (nearest && visibleCandidate(nearest) && !ignoredRoot?.contains(nearest)) {
      candidates.push({ element: nearest, depth: depth + 0.2, rank: candidateRank(nearest) });
    }
  });
  candidates.sort((left, right) => {
    if (right.rank !== left.rank) return right.rank - left.rank;
    return left.depth - right.depth;
  });
  return candidates[0]?.element || null;
}

function escapeSelector(value) {
  if (globalThis.CSS?.escape) return CSS.escape(value);
  return value.replace(/[^a-zA-Z0-9_-]/g, "\\$&");
}

function selectorFor(element) {
  const source = sourceFor(element);
  if (source) return `[data-omega-source="${source.replace(/"/g, '\\"')}"]`;
  if (element.id) return `#${escapeSelector(element.id)}`;
  const parts = [];
  let cursor = element;
  while (cursor && cursor !== document.body && parts.length < 5) {
    const tag = cursor.tagName.toLowerCase();
    const className = Array.from(cursor.classList || []).slice(0, 2).map((name) => `.${escapeSelector(name)}`).join("");
    const parent = cursor.parentElement;
    const siblings = parent ? Array.from(parent.children).filter((child) => child.tagName === cursor.tagName) : [];
    const nth = siblings.length > 1 && parent ? `:nth-of-type(${siblings.indexOf(cursor) + 1})` : "";
    parts.unshift(`${tag}${className}${nth}`);
    cursor = parent;
  }
  return parts.join(" > ");
}

function selectionFor(element) {
  const source = sourceFor(element);
  const { file, symbol } = sourceParts(source);
  const sourceElement = sourceElementFor(element) || element;
  const rect = element.getBoundingClientRect();
  const styles = window.getComputedStyle(element);
  const parent = element.parentElement;
  return {
    elementKind: kindFor(element),
    stableSelector: selectorFor(element),
    textSnapshot: (sourceElement.textContent || "").trim().replace(/\s+/g, " ").slice(0, 500),
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
      role: element.getAttribute("role") || "",
      ariaLabel: element.getAttribute("aria-label") || "",
      className: element.getAttribute("class") || "",
      parentTagName: parent?.tagName.toLowerCase() || "",
      parentClassName: parent?.getAttribute("class") || "",
      route: `${window.location.pathname}${window.location.hash}`,
      viewport: { width: window.innerWidth, height: window.innerHeight },
      rect: { x: Math.round(rect.x), y: Math.round(rect.y), width: Math.round(rect.width), height: Math.round(rect.height) },
    },
    sourceMapping: { source, file, symbol },
  };
}

function createElement(tag, className, text) {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text !== undefined) element.textContent = text;
  return element;
}

function createSvgIcon(pathData) {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("width", "17");
  svg.setAttribute("height", "17");
  svg.setAttribute("fill", "none");
  svg.setAttribute("stroke", "currentColor");
  svg.setAttribute("stroke-width", "2.2");
  svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");
  pathData.forEach((value) => {
    const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
    path.setAttribute("d", value);
    svg.appendChild(path);
  });
  return svg;
}

function injectStyles() {
  const style = document.createElement("style");
  style.textContent = `
    .omega-pilot-root {
      position: fixed;
      inset: 0;
      z-index: 2147483647;
      pointer-events: none;
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      --omega-feishu-bg: rgba(255, 255, 255, 0.94);
      --omega-feishu-bg-strong: #ffffff;
      --omega-feishu-surface: #f7f8fa;
      --omega-feishu-surface-blue: #eff6ff;
      --omega-feishu-border: #dee0e3;
      --omega-feishu-border-strong: #c9d8ff;
      --omega-feishu-text: #1f2329;
      --omega-feishu-muted: #646a73;
      --omega-feishu-primary: #3370ff;
      --omega-feishu-primary-dark: #245bdb;
      --omega-feishu-danger: #f54a45;
      --omega-feishu-danger-soft: #fde2e2;
      --omega-feishu-shadow: 0 18px 54px rgba(31, 35, 41, 0.16);
    }
    .omega-pilot-fab {
      position: fixed;
      right: 22px;
      bottom: 22px;
      display: grid;
      place-items: center;
      width: 58px;
      height: 58px;
      border: 0;
      border-radius: 999px;
      background: transparent;
      color: #fff;
      box-shadow: 0 18px 44px rgba(20, 184, 166, 0.22);
      font-size: 20px;
      font-weight: 900;
      pointer-events: auto;
      cursor: pointer;
      overflow: hidden;
      text-shadow: 0 1px 10px rgba(9, 20, 42, 0.28);
    }
    .omega-pilot-fab::before {
      content: "";
      position: absolute;
      inset: -190%;
      background: conic-gradient(
        from 220deg,
        transparent 0deg 118deg,
        #a7f3d0 138deg,
        #2dd4bf 188deg,
        #38bdf8 244deg,
        transparent 286deg 360deg
      );
      filter: saturate(1.18);
      animation: omegaPilotRequirementOrbit 3.6s linear infinite;
      pointer-events: none;
    }
    .omega-pilot-fab::after {
      content: "";
      position: absolute;
      inset: 5px;
      border: 1px solid rgba(255, 255, 255, 0.16);
      border-radius: inherit;
      background: linear-gradient(135deg, #4f8cff 0%, #3478ff 42%, #20c9f3 76%, #5eead4 100%);
      box-shadow:
        inset 0 1px 0 rgba(255, 255, 255, 0.24),
        0 14px 30px rgba(20, 184, 166, 0.2);
      pointer-events: none;
    }
    .omega-pilot-fab span {
      position: relative;
      z-index: 1;
    }
    .omega-pilot-fab:hover:not(:disabled),
    .omega-pilot-fab:focus-visible {
      transform: translateY(-1px);
      box-shadow: 0 22px 48px rgba(20, 184, 166, 0.28);
    }
    @keyframes omegaPilotRequirementOrbit {
      to { transform: rotate(1turn); }
    }
    .omega-pilot-highlight {
      position: fixed;
      border: 2px solid #1d9bf0;
      border-radius: 8px;
      background: rgba(29, 155, 240, 0.14);
      box-shadow: 0 0 0 9999px rgba(15, 23, 42, 0.34);
      pointer-events: none;
    }
    .omega-pilot-tooltip {
      position: fixed;
      min-width: 230px;
      max-width: 360px;
      padding: 10px 12px;
      border-radius: 10px;
      border: 1px solid rgba(51, 112, 255, 0.16);
      background: var(--omega-feishu-bg);
      color: var(--omega-feishu-text);
      box-shadow: var(--omega-feishu-shadow);
      backdrop-filter: blur(18px);
      pointer-events: none;
    }
    .omega-pilot-tooltip strong,
    .omega-pilot-tooltip span,
    .omega-pilot-tooltip small {
      display: block;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .omega-pilot-tooltip span {
      color: var(--omega-feishu-primary);
      font-size: 12px;
      font-weight: 800;
      text-transform: uppercase;
    }
    .omega-pilot-tooltip strong {
      margin-top: 4px;
      font-size: 13px;
    }
    .omega-pilot-tooltip small {
      margin-top: 4px;
      color: var(--omega-feishu-muted);
      font-size: 11px;
    }
    .omega-pilot-pin {
      position: fixed;
      display: grid;
      place-items: center;
      width: 36px;
      height: 36px;
      border: 2px solid rgba(255, 255, 255, 0.92);
      border-radius: 999px;
      background:
        radial-gradient(circle at 32% 24%, rgba(255, 255, 255, 0.34), transparent 34%),
        linear-gradient(145deg, #60a5fa 0%, #2563eb 48%, #1d4ed8 100%);
      color: #fff;
      box-shadow: 0 12px 26px rgba(37, 99, 235, 0.42), 0 0 0 5px rgba(37, 99, 235, 0.16);
      font-size: 16px;
      font-weight: 900;
      pointer-events: auto;
      cursor: pointer;
    }
    .omega-pilot-pin:hover {
      transform: translateY(-1px) scale(1.04);
    }
    .omega-pilot-tray {
      position: fixed;
      left: 50%;
      bottom: 24px;
      display: grid;
      grid-template-columns: 1fr auto;
      gap: 10px 12px;
      align-items: center;
      width: min(820px, calc(100vw - 40px));
      padding: 12px;
      border: 1px solid var(--omega-feishu-border);
      border-radius: 18px;
      background: var(--omega-feishu-bg);
      color: var(--omega-feishu-text);
      box-shadow: var(--omega-feishu-shadow);
      transform: translateX(-50%);
      pointer-events: auto;
      backdrop-filter: blur(18px);
    }
    .omega-pilot-chip-row {
      grid-column: 1 / -1;
      display: flex;
      flex-wrap: wrap;
      gap: 7px;
      align-items: center;
    }
    .omega-pilot-chip-row span,
    .omega-pilot-chip-row button.omega-pilot-chip {
      max-width: 220px;
      overflow: hidden;
      padding: 5px 9px;
      border: 1px solid var(--omega-feishu-border);
      border-radius: 999px;
      background: var(--omega-feishu-surface);
      color: var(--omega-feishu-text);
      font-size: 13px;
      font-weight: 800;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .omega-pilot-chip-row button.omega-pilot-chip {
      cursor: pointer;
    }
    .omega-pilot-chip-row button.omega-pilot-chip:hover {
      border-color: var(--omega-feishu-border-strong);
      background: var(--omega-feishu-surface-blue);
      color: var(--omega-feishu-primary);
    }
    .omega-pilot-history-toggle {
      min-height: 34px;
      border: 1px solid var(--omega-feishu-border-strong);
      border-radius: 999px;
      padding: 0 13px;
      background: var(--omega-feishu-surface-blue);
      color: var(--omega-feishu-primary);
      font-weight: 900;
      cursor: pointer;
      box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.1);
    }
    .omega-pilot-history-toggle:hover {
      border-color: var(--omega-feishu-primary);
      background: #e8f0ff;
    }
    .omega-pilot-process-list {
      display: grid;
      gap: 6px;
      margin: 0;
      padding: 0;
      list-style: none;
    }
    .omega-pilot-process-list li {
      display: flex;
      gap: 8px;
      align-items: baseline;
      padding: 7px 9px;
      border-radius: 10px;
      border: 1px solid rgba(31, 35, 41, 0.04);
      background: var(--omega-feishu-surface);
      color: var(--omega-feishu-text);
      font-size: 13px;
      line-height: 1.35;
    }
    .omega-pilot-process-list b {
      color: var(--omega-feishu-primary);
      font-size: 11px;
      text-transform: uppercase;
    }
    .omega-pilot-global-input {
      height: 42px;
      min-height: 42px;
      max-height: 42px;
      resize: none;
      border: 1px solid transparent;
      outline: none;
      border-radius: 12px;
      padding: 10px 12px;
      background: var(--omega-feishu-surface);
      color: var(--omega-feishu-text);
      font: inherit;
      font-size: 15px;
      line-height: 1.45;
    }
    .omega-pilot-global-input::placeholder {
      color: #8f959e;
    }
    .omega-pilot-global-input:focus {
      border-color: var(--omega-feishu-border-strong);
      background: #fff;
      box-shadow: 0 0 0 3px rgba(51, 112, 255, 0.12);
    }
    .omega-pilot-tray-actions {
      display: flex;
      gap: 8px;
      align-items: center;
      align-self: center;
    }
    .omega-pilot-tray-actions button {
      height: 42px;
      min-height: 42px;
      border: 1px solid var(--omega-feishu-border);
      border-radius: 999px;
      padding: 0 12px;
      background: var(--omega-feishu-surface);
      color: var(--omega-feishu-text);
      font-weight: 800;
      cursor: pointer;
    }
    .omega-pilot-tray-actions button.secondary {
      background: #fff;
      color: var(--omega-feishu-text);
    }
    .omega-pilot-tray-actions button.danger {
      border-color: #f8c7c5;
      background: var(--omega-feishu-danger-soft);
      color: var(--omega-feishu-danger);
    }
    .omega-pilot-tray-actions .send {
      width: 52px;
      min-width: 52px;
      padding: 0;
      border: 0;
      background: linear-gradient(135deg, #4e83fd, var(--omega-feishu-primary));
      color: #fff;
      box-shadow: 0 12px 28px rgba(51, 112, 255, 0.24);
      font-size: 22px;
      font-weight: 900;
    }
    .omega-pilot-tray-actions .send::before {
      content: "➜";
      display: block;
      transform: translateX(1px);
    }
    .omega-pilot-result {
      top: 18px;
      bottom: auto;
      grid-template-columns: 1fr;
      align-items: stretch;
      max-height: none;
      overflow: visible;
    }
    .omega-pilot-topbar {
      position: fixed;
      top: auto;
      bottom: 0;
      left: 50%;
      display: grid;
      grid-template-columns: auto 1fr auto;
      gap: 12px;
      align-items: center;
      width: min(980px, calc(100vw - 40px));
      min-height: 46px;
      padding: 8px 10px;
      border: 1px solid var(--omega-feishu-border);
      border-radius: 999px;
      background: var(--omega-feishu-bg);
      color: var(--omega-feishu-text);
      box-shadow: var(--omega-feishu-shadow);
      transform: translateX(-50%);
      pointer-events: auto;
      backdrop-filter: blur(18px);
      transition: width 160ms ease, border-radius 160ms ease, transform 160ms ease, opacity 160ms ease;
    }
    .omega-pilot-topbar.is-tucked {
      opacity: 0.96;
    }
    .omega-pilot-topbar.is-tucked:not(.is-top) {
      transform: translate(-50%, calc(100% - 8px));
    }
    .omega-pilot-topbar.is-tucked.is-top {
      transform: translate(-50%, calc(-100% + 8px));
    }
    .omega-pilot-topbar.is-top {
      top: 0;
      bottom: auto;
    }
    .omega-pilot-topbar.is-floating {
      top: var(--omega-pilot-floating-top);
      bottom: auto;
      left: var(--omega-pilot-floating-left);
      width: min(820px, calc(100vw - 40px));
      transform: none;
    }
    .omega-pilot-topbar.is-expanded {
      border-radius: 18px;
      align-items: start;
    }
    .omega-pilot-topbar-summary {
      display: flex;
      gap: 10px;
      align-items: center;
      min-width: 0;
      color: var(--omega-feishu-text);
      font-weight: 800;
    }
    .omega-pilot-topbar-summary span:last-child {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .omega-pilot-pin-toggle {
      display: grid;
      place-items: center;
      width: 30px;
      height: 30px;
      border: 1px solid var(--omega-feishu-border-strong);
      border-radius: 999px;
      background: var(--omega-feishu-surface-blue);
      color: var(--omega-feishu-primary);
      font-size: 16px;
      cursor: pointer;
    }
    .omega-pilot-dock-toggle {
      display: grid;
      place-items: center;
      width: 30px;
      height: 30px;
      border: 1px solid var(--omega-feishu-border);
      border-radius: 999px;
      background: var(--omega-feishu-surface);
      color: var(--omega-feishu-muted);
      cursor: grab;
      touch-action: none;
    }
    .omega-pilot-dock-toggle:active {
      cursor: grabbing;
    }
    .omega-pilot-topbar-actions {
      display: flex;
      justify-content: flex-end;
      gap: 7px;
      align-items: center;
    }
    .omega-pilot-topbar-actions button {
      height: 34px;
      min-height: 34px;
      border: 1px solid var(--omega-feishu-border);
      border-radius: 999px;
      padding: 0 13px;
      background: var(--omega-feishu-surface);
      color: var(--omega-feishu-text);
      font-weight: 900;
      cursor: pointer;
    }
    .omega-pilot-topbar-actions .primary {
      border-color: var(--omega-feishu-primary);
      background: var(--omega-feishu-primary);
      color: #fff;
    }
    .omega-pilot-topbar-actions .danger {
      border-color: #f8c7c5;
      background: var(--omega-feishu-danger-soft);
      color: var(--omega-feishu-danger);
    }
    .omega-pilot-detail-popover {
      grid-column: 1 / -1;
      display: grid;
      gap: 10px;
      max-height: min(420px, calc(100vh - 92px));
      overflow: auto;
      padding: 8px 2px 2px;
    }
    .omega-pilot-result-main {
      display: grid;
      gap: 10px;
    }
    .omega-pilot-result-main h2 {
      margin: 0;
      color: var(--omega-feishu-text);
      font-size: 18px;
      line-height: 1.2;
    }
    .omega-pilot-title-line {
      display: flex;
      gap: 10px;
      align-items: center;
    }
    .omega-pilot-spinner {
      width: 18px;
      height: 18px;
      border: 3px solid rgba(147, 197, 253, 0.28);
      border-top-color: var(--omega-feishu-primary);
      border-radius: 999px;
      animation: omegaPilotSpin 0.85s linear infinite;
    }
    @keyframes omegaPilotSpin {
      to { transform: rotate(360deg); }
    }
    .omega-pilot-result-main p,
    .omega-pilot-result-main small {
      margin: 0;
      color: var(--omega-feishu-muted);
      line-height: 1.45;
    }
    .omega-pilot-result-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 8px;
    }
    .omega-pilot-result-grid span {
      overflow: hidden;
      padding: 8px 10px;
      border-radius: 10px;
      border: 1px solid rgba(31, 35, 41, 0.05);
      background: var(--omega-feishu-surface);
      color: var(--omega-feishu-text);
      font-size: 13px;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .omega-pilot-result-summary {
      max-height: 150px;
      overflow: auto;
      margin: 0;
      padding: 10px;
      border-radius: 12px;
      border: 1px solid var(--omega-feishu-border);
      background: #fbfcff;
      color: var(--omega-feishu-text);
      white-space: pre-wrap;
      font: 12px/1.45 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
    }
    .omega-pilot-result-link {
      color: var(--omega-feishu-primary);
      font-weight: 800;
      text-decoration: none;
    }
    .omega-pilot-composer {
      position: fixed;
      left: 50%;
      bottom: 34px;
      display: grid;
      grid-template-columns: auto minmax(280px, 620px) auto;
      gap: 10px;
      align-items: center;
      width: min(760px, calc(100vw - 40px));
      padding: 10px;
      border: 1px solid var(--omega-feishu-border);
      border-radius: 999px;
      background: var(--omega-feishu-bg);
      color: var(--omega-feishu-text);
      box-shadow: var(--omega-feishu-shadow);
      transform: translateX(-50%);
      pointer-events: auto;
      backdrop-filter: blur(18px);
    }
    .omega-pilot-badge {
      display: grid;
      place-items: center;
      width: 54px;
      height: 54px;
      border-radius: 999px;
      background:
        radial-gradient(circle at 32% 24%, rgba(255, 255, 255, 0.34), transparent 34%),
        linear-gradient(145deg, #60a5fa 0%, #2563eb 48%, #1d4ed8 100%);
      color: #fff;
      box-shadow: 0 16px 36px rgba(37, 99, 235, 0.42), 0 0 0 6px rgba(37, 99, 235, 0.16);
      font-size: 20px;
      font-weight: 900;
    }
    .omega-pilot-input {
      min-height: 42px;
      border: 0;
      outline: none;
      background: transparent;
      color: var(--omega-feishu-text);
      font: inherit;
      font-size: 16px;
    }
    .omega-pilot-input::placeholder {
      color: #8f959e;
    }
    .omega-pilot-buttons {
      display: flex;
      gap: 6px;
      align-items: center;
    }
    .omega-pilot-buttons button {
      min-height: 36px;
      border: 1px solid var(--omega-feishu-border);
      border-radius: 999px;
      padding: 0 12px;
      background: var(--omega-feishu-surface);
      color: var(--omega-feishu-text);
      font-weight: 800;
      cursor: pointer;
    }
    .omega-pilot-buttons button.secondary {
      background: #fff;
      color: var(--omega-feishu-text);
    }
    .omega-pilot-submit {
      width: auto;
      height: 42px;
      min-height: 42px;
      padding: 0 18px;
      border: 0;
      border-radius: 999px;
      background: linear-gradient(135deg, #4e83fd, var(--omega-feishu-primary));
      color: #fff;
      box-shadow: 0 12px 28px rgba(51, 112, 255, 0.24);
      font-size: 14px;
      font-weight: 900;
      cursor: pointer;
    }
    .omega-pilot-choice {
      position: fixed;
      display: flex;
      gap: 8px;
      align-items: center;
      padding: 8px;
      border-radius: 999px;
      border: 1px solid var(--omega-feishu-border);
      background: var(--omega-feishu-bg);
      box-shadow: var(--omega-feishu-shadow);
      pointer-events: auto;
      backdrop-filter: blur(18px);
    }
    .omega-pilot-choice button {
      width: 42px;
      height: 42px;
      border: 1px solid var(--omega-feishu-border);
      border-radius: 999px;
      color: #fff;
      font-size: 21px;
      font-weight: 900;
      cursor: pointer;
    }
    .omega-pilot-choice .accept {
      border-color: var(--omega-feishu-primary);
      background: var(--omega-feishu-primary);
    }
    .omega-pilot-choice .reject {
      background: var(--omega-feishu-surface);
      color: var(--omega-feishu-text);
    }
    .omega-pilot-status {
      position: fixed;
      left: 50%;
      bottom: 96px;
      max-width: min(720px, calc(100vw - 40px));
      padding: 8px 12px;
      border-radius: 999px;
      border: 1px solid var(--omega-feishu-border);
      background: var(--omega-feishu-bg);
      color: var(--omega-feishu-text);
      box-shadow: var(--omega-feishu-shadow);
      font-size: 13px;
      transform: translateX(-50%);
      pointer-events: none;
    }
  `;
  document.head.appendChild(style);
}

async function postJson(path, payload) {
  const response = await fetch(`${runtimeUrl}${path}`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload),
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `Omega runtime failed: ${response.status}`);
  return body;
}

function readPersistedRun() {
  try {
    const raw = window.localStorage.getItem(persistedRunKey);
    return raw ? JSON.parse(raw) : null;
  } catch {
    return null;
  }
}

function persistRun(run) {
  if (!run) return;
  try {
    window.localStorage.setItem(persistedRunKey, JSON.stringify(run));
  } catch {
    // Ignore persistence failures; the runtime still stores the run.
  }
}

function clearPersistedRun() {
  try {
    window.localStorage.removeItem(persistedRunKey);
  } catch {
    // Ignore localStorage failures.
  }
}

function readConversationHistory() {
  try {
    const raw = window.localStorage.getItem(conversationHistoryKey);
    const parsed = raw ? JSON.parse(raw) : [];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function persistConversationBatch(batch) {
  if (!batch) return [];
  const history = readConversationHistory();
  const next = [batch, ...history.filter((item) => item.id !== batch.id)].slice(0, 8);
  try {
    window.localStorage.setItem(conversationHistoryKey, JSON.stringify(next));
  } catch {
    // Ignore localStorage failures.
  }
  return next;
}

function readStatusBarDock() {
  try {
    const value = window.localStorage.getItem(statusBarDockKey);
    return value === "top" || value === "floating" ? value : "bottom";
  } catch {
    return "bottom";
  }
}

function persistStatusBarDock(value) {
  try {
    const normalized = value === "top" || value === "floating" ? value : "bottom";
    window.localStorage.setItem(statusBarDockKey, normalized);
  } catch {
    // Ignore localStorage failures.
  }
}

function readStatusBarPosition() {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(statusBarPositionKey) || "null");
    return parsed && Number.isFinite(parsed.left) && Number.isFinite(parsed.top) ? parsed : null;
  } catch {
    return null;
  }
}

function persistStatusBarPosition(position) {
  try {
    if (position) {
      window.localStorage.setItem(statusBarPositionKey, JSON.stringify(position));
    } else {
      window.localStorage.removeItem(statusBarPositionKey);
    }
  } catch {
    // Ignore localStorage failures.
  }
}

window.addEventListener("DOMContentLoaded", () => {
  injectStyles();
  const root = createElement("div", "omega-pilot-root");
  const fab = createElement("button", "omega-pilot-fab");
  fab.appendChild(createElement("span", "", "☝"));
  fab.type = "button";
  fab.title = "Select a page element";
  root.appendChild(fab);
  document.body.appendChild(root);

  let selecting = false;
  let hovered = null;
  let selected = null;
  let run = null;
  let highlight = null;
  let tooltip = null;
  let composer = null;
  let choice = null;
  let status = null;
  let tray = null;
  let globalInstruction = "";
  let historyExpanded = false;
  let statusBarExpanded = false;
  let statusBarPeeked = false;
  let statusBarPeekTimer = null;
  let statusBarDock = readStatusBarDock();
  let statusBarPosition = readStatusBarPosition();
  const annotations = [];

  function setStatus(text) {
    if (!text) {
      status?.remove();
      status = null;
      return;
    }
    if (!status) {
      status = createElement("div", "omega-pilot-status");
      root.appendChild(status);
    }
    status.textContent = text;
    window.clearTimeout(status._timer);
    status._timer = window.setTimeout(() => setStatus(""), 4500);
  }

  function setHighlight(element) {
    hovered = element;
    if (!element) {
      highlight?.remove();
      tooltip?.remove();
      highlight = null;
      tooltip = null;
      return;
    }
    const selection = selectionFor(element);
    const rect = element.getBoundingClientRect();
    if (!highlight) {
      highlight = createElement("div", "omega-pilot-highlight");
      root.appendChild(highlight);
    }
    Object.assign(highlight.style, {
      left: `${rect.left}px`,
      top: `${rect.top}px`,
      width: `${rect.width}px`,
      height: `${rect.height}px`,
    });
    if (!tooltip) {
      tooltip = createElement("div", "omega-pilot-tooltip");
      root.appendChild(tooltip);
    }
    tooltip.innerHTML = "";
    tooltip.appendChild(createElement("span", "", selection.elementKind));
    tooltip.appendChild(createElement("strong", "", selection.textSnapshot || selection.stableSelector || element.tagName.toLowerCase()));
    tooltip.appendChild(createElement("small", "", selection.sourceMapping.source || "DOM context captured"));
    Object.assign(tooltip.style, {
      left: `${Math.min(Math.max(12, rect.left), window.innerWidth - 380)}px`,
      top: `${Math.max(12, rect.top - 96)}px`,
    });
  }

  function clearChoice() {
    choice?.remove();
    choice = null;
  }

  function cancelCurrentSelection(message = "已取消当前选择。") {
    clearChoice();
    composer?.remove();
    composer = null;
    selected = null;
    stopSelecting();
    setHighlight(null);
    setStatus(message);
  }

  function startSelecting(message = "继续选择下一个元素，或在底部补充整体需求。") {
    selecting = true;
    document.body.style.cursor = "crosshair";
    fab.querySelector("span").textContent = "×";
    setStatus(message);
  }

  function stopSelecting(options = {}) {
    selecting = false;
    document.body.style.cursor = "";
    if (!options.keepHighlight) setHighlight(null);
    fab.querySelector("span").textContent = "☝";
  }

  function annotationLabel(annotation) {
    return `#${annotation.id} ${annotation.selection.sourceMapping.symbol || annotation.selection.elementKind}`;
  }

  function primaryAnnotation(list = annotations) {
    if (list.length === 0) return null;
    return [...list].reverse().find((annotation) => annotation.selection.sourceMapping.file) || list[list.length - 1];
  }

  function processEvent(text) {
    return {
      at: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
      text,
    };
  }

  function renderAnnotationHistory(list, rerender, options = {}) {
    const chipRow = createElement("div", "omega-pilot-chip-row");
    const toggle = createElement("button", "omega-pilot-history-toggle", historyExpanded ? "收起批注" : "展开批注");
    toggle.type = "button";
    toggle.title = historyExpanded ? "折叠批注列表" : "展开批注列表";
    toggle.addEventListener("click", () => {
      historyExpanded = !historyExpanded;
      rerender();
    });
    chipRow.appendChild(toggle);
    chipRow.appendChild(createElement("span", "", `${list.length} 条批注`));
    const visible = historyExpanded ? list : list.slice(-1);
    visible.forEach((annotation) => {
      const source = annotation.selection.sourceMapping.source || "DOM-only";
      const chip = options.editable
        ? createElement("button", "omega-pilot-chip", `${annotationLabel(annotation)} · ${source}`)
        : createElement("span", "", `${annotationLabel(annotation)} · ${source}`);
      if (options.editable) {
        chip.type = "button";
        chip.title = "编辑这条批注";
        chip.addEventListener("click", () => editAnnotation(annotation.id));
      }
      chipRow.appendChild(chip);
    });
    return chipRow;
  }

  function annotationSummary(list) {
    return list.map((annotation) => annotationLabel(annotation)).join(", ");
  }

  function renderConversationHistory(batches, rerender) {
    const chipRow = createElement("div", "omega-pilot-chip-row");
    const toggle = createElement("button", "omega-pilot-history-toggle", historyExpanded ? "收起对话" : "展开对话");
    toggle.type = "button";
    toggle.title = historyExpanded ? "折叠 Page Pilot 对话历史" : "展开 Page Pilot 对话历史";
    toggle.addEventListener("click", () => {
      historyExpanded = !historyExpanded;
      rerender();
    });
    chipRow.appendChild(toggle);
    chipRow.appendChild(createElement("span", "", `${batches.length} 轮对话`));
    const visible = historyExpanded ? batches : batches.slice(0, 1);
    visible.forEach((batch, index) => {
      const label = index === 0
        ? `最新一轮 · ${batch.annotations?.length || 0} 条批注`
        : `${batch.createdAt || "历史对话"} · ${batch.annotations?.length || 0} 条批注`;
      const chip = createElement("span", "", label);
      chip.title = annotationSummary(batch.annotations || []);
      chipRow.appendChild(chip);
    });
    return chipRow;
  }

  function annotationInstruction(primary, list = annotations, overall = globalInstruction) {
    return [
      "根据以下 Page Pilot 页面批注修改源码。每条批注都包含用户在页面上选中的元素上下文、selector、文本快照和用户修改要求。",
      primary ? `主目标是第 ${primary.id} 条批注：${annotationLabel(primary)}。如果多条批注之间存在冲突，以主目标和用户整体补充说明为准。` : "",
      overall.trim() ? `\n用户整体补充说明：\n${overall.trim()}\n` : "",
      "",
      ...list.map((annotation, index) => [
        `${index + 1}. ${annotation.comment}`,
        `   kind: ${annotation.selection.elementKind}`,
        `   selector: ${annotation.selection.stableSelector}`,
        `   source: ${annotation.selection.sourceMapping.source || "DOM-only"}`,
        `   text: ${annotation.selection.textSnapshot}`,
      ].join("\n")),
    ].join("\n");
  }

  function resetAnnotations() {
    annotations.splice(0, annotations.length);
    globalInstruction = "";
    root.querySelectorAll(".omega-pilot-pin").forEach((pin) => pin.remove());
  }

  function annotationById(id) {
    return annotations.find((annotation) => annotation.id === id) || null;
  }

  function elementForSelection(selection) {
    if (!selection?.stableSelector) return null;
    try {
      return document.querySelector(selection.stableSelector);
    } catch {
      return null;
    }
  }

  function editAnnotation(id) {
    const annotation = annotationById(id);
    if (!annotation) return;
    clearChoice();
    stopSelecting();
    const element = elementForSelection(annotation.selection);
    if (element) setHighlight(element);
    showComposer(annotation.selection, { annotation });
    setStatus(`正在编辑第 ${id} 条批注。`);
  }

  function enrichRunRecord(record, extras = {}) {
    return {
      ...record,
      submittedAnnotations: extras.submittedAnnotations || record.submittedAnnotations || [],
      processEvents: extras.processEvents || record.processEvents || [],
      conversationBatch: extras.conversationBatch || record.conversationBatch || null,
    };
  }

  function latestBatchSummary(batches) {
    const latest = batches[0];
    if (!latest) return "No Page Pilot conversation yet.";
    return `${latest.status || "running"} · ${latest.annotations?.length || 0} 条批注 · ${latest.createdAt || "now"}`;
  }

  function applyStatusBarPlacement(element) {
    if (statusBarDock !== "floating" || !statusBarPosition) return;
    const left = Math.max(20, Math.min(window.innerWidth - 260, statusBarPosition.left));
    const top = Math.max(20, Math.min(window.innerHeight - 60, statusBarPosition.top));
    element.style.setProperty("--omega-pilot-floating-left", `${left}px`);
    element.style.setProperty("--omega-pilot-floating-top", `${top}px`);
  }

  function enableStatusBarDrag(handle, context) {
    const dockSnapThreshold = 16;
    let dragState = null;
    handle.addEventListener("pointerdown", (event) => {
      if (!tray) return;
      event.preventDefault();
      event.stopPropagation();
      const rect = tray.getBoundingClientRect();
      dragState = {
        pointerId: event.pointerId,
        offsetX: event.clientX - rect.left,
        offsetY: event.clientY - rect.top,
      };
      handle.setPointerCapture(event.pointerId);
      statusBarDock = "floating";
      tray.classList.add("is-floating");
      tray.classList.remove("is-top");
      tray.style.left = `${rect.left}px`;
      tray.style.top = `${rect.top}px`;
      tray.style.bottom = "auto";
      tray.style.transform = "none";
    });
    handle.addEventListener("pointermove", (event) => {
      if (!dragState || event.pointerId !== dragState.pointerId || !tray) return;
      const width = tray.offsetWidth;
      const height = tray.offsetHeight;
      const left = Math.max(12, Math.min(window.innerWidth - width - 12, event.clientX - dragState.offsetX));
      const top = Math.max(12, Math.min(window.innerHeight - height - 12, event.clientY - dragState.offsetY));
      tray.style.left = `${left}px`;
      tray.style.top = `${top}px`;
    });
    handle.addEventListener("pointerup", (event) => {
      if (!dragState || event.pointerId !== dragState.pointerId || !tray) return;
      const rect = tray.getBoundingClientRect();
      dragState = null;
      handle.releasePointerCapture(event.pointerId);
      if (rect.top <= dockSnapThreshold) {
        statusBarDock = "top";
        statusBarPosition = null;
        statusBarPeeked = false;
        persistStatusBarPosition(null);
      } else if (window.innerHeight - rect.bottom <= dockSnapThreshold) {
        statusBarDock = "bottom";
        statusBarPosition = null;
        statusBarPeeked = false;
        persistStatusBarPosition(null);
      } else {
        statusBarDock = "floating";
        statusBarPeeked = false;
        statusBarPosition = { left: Math.round(rect.left), top: Math.round(rect.top) };
        persistStatusBarPosition(statusBarPosition);
      }
      persistStatusBarDock(statusBarDock);
      renderTopStatusBar(context);
    });
  }

  function renderTopStatusBar({ title, subtitle, spinning = false, conversationBatches = [], events = [], runRecord = null, errorText = "" }) {
    tray?.remove();
    const context = { title, subtitle, spinning, conversationBatches, events, runRecord, errorText };
    const tucked = !statusBarExpanded && !statusBarPeeked && (statusBarDock === "top" || statusBarDock === "bottom");
    tray = createElement("div", `omega-pilot-topbar${statusBarExpanded ? " is-expanded" : ""}${statusBarDock === "top" ? " is-top" : ""}${statusBarDock === "floating" ? " is-floating" : ""}${tucked ? " is-tucked" : ""}`);
    applyStatusBarPlacement(tray);
    const summary = createElement("div", "omega-pilot-topbar-summary");
    if (spinning) summary.appendChild(createElement("span", "omega-pilot-spinner"));
    const pin = createElement("button", "omega-pilot-pin-toggle");
    pin.type = "button";
    pin.title = statusBarExpanded ? "收起 Page Pilot 详情" : "展开 Page Pilot 详情";
    pin.appendChild(createSvgIcon(statusBarExpanded
      ? ["M8 3v5H3", "M16 3v5h5", "M8 21v-5H3", "M16 21v-5h5"]
      : ["M3 9V3h6", "M21 9V3h-6", "M3 15v6h6", "M21 15v6h-6"]));
    pin.addEventListener("click", () => {
      statusBarExpanded = !statusBarExpanded;
      renderTopStatusBar(context);
    });
    summary.appendChild(pin);
    const dock = createElement("button", "omega-pilot-dock-toggle");
    dock.type = "button";
    dock.title = "拖动状态栏，靠近顶部或底部时自动吸附";
    dock.appendChild(createSvgIcon(["M12 2v20", "M2 12h20", "M12 2l-3 3", "M12 2l3 3", "M12 22l-3-3", "M12 22l3-3", "M2 12l3-3", "M2 12l3 3", "M22 12l-3-3", "M22 12l-3 3"]));
    enableStatusBarDrag(dock, context);
    summary.appendChild(dock);
    summary.appendChild(createElement("span", "", `${title} · ${subtitle || latestBatchSummary(conversationBatches)}`));

    const actions = createElement("div", "omega-pilot-topbar-actions");
    if (runRecord) {
      const confirm = createElement("button", "primary", "Confirm");
      const discard = createElement("button", "danger", "Discard");
      const reload = createElement("button", "", "Reload");
      const startNew = createElement("button", "", "New");
      confirm.type = discard.type = reload.type = startNew.type = "button";
      confirm.disabled = runRecord.status !== "applied";
      discard.disabled = runRecord.status !== "applied";
      actions.append(confirm, discard, reload, startNew);
      confirm.addEventListener("click", () => deliverRun(runRecord, confirm, discard));
      discard.addEventListener("click", () => discardRun(runRecord, confirm, discard));
      reload.addEventListener("click", () => window.location.reload());
      startNew.addEventListener("click", () => {
        run = null;
        clearPersistedRun();
        resetAnnotations();
        tray?.remove();
        tray = null;
        setStatus("Ready for a new Page Pilot selection.");
      });
    }

    tray.append(summary, createElement("span"), actions);
    if (statusBarExpanded) {
      const detail = createElement("div", "omega-pilot-detail-popover");
      if (errorText) detail.appendChild(createElement("p", "", errorText));
      if (conversationBatches.length > 0) {
        detail.appendChild(renderConversationHistory(conversationBatches, () => renderTopStatusBar(context)));
      }
      if (runRecord?.diffSummary || runRecord?.lineDiffSummary) {
        detail.appendChild(createElement("pre", "omega-pilot-result-summary", [runRecord.diffSummary, runRecord.lineDiffSummary].filter(Boolean).join("\n\n")));
      }
      if (events.length > 0) {
        const list = createElement("ul", "omega-pilot-process-list");
        events.forEach((event) => {
          const item = createElement("li");
          item.appendChild(createElement("b", "", event.at || "now"));
          item.appendChild(createElement("span", "", event.text || String(event)));
          list.appendChild(item);
        });
        detail.appendChild(list);
      }
      tray.appendChild(detail);
    }
    root.appendChild(tray);
    if (statusBarDock === "top" || statusBarDock === "bottom") {
      tray.addEventListener("mouseenter", () => {
        window.clearTimeout(statusBarPeekTimer);
        if (!statusBarExpanded && !statusBarPeeked) {
          statusBarPeeked = true;
          renderTopStatusBar(context);
        }
      });
      tray.addEventListener("mouseleave", () => {
        window.clearTimeout(statusBarPeekTimer);
        statusBarPeekTimer = window.setTimeout(() => {
          if (!statusBarExpanded && statusBarPeeked) {
            statusBarPeeked = false;
            renderTopStatusBar(context);
          }
        }, 360);
      });
    }
  }

  function renderProcessPanel(conversationBatches, events, errorText = "") {
    renderTopStatusBar({
      title: errorText ? "Page Pilot needs attention" : "Page Pilot is working",
      subtitle: errorText || latestBatchSummary(conversationBatches),
      spinning: !errorText,
      conversationBatches,
      events,
      errorText,
    });
  }

  function deliverRun(runRecord, confirm, discard) {
    const submittedAnnotations = runRecord.submittedAnnotations || [];
    const events = runRecord.processEvents || [];
    if (!runRecord.id || !runRecord.selection) {
      setStatus("This run is missing selection context and cannot be delivered.");
      return;
    }
    confirm.disabled = true;
    discard.disabled = true;
    setStatus("正在确认并创建 branch / commit / PR...");
    postJson("/page-pilot/deliver", {
      runId: runRecord.id,
      projectId,
      repositoryTargetId,
      instruction: runRecord.instruction || "",
      selection: runRecord.selection,
    }).then((delivered) => {
      const deliveredBatch = runRecord.conversationBatch ? { ...runRecord.conversationBatch, status: "delivered" } : null;
      if (deliveredBatch) persistConversationBatch(deliveredBatch);
      const enriched = enrichRunRecord(delivered, {
        submittedAnnotations,
        processEvents: [...events, processEvent(delivered.pullRequestUrl ? "Created pull request for confirmed Page Pilot changes." : "Created local branch and commit for confirmed Page Pilot changes.")],
        conversationBatch: deliveredBatch,
      });
      run = enriched;
      persistRun(enriched);
      renderRunPanel(enriched);
      setStatus(delivered.pullRequestUrl ? "已创建 PR。" : "已创建本地 branch 和 commit。");
    }).catch((error) => {
      confirm.disabled = false;
      discard.disabled = false;
      setStatus(error instanceof Error ? error.message : "Page Pilot delivery failed.");
    });
  }

  function discardRun(runRecord, confirm, discard) {
    const submittedAnnotations = runRecord.submittedAnnotations || [];
    const events = runRecord.processEvents || [];
    if (!runRecord.id) {
      setStatus("This run is missing an id and cannot be discarded.");
      return;
    }
    confirm.disabled = true;
    discard.disabled = true;
    setStatus("正在撤销 Page Pilot 代码变更...");
    postJson(`/page-pilot/runs/${encodeURIComponent(runRecord.id)}/discard`, {}).then((discarded) => {
      const discardedBatch = runRecord.conversationBatch ? { ...runRecord.conversationBatch, status: "discarded" } : null;
      if (discardedBatch) persistConversationBatch(discardedBatch);
      const enriched = enrichRunRecord(discarded, {
        submittedAnnotations,
        processEvents: [...events, processEvent("Discarded the Page Pilot changes and restored changed source files.")],
        conversationBatch: discardedBatch,
      });
      run = enriched;
      persistRun(enriched);
      renderRunPanel(enriched);
      setStatus("已撤销代码变更，正在刷新预览...");
      window.setTimeout(() => window.location.reload(), 450);
    }).catch((error) => {
      confirm.disabled = false;
      discard.disabled = false;
      setStatus(error instanceof Error ? error.message : "Page Pilot discard failed.");
    });
  }

  function renderRunPanel(runRecord) {
    const events = runRecord.processEvents || [];
    const conversationBatches = runRecord.conversationBatch
      ? [runRecord.conversationBatch, ...readConversationHistory().filter((batch) => batch.id !== runRecord.conversationBatch.id)]
      : readConversationHistory();
    const statusLabel = runRecord.status === "delivered"
      ? "Delivered"
      : runRecord.status === "discarded"
        ? "Discarded"
        : "Applied";
    renderTopStatusBar({
      title: `Page Pilot ${statusLabel}`,
      subtitle: (runRecord.changedFiles || []).slice(0, 2).join(", ") || latestBatchSummary(conversationBatches),
      conversationBatches,
      events,
      runRecord,
    });
  }

  function renderTray() {
    tray?.remove();
    tray = null;
    if (annotations.length === 0) return;
    tray = createElement("div", "omega-pilot-tray");
    const chipRow = renderAnnotationHistory(annotations, renderTray, { editable: true });
    const input = document.createElement("textarea");
    input.className = "omega-pilot-global-input";
    input.placeholder = "继续描述整体修改需求，或者继续点击右下角手指选择更多元素...";
    input.value = globalInstruction;
    const actions = createElement("div", "omega-pilot-tray-actions");
    const submit = createElement("button", "send");
    const clear = createElement("button", "secondary", "清空");
    submit.type = clear.type = "button";
    submit.title = "提交批注和整体说明给 Agent";
    input.addEventListener("input", () => {
      globalInstruction = input.value;
    });
    actions.append(submit, clear);
    tray.append(chipRow, input, actions);
    root.appendChild(tray);
    input.focus();

    submit.addEventListener("click", async () => {
      const submittedAnnotations = annotations.map((annotation) => ({ ...annotation }));
      const primary = primaryAnnotation(submittedAnnotations);
      if (!primary) {
        setStatus("先选择并批注至少一个页面元素。");
        return;
      }
      const overall = globalInstruction;
      const instruction = annotationInstruction(primary, submittedAnnotations, overall);
      const batch = {
        id: `page_pilot_batch_${Date.now()}`,
        createdAt: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
        annotations: submittedAnnotations,
        primaryAnnotationId: primary.id,
        instruction,
        overallInstruction: overall,
        status: "running",
      };
      const conversationBatches = persistConversationBatch(batch);
      const events = [
        processEvent(`Captured ${submittedAnnotations.length} page annotation(s).`),
        processEvent(`Primary target: ${annotationLabel(primary)} (${primary.selection.sourceMapping.source || "DOM-only"}).`),
        processEvent("Submitting selection context to the single Page Pilot Agent."),
      ];
      resetAnnotations();
      historyExpanded = false;
      renderProcessPanel(conversationBatches, events);
      setStatus("正在把批注提交给 Omega Agent...");
      try {
        const applied = await postJson("/page-pilot/apply", {
          projectId,
          repositoryTargetId,
          runner: "profile",
          instruction,
          selection: primary.selection,
        });
        const completedEvents = [
          ...events,
          processEvent(`Agent applied source changes: ${(applied.changedFiles || []).join(", ") || "source changed"}.`),
          applied.workItemId ? processEvent(`Linked to Work Item ${applied.workItemId} and Pipeline ${applied.pipelineId || "page-pilot"}.`) : processEvent("Stored Page Pilot run in Omega runtime."),
          processEvent("Refreshing the live preview so HMR/dev-server reload shows the change."),
        ];
        batch.status = "applied";
        batch.runId = applied.id;
        persistConversationBatch(batch);
        run = enrichRunRecord(applied, { submittedAnnotations, processEvents: completedEvents, conversationBatch: batch });
        persistRun(run);
        renderRunPanel(run);
        setStatus(`Agent 已应用：${(run.changedFiles || []).join(", ") || "source changed"}。正在刷新预览...`);
        window.setTimeout(() => window.location.reload(), 650);
      } catch (error) {
        const failedEvents = [...events, processEvent(error instanceof Error ? error.message : "Page Pilot agent apply failed.")];
        batch.status = "failed";
        batch.error = error instanceof Error ? error.message : "Page Pilot agent apply failed.";
        const failedBatches = persistConversationBatch(batch);
        renderProcessPanel(failedBatches, failedEvents, error instanceof Error ? error.message : "Page Pilot agent apply failed.");
        setStatus(error instanceof Error ? error.message : "Page Pilot agent apply failed.");
      }
    });

    clear.addEventListener("click", () => {
      resetAnnotations();
      renderTray();
      setStatus("已清空批注。");
    });
  }

  function renderAnnotationPin(annotation) {
    const rect = annotation.selection.domContext.rect || { x: 20, y: 20, width: 0, height: 0 };
    const pin = createElement("div", "omega-pilot-pin", String(annotation.id));
    pin.dataset.annotationId = String(annotation.id);
    pin.title = `编辑第 ${annotation.id} 条批注`;
    Object.assign(pin.style, {
      left: `${Math.min(window.innerWidth - 40, Math.max(8, rect.x + rect.width - 18))}px`,
      top: `${Math.min(window.innerHeight - 40, Math.max(8, rect.y - 14))}px`,
    });
    pin.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      editAnnotation(annotation.id);
    });
    root.appendChild(pin);
  }

  function addAnnotation(selection, comment) {
    const annotation = { id: annotations.length + 1, selection, comment };
    annotations.push(annotation);
    renderAnnotationPin(annotation);
    renderTray();
  }

  function showChoice(element, selection) {
    clearChoice();
    composer?.remove();
    composer = null;
    setHighlight(element);
    const rect = element.getBoundingClientRect();
    choice = createElement("div", "omega-pilot-choice");
    const accept = createElement("button", "accept", "✓");
    const reject = createElement("button", "reject", "×");
    accept.type = reject.type = "button";
    accept.title = "Use this element";
    reject.title = "Cancel selection";
    choice.append(accept, reject);
    root.appendChild(choice);
    const left = Math.min(Math.max(12, rect.left + rect.width / 2 - 50), window.innerWidth - 110);
    const top = Math.min(Math.max(12, rect.bottom + 10), window.innerHeight - 70);
    Object.assign(choice.style, { left: `${left}px`, top: `${top}px` });
    setStatus("确认选择这个元素？");

    accept.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      clearChoice();
      showComposer(selection);
    });
    reject.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      cancelCurrentSelection("已取消当前选择。");
    });
  }

  function showComposer(selection, options = {}) {
    const editingAnnotation = options.annotation || null;
    const annotationId = editingAnnotation?.id || annotations.length + 1;
    composer?.remove();
    composer = createElement("div", "omega-pilot-composer");
    const badge = createElement("span", "omega-pilot-badge", String(annotationId));
    const input = createElement("input", "omega-pilot-input");
    input.placeholder = selection.sourceMapping.source
      ? `批注 ${selection.sourceMapping.source}...`
      : "批注这个元素，Omega 会记录 DOM context 和 selector...";
    input.value = editingAnnotation?.comment || "";
    const buttons = createElement("div", "omega-pilot-buttons");
    const send = createElement("button", "omega-pilot-submit", editingAnnotation ? "保存" : "添加");
    const close = createElement("button", "secondary", "×");
    send.type = close.type = "button";
    buttons.append(send, close);
    composer.append(badge, input, buttons);
    root.appendChild(composer);
    input.focus();

    function submitComment() {
      if (!input.value.trim()) {
        setStatus("先输入这条批注。");
        return;
      }
      if (editingAnnotation) {
        editingAnnotation.comment = input.value.trim();
        renderTray();
      } else {
        addAnnotation(selection, input.value.trim());
      }
      composer?.remove();
      composer = null;
      selected = null;
      setHighlight(null);
      setStatus(editingAnnotation
        ? `已更新第 ${editingAnnotation.id} 条批注。`
        : `已添加第 ${annotations.length} 条批注，可以继续选择其他元素。`);
      if (!editingAnnotation) startSelecting();
    }

    send.addEventListener("click", () => {
      submitComment();
    });
    input.addEventListener("keydown", (event) => {
      if (event.key === "Enter" && !event.shiftKey) {
        event.preventDefault();
        submitComment();
      }
    });

    close.addEventListener("click", () => {
      cancelCurrentSelection("已取消当前批注。");
    });
  }

  fab.addEventListener("click", () => {
    selecting = !selecting;
    fab.querySelector("span").textContent = selecting ? "×" : "☝";
    document.body.style.cursor = selecting ? "crosshair" : "";
    if (selecting) setHighlight(null);
    setStatus(selecting ? "移动鼠标选择页面元素，点击后输入修改要求。" : "");
    if (!selecting) setHighlight(null);
  });

  document.addEventListener("pointermove", (event) => {
    if (!selecting) return;
    const candidate = candidateForEvent(event, root);
    if (candidate && !root.contains(candidate)) setHighlight(candidate);
  }, true);

  document.addEventListener("click", (event) => {
    if (!selecting) return;
    const candidate = candidateForEvent(event, root);
    if (!candidate || root.contains(candidate)) return;
    event.preventDefault();
    event.stopPropagation();
    selected = selectionFor(candidate);
    stopSelecting({ keepHighlight: true });
    showChoice(candidate, selected);
  }, true);

  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    if (!selecting && !choice && !composer && !selected) return;
    event.preventDefault();
    event.stopPropagation();
    cancelCurrentSelection("已取消当前选择，已保存的批注不受影响。");
  }, true);

  const persisted = readPersistedRun();
  if (persisted?.id) {
    run = persisted;
    renderRunPanel(persisted);
  }
});
