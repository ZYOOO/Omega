# Omega Current Product Design

## 1. Competition Goal

The competition target is an AI-driven requirement delivery engine that connects the full path:

```text
requirement -> solution -> coding -> testing -> review -> delivery
```

The product should solve the current pain point that AI often only covers coding while the real delivery flow still depends on fragmented human coordination across issue systems, chat tools, code review, CI, and release communication.

## 2. Current Product Direction

Omega 的核心能力内核不是“调用一个本地命令”，而是把研发交付里最容易失控的三件事产品化：

```text
需求拆分
  -> 流程编排
  -> Agent 协作与安排
```

其中：

- **需求拆分**：把一句自然语言需求拆成结构化 requirement、验收标准、风险、影响范围和可执行 work items。
- **流程编排**：把 work item 转成可追踪的 Pipeline run，并控制 `需求 -> 方案 -> 编码 -> 测试 -> 评审 -> 交付` 的阶段顺序、依赖、回退和人工检查点。
- **Agent 协作与安排**：为每个阶段分配合适 Agent，明确 system prompt、输入输出契约、可用工具、模型配置和执行上下文，让 Agent 不只是“写代码”，而是按交付流程协作。

Omega 的核心工程管理依赖是 **GitHub**。GitHub 在当前产品中承担：

- Repository target：真实代码库和分支的来源。
- Issue / requirement source：可以从 GitHub issue 导入或同步需求。
- Branch / commit / diff：Agent 执行后的工程证据。
- PR / review / checks：交付前的协作、评审和 CI 状态。
- Long-term collaboration anchor：多人协作时，GitHub OAuth / GitHub App 提供身份、权限和 repo 访问边界。

因此，Feishu 是重要的人机协作与通知入口，但第一阶段的核心闭环应优先围绕 `Omega Workboard + Pipeline/Agent Orchestrator + GitHub engineering loop` 打通。

Omega is currently designed as:

```text
Omega Workboard
  +
Local-first Mission Control / Orchestrator
  +
GitHub-backed engineering management and evidence loop
  +
Feishu-oriented human gate and tool-call entry points
```

Omega owns its own Workboard and product state. External products are references or integration targets only; they are not the internal source of truth.

The latest architecture decision is:

```text
Local Omega App can create, arrange, and execute work by itself.
Optional cloud/shared control plane syncs state, remote issues, approvals, and history.
Feishu/lark-cli carries notifications, review prompts, and lightweight human actions.
```

So Omega should not become a cloud runner that directly operates every developer's machine, and local Apps should not be treated as subordinate workers. The better shape is local-first execution with optional shared sync: local and remote are two sources of the same product state.

## 3. Product Definition

### 3.1 What the user sees

Omega should present:

- A portal-style Home page that exposes product modules, workflow templates, and entry points into the real Workboard.
- A Workboard where projects, repository workspaces, requirements, and work items are managed as Omega-owned product state.
- A Mission Control view showing execution status, proof, gates, and activity.
- A right-side inspector for properties, provider access, and execution context.
- A visible pipeline from requirement intake to delivery.

Current UI shape:

- `/` opens a portal-style Home page inspired by a modern productivity suite dashboard.
- `/#workboard` opens the real functional Workboard.
- The Home page is implemented in `apps/web/src/components/PortalHome.tsx`.
- Workboard keeps the existing execution features but now uses the same light blue / white visual system as the portal: repository workspace, GitHub Issues, Work item groups, runner status, and right-side rail are visually aligned.

### 3.2 What the system does

Omega should:

- Store work items as first-class product data.
- Decompose raw requirements into structured work items, acceptance criteria, and pipeline-ready context.
- Convert work items into missions and operations.
- Orchestrate stage order, stage dependencies, human checkpoints, and retry/reject loops.
- Assign stage operations to role-specific Agents with explicit prompts, contracts, model selection, and tool access.
- Dispatch operations to local or Codex runners in isolated workspaces.
- Capture proof and event logs.
- Stop at explicit human checkpoints.
- Sync selected intents to GitHub / CI / Feishu.
- Sync remote GitHub/shared-control-plane issues into the local App.
- Treat GitHub branch / commit / PR / checks as the main engineering management backbone.
- Use Feishu as a notification, review, and requirement-entry channel.

### 3.3 Core work model

Omega 的 Workboard 对象边界要为 AI DevFlow 做扩展：

```text
Workspace
  -> Team
      -> Project
          -> Work item
              -> Pipeline run
                  -> Stage
                      -> Mission / Operation / Proof / Checkpoint
```

关键定义：

- `Project` 不是 GitHub repo。Project 是一个产品/工程目标，可以绑定一个或多个 `Repository target`。
- `Repository target` 才是 GitHub repo 或本地 repo path。
- `Work item` 对应 Omega Workboard 的 Issue，是最小可排期、可执行、可审核的需求/任务/缺陷。它可以来自 Omega App 内部的新建需求，也可以从 GitHub issue、Feishu 消息或未来共享控制面同步而来。
- `Pipeline run` 是某个 Work item 被 AI 执行时生成的一次流程运行记录。
- `Stage` 是 Pipeline 内部步骤，不应该长期被建模成独立 Work item。
- `Mission` 是某个 Stage 的执行任务包；`Operation` 是这次任务包的具体执行 attempt。

更详细的对象关系见 `docs/work-model-reference.md`。

当前 Project 页面交互主线：

1. 用户通过本机 `gh` 登录态读取可访问的 GitHub repositories。
2. 用户选择一个 repository 后，创建或打开该 repo 在当前 Project 下的 `Repository workspace`。
3. Repository workspace 负责承载该 repo 的 issues、PR、pipeline runs、runner proof 和交付状态。
4. Project overview 只展示目标、repository workspace 数量和已绑定 repo chip，避免把 issues / PR 操作堆在 repo 列表上。
5. 用户在打开的 Repository workspace 中可以直接新建 Omega 内部需求；该需求会继承当前 repo target，`source = manual`，并可以直接进入 Pipeline / runner。
6. GitHub issue import 是外部来源同步，不是唯一入口；导入后的 Work item 使用 `source = github_issue` 和 `sourceExternalRef` 保留可追溯关系。
7. 后续 Work item、Pipeline run、demo-code / codex runner、GitHub issue sync 和 PR delivery 都应优先使用打开的 Repository workspace，而不是让用户反复手填 owner/repo。

## 4. Two-Layer Shape

Omega now follows a two-layer structure:

### Product Layer

Owns:

- Workboard
- Mission Control state
- provider permissions and connection state
- event log and proof references
- connector sync planning
- persistent source of truth

### Execution Layer

Owns:

- runner dispatch
- isolated workspaces
- local/Codex command execution
- proof file collection
- execution events

### Execution Safety Contract

Omega 的本地执行层必须遵守这些硬边界：

- 每个 operation / DevFlow cycle 创建独立 workspace。
- workspace 必须由配置的 `local_workspace_root` 派生，并经过 root 内路径校验，禁止通过 issue key、stage id 或自由文本逃逸到 root 外。
- 每个 workspace 写入 `.omega/job.json`、`.omega/prompt.md` 和 `.omega/agent-runtime.json`，记录 runner、agent、operation、repo target、workspace root、workspace path 和 sandbox policy。
- 默认 sandbox policy 是 `workspace-write`：Codex / opencode / Claude Code 等本地 Agent 只能在当前 operation workspace 内写入。
- 每个 Agent runner 独立进程执行，stdout/stderr/proof/exit status 要回写到 operation；后续需要补 timeout、cancel、retry、queue 和 execution lock。
- Agent 启动不能只靠全局默认 prompt，必须消费当前 issue/work item、workspace、repo target、stage artifact 和 runner prompt。

## 5. Current Local Implementation

The current primary local service path is now:

```text
React UI
  -> Go Local Service
      -> SQLite
      -> Pipeline / Mission / Checkpoint API
      -> local git / gh / codex / opencode / lark-cli / tests
```

The older Node local server remains only as a compatibility fallback while the Go service becomes the main implementation.

### Already implemented

- Go local service-backed Workboard persistence in SQLite.
- Work item creation and patch APIs.
- Mission build API from work item.
- Operation run API that persists Mission Control events.
- Pipeline lifecycle APIs.
- Checkpoint approve / request-changes APIs.
- Mission / operation / proof record query APIs.
- Frontend integration with the local server instead of relying on browser-only state.
- Right inspector rail + panel behavior.
- Operator panel integration with Go observability, LLM provider selection, pipeline templates, and human checkpoints.
- Checkpoint Reject now moves the related pipeline stage back to `ready` with a rejection reason.
- App-driven GitHub OAuth configuration, repo inspection, and GitHub issue import into Workboard.
- GitHub PR creation through the Go local service using `gh pr create`, with PR body generated from runner proof summary and changed files.
- GitHub PR/check status reading through the Go local service using `gh pr view` and `gh pr checks`, returning delivery gate state and proof-record-shaped results.
- App-driven Feishu/lark-cli text notification for checkpoint or pipeline status.
- Repository workspace-scoped app requirement creation: the user can create a requirement inside Omega, inherit the active repo target, and run it without starting from a GitHub issue.
- GitHub repo URL and GitHub issue URL can both be resolved into a repository clone target by the Go local runner, so imported issues and app-created requirements share the same execution path once they point at a repo.
- App-configurable local workspace root for isolated execution directories; default is `~/Omega/workspaces` instead of a temporary directory.
- Built-in `devflow-pr` Pipeline template: clone the bound repo target, create a branch, run the implementation runner, commit real changes, create a GitHub PR, run Review Agents, then stop at a Human Review checkpoint. Merge/delivery only happens after the user explicitly approves the checkpoint.
- Local orchestrator tick: reads eligible GitHub issues from a bound repository target, skips already claimed issues, creates repository-scoped Work items and `devflow-pr` Pipelines, and can explicitly `autoRun` the cycle when the operator enables it.
- DevFlow Run semantics: `run-devflow-cycle` now creates an Attempt first, returns `accepted` immediately, and continues the cycle as a backend background job. The Workboard observes progress through polling rather than waiting for the HTTP request to finish.
- Codex runner process supervision: Codex runs as an isolated child process and Omega records pid, exit code, duration, stdout, stderr, and status on the operation for debugging and audit.

### DevFlow Template Adaptation

The external reference template assumes each project keeps a local `.env.local` / `WORKFLOW.md` with values such as project slug, target repository URL, default branch, and workspace root.

Omega maps that template into first-class product configuration:

| Reference template setting | Omega location |
| --- | --- |
| Project slug | Omega `Project` / `Repository workspace`; Omega implements the Workboard itself. |
| Target repository URL | `Repository target` selected from the App through local `gh` login state. |
| Default branch | Stored on the repository target from GitHub repository metadata. |
| Workspace root | App runtime setting, persisted through `GET/PUT /local-workspace-root`. |
| Workflow markdown | Built-in Pipeline template plus future editable template records. |
| Human review gates | Pipeline checkpoints and proof records, with Feishu/lark-cli as the notification channel. |

The current executable mapping is `devflow-pr`:

```text
Work item
  -> todo / requirement intake
  -> in_progress / coding
  -> code_review_round_1
  -> code_review_round_2
  -> human_review
  -> merging
  -> done
```

Every run must resolve a `repositoryTargetId` before touching git. If the Work item is not scoped to the active repository workspace, the App and service must refuse execution instead of guessing from a URL or global default.

The orchestrator only treats a GitHub issue as executable when it carries an explicit ready label: `omega-ready`, `devflow-ready`, `agent-ready`, or `omega-run`. This keeps open backlog issues visible without letting the local runner silently modify repositories for work that has not been authorized.

### Current local API

- `GET /health`
- `GET /workspace`
- `PUT /workspace`
- `GET /events`
- `GET /pipelines`
- `GET /checkpoints`
- `GET /missions`
- `GET /operations`
- `GET /proof-records`
- `POST /orchestrator/tick`
- `POST /work-items`
- `PATCH /work-items/:id`
- `POST /pipelines/from-work-item`
- `POST /pipelines/:id/run-devflow-cycle`
- `POST /missions/from-work-item`
- `POST /operations/run`
- `POST /run-operation` (compatibility path)
- `POST /github/repo-info`
- `POST /github/import-issues`
- `POST /github/create-pr`
- `POST /github/pr-status`
- `GET /github/oauth/config`
- `PUT /github/oauth/config`
- `POST /github/oauth/start`
- `GET /auth/github/callback`
- `POST /feishu/notify`

### Local persistence

```text
.omega/omega.db
```

## 6. Current Gaps Against the Competition Goal

These are the main gaps after the v0Beta audit:

1. 复杂任意代码生成还不稳：当前 `devflow-pr` 可以真实完成 repo clone、branch、commit、PR、Review Agent verdict、Human Review checkpoint、approve 后 merge proof；但复杂需求仍需要更完整的 Codex / opencode / Claude Code runner registry、prompt template 和执行策略。
2. LLM Provider 已有 registry 和运行时选择，但 provider selection 尚未完整映射到每一种 runner 的真实模型调用；这是赛题 Must-have 的验收风险。
3. Agent 协作已有基础：`master` dispatch、stage contracts、`agentIds`、artifact handoff、handoff bundle 已落地；但并行协商、自动修复重试、attempt/retry policy 仍未完成。
4. Pipeline 编排已有生命周期、dataFlow、execution lock、异步 Attempt job 和 `orchestrator/tick` 基础；但正式 JobSupervisor、heartbeat、retry、cancel、stuck run 检测、worker host 分配和多 turn continuation 仍需增强。
5. GitHub 工程闭环已有 OAuth、repo info、issue import、branch/commit/PR/checks/merge proof；但 issue 状态回写、CI proof 深度解析和 PR lifecycle UI 仍不完整。
6. Workboard 数据已持久化，但 projects/missions/operations/checkpoints/proof 还没有全部从 snapshot-first 演进到 repository-first relational model。
7. Product Layer 仍让前端承担部分业务 reducer 逻辑，服务端还没有成为 Mission Control 的唯一写入者。
8. 当前 Workboard 仍是轻量 issue list，还不是完整的项目、视图、队列、operator workflow 系统。
9. Feishu 目前有 lark-cli 文本通知基础，但还没有真实 approval card callback 和工具调用链。
10. Proof 虽然有文件和记录，但缺少 artifact 类型、diff/test/check/review 等结构化 proof 解析。
11. Shared sync 未实现，本地与远端/GitHub/Feishu 状态还不能双向 reconciled。

## 7. Near-Term Product Goal

The near-term demo target should be:

```text
Create work in Omega
  -> Decompose requirement into structured work + acceptance criteria
  -> Build a mission
  -> Orchestrate staged Agent handoffs
  -> Run a local/Codex operation in an isolated workspace
  -> Create branch + commit + diff proof against a GitHub-backed repo
  -> Record proof + events
  -> Show state transitions and checkpoints in the UI
  -> Prepare sync intents for GitHub issue / PR / CI / Feishu
  -> Send a Feishu notification/checkpoint prompt through lark-cli
```

That is the smallest believable version of “requirement decomposition + orchestration + Agent collaboration + GitHub management loop” for the competition story.

## 8. Future Product Shape

Omega should evolve in two product forms:

### 8.1 当前阶段：本地一体化产品

Primary characteristics:

- Local app shell
- Local service and SQLite
- Local workspace isolation
- Local `git` / `gh` / runner execution
- Local `lark-cli` notification/checkpoint execution

这是第一阶段最适合比赛实现与现场演示的形态。

### 8.2 长期阶段：共享控制面 + 本地执行 App

长期特征：

- 共享的 Workboard / Pipeline / 历史 / 审批
- 本地与远端 requirement / issue / task 双向同步
- 多人项目协作与权限
- GitHub 网页 OAuth / GitHub App
- 统一的控制面状态
- 本地执行 App 继续负责 workspace 和本地工具
- 本地执行 App 继续负责 local-first 编排执行
- 飞书承担通知、审批、轻量指令入口

这个长期形态非常重要，即使第一阶段不需要把它全部做出来。

## 9. Future GitHub and Collaboration Requirements

即使第一阶段仍然优先使用本地 `git` + `gh`，Omega 的未来产品形态也必须明确预留：

1. **GitHub web OAuth / GitHub App**
   - Needed when Omega acts as a true multi-user platform rather than only a local tool.

2. **Multi-user collaboration**
   - Shared project state
   - user roles
   - per-project permissions
   - human approvals owned by real actors

3. **Shared sync + local orchestration**
   - Cloud/server hosts remote issue entry, GitHub sync, shared status, approvals, and history.
   - Local Apps can also create and arrange work directly.
   - Execution locks prevent two local Apps from running the same task unintentionally.
   - Local Omega App hosts the actual local orchestration and local tool execution.

4. **Feishu / lark-cli integration**
   - `lark-cli` should be the first real Feishu execution path for notifications and checkpoint prompts.
   - Later, server-side Feishu OAuth/webhook can complement it for multi-user and always-on scenarios.
