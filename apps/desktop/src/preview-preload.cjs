const { ipcRenderer } = require("electron");

function omegaSource(element) {
  const sourceElement = element.closest("[data-omega-source]");
  return sourceElement ? sourceElement.getAttribute("data-omega-source") || "" : "";
}

function elementKind(element) {
  const tag = element.tagName.toLowerCase();
  if (tag === "button" || element.getAttribute("role") === "button") return "button";
  if (/^h[1-6]$/.test(tag)) return "title";
  if (element.closest("article, section, .card, [class*='card']")) return "card-copy";
  return "other";
}

function selectorFor(element) {
  const source = omegaSource(element);
  if (source) return `[data-omega-source="${source.replace(/"/g, '\\"')}"]`;
  if (element.id) return `#${CSS.escape(element.id)}`;
  const parts = [];
  let cursor = element;
  while (cursor && cursor !== document.body && parts.length < 5) {
    const tag = cursor.tagName.toLowerCase();
    const className = Array.from(cursor.classList || []).slice(0, 2).map((name) => `.${CSS.escape(name)}`).join("");
    parts.unshift(`${tag}${className}`);
    cursor = cursor.parentElement;
  }
  return parts.join(" > ");
}

function collectSelection(element) {
  const rect = element.getBoundingClientRect();
  const styles = window.getComputedStyle(element);
  const source = omegaSource(element);
  const [file = "", symbol = ""] = source.split(":");
  return {
    elementKind: elementKind(element),
    stableSelector: selectorFor(element),
    textSnapshot: (element.textContent || "").trim().replace(/\s+/g, " ").slice(0, 500),
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
      route: `${window.location.pathname}${window.location.hash}`,
      viewport: { width: window.innerWidth, height: window.innerHeight },
      rect: { x: Math.round(rect.x), y: Math.round(rect.y), width: Math.round(rect.width), height: Math.round(rect.height) },
    },
    sourceMapping: { source, file, symbol },
  };
}

function installPagePilotToolbar() {
  if (document.getElementById("omega-page-pilot-toolbar")) return;
  const style = document.createElement("style");
  style.textContent = `
    #omega-page-pilot-toolbar {
      position: fixed;
      right: 18px;
      bottom: 18px;
      z-index: 2147483647;
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 10px;
      border: 1px solid rgba(148, 163, 184, 0.45);
      border-radius: 999px;
      background: rgba(15, 23, 42, 0.88);
      box-shadow: 0 18px 50px rgba(15, 23, 42, 0.28);
      color: #f8fafc;
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      backdrop-filter: blur(14px);
    }
    #omega-page-pilot-toolbar button {
      min-width: 0;
      height: 36px;
      padding: 0 14px;
      border: 1px solid rgba(226, 232, 240, 0.22);
      border-radius: 999px;
      background: rgba(255, 255, 255, 0.1);
      color: inherit;
      font: inherit;
      font-size: 13px;
      font-weight: 700;
      cursor: pointer;
    }
    #omega-page-pilot-toolbar button[data-primary="true"] {
      border-color: rgba(45, 212, 191, 0.8);
      background: #2563eb;
    }
    #omega-page-pilot-toolbar .omega-page-pilot-status {
      max-width: 220px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      color: rgba(226, 232, 240, 0.88);
      font-size: 12px;
      font-weight: 700;
    }
    .omega-page-pilot-selecting *:hover {
      outline: 2px solid #2563eb !important;
      outline-offset: 3px !important;
      cursor: crosshair !important;
    }
  `;
  document.documentElement.appendChild(style);

  const toolbar = document.createElement("div");
  toolbar.id = "omega-page-pilot-toolbar";
  toolbar.innerHTML = `
    <span class="omega-page-pilot-status">Page Pilot</span>
    <button type="button" data-action="select" data-primary="true">圈选元素</button>
    <button type="button" data-action="reload">刷新</button>
    <button type="button" data-action="close">返回</button>
  `;
  toolbar.addEventListener("click", (event) => {
    const button = event.target instanceof Element ? event.target.closest("button[data-action]") : null;
    if (!button) return;
    event.preventDefault();
    event.stopPropagation();
    const action = button.getAttribute("data-action");
    if (action === "select") {
      window.__OMEGA_PAGE_PILOT_SELECTING__ = true;
      document.documentElement.classList.add("omega-page-pilot-selecting");
      toolbar.querySelector(".omega-page-pilot-status").textContent = "点击页面元素";
    } else if (action === "reload") {
      window.location.reload();
    } else if (action === "close") {
      ipcRenderer.invoke("omega-preview:close");
    }
  }, true);
  document.body.appendChild(toolbar);
}

window.addEventListener("DOMContentLoaded", () => {
  installPagePilotToolbar();

  ipcRenderer.on("omega-preview:set-selecting", (_event, value) => {
    window.__OMEGA_PAGE_PILOT_SELECTING__ = Boolean(value);
    document.documentElement.classList.toggle("omega-page-pilot-selecting", Boolean(value));
    const status = document.querySelector("#omega-page-pilot-toolbar .omega-page-pilot-status");
    if (status) status.textContent = value ? "点击页面元素" : "Page Pilot";
  });

  document.addEventListener("click", (event) => {
    if (!window.__OMEGA_PAGE_PILOT_SELECTING__) return;
    if (event.target instanceof Element && event.target.closest("#omega-page-pilot-toolbar")) return;
    const target = event.target instanceof Element
      ? event.target.closest("button, h1, h2, h3, p, small, strong, span, article, section")
      : null;
    if (!target) return;
    event.preventDefault();
    event.stopPropagation();
    window.__OMEGA_PAGE_PILOT_SELECTING__ = false;
    document.documentElement.classList.remove("omega-page-pilot-selecting");
    ipcRenderer.send("omega-preview:selection", collectSelection(target));
  }, true);
});
