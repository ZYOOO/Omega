const { spawn } = require("node:child_process");
const fs = require("node:fs");
const http = require("node:http");
const https = require("node:https");
const os = require("node:os");
const path = require("node:path");

const DEFAULT_RUNTIME_URL = "http://127.0.0.1:3888/health";
const DEFAULT_WEB_URL = "http://127.0.0.1:5173/";
const DEFAULT_PREVIEW_URL = "http://127.0.0.1:3009/";
const PREVIEW_RUNTIME_AGENT_ID = "preview-runtime-agent";
const PREVIEW_RUNTIME_STAGE_ID = "preview_runtime";

function repoRootFromDesktopSource() {
  return path.resolve(__dirname, "../../..");
}

function envFlag(value, defaultValue = true) {
  if (value === undefined || value === "") return defaultValue;
  return !["0", "false", "no", "off"].includes(String(value).toLowerCase());
}

function logService(service, message, details = "") {
  const suffix = details ? ` ${details}` : "";
  console.log(`[omega-desktop:${service}] ${message}${suffix}`);
}

function httpGetStatus(url, timeoutMs = 1200) {
  return new Promise((resolve) => {
    let settled = false;
    let parsed;
    try {
      parsed = new URL(url);
    } catch (_error) {
      resolve({ ok: false, status: 0, error: "invalid-url" });
      return;
    }
    const client = parsed.protocol === "https:" ? https : http;
    const request = client.get(parsed, (response) => {
      response.resume();
      response.on("end", () => {
        if (settled) return;
        settled = true;
        resolve({ ok: response.statusCode >= 200 && response.statusCode < 400, status: response.statusCode || 0 });
      });
    });
    request.setTimeout(timeoutMs, () => {
      if (settled) return;
      settled = true;
      request.destroy();
      resolve({ ok: false, status: 0, error: "timeout" });
    });
    request.on("error", (error) => {
      if (settled) return;
      settled = true;
      resolve({ ok: false, status: 0, error: error.message });
    });
  });
}

async function waitForHttp(url, options = {}) {
  const timeoutMs = options.timeoutMs ?? 30000;
  const intervalMs = options.intervalMs ?? 500;
  const startedAt = Date.now();
  let last = { ok: false, status: 0, error: "not-started" };
  while (Date.now() - startedAt < timeoutMs) {
    last = await httpGetStatus(url, options.requestTimeoutMs ?? 1200);
    if (last.ok) return { ok: true, url, status: last.status };
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
  }
  return { ok: false, url, status: last.status, error: last.error || "timeout" };
}

function spawnManagedProcess(service, command, args, options = {}) {
  const child = spawn(command, args, {
    cwd: options.cwd,
    env: { ...process.env, ...(options.env || {}) },
    shell: Boolean(options.shell),
    detached: process.platform !== "win32",
    stdio: ["ignore", "pipe", "pipe"],
  });

  const record = {
    service,
    command,
    args,
    cwd: options.cwd,
    pid: child.pid,
    status: "starting",
    startedAt: new Date().toISOString(),
    stdoutTail: [],
    stderrTail: [],
    stop: () => stopManagedProcess(record),
  };
  record.child = child;

  const capture = (streamName, data) => {
    const text = String(data);
    const tail = streamName === "stdout" ? record.stdoutTail : record.stderrTail;
    tail.push(...text.split(/\r?\n/).filter(Boolean).slice(-12));
    while (tail.length > 60) tail.shift();
    for (const line of text.split(/\r?\n/).filter(Boolean)) {
      logService(service, line);
    }
  };
  child.stdout.on("data", (data) => capture("stdout", data));
  child.stderr.on("data", (data) => capture("stderr", data));
  child.on("error", (error) => {
    record.status = "failed";
    record.error = error.message;
    logService(service, "failed to start", error.message);
  });
  child.on("exit", (code, signal) => {
    record.status = code === 0 ? "exited" : "failed";
    record.exitCode = code;
    record.signal = signal;
    logService(service, "exited", `code=${code} signal=${signal || ""}`.trim());
  });
  return record;
}

function stopManagedProcess(record) {
  if (!record || !record.child || record.child.killed) return;
  record.status = "stopping";
  try {
    if (process.platform !== "win32" && record.child.pid) {
      process.kill(-record.child.pid, "SIGTERM");
    } else {
      record.child.kill("SIGTERM");
    }
  } catch (_error) {
    try {
      record.child.kill("SIGTERM");
    } catch (_secondError) {
      // Best-effort shutdown on app quit.
    }
  }
}

function detectPackageManager(repoPath) {
  if (fs.existsSync(path.join(repoPath, "pnpm-lock.yaml"))) return "pnpm";
  if (fs.existsSync(path.join(repoPath, "yarn.lock"))) return "yarn";
  if (fs.existsSync(path.join(repoPath, "bun.lockb")) || fs.existsSync(path.join(repoPath, "bun.lock"))) return "bun";
  return "npm";
}

function packageManagerCommand(packageManager, script, scriptCommand, port) {
  const argsByManager = {
    npm: ["run", script],
    pnpm: ["run", script],
    yarn: [script],
    bun: ["run", script],
  };
  const args = [...(argsByManager[packageManager] || argsByManager.npm)];
  const command = packageManager;
  const lower = String(scriptCommand || "").toLowerCase();
  if (script === "dev" && /\b(vite|astro)\b/.test(lower)) {
    if (packageManager === "yarn") args.push("--host", "127.0.0.1", "--port", String(port));
    else args.push("--", "--host", "127.0.0.1", "--port", String(port));
  } else if (script === "dev" && /\bnext\b/.test(lower)) {
    if (packageManager === "yarn") args.push("--hostname", "127.0.0.1", "-p", String(port));
    else args.push("--", "--hostname", "127.0.0.1", "-p", String(port));
  }
  return { command, args, shell: false };
}

function commandLine(command, args = []) {
  return [command, ...args].filter(Boolean).join(" ");
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function readJSON(filePath) {
  try {
    return JSON.parse(fs.readFileSync(filePath, "utf8"));
  } catch (_error) {
    return undefined;
  }
}

async function resolveRepositoryPreviewTarget(target, env = process.env) {
  const repoPath = await resolveRepositoryWorktree(target, env);
  if (!repoPath) {
    return {
      ok: false,
      error: "isolated preview workspace could not be prepared for the selected repository",
    };
  }
  const indexPath = path.join(repoPath, "index.html");
  const packagePath = path.join(repoPath, "package.json");
  return {
    ok: true,
    repoPath,
    htmlFile: fs.existsSync(indexPath) ? indexPath : "",
    hasPackageJson: fs.existsSync(packagePath),
    previewUrl: env.OMEGA_PREVIEW_URL || env.OMEGA_PAGE_PILOT_URL || DEFAULT_PREVIEW_URL,
  };
}

async function resolveRepositoryWorktree(target, env = process.env) {
  if (!target || typeof target !== "object") return "";
  if (target.kind === "local" && target.path) {
    const repoPath = path.resolve(String(target.path));
    return fs.existsSync(repoPath) ? repoPath : "";
  }
  if (target.kind !== "github" || !target.owner || !target.repo) return "";
  const repoPath = isolatedPreviewWorkspacePath(target, env);
  const gitDir = path.join(repoPath, ".git");
  if (!fs.existsSync(repoPath)) {
    fs.mkdirSync(path.dirname(repoPath), { recursive: true });
    await runCommand("git", ["clone", repositoryCloneURL(target), repoPath], {
      timeoutMs: Number(env.OMEGA_PREVIEW_CLONE_TIMEOUT_MS || 120000),
    });
    return repoPath;
  }
  if (!fs.existsSync(gitDir)) return "";
  if (!gitRemoteMatches(repoPath, String(target.owner), String(target.repo))) return "";
  const status = await runCommand("git", ["status", "--porcelain"], { cwd: repoPath, timeoutMs: 10000 });
  if (!status.stdout.trim()) {
    try {
      await runCommand("git", ["pull", "--ff-only"], { cwd: repoPath, timeoutMs: 60000 });
    } catch (error) {
      logService("preview", "workspace update skipped", error instanceof Error ? error.message : String(error));
    }
  }
  return repoPath;
}

function isolatedPreviewWorkspacePath(target, env = process.env) {
  const root = path.resolve(env.OMEGA_PAGE_PILOT_WORKSPACE_ROOT || path.join(os.homedir(), "Omega", "workspaces", "page-pilot"));
  return path.join(root, safePathSegment(`${target.owner}_${target.repo}`));
}

function repositoryCloneURL(target) {
  return target.url || `https://github.com/${target.owner}/${target.repo}.git`;
}

function safePathSegment(value) {
  const segment = String(value).replace(/[^a-zA-Z0-9._-]+/g, "-").replace(/^-+|-+$/g, "");
  return segment || "repository";
}

function runCommand(command, args, options = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: options.cwd,
      env: { ...process.env, ...(options.env || {}) },
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    let settled = false;
    const timeout = setTimeout(() => {
      if (settled) return;
      settled = true;
      child.kill("SIGTERM");
      reject(new Error(`${command} ${args.join(" ")} timed out`));
    }, options.timeoutMs || 30000);
    child.stdout.on("data", (data) => {
      stdout += String(data);
      if (stdout.length > 8000) stdout = stdout.slice(-8000);
    });
    child.stderr.on("data", (data) => {
      stderr += String(data);
      if (stderr.length > 8000) stderr = stderr.slice(-8000);
    });
    child.on("error", (error) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      reject(error);
    });
    child.on("exit", (code) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      if (code === 0) resolve({ stdout, stderr });
      else reject(new Error(`${command} ${args.join(" ")} failed: ${stderr || stdout || `exit ${code}`}`));
    });
  });
}

function uniquePaths(entries) {
  const seen = new Set();
  const result = [];
  for (const entry of entries) {
    const resolved = path.resolve(entry);
    if (seen.has(resolved)) continue;
    seen.add(resolved);
    result.push(resolved);
  }
  return result;
}

function gitRemoteMatches(repoPath, owner, repo) {
  const configPath = path.join(repoPath, ".git", "config");
  if (!fs.existsSync(configPath)) return false;
  const config = fs.readFileSync(configPath, "utf8").toLowerCase();
  const normalizedOwner = owner.toLowerCase();
  const normalizedRepo = repo.toLowerCase().replace(/\.git$/, "");
  return (
    config.includes(`github.com/${normalizedOwner}/${normalizedRepo}`) ||
    config.includes(`github.com:${normalizedOwner}/${normalizedRepo}`)
  );
}

function buildPreviewRuntimePlan(input = {}) {
  const env = input.env || process.env;
  const repoPath = path.resolve(env.OMEGA_PREVIEW_REPO_PATH || env.OMEGA_PAGE_PILOT_REPO_PATH || "");
  const previewUrl = env.OMEGA_PREVIEW_URL || env.OMEGA_PAGE_PILOT_URL || DEFAULT_PREVIEW_URL;
  const port = Number(env.OMEGA_PREVIEW_PORT || new URL(previewUrl).port || "5173");

  if (!env.OMEGA_PREVIEW_REPO_PATH && !env.OMEGA_PAGE_PILOT_REPO_PATH) {
    return { enabled: false, reason: "OMEGA_PREVIEW_REPO_PATH is not set", previewUrl };
  }
  if (!fs.existsSync(repoPath) || !fs.statSync(repoPath).isDirectory()) {
    return { enabled: false, reason: `preview repository path does not exist: ${repoPath}`, previewUrl, repoPath };
  }
  if (env.OMEGA_PREVIEW_COMMAND) {
    return {
      enabled: true,
      repoPath,
      previewUrl,
      command: env.OMEGA_PREVIEW_COMMAND,
      args: [],
      shell: true,
      source: "env-command",
    };
  }

  const packagePath = path.join(repoPath, "package.json");
  const packageJSON = readJSON(packagePath);
  const scripts = packageJSON?.scripts || {};
  const requestedScript = env.OMEGA_PREVIEW_SCRIPT;
  const script = requestedScript && scripts[requestedScript]
    ? requestedScript
    : ["dev", "start", "preview"].find((candidate) => scripts[candidate]);
  if (script) {
    const packageManager = env.OMEGA_PREVIEW_PACKAGE_MANAGER || detectPackageManager(repoPath);
    return {
      enabled: true,
      repoPath,
      previewUrl,
      ...packageManagerCommand(packageManager, script, scripts[script], port),
      source: `${packageManager}:${script}`,
    };
  }

  if (fs.existsSync(path.join(repoPath, "index.html"))) {
    return {
      enabled: true,
      repoPath,
      previewUrl,
      command: "python3",
      args: ["-m", "http.server", String(port), "--bind", "127.0.0.1"],
      shell: false,
      source: "static-index",
    };
  }

  return { enabled: false, reason: "no preview command could be detected", previewUrl, repoPath };
}

function previewRuntimeEvidence(repoPath) {
  const files = [
    "package.json",
    "pnpm-lock.yaml",
    "yarn.lock",
    "bun.lockb",
    "bun.lock",
    "vite.config.ts",
    "vite.config.js",
    "next.config.js",
    "next.config.mjs",
    "astro.config.mjs",
    "README.md",
    "index.html",
  ];
  return files.filter((file) => fs.existsSync(path.join(repoPath, file)));
}

function buildPreviewRuntimeProfile(input = {}) {
  const env = input.env || process.env;
  const repoPath = path.resolve(input.repoPath || env.OMEGA_PREVIEW_REPO_PATH || env.OMEGA_PAGE_PILOT_REPO_PATH || "");
  const previewUrl = input.previewUrl || env.OMEGA_PREVIEW_URL || env.OMEGA_PAGE_PILOT_URL || DEFAULT_PREVIEW_URL;
  const plan = buildPreviewRuntimePlan({
    env: {
      ...env,
      OMEGA_PREVIEW_REPO_PATH: repoPath,
      OMEGA_PREVIEW_URL: previewUrl,
    },
  });
  const evidence = fs.existsSync(repoPath) ? previewRuntimeEvidence(repoPath) : [];
  const devCommand = plan.command ? commandLine(plan.command, plan.args || []) : "";
  const profile = {
    agentId: PREVIEW_RUNTIME_AGENT_ID,
    stageId: PREVIEW_RUNTIME_STAGE_ID,
    repositoryTargetId: input.repositoryTargetId || "",
    workingDirectory: repoPath,
    installCommand: "",
    devCommand,
    previewUrl: plan.previewUrl || previewUrl,
    healthCheck: {
      url: plan.previewUrl || previewUrl,
      expectedStatus: 200,
    },
    reloadStrategy: plan.source === "static-index" ? "browser-reload" : "hmr-wait",
    source: plan.source || "not-detected",
    evidence,
    intent: input.intent || "",
    responsibilities: [
      "Read the locked repository workspace before starting preview.",
      "Resolve package manager, command, port, preview URL, and health check.",
      "Start only inside the selected repository workspace.",
      "Return an auditable profile before Electron opens the target page.",
    ],
    createdAt: new Date().toISOString(),
  };
  return { plan, profile };
}

function selectPreviewRefreshStrategy(profile = {}, options = {}) {
  const changedFiles = options.changedFiles || [];
  const source = String(profile.source || "");
  const configured = String(profile.reloadStrategy || "").trim();
  const restartPatterns = [
    /^package\.json$/,
    /^package-lock\.json$/,
    /^pnpm-lock\.yaml$/,
    /^yarn\.lock$/,
    /^bun\.lockb?$/,
    /^vite\.config\./,
    /^next\.config\./,
    /^astro\.config\./,
    /^docker-compose\./,
    /^Dockerfile$/,
    /^\.env(\.|$)/,
  ];
  if (changedFiles.some((file) => restartPatterns.some((pattern) => pattern.test(String(file))))) {
    return "server-restart";
  }
  if (configured) return configured;
  if (source === "static-index") return "browser-reload";
  return "hmr-wait";
}

async function startRepositoryPreviewRuntime(target, options = {}, env = process.env) {
  const repoPath = await resolveRepositoryWorktree(target, env);
  if (!repoPath) {
    return {
      ok: false,
      agentId: PREVIEW_RUNTIME_AGENT_ID,
      stageId: PREVIEW_RUNTIME_STAGE_ID,
      error: "Preview Runtime Agent could not prepare a repository workspace for the selected target.",
    };
  }
  const { plan, profile } = buildPreviewRuntimeProfile({
    env,
    repoPath,
    repositoryTargetId: options.repositoryTargetId || target?.id || "",
    previewUrl: options.previewUrl,
    intent: options.intent,
  });
  if (!plan.enabled) {
    return {
      ok: false,
      agentId: PREVIEW_RUNTIME_AGENT_ID,
      stageId: PREVIEW_RUNTIME_STAGE_ID,
      repoPath,
      previewUrl: plan.previewUrl,
      profile,
      error: plan.reason || "Preview Runtime Agent could not detect a dev server command.",
    };
  }
  logService("preview-runtime", "profile resolved", `${profile.source} ${profile.previewUrl}`);
  const service = await ensureService("preview-runtime", plan, {
    url: plan.previewUrl,
    timeoutMs: Number(env.OMEGA_PREVIEW_START_TIMEOUT_MS || 45000),
  });
  const ok = ["external", "running", "started"].includes(service.status);
  return {
    ok,
    agentId: PREVIEW_RUNTIME_AGENT_ID,
    stageId: PREVIEW_RUNTIME_STAGE_ID,
    status: service.status,
    repoPath,
    previewUrl: service.url || plan.previewUrl,
    plan,
    profile,
    error: ok ? "" : service.error || service.reason || "Preview Runtime Agent failed to start the dev server.",
    child: service.child,
  };
}

async function restartPreviewRuntimeService(session, timeoutMs) {
  if (!session?.plan?.command) {
    return {
      ok: false,
      error: "Preview Runtime Supervisor cannot restart because the profile has no dev command.",
      status: "failed",
    };
  }
  if (session.child) {
    stopManagedProcess(session.child);
    await delay(500);
  }
  const service = await ensureService("preview-runtime", session.plan, {
    url: session.plan.previewUrl,
    timeoutMs: timeoutMs || 45000,
  });
  return service;
}

async function refreshPreviewRuntime(session, options = {}) {
  if (!session?.profile) {
    return {
      ok: true,
      agentId: PREVIEW_RUNTIME_AGENT_ID,
      stageId: PREVIEW_RUNTIME_STAGE_ID,
      action: "browser-reload",
      reloadStrategy: "browser-reload",
      browserReload: true,
      status: "no-profile",
      message: "No Preview Runtime Profile is active; falling back to browser reload.",
    };
  }
  const profile = session.profile;
  const healthUrl = profile.healthCheck?.url || profile.previewUrl || session.plan?.previewUrl;
  const reloadStrategy = selectPreviewRefreshStrategy(profile, options);
  const timeoutMs = Number(options.timeoutMs || process.env.OMEGA_PREVIEW_RELOAD_TIMEOUT_MS || 45000);
  let action = reloadStrategy;
  let service = null;
  let health = healthUrl ? await httpGetStatus(healthUrl, 1200) : { ok: true, status: 0 };

  if (reloadStrategy === "server-restart" || !health.ok) {
    action = "server-restart";
    service = await restartPreviewRuntimeService(session, timeoutMs);
    if (!["external", "running", "started"].includes(service.status)) {
      return {
        ok: false,
        agentId: PREVIEW_RUNTIME_AGENT_ID,
        stageId: PREVIEW_RUNTIME_STAGE_ID,
        action,
        reloadStrategy,
        browserReload: false,
        status: service.status,
        previewUrl: session.plan?.previewUrl || profile.previewUrl,
        profile,
        child: service.child,
        error: service.error || service.reason || "Preview Runtime Supervisor failed to restart the dev server.",
      };
    }
    session.child = service.child || session.child;
    health = { ok: true, status: 200 };
  } else if (reloadStrategy === "hmr-wait") {
    const wait = await waitForHttp(healthUrl, { timeoutMs: Math.min(timeoutMs, 10000), intervalMs: 400 });
    health = wait;
    if (!wait.ok) {
      return {
        ok: false,
        agentId: PREVIEW_RUNTIME_AGENT_ID,
        stageId: PREVIEW_RUNTIME_STAGE_ID,
        action,
        reloadStrategy,
        browserReload: false,
        status: "failed",
        previewUrl: healthUrl,
        profile,
        error: wait.error || "Preview Runtime health check failed after source changes.",
      };
    }
  }

  return {
    ok: true,
    agentId: PREVIEW_RUNTIME_AGENT_ID,
    stageId: PREVIEW_RUNTIME_STAGE_ID,
    action,
    reloadStrategy,
    browserReload: true,
    status: service?.status || "ready",
    previewUrl: service?.url || healthUrl || profile.previewUrl,
    profile,
    child: service?.child || session.child,
    health,
  };
}

function buildWebLaunchPlan(app, env = process.env) {
  const repoRoot = repoRootFromDesktopSource();
  const webUrl = env.OMEGA_WEB_URL || DEFAULT_WEB_URL;
  const webPort = new URL(webUrl).port || "5173";
  const mode = env.OMEGA_DESKTOP_WEB_MODE || (app?.isPackaged ? "static" : "dev");
  if (mode === "static") {
    const filePath = env.OMEGA_WEB_DIST_INDEX || path.join(repoRoot, "dist", "apps", "web", "index.html");
    return { mode: "static", filePath, url: webUrl, repoRoot };
  }
  return {
    mode: "dev",
    url: webUrl,
    repoRoot,
    command: env.OMEGA_WEB_COMMAND || "npm",
    args: env.OMEGA_WEB_COMMAND ? [] : ["run", "web:dev", "--", "--host", "127.0.0.1", "--port", webPort],
    shell: Boolean(env.OMEGA_WEB_COMMAND),
  };
}

function buildRuntimeLaunchPlan(env = process.env) {
  const repoRoot = repoRootFromDesktopSource();
  const runtimeUrl = env.OMEGA_RUNTIME_URL || DEFAULT_RUNTIME_URL;
  return {
    url: runtimeUrl,
    repoRoot,
    command: env.OMEGA_RUNTIME_COMMAND || "go",
    args: env.OMEGA_RUNTIME_COMMAND
      ? []
      : ["run", "./services/local-runtime/cmd/omega-local-runtime", "--host", "127.0.0.1", "--port", "3888"],
    shell: Boolean(env.OMEGA_RUNTIME_COMMAND),
  };
}

async function ensureService(service, plan, options = {}) {
  if (options.disabled) return { service, status: "disabled", plan };
  const url = options.url || plan.url || plan.previewUrl;
  if (url) {
    const probe = await httpGetStatus(url, options.requestTimeoutMs ?? 1000);
    if (probe.ok) {
      logService(service, "already running", url);
      return { service, status: "external", url, plan };
    }
  }
  if (!plan.command) return { service, status: "skipped", reason: plan.reason || "no command", plan };
  const child = spawnManagedProcess(service, plan.command, plan.args || [], {
    cwd: plan.repoRoot || plan.repoPath,
    shell: plan.shell,
  });
  if (!url) return { service, status: "started", child, plan };
  const ready = await waitForHttp(url, { timeoutMs: options.timeoutMs ?? 30000 });
  if (!ready.ok) {
    child.status = "failed";
    child.error = ready.error || "health check failed";
    logService(service, "health check failed", `${url} ${child.error || ""}`.trim());
    return { service, status: "failed", url, child, plan, error: child.error };
  }
  child.status = "running";
  logService(service, "ready", url);
  return { service, status: "running", url, child, plan };
}

async function startDesktopServices(app, env = process.env) {
  if (!envFlag(env.OMEGA_DESKTOP_AUTOSTART, true)) {
    return {
      web: { status: "disabled", url: env.OMEGA_WEB_URL || DEFAULT_WEB_URL },
      runtime: { status: "disabled" },
      preview: { status: "disabled" },
      children: [],
    };
  }

  const runtimePlan = buildRuntimeLaunchPlan(env);
  const webPlan = buildWebLaunchPlan(app, env);
  const previewPlan = buildPreviewRuntimePlan({ env });
  const children = [];

  const runtime = await ensureService("runtime", runtimePlan, {
    disabled: !envFlag(env.OMEGA_RUNTIME_AUTOSTART, true),
    url: runtimePlan.url,
    timeoutMs: Number(env.OMEGA_RUNTIME_START_TIMEOUT_MS || 30000),
  });
  if (runtime.child) children.push(runtime.child);

  let web;
  if (webPlan.mode === "static") {
    if (fs.existsSync(webPlan.filePath)) {
      web = { service: "web", status: "static", filePath: webPlan.filePath, plan: webPlan };
    } else {
      web = { service: "web", status: "failed", error: `web static entry not found: ${webPlan.filePath}`, plan: webPlan };
      logService("web", web.error);
    }
  } else {
    web = await ensureService("web", webPlan, {
      disabled: !envFlag(env.OMEGA_WEB_AUTOSTART, true),
      url: webPlan.url,
      timeoutMs: Number(env.OMEGA_WEB_START_TIMEOUT_MS || 30000),
    });
    if (web.child) children.push(web.child);
  }

  let preview = { service: "preview", status: "skipped", reason: previewPlan.reason, plan: previewPlan };
  if (previewPlan.enabled && envFlag(env.OMEGA_PREVIEW_AUTOSTART, true)) {
    preview = await ensureService("preview", previewPlan, {
      url: previewPlan.previewUrl,
      timeoutMs: Number(env.OMEGA_PREVIEW_START_TIMEOUT_MS || 45000),
    });
    if (preview.child) children.push(preview.child);
  } else if (previewPlan.reason) {
    logService("preview", "skipped", previewPlan.reason);
  }

  return { runtime, web, preview, children };
}

function stopDesktopServices(state) {
  for (const child of state?.children || []) {
    stopManagedProcess(child);
  }
}

module.exports = {
  DEFAULT_PREVIEW_URL,
  DEFAULT_RUNTIME_URL,
  DEFAULT_WEB_URL,
  buildPreviewRuntimePlan,
  buildRuntimeLaunchPlan,
  buildPreviewRuntimeProfile,
  buildWebLaunchPlan,
  detectPackageManager,
  envFlag,
  httpGetStatus,
  resolveRepositoryPreviewTarget,
  resolveRepositoryWorktree,
  refreshPreviewRuntime,
  selectPreviewRefreshStrategy,
  startRepositoryPreviewRuntime,
  startDesktopServices,
  stopDesktopServices,
  waitForHttp,
};
