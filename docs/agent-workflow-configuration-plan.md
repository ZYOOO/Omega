# Agent / Workflow Configuration Plan

## Goal

Omega needs a project-level configuration layer for DevFlow execution. The user should be able to say:

- which Workflow template this Project or Repository Workspace uses
- which Agent role runs each stage
- which model, runner, MCP tools, and Skills each Agent may use
- which local runtime files, such as `.codex`-style policy files, are generated for isolated workspaces
- which human gates and rework rules are mandatory

This must remain tied to real execution. The UI should not be a decorative settings page: the configuration should become the source for pipeline creation, runner dispatch, and `.omega` / `.codex` runtime bundles.

## Product Model

Introduce a first-class `Project Agent Profile`.

```text
Project
  -> Repository Workspace override
      -> Workflow Template
          -> Stage policy
              -> Agent profile
                  -> Runner profile
                  -> MCP allowlist
                  -> Skill allowlist
                  -> Runtime file templates
```

The default profile belongs to a Project. A Repository Workspace can override it when a repo needs a stricter model, different skills, or a special test command.

## Configuration Objects

### Workflow Profile

- template id, such as `devflow-pr`
- stage order and dependencies
- human gates
- rework routing
- input and output artifact contract

The current markdown workflow remains the canonical default, but the app should persist overrides in SQLite.

### Agent Profile

- role id, such as `requirement`, `architect`, `coding`, `testing`, `review`, `delivery`
- system prompt override
- model provider and model
- reasoning effort
- runner choice: `codex`, `opencode`, `Claude Code`, `local-proof`
- stage-specific tool policy

### Runtime Profile

- workspace sandbox policy
- generated `.omega/agent-runtime.json`
- generated `.codex`-style constraints and prompt files
- required CLI capabilities
- timeout, retry, and cancel policy

### Tool Profile

- MCP allowlist
- Skill allowlist
- repository-safe filesystem scope
- GitHub capability scope
- browser preview scope

## UI Entry

The UI entry should live under Workspace / Project configuration, not the observability-oriented `Views` page.

Current entry points:

- Project page `Project config`
- Repository Workspace row gear button
- Repository Workspace subnav `Workspace config`

The entry should show:

- active Project / Repository Workspace scope
- selected Workflow template
- number of available Agent contracts
- MCP and Skill draft
- generated runtime spec preview
- a three-part editor: Workflow, Agents, Runtime files
- Workflow markdown draft that can later be parsed into stage / Agent / gate objects
- per-Agent overrides for runner, model, Skills, MCP, `.codex`, and `.claude` policy

The first implementation can save a local draft, but must label it clearly as a draft until the Go API consumes it. The next step is to persist it through the local runtime.

## Runtime Integration Steps

1. Add SQLite tables:
   - `agent_profiles`
   - `workflow_profile_overrides`
   - `repository_agent_overrides`
   - `runtime_file_templates`
2. Add REST APIs:
   - `GET /agent-profiles`
   - `PUT /projects/{id}/agent-profile`
   - `PUT /repository-targets/{id}/agent-profile`
   - `POST /agent-profiles/validate`
3. During pipeline creation, resolve config in this order:
   - repository override
   - project profile
   - workflow markdown default
4. During runner dispatch, write resolved policy into:
   - `.omega/agent-runtime.json`
   - `.omega/prompt.md`
   - `.codex/OMEGA.md` or equivalent runner-specific policy
5. In the Work Item detail page, show which profile and tool policy were used for the current Attempt.

## Acceptance Criteria

- A user can open Project Agent Profile from Project / Workspace config.
- A user can edit workflow markdown, per-Agent runner/model/Skill/MCP settings, stage policy, and `.codex` / `.claude` policy draft.
- The UI shows generated `.omega/agent-runtime.json`, `.codex/OMEGA.md`, and `.claude/CLAUDE.md` previews.
- The draft is saved locally in the current frontend milestone.
- Follow-up runtime work has clear API and storage targets.
- Future execution must refuse to run if a Work Item has no repository target or if the resolved Agent profile would escape the repository workspace.
