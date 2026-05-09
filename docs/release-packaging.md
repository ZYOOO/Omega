# Omega Desktop Release Packaging

This guide describes how to build Omega as a desktop app and publish the artifacts to a GitHub Release.

## Current Release Status

The macOS packaging path is available and has been dry-run locally:

- `npm run release:prepare` builds the React Web UI and Go binaries.
- `npm run desktop:pack` builds an unpacked `.app` for local smoke testing.
- `npm run desktop:dist` builds release artifacts under `dist/desktop`.

The local dry-run produced:

```text
dist/desktop/Omega-0.1.0-arm64.dmg
dist/desktop/Omega-0.1.0-mac-arm64.zip
dist/desktop/latest-mac.yml
```

The packaged app contains:

- the Electron desktop shell;
- static Web UI assets under `Resources/web`;
- `omega-local-runtime` and `omega` Go binaries under `Resources/bin`;
- `docs/openapi.yaml`;
- built-in workflow contracts under `Resources/workflows`, including `devflow-pr` and `saas-launch`.

## Required Environment

Build machine:

- macOS for macOS `.dmg` / `.zip` artifacts.
- Node.js 22 or newer. Node 20 can run the current local build, but `electron-builder` dependencies warn that Node 22.12+ is expected.
- npm.
- Go 1.21 or newer.
- GitHub CLI `gh` if publishing manually.
- Git.

Verified versions for this submission:

```text
Node.js v20.19.4
npm 10.8.2
Go go1.21.6 darwin/arm64
Git 2.39.3 (Apple Git-145)
GitHub CLI 2.88.0
Electron 41.3.0
electron-builder 26.8.1
```

Runtime tools for end users:

- GitHub delivery features require `git` and GitHub CLI `gh` login.
- Codex / Claude Code / opencode / Trae Agent features require those CLIs or runner credentials configured on the user's machine.
- Feishu features require the configured Feishu delivery route or `lark-cli` current-user fallback.

Optional signing/notarization for public macOS releases:

- Apple Developer ID certificate.
- `CSC_LINK` and `CSC_KEY_PASSWORD` for `electron-builder` signing, or a locally installed signing identity.
- `APPLE_ID`, `APPLE_APP_SPECIFIC_PASSWORD`, and `APPLE_TEAM_ID` for notarization.

Unsigned local builds can be generated with:

```bash
CSC_IDENTITY_AUTO_DISCOVERY=false npm run desktop:dist
```

## Build Commands

Install dependencies:

```bash
npm ci
```

Run validation:

```bash
npm run lint
npm run test -- --reporter=dot
npm run go:test:focused
```

Prepare release inputs:

```bash
npm run release:prepare
```

Create an unpacked desktop app for smoke testing:

```bash
CSC_IDENTITY_AUTO_DISCOVERY=false npm run desktop:pack
```

Create distributable macOS artifacts:

```bash
CSC_IDENTITY_AUTO_DISCOVERY=false npm run desktop:dist
```

For a signed and notarized release, export the signing/notarization environment first, then run:

```bash
npm run desktop:dist
```

## Smoke Test

After `desktop:pack`, verify that the packaged app contains the expected resources:

```bash
APP="dist/desktop/mac-arm64/Omega.app"
test -f "$APP/Contents/Resources/web/index.html"
test -x "$APP/Contents/Resources/bin/omega-local-runtime"
test -f "$APP/Contents/Resources/docs/openapi.yaml"
test -f "$APP/Contents/Resources/workflows/devflow-pr.md"
```

Verify the packaged runtime binary:

```bash
TMPROOT="$(mktemp -d /tmp/omega-release-smoke.XXXXXX)"
"$APP/Contents/Resources/bin/omega-local-runtime" \
  --host 127.0.0.1 \
  --port 3899 \
  --database "$TMPROOT/.omega/omega.db" \
  --workspace-root "$TMPROOT/workspaces" \
  --openapi "$APP/Contents/Resources/docs/openapi.yaml" \
  --job-supervisor=false
```

Then open `http://127.0.0.1:3899/health` and stop the process.

Verify the packaged CLI binary:

```bash
"$APP/Contents/Resources/bin/omega" --help
"$APP/Contents/Resources/bin/omega" --api-url http://127.0.0.1:3899 health
```

## Publishing To GitHub Release

Recommended manual flow:

```bash
VERSION="v0.1.0"
git tag "$VERSION"
git push origin "$VERSION"

gh release create "$VERSION" \
  "dist/desktop/Omega-0.1.0-arm64.dmg" \
  "dist/desktop/Omega-0.1.0-arm64.dmg.blockmap" \
  "dist/desktop/Omega-0.1.0-mac-arm64.zip" \
  "dist/desktop/Omega-0.1.0-mac-arm64.zip.blockmap" \
  "dist/desktop/latest-mac.yml" \
  --draft \
  --title "Omega 0.1.0" \
  --notes "Initial desktop release."
```

`electron-builder` can also publish when `GH_TOKEN` is available:

```bash
GH_TOKEN="..." npm run release:github
```

Use a draft release first. Download and install the draft artifact on a clean machine before marking the release public.

## Installation

macOS:

1. Download the `.dmg` from the GitHub Release.
2. Open the `.dmg`.
3. Drag `Omega.app` to `Applications`.
4. Launch the app.

For unsigned local builds, macOS Gatekeeper may block the first launch. For internal testing only:

```bash
xattr -dr com.apple.quarantine "/Applications/Omega.app"
```

Public releases should be signed and notarized instead of asking users to remove quarantine.

## First Run

On launch, the desktop shell starts:

- the bundled Go local runtime on `127.0.0.1:3888`;
- the bundled static Web UI from `Resources/web`.

Runtime data is stored under the app user data directory:

```text
~/Library/Application Support/Omega/.omega/omega.db
~/Library/Application Support/Omega/workspaces
```

## Basic Usage

1. Open `Settings`.
2. Configure GitHub access with local `gh` login or OAuth settings.
3. Configure Agent access:
   - Codex and Claude Code inherit local CLI accounts.
   - opencode and Trae Agent can use provider/model/base URL/API key settings.
4. Bind a Repository Workspace from `Projects`.
5. Create or import a Requirement.
6. Convert it into a Work Item.
7. Run the DevFlow.
8. Review the Work Item detail page:
   - Plan;
   - Acceptance Criteria;
   - Validation;
   - Review Packet;
   - stage Agent statistics;
   - PR, blockers, retry reason, notes, proof.
9. Approve or request changes at Human Review.

## Known Release Gaps

- The local dry-run used ad-hoc macOS signing and skipped notarization.
- A custom app icon has not been configured yet; Electron's default icon is currently used.
- Windows and Linux packaging configurations exist but have not been smoke-tested in this run.
- Auto-update metadata is generated for macOS, but an in-app updater flow has not been wired.
