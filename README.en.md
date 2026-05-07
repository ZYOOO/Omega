# Omega AI Delivery Engine

Chinese version: [README.md](README.md)

Omega is a **local-first AI DevFlow engine** that turns requirements into repository-bound delivery work. It combines a Workboard, Workflow Templates, multi-agent orchestration, isolated Repository Workspaces, GitHub pull requests and CI, Feishu human review, Page Pilot visual editing, and proof artifacts into one auditable delivery loop.

Omega is currently `v0Beta`. It is optimized for local demos, local development, and single-user or small-team trials: source code, credentials, workspaces, the SQLite runtime database, and agent configuration stay on your machine.

## Core Capabilities

- **AI DevFlow Workboard**
  - Create Work Items from Requirements.
  - Bind every run to an explicit Repository Workspace.
  - Execute Requirement, Plan / Architect, Coding, Testing, GitHub PR, GitHub Actions CI, Review, Human Review, Rework, Merging, and Done through a Workflow Template.
  - Show Plan, TODOs, Acceptance Criteria, Validation, Review Packet, Blockers, Retry Reason, Notes, PR, Proof, and per-stage Agent statistics in the Work Item detail page.

- **Page Pilot**
  - Open a real preview page.
  - Select real DOM elements and describe the desired change.
  - Let an Agent modify source code using selectors, text snapshots, page context, and repository boundaries.
  - Confirm or discard the change; confirmed changes become Work Items, Pipelines, diffs, proof records, and PR-ready repository changes.

- **GitHub Delivery**
  - Create branches, commits, and pull requests.
  - Read PR diffs.
  - Collect GitHub Actions checks and failed run logs.
  - Persist PR, CI, review, and merge output as proof.

- **Feishu Human Review**
  - Send Human Review checkpoints to Feishu.
  - Use Feishu Task review: completing a task means approve; task comments can become request changes.
  - Omega UI, Feishu callback, and Feishu Task bridge share the same checkpoint decision path.

- **Configurable Agents and Workflows**
  - Supports Codex, Claude Code, opencode, and Trae Agent.
  - Supports OpenAI-compatible providers such as Kimi / Moonshot.
  - Built-in templates include `devflow-pr` and `saas-launch`; templates can define stages, agents, actions, artifacts, transitions, review rounds, and human gates.

## Architecture

```text
React Web UI / Electron Desktop
  -> Go Local Runtime API
      -> SQLite session / execution read models
      -> Workflow Contract State Runner
      -> Action Executor
      -> Agent Runner Registry
          -> Codex / Claude Code / opencode / Trae Agent
      -> Isolated Repository Workspace
          -> git / gh / GitHub PR / GitHub Actions / merge
      -> Proof / Runtime Logs / Run Workpads / Checkpoints
```

The Go Local Runtime is the source of truth. Web UI, Electron, CLI, Page Pilot, Feishu callbacks, and JobSupervisor all use the same local API.

See [docs/architecture.md](docs/architecture.md) for details.

## Requirements

Recommended development environment:

- macOS. Desktop packaging and Electron direct Page Pilot are currently verified primarily on macOS.
- Node.js 22+. Node 20 can run most local development commands, but Electron tooling is happier with Node 22.12+.
- npm.
- Go 1.21+.
- Git.
- GitHub CLI `gh`.

Verified environment for this submission:

```text
macOS arm64
Node.js v20.19.4
npm 10.8.2
Go go1.21.6 darwin/arm64
Git 2.39.3 (Apple Git-145)
GitHub CLI 2.88.0
Electron 41.3.0
electron-builder 26.8.1
Vite 7.1.12
TypeScript 5.9.3
Vitest 3.2.4
```

Optional integrations:

- Codex CLI, Claude Code CLI, opencode, Trae Agent / trae-cli.
- `lark-cli` for Feishu messages, Feishu Tasks, and current-user fallback.

## GitHub Setup

```bash
gh auth login
gh auth status
```

Recommended scopes:

```text
repo
workflow
read:org optional
```

If scopes are missing:

```bash
gh auth refresh -s repo -s workflow -s read:org
```

For demos, initialize target repositories with a `README.md` on `main` before running Omega. This avoids GitHub treating Omega's first feature branch as the default branch.

## Feishu Setup

Recommended local path: **Feishu custom app + lark-cli + Task review**.

1. Create an internal app in the Feishu developer platform.
2. Copy App ID and App Secret.
3. Enable the bot capability.
4. Add permissions:
   - `im:message`
   - `im:message:send_as_bot`
   - `im:chat`
   - `task:task:read`
   - `task:task:write`
5. Publish and install the app to your tenant.
6. Configure local `lark-cli`:

```bash
lark-cli config init
lark-cli doctor
lark-cli auth scopes
```

For current-user fallback or reviewer lookup:

```bash
lark-cli auth login
lark-cli contact +get-user --as user --format json
```

Then configure Omega in:

```text
Settings -> Provider access -> Feishu
```

See [docs/feishu-bot-permissions.md](docs/feishu-bot-permissions.md) for the full permission guide.

## Download

macOS Apple Silicon users can install Omega directly from the GitHub Release `.dmg`:

- [Omega-0.1.0-arm64.dmg](https://github.com/ZYOOO/Omega/releases/download/v0.1.0/Omega-0.1.0-arm64.dmg)
- [Omega v0.1.0 Release page](https://github.com/ZYOOO/Omega/releases/tag/v0.1.0)

This build is ad-hoc signed and not notarized. If macOS blocks it, right-click Omega and choose `Open`, or allow it from `System Settings -> Privacy & Security`.

## Install

```bash
git clone <repo-url> omega
cd omega
npm install
```

For repeatable CI-style installs:

```bash
npm ci
```

## Run From Source

Use three terminals.

Terminal 1:

```bash
npm run local-runtime:dev
```

Default runtime:

```text
http://127.0.0.1:3888
```

Terminal 2:

```bash
npm run web:dev
```

Open:

```text
http://127.0.0.1:5173
```

Terminal 3:

```bash
npm run desktop:dev
```

## Basic Flow

1. Open Omega.
2. Check GitHub / Feishu / Agent Access in Settings.
3. Bind a GitHub repository or local repository path.
4. Choose a Workflow Template in Agent Studio.
5. Create a Requirement.
6. Convert it into a Work Item.
7. Run DevFlow.
8. Inspect Plan, TODOs, Validation, Review Packet, Agent statistics, PR, CI, and Proof.
9. Approve or request changes in Omega or Feishu.
10. Continue to Merging / Done and inspect final proof.

## Page Pilot Flow

1. Open Page Pilot.
2. Select a Repository Workspace.
3. Start the Preview Runtime Agent for package projects, or open `index.html` for static projects.
4. Select DOM elements in the preview.
5. Describe the desired change.
6. Choose an Agent runner.
7. Confirm to materialize the change, or discard the run.

## CLI

```bash
npm run omega -- health
npm run omega -- work-items list
npm run omega -- attempts list --limit 20
npm run omega -- operations list --limit 20
npm run omega -- workpads list --limit 20
npm run omega -- proof list --limit 20
```

See [docs/omega-cli.md](docs/omega-cli.md).

## Build Desktop App

```bash
npm run release:prepare
CSC_IDENTITY_AUTO_DISCOVERY=false npm run desktop:pack
CSC_IDENTITY_AUTO_DISCOVERY=false npm run desktop:dist
```

Expected artifacts:

```text
dist/desktop/Omega-0.1.0-arm64.dmg
dist/desktop/Omega-0.1.0-mac-arm64.zip
dist/release/bin/omega
dist/release/bin/omega-local-runtime
```

See [docs/release-packaging.md](docs/release-packaging.md).

## Roadmap

- Visual Workflow Template editing: stage drag-and-drop, transitions, version history, import/export.
- Richer Agent observability: token usage, duration, cost, retry count, failure rate.
- Better Page Pilot source mapping from DOM selections to source files and line numbers.
- Team sync and remote control plane support.
- Long-term retention policies for stdout, stderr, runner details, and proof previews.
- More delivery templates for bugfix, CI repair, frontend iteration, documentation, and SaaS launch workflows.

## Public Docs

- [Architecture](docs/architecture.md)
- [Product summary](docs/product.md)
- [Data model](docs/data-model.md)
- [Page Pilot](docs/page-pilot.md)
- [GitHub Actions CI](docs/github-actions-ci-chain.md)
- [Feishu review](docs/feishu-review-chain.md)
- [Feishu permissions](docs/feishu-bot-permissions.md)
- [Agent Skills / MCP](docs/agent-skills-and-mcp.md)
- [CLI](docs/omega-cli.md)
- [Demo playbook](docs/demo-playbook.md)
- [Release packaging](docs/release-packaging.md)
- [OpenAPI](docs/openapi.yaml)
- [TODO](docs/todo.md)
- [Bug log](docs/bug-log.md)
