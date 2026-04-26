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

Development remains:

```bash
npm run local-runtime:dev
npm run web:dev
```

Packaging will later add Electron dependencies, platform-specific Go binary builds, and an app builder config.
