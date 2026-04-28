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

window.addEventListener("DOMContentLoaded", () => {
  document.addEventListener("click", (event) => {
    if (!window.__OMEGA_PAGE_PILOT_SELECTING__) return;
    const target = event.target instanceof Element
      ? event.target.closest("button, h1, h2, h3, p, small, strong, span, article, section")
      : null;
    if (!target) return;
    event.preventDefault();
    event.stopPropagation();
    window.__OMEGA_PAGE_PILOT_SELECTING__ = false;
    ipcRenderer.send("omega-preview:selection", collectSelection(target));
  }, true);
});
