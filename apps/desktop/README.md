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

Development mode does not require packaging. Run the existing web and runtime services, then start Electron as a desktop shell:

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
