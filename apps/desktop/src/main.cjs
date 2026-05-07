const { app, BrowserWindow, BrowserView, dialog, ipcMain } = require("electron");
const path = require("node:path");
const { refreshPreviewRuntime, resolveRepositoryPreviewTarget, startDesktopServices, startRepositoryPreviewRuntime, stopDesktopServices } = require("./process-supervisor.cjs");

if (process.env.OMEGA_DISABLE_GPU_ACCELERATION === "1") {
  app.disableHardwareAcceleration();
  app.commandLine.appendSwitch("disable-gpu-compositing");
}

let mainWindow;
let previewView;
let previewViewAttached = false;
let desktopServices;
let previewRuntimeSession;
let omegaAppURL = "";
const appIconPath = path.join(__dirname, "assets", "omega-icon.png");

function layoutPreviewView() {
  if (!mainWindow || !previewView) return;
  const [width, height] = mainWindow.getContentSize();
  previewView.setBounds({
    x: 0,
    y: 0,
    width,
    height,
  });
  previewView.setAutoResize({ width: true, height: true });
}

function encodePilotConfig(config) {
  return encodeURIComponent(JSON.stringify(config || {}));
}

function runtimeApiBase() {
  const raw = desktopServices?.runtime?.url || desktopServices?.runtime?.plan?.url || process.env.OMEGA_RUNTIME_URL || "http://127.0.0.1:3888/health";
  try {
    const url = new URL(raw);
    url.search = "";
    url.hash = "";
    if (url.pathname === "/health") {
      url.pathname = "/";
    }
    return url.toString().replace(/\/$/, "");
  } catch (_error) {
    return "http://127.0.0.1:3888";
  }
}

function normalizePreviewRequest(input) {
  if (typeof input === "string") return { url: input, config: { url: input, runtimeUrl: runtimeApiBase(), hostedInOmega: true } };
  if (!input || typeof input !== "object") return { url: "", config: {} };
  const url = typeof input.url === "string" ? input.url : "";
  return {
    url,
    config: {
      ...input,
      url,
      runtimeUrl: input.runtimeUrl || runtimeApiBase(),
      hostedInOmega: true,
    },
  };
}

function attachPreviewView() {
  if (!mainWindow || !previewView || previewViewAttached) return;
  mainWindow.setBrowserView(previewView);
  previewViewAttached = true;
  layoutPreviewView();
}

function ensurePreviewView(config = {}, options = {}) {
  if (!mainWindow) return previewView;
  if (previewView) closePreviewView();
  previewView = new BrowserView({
    webPreferences: {
      preload: path.join(__dirname, "pilot-preload.cjs"),
      additionalArguments: [`--omega-page-pilot-config=${encodePilotConfig(config)}`],
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
    },
  });
  previewViewAttached = false;
  if (options.attach !== false) attachPreviewView();
  return previewView;
}

function closePreviewView() {
  if (!mainWindow || !previewView) return { ok: false, error: "preview is not open" };
  const view = previewView;
  previewView = null;
  const wasAttached = previewViewAttached;
  previewViewAttached = false;
  if (wasAttached) mainWindow.removeBrowserView(view);
  view.webContents.destroy();
  return { ok: true };
}

async function loadPreviewURLWithStatus(view, url) {
  let navigationStatus = { code: 0, text: "", url };
  const onNavigate = (_event, navigatedUrl, httpResponseCode, httpStatusText) => {
    navigationStatus = {
      code: Number(httpResponseCode || 0),
      text: httpStatusText || "",
      url: navigatedUrl || url,
    };
  };
  const loadPromise = new Promise((resolve, reject) => {
    const cleanup = () => {
      view.webContents.removeListener("did-fail-load", onFail);
      view.webContents.removeListener("did-navigate", onNavigate);
    };
    const onFail = (_event, errorCode, errorDescription, validatedURL, isMainFrame) => {
      if (!isMainFrame) return;
      cleanup();
      reject(new Error(`${errorDescription} (${errorCode}) loading '${validatedURL || url}'`));
    };
    view.webContents.on("did-navigate", onNavigate);
    view.webContents.on("did-fail-load", onFail);
    view.webContents.loadURL(url).then(
      () => {
        cleanup();
        resolve();
      },
      (error) => {
        cleanup();
        reject(error);
      },
    );
  });
  await loadPromise;
  if (navigationStatus.code >= 400) {
    throw new Error(`HTTP ${navigationStatus.code}${navigationStatus.text ? ` ${navigationStatus.text}` : ""} loading '${navigationStatus.url}'`);
  }
  return navigationStatus;
}

async function createWindow() {
  desktopServices = await startDesktopServices(app);
  const omegaUrl = desktopServices.web?.url || process.env.OMEGA_WEB_URL || "http://127.0.0.1:5173/";
  omegaAppURL = omegaUrl;

  mainWindow = new BrowserWindow({
    width: 1440,
    height: 960,
    minWidth: 1100,
    minHeight: 720,
    title: "Omega",
    icon: appIconPath,
    webPreferences: {
      preload: path.join(__dirname, "omega-preload.cjs"),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });
  if (process.platform === "darwin" && app.dock) {
    app.dock.setIcon(appIconPath);
  }

  if (desktopServices.web?.status === "static" && desktopServices.web.filePath) {
    mainWindow.loadFile(desktopServices.web.filePath);
  } else {
    mainWindow.loadURL(omegaUrl);
  }
  mainWindow.on("resize", layoutPreviewView);
  mainWindow.webContents.on("render-process-gone", (_event, details) => {
    const reason = details?.reason || "unknown";
    const exitCode = typeof details?.exitCode === "number" ? details.exitCode : "";
    console.error(`[omega-desktop] renderer stopped: ${reason}${exitCode !== "" ? ` (${exitCode})` : ""}`);
    showRendererRecoveryPage(reason, exitCode);
  });
  mainWindow.on("closed", () => {
    mainWindow = null;
    previewView = null;
  });

  const previewUrl = desktopServices.preview?.url || desktopServices.preview?.plan?.previewUrl;
  if (previewUrl && ["external", "running"].includes(desktopServices.preview?.status)) {
    try {
      const view = ensurePreviewView({ url: previewUrl, runtimeUrl: runtimeApiBase(), hostedInOmega: true }, { attach: false });
      await loadPreviewURLWithStatus(view, previewUrl);
      attachPreviewView();
    } catch (error) {
      console.warn(`[omega-desktop:preview] initial preview failed ${previewUrl}: ${error instanceof Error ? error.message : String(error)}`);
      closePreviewView();
    }
  }
}

ipcMain.handle("omega-preview:open", async (_event, url) => {
  const request = normalizePreviewRequest(url);
  if (!request.url) return { ok: false, error: "preview url is required" };
  console.log(`[omega-desktop:preview] open ${request.url}`);
  try {
    const view = ensurePreviewView(request.config, { attach: false });
    await loadPreviewURLWithStatus(view, request.url);
    attachPreviewView();
    console.log(`[omega-desktop:preview] opened ${request.url}`);
    return { ok: true, url: request.url };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    console.error(`[omega-desktop:preview] open failed ${request.url}: ${message}`);
    closePreviewView();
    return { ok: false, url: request.url, error: message };
  }
});

ipcMain.handle("omega-preview:reload", async (_event, input = {}) => {
  if (!previewView) return { ok: false, error: "preview is not open" };
  console.log(`[omega-desktop:preview] reload requested reason=${input?.reason || "unknown"} run=${input?.runId || ""}`);
  const runtime = await refreshPreviewRuntime(previewRuntimeSession, input || {});
  if (!runtime.ok) return sanitizePreviewRuntimeResult(runtime);
  if (runtime.child && desktopServices?.children && !desktopServices.children.includes(runtime.child)) {
    desktopServices.children.push(runtime.child);
  }
  if (runtime.child && previewRuntimeSession) previewRuntimeSession.child = runtime.child;
  previewView.webContents.reloadIgnoringCache();
  console.log(`[omega-desktop:preview] reloaded action=${runtime.action || "browser-reload"} strategy=${runtime.reloadStrategy || ""}`);
  return sanitizePreviewRuntimeResult({ ...runtime, ok: true, browserReload: true });
});

ipcMain.handle("omega-preview:close", async () => closePreviewView());

ipcMain.handle("omega-preview:resolve-target", async (_event, target) => {
  const result = await resolveRepositoryPreviewTarget(target);
  if (result.ok) {
    console.log(`[omega-desktop:preview] resolved target ${result.repoPath}`);
  } else {
    console.warn(`[omega-desktop:preview] resolve target failed: ${result.error}`);
  }
  return result;
});

ipcMain.handle("omega-preview:start-dev-server", async (_event, input) => {
  const target = input?.target;
  if (!target) return { ok: false, error: "repository target is required" };
  console.log(`[omega-desktop:preview-runtime] start ${target.id || target.repo || target.path || "target"}`);
  const result = await startRepositoryPreviewRuntime(target, {
    projectId: input?.projectId,
    repositoryTargetId: input?.repositoryTargetId || target.id,
    intent: input?.intent,
    previewUrl: input?.previewUrl,
  });
  if (result.child && desktopServices?.children) desktopServices.children.push(result.child);
  if (!result.ok) {
    console.warn(`[omega-desktop:preview-runtime] failed ${result.error || "unknown error"}`);
  } else {
    console.log(`[omega-desktop:preview-runtime] ready ${result.previewUrl}`);
    previewRuntimeSession = {
      target,
      plan: result.plan,
      profile: result.profile,
      child: result.child,
      previewUrl: result.previewUrl,
    };
  }
  return sanitizePreviewRuntimeResult(result);
});

ipcMain.handle("omega-preview:start-selection", async () => {
  if (!previewView) return { ok: false, error: "preview is not open" };
  previewView.webContents.send("omega-preview:set-selecting", true);
  return { ok: true };
});

ipcMain.handle("omega-app:reload", async () => {
  if (!mainWindow) return { ok: false, error: "Omega window is not open" };
  await reloadOmegaApp();
  return { ok: true };
});

ipcMain.on("omega-preview:selection", (_event, payload) => {
  if (!mainWindow) return;
  closePreviewView();
  mainWindow.webContents.send("omega-preview:selection", payload);
});

ipcMain.handle("omega-desktop:services", async () => ({
  runtime: sanitizeServiceState(desktopServices?.runtime),
  web: sanitizeServiceState(desktopServices?.web),
  preview: sanitizeServiceState(desktopServices?.preview),
}));

ipcMain.handle("omega-desktop:select-directory", async () => {
  if (!mainWindow) return undefined;
  const result = await dialog.showOpenDialog(mainWindow, {
    title: "Choose project directory",
    properties: ["openDirectory", "createDirectory"],
  });
  if (result.canceled || result.filePaths.length === 0) return undefined;
  return result.filePaths[0];
});

function sanitizeServiceState(service) {
  if (!service) return { status: "unknown" };
  const child = service.child
    ? {
        pid: service.child.pid,
        status: service.child.status,
        error: service.child.error,
        stdoutTail: service.child.stdoutTail,
        stderrTail: service.child.stderrTail,
      }
    : undefined;
  return {
    service: service.service,
    status: service.status,
    url: service.url,
    filePath: service.filePath,
    reason: service.reason,
    error: service.error,
    plan: service.plan
      ? {
          mode: service.plan.mode,
          source: service.plan.source,
          repoPath: service.plan.repoPath,
          previewUrl: service.plan.previewUrl,
        }
      : undefined,
    child,
  };
}

function sanitizePreviewRuntimeResult(result) {
  if (!result) return { ok: false, error: "Preview Runtime Agent did not return a result." };
  return {
    ok: Boolean(result.ok),
    agentId: result.agentId,
    stageId: result.stageId,
    status: result.status,
    action: result.action,
    reloadStrategy: result.reloadStrategy,
    browserReload: result.browserReload,
    repoPath: result.repoPath,
    previewUrl: result.previewUrl,
    profile: result.profile,
    health: result.health,
    message: result.message,
    error: result.error,
    child: result.child
      ? {
          pid: result.child.pid,
          status: result.child.status,
          error: result.child.error,
          stdoutTail: result.child.stdoutTail,
          stderrTail: result.child.stderrTail,
        }
      : undefined,
  };
}

async function reloadOmegaApp() {
  if (!mainWindow) return;
  if (desktopServices?.web?.status === "static" && desktopServices.web.filePath) {
    await mainWindow.loadFile(desktopServices.web.filePath);
    return;
  }
  await mainWindow.loadURL(omegaAppURL || desktopServices?.web?.url || process.env.OMEGA_WEB_URL || "http://127.0.0.1:5173/");
}

function showRendererRecoveryPage(reason, exitCode) {
  if (!mainWindow || mainWindow.isDestroyed()) return;
  const detail = `${reason || "unknown"}${exitCode !== "" ? ` (${exitCode})` : ""}`;
  const html = `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <title>Omega is recovering</title>
  <style>
    body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #eef5ff; color: #172033; }
    main { display: grid; min-height: 100vh; place-items: center; padding: 32px; }
    article { width: min(620px, 100%); border: 1px solid #bfd1eb; border-radius: 8px; background: #fff; padding: 28px; box-shadow: 0 20px 48px rgba(44, 78, 134, 0.14); }
    span { color: #7a8699; font-size: 12px; font-weight: 900; letter-spacing: .08em; text-transform: uppercase; }
    h1 { margin: 10px 0 8px; font-size: 28px; }
    p { margin: 0; color: #667085; line-height: 1.55; }
  </style>
</head>
<body>
  <main>
    <article>
      <span>Omega Desktop</span>
      <h1>Renderer restarted</h1>
      <p>The UI process stopped (${escapeHTML(detail)}). Omega is reopening the app instead of leaving a blank window.</p>
    </article>
  </main>
</body>
</html>`;
  mainWindow.loadURL(`data:text/html;charset=utf-8,${encodeURIComponent(html)}`).catch((error) => {
    console.error("[omega-desktop] failed to show recovery page", error);
  });
  setTimeout(() => {
    reloadOmegaApp().catch((error) => {
      console.error("[omega-desktop] renderer recovery reload failed", error);
    });
  }, 900);
}

function escapeHTML(value) {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

app.whenReady().then(createWindow).catch((error) => {
  console.error("[omega-desktop] failed to create window", error);
  app.quit();
});

app.on("window-all-closed", () => {
  app.quit();
});

app.on("before-quit", () => {
  stopDesktopServices(desktopServices);
});

app.on("activate", () => {
  if (BrowserWindow.getAllWindows().length === 0) createWindow();
});
