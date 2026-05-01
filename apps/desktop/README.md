# Omega Desktop Shell

This directory is reserved for the Electron desktop shell.

The desktop shell should not replace the web development loop. It will package and launch:

- the React SPA from `apps/web`
- the Go local runtime binary from `services/local-runtime`

Target responsibilities:

- start and stop the local runtime process
- wait for `http://127.0.0.1:3888/health`
- open the React app in Electron Chromium
- provide desktop-only affordances such as workspace folder selection, preview webviews, and deep-link callbacks

Development mode can now start the Omega services from the Electron main process:

```bash
npm run desktop
```

The shell will:

- reuse an already-running Go local runtime on `http://127.0.0.1:3888/health`, or start `go run ./services/local-runtime/cmd/omega-local-runtime`
- reuse an already-running Omega Web UI on `http://127.0.0.1:5174/`, or start `npm run web:dev -- --host 127.0.0.1 --port 5174`
- optionally start a target project preview server when `OMEGA_PREVIEW_REPO_PATH` is set

Target preview examples:

```bash
OMEGA_PREVIEW_REPO_PATH=/Users/zyong/Projects/TestRepo npm run desktop
OMEGA_PREVIEW_REPO_PATH=/Users/zyong/Projects/TestRepo OMEGA_PREVIEW_COMMAND="npm run dev -- --host 127.0.0.1 --port 5173" npm run desktop
```

If `OMEGA_PREVIEW_COMMAND` is omitted, the shell tries a conservative local profile:

- `package.json` scripts: `dev`, then `start`, then `preview`
- package manager from lockfile: `pnpm`, `yarn`, `bun`, otherwise `npm`
- static `index.html` fallback through `python3 -m http.server`

The target preview is only started from an explicit `OMEGA_PREVIEW_REPO_PATH` / `OMEGA_PAGE_PILOT_REPO_PATH`. The shell does not guess from the Omega cwd.

From the Page Pilot launcher, choosing `Dev server by Agent` uses the selected Repository Workspace instead of a raw URL. The Electron Preview Runtime Agent resolves the local or isolated workspace, records a preview profile, starts the dev server inside that workspace, waits for the health check, and only then opens the direct pilot BrowserView.

Page Pilot preview refreshes also go through the Preview Runtime Supervisor. The target page can request reload after apply/discard or from the `Reload` action; the supervisor checks the active profile, waits for HMR when possible, restarts the dev server when runtime files changed or health checks fail, and then refreshes the BrowserView.

The older manual development loop still works:

```bash
npm run local-runtime:dev
npm run web:dev -- --host 127.0.0.1 --port 5174
npm run desktop:dev
```

The development shell loads `OMEGA_WEB_URL` or `http://127.0.0.1:5174/` by default. It keeps the current architecture:

- React SPA remains the Omega product UI.
- Go local runtime remains the execution engine and API server.
- Electron adds desktop-only browser capabilities for Page Pilot preview, selection injection, reload, and future process management.

Packaging will later add platform-specific Go binary builds and an app builder config.
