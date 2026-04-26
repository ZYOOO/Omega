# Omega Local DevFlow Manual Testing Guide

This guide is for testing Omega with a real local workspace and a real GitHub repository.

## 1. Current Reality

Omega currently supports:

- Creating and viewing Workboard items locally
- Running a local Mission Control API with safe runner presets
- Producing local proof files
- Reading GitHub repositories through the local `gh` login state
- Binding a GitHub repository as a Project repository workspace
- Creating Omega requirements inside a repository workspace and running them against that repo
- Turning an incoming requirement into a structured requirement artifact through the `master` orchestration agent
- Creating a Pipeline run that includes stage dependencies, agent contracts, artifact handoff contracts, and dataFlow
- Importing GitHub issues as external Workboard sources
- Running a local code-change proof loop in an isolated clone
- Running the `devflow-pr` cycle: branch, commit, PR, Review Agent verdict, Human Review checkpoint, approved merge proof, and handoff bundle
- Configuring GitHub OAuth from the App, with `gh` login as a local-first fallback
- Sending basic Feishu/lark-cli text notifications

Omega does not yet fully support:

- Fully editable App-side Pipeline templates
- Fully model-driven code generation for arbitrary complex requirements
- Real Feishu card approval callbacks
- Full GitHub issue status writeback and PR delivery gates from the UI

The recommended test path is:

1. sign in with `gh auth login`
2. select a real GitHub repository in Omega and create/open its repository workspace
3. create an Omega requirement in that workspace
4. click `Run` on the created Item
5. verify branch / commit / PR / review proof in the isolated runner workspace, then approve the Human Review checkpoint and verify merge proof
6. optionally import GitHub issues and run the same execution path from an imported issue

## 2. Prepare A Real GitHub Repository And Login

Create or choose a repository on GitHub, for example:

```text
omega-mission-test
```

Then make sure local `gh` is authenticated:

```bash
gh auth status
```

Omega reads repositories through this local login state for the local-first demo. GitHub OAuth is still part of the product direction, but `gh` keeps the local App demo simple and avoids requiring a deployed callback.

## 3. Configure Environment

Create `.env` in the project root:

```bash
VITE_MISSION_CONTROL_API_URL=http://127.0.0.1:3888
VITE_GITHUB_CLIENT_ID=
VITE_GOOGLE_CLIENT_ID=
```

Only `VITE_MISSION_CONTROL_API_URL` is required for local runner testing.

GitHub client id is only needed when testing real OAuth.

## 4. Start Services

Terminal 1:

```bash
npm run local-runtime:dev
```

Terminal 2:

```bash
npm run web:dev
```

Open:

```text
http://localhost:5173
```

## 5. App Requirement -> Repository Workspace -> Runner Test

1. Open the app.
2. Open `Projects`.
3. In `GitHub repositories`, choose a repository and click `Create workspace`.
4. Omega should open the repository workspace under `Work items`.
5. Click `New requirement` and create a requirement.
6. Leave the target field empty; Omega should inherit the active repository workspace.
7. Example:

```text
Title: Add empty markdown proof
Description: Create an empty markdown file and produce local runner proof.
Assignee: requirement
```

8. Click `Run`.

Expected result:

- The Run button enters a running/disabled state quickly; the request should not wait for the whole cycle to finish.
- Runner message reports that the pipeline started, then polling updates the row and detail view until completion or failure.
- The Go local service clones the active repo into an isolated workspace under the configured workspace root.
- It creates an `omega/...` branch.
- It creates a commit, opens a PR, records review proof under `.omega/proof`, then stops at a Human Review checkpoint.
- After the user approves the checkpoint, it records human / merge / delivery proof under `.omega/proof`.
- The Work item came from `source = manual`; it did not need to start from a GitHub issue.
- The corresponding Requirement contains `master` dispatch metadata and suggested stage work.
- The detail page shows an Attempt record, stage progress, Agent orchestration, proof files, branch, PR, and any error.

## 6. GitHub Issue Import Test

1. Create or choose an issue in the selected GitHub repository.
2. In the repository workspace sidebar, click `Sync issues`.
3. The GitHub issue should appear as a Work item with `source = github_issue`.
4. Click `Run`.

Expected result:

- The imported issue runs through the same local runner path as the App-created requirement.
- GitHub issue URLs are resolved back to the owning repository before clone.
- The runner produces branch / commit / diff / summary proof in an isolated workspace.

## 7. Demo Code Runner Test

Use this when you want to show that Omega can produce real code changes without spending model tokens.

1. Create or select a work item whose target points to a real local git repository or GitHub repository workspace.
2. In the UI, choose runner:

```text
demo-code
```

3. Click `Run` or `Run stage`, depending on whether you are testing from Workboard or Operator view.

Expected result:

- Omega clones the target repo into its isolated local workspace
- Creates an `omega/...` branch
- Writes `src/omega-demo-change.ts`
- Commits the change
- Stores `git-diff.patch` and `change-summary.md` proof files

## 8. Codex Runner Test

Only run this after local-proof works.

1. Confirm Codex CLI works:

```bash
codex exec --skip-git-repo-check "Reply with exactly: OMEGA_OK"
```

2. Create or select a work item whose `Local repository path` points to a real local git repository.
3. Move the pipeline to the `coding` stage.
4. In the UI, choose runner:

```text
codex
```

5. Click `Run stage`.

Expected result:

- Codex runs inside an isolated clone of the target repository
- Codex produces a real git change during the coding stage
- Omega commits the change on an `omega/...-codex` branch
- Omega writes diff / summary proof files under `.omega/proof`
- Activity shows Mission Control events

Use sparingly because it consumes tokens.

## 9. GitHub Real Connection Test

Prepare:

- GitHub OAuth App client id
- GitHub OAuth App client secret
- callback URL:

```text
http://127.0.0.1:3888/auth/github/callback
```

Start the Go local service:

```bash
npm run local-runtime:dev
```

Then open the GitHub provider panel, enter the Client ID, Client Secret, and callback URL, click
`Save OAuth app`, and then click `Continue with GitHub`. The browser will complete the GitHub OAuth flow against
the Go local service callback, store the token in local SQLite settings, and mark the GitHub provider
as connected in the workspace snapshot. Environment variables are still supported as a developer
fallback, but the product path is App-driven configuration.

## 9. What To Report During Testing

Please capture:

1. whether local-proof runner succeeds
2. whether Activity shows events
3. whether Workboard state changes
4. browser console errors if any
5. Mission Control API terminal logs
6. whether demo-code creates branch / commit / diff proof
7. whether Codex runner creates proof when selected

## 10. Safe Defaults

Omega avoids dangerous actions by default:

- UI does not send arbitrary shell commands
- local API accepts named runner presets
- `local-proof` is default
- `demo-code` requires an explicit local repository target
- `codex` must be explicitly selected
- cleanup planner does not delete files
