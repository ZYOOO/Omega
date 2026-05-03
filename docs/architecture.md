# Omega 架构说明

最新架构快照见 `docs/latest-architecture.md`。本文保留较完整的历史结构说明和设计上下文；新增能力优先同步到最新架构快照，再按需要回填到本文对应章节。

本文描述 Omega 当前 v0Beta 的真实架构。它以本地可运行、可验证、可审计为第一目标，围绕“需求输入 -> 流程编排 -> Agent 执行 -> GitHub 交付证据”完成研发流程闭环。

## 1. 产品目标

Omega 要实现的是 AI 驱动的研发全流程引擎，而不是单点代码生成工具。

核心链路：

```text
Requirement
  -> Item
  -> Pipeline run
  -> Stage
  -> Agent / Mission / Operation
  -> Proof
  -> GitHub branch / commit / pull request / checks / review / merge
```

当前第一阶段优先完成：

- 用户可以在 App 内输入需求。
- 需求会成为一等 `Requirement` 记录。
- 主 Agent 负责理解需求、生成 dispatch plan，并创建可执行 `Item`。
- `Item` 绑定明确的 repository target，防止 Agent 在错误仓库执行。
- Pipeline 按阶段流转，并把每个阶段的 Agent、输入、输出、依赖和 proof 记录下来。
- 本地 Go 服务创建隔离 workspace，调用 `git`、`gh`、Codex 等本地能力完成真实代码变更与 PR 交付。

## 2. 总体分层

```text
React Workboard
  -> Go Local Service
      -> SQLite product state
      -> Pipeline / Agent orchestration
      -> Background attempt jobs
      -> Workspace isolation
      -> Local tool runners
      -> GitHub delivery integration
```

### 2.1 Product Layer

职责：

- 管理 Project、Repository target、Requirement、Item。
- 展示 Workboard 列表、详情页、右侧 inspector、Operator 面板。
- 管理 GitHub OAuth / CLI 登录状态、workspace root、LLM provider selection。
- 展示 Pipeline stages、execution locks、runner telemetry、proof、checkpoint。
- 作为用户创建需求、选择仓库、启动运行、查看证据的主界面。

当前主要代码：

- `/Users/zyong/Projects/Omega/apps/web/src/App.tsx`
- `/Users/zyong/Projects/Omega/apps/web/src/components/PortalHome.tsx`
- `/Users/zyong/Projects/Omega/apps/web/src/core/workboard.ts`
- `/Users/zyong/Projects/Omega/apps/web/src/workspaceApiClient.ts`
- `/Users/zyong/Projects/Omega/apps/web/src/omegaControlApiClient.ts`
- `/Users/zyong/Projects/Omega/apps/web/src/styles.css`

仓库位置：

- `/Users/zyong/Projects/Omega/apps/web`

### 2.1.1 Desktop Shell

Electron shell 预留在：

- `/Users/zyong/Projects/Omega/apps/desktop`

当前不会替代 Web 开发路径。后续打包时它负责启动 Go local runtime、等待 health check、加载 React SPA，并承载本地预览 webview、workspace picker、deep link callback 等桌面能力。

### 2.2 Local Service Layer

职责：

- 暴露 REST API。
- 读写 SQLite。
- 创建和更新 Requirement、Item、Pipeline、Checkpoint、Mission、Operation、Proof。
- 读取本机 `gh` 登录态和 GitHub repositories。
- 执行 OAuth callback 和 token 持久化。
- 创建隔离 workspace。
- 执行本地 runner，并记录 stdout、stderr、exit code、duration。
- 生成 GitHub branch、commit、PR、checks / review / merge proof。

当前主要代码：

- `/Users/zyong/Projects/Omega/services/local-runtime/cmd/omega-local-runtime`
- `/Users/zyong/Projects/Omega/services/local-runtime/internal/omegalocal/server.go`
- `/Users/zyong/Projects/Omega/services/local-runtime/internal/omegalocal/providers.go`
- `/Users/zyong/Projects/Omega/services/local-runtime/internal/omegalocal/requirements.go`
- `/Users/zyong/Projects/Omega/services/local-runtime/internal/omegalocal/repository.go`

仓库位置：

- `/Users/zyong/Projects/Omega/services/local-runtime`

### 2.3 Persistence Layer

当前数据库：

```text
/Users/zyong/Projects/Omega/.omega/omega.db
```

当前仍保留 workspace snapshot 兼容结构，同时已经将部分核心对象抽成一等表和可查询 API。下一步会继续把 missions、operations、proof records、templates 等逐步从 snapshot-first 推进到 repository-first。

## 3. 核心对象模型

```text
Project
  -> Repository target
      -> Requirement
          -> Item
              -> Pipeline run
                  -> Attempt
                      -> Stage
                          -> Mission
                              -> Operation
                                  -> Proof
```

### Project

产品或工程目标，不等同于代码仓库。一个 Project 可以绑定多个 repository target。

### Repository Target

真实代码目标，可以是 GitHub repo，也可以是本地 repo path。所有真实执行都必须解析到 repository target。

### Requirement

需求源。可以来自 App 手动输入、GitHub issue、飞书消息或未来共享控制面。Requirement 承载原始需求、结构化需求、验收标准、风险、主 Agent dispatch plan。

### Item

Omega 内部可执行工作项。一个 Requirement 可以拆成一个或多个 Item。Item 是 Workboard 列表里可排期、可运行、可审计的基本单位。

当前状态语义：

- `Planning`：正在准备编排、创建 Pipeline 或分配 Agent。
- `Ready`：未开始，UI 显示为 `Not started`。
- `In Review`：正在运行或等待执行链路完成，UI 显示为 `Running`。
- `Backlog`：暂不执行。
- `Done`：流程已完成，Run 禁用。
- `Blocked`：被问题或人工决策阻塞。

### Pipeline Run

某个 Item 的一次流程运行。Pipeline 包含阶段顺序、依赖、Agent 分配、artifact 输入输出、人类检查点和 dataFlow。

### Attempt

一次具体执行记录。每次手动 Run、自动认领、Reject 后重跑，都会形成 Attempt。当前 `run-devflow-cycle` 默认会先创建 Attempt 并返回 `202 Accepted`，后端后台 job 继续执行，前端通过轮询读取 Attempt / Pipeline / Operation / Proof 状态。Attempt 串联 runner、workspace、branch、PR、stage snapshot、错误、耗时和 proof 摘要，避免多次执行互相覆盖。

### Mission / Operation

Mission 是某个执行阶段的任务包。Attempt 是一次点击 Run 或一次自动调度形成的完整执行记录。Operation 是 Attempt 内部某个阶段的具体 runner 执行，会记录 runner、workspace、命令、stdout、stderr、exit status、proof。

## 4. Agent 编排

当前内置 Agent：

- `master`：理解需求，生成结构化需求、验收标准、风险、dispatch plan、suggested items。
- `requirement`：需求分析与验收标准细化。
- `architect`：方案设计、影响范围、文件计划。
- `coding`：实现代码变更。
- `testing`：补充测试、运行验证。
- `review`：审查正确性、安全性、规范性。
- `rework`：消费 review feedback，在同一 workspace / branch / PR 上继续修改并重新验证。
- `delivery`：整合交付物、PR 摘要、merge proof、handoff bundle。

每个 Agent 在后端有明确：

- System Prompt
- 输入契约
- 输出契约
- 默认工具
- 默认模型配置

Pipeline stage 支持一个或多个 `agentIds`，implementation 阶段已能同时绑定 architecture、coding、testing 类职责。

本地执行层已经抽出 `AgentRunner` 接口。当前默认实现是 Codex CLI 的 `exec` runner，并已接入 opencode、Claude Code 和 Trae Agent 的 registry / capability preflight。Codex / Claude Code 默认使用本机 CLI 登录态；opencode / Trae Agent 支持 Workspace Agent Studio 中配置本地账号凭据。凭据以密文写入 SQLite，运行时只解密到子进程环境变量，避免出现在命令参数、API 回包或 runtime log 中。

## 5. 默认 Workflow Template

当前主要模板为 `devflow-pr`，但它不再只存在于 Go 代码中。默认定义位于：

```text
/Users/zyong/Projects/Omega/services/local-runtime/workflows/devflow-pr.md
```

Go local runtime 会读取该 Markdown 的 front matter，编译出 Pipeline stages、Agent 分配、artifact 输入输出和 review rounds。Markdown body 的 `Prompt: requirement / architect / coding / testing / rework / review / delivery` 会作为全 Agent 交接契约，要求每个阶段输出可被下一阶段、Human Review 和 Retry 消费的结构化内容。代码仍保留运行时安全边界、workspace、Attempt、runner、checkpoint、proof 和 GitHub 操作；流程策略本身则逐步迁移到可替换的 Workflow Template。

```text
Requirement intake
  -> Solution planning
  -> Implementation and PR
  -> Review round 1
  -> Review round 2
  -> Rework (only when review requests changes)
  -> Human review
  -> Merge / delivery
```

执行时会：

1. 校验 Item 已绑定 repository target。
2. 在配置的 workspace root 下创建独立 workspace。
3. 写入 `.omega/job.json`、`.omega/prompt.md`、`.omega/agent-runtime.json`。
4. clone 目标仓库。
5. 创建专属分支。
6. 运行本地 runner 或稳定 demo runner。
7. 检测 git diff。
8. commit 变更。
9. 通过 `gh` 创建 PR。
10. 读取 PR / checks 状态。
11. 启动 Review Agent 做两轮只读审查，并要求输出明确 verdict。
12. `CHANGES_REQUESTED` 不会把 Attempt 直接标失败，而是记录 review artifact，进入 `rework`，由 Coding/Rework Agent 在同一 workspace、同一 branch、同一 PR 上继续修改，再回到 Code Review。
13. 自动 rework 达到 workflow 配置的最大轮次，或 Review Agent 输出 `NEEDS_HUMAN_INFO` 时，进入 Human Review 等待人类决策。
14. 审查通过后进入 Human Review checkpoint，等待用户 Approve / Reject。
15. 用户 Approve 后才执行 merge / delivery，并补齐 proof 和 handoff bundle。
16. 用户 Reject 时保留原因，回退到可重做状态。

当前执行入口语义：

- `POST /pipelines/:id/run-devflow-cycle`：默认异步执行，立即返回 `status = accepted`、`pipeline` 和 `attempt`。
- `wait: true`：保留同步执行路径，只用于测试、兼容或命令式脚本，不作为产品主路径。
- `POST /orchestrator/tick` + `autoRun`：认领可执行任务后同样创建 Attempt，并交给后台 job 跑完整 PR cycle。
- `GET /workflow-templates`：返回当前可用 Workflow Template，包括 source、markdown、stages、review rounds。

Human Review 是真实阻塞点，不再由服务端默认自动通过。`run-devflow-cycle` 最多推进到 `waiting-human`：此时 PR 已创建，Review Agent 的 verdict 已记录，Pipeline 停在 `human_review` stage。Pending checkpoint 会绑定具体 `attemptId`，确保人工审核继续的是同一次真实执行的 workspace / branch / PR / proof。只有调用 `POST /checkpoints/:id/approve` 后，后端才继续执行 merge / delivery；`request-changes` 会携带拒绝原因回退流程。飞书侧审核也复用同一 checkpoint decision helper：webhook callback 可直接 approve / request changes；无公网 Task 模式则用 `lark-cli` 创建审核任务，任务完成经 `/feishu/review-task/sync` 写回 approved，任务评论经 `/feishu/review-task/comment` 写回 request changes 或 need-info。

Review / Rework / Retry 的反馈不再只散落在日志和 artifact 中。Runtime 会为失败、取消、人工 request changes、retry 和 Workpad 刷新生成 `reworkChecklist`，把 Review Agent 输出、人工意见、失败原因、operation/event 和 PR/check 推荐动作合并为下一轮可执行清单。Retry API 和 Rework Agent prompt 会优先消费这份 checklist，避免用户或调用方重新拼接原因。详细说明见 `docs/devflow-rework-checklist.md`。

这已经解决“HTTP 请求等待整条流程完成”的问题，并完成第一版 `AgentRunner` 抽象。`JobSupervisor` 已进入 v1：integrity tick、Attempt heartbeat、runner stdout/stderr heartbeat、stalled detection、retry、Attempt cancel、contract-driven timeout/retry、workspace execution lock、workspace cleanup、worker host lease 和 continuation policy metadata 基础版已接入；Codex / opencode / Claude Code runner 已接入 context-aware supervisor，可在 deadline/cancel 时终止子进程。`GET /attempts/:id/timeline` 会按一次 Attempt 聚合 attempt events、pipeline events、stage snapshots、operations、proof records、checkpoints 和 runtime logs，作为排障和人工审核的真实运行时间线。下一步继续补 GitHub polling heartbeat、Git/GitHub command timeout、远端 worker 分配和远端崩溃恢复。功能一生产化内核的当前口径见 `docs/devflow-production-core.md`。

## 6. 执行安全边界

Omega 同时处理三类“workspace”，必须明确区分：

1. **真实项目目录 / 远端仓库**：用户已有项目或 GitHub remote，是代码来源、权限和默认分支的事实来源。Omega 可以读取 clone URL、default branch、repo-owned `.omega/WORKFLOW.md`，但默认不让 Agent 直接在用户原目录里写代码。
2. **Omega 隔离执行 workspace**：由 `local_workspace_root` 派生，例如 `~/Omega/workspaces/<item>/repo`。Agent、git commit、push、PR 创建、PR merge、proof 和 lifecycle 都在这里执行。这个目录是写入边界，也是 Human Review approve 后继续 delivery 的工作目录。
3. **产品里的 Repository Workspace**：Workboard 中的项目上下文，用来绑定 repository target、展示 issues / work items / attempts / proof，不等同于本机真实项目目录。

因此，DevFlow 的原则是：真实项目目录或 remote 负责提供来源，Omega 隔离 workspace 负责执行和写入，Repository Workspace 负责产品语义。即使 repository target 是一个本地路径，运行时也会 clone / checkout 到 Omega workspace 中执行，避免 Agent 直接修改用户正在开发的原目录。`gh pr merge`、branch sync、proof collection 也应基于 Attempt 记录的隔离 `repositoryPath` 执行，而不是基于 Omega 自身仓库或任意全局 cwd。

当前安全原则：

- 每个 Item 的 DevFlow run 使用稳定 workspace、稳定 branch 和同一个 PR，便于 review -> rework -> review 连续推进。
- 每个 Operation 仍然使用隔离 workspace。
- workspace 必须位于配置的 workspace root 内。
- workspace path 通过后端校验，避免路径逃逸。
- Agent runtime 文件记录当前 repo target、workspace root、runner、sandbox policy；runner 账号凭据不写入 runtime 文件，只在启动对应子进程时临时注入环境变量。
- 默认 sandbox policy 为 `workspace-write`。
- execution lock 防止同一个 GitHub issue 或同一任务被重复认领。
- 已完成任务不能重复点击 Run。
- App 在 active repository workspace 下运行时，会拒绝执行不属于当前 repo 的 Item。

## 7. REST API

当前 Go local service 主要 API：

```text
GET  /health
GET  /openapi.yaml
GET  /workspace
PUT  /workspace
GET  /requirements
POST /requirements/decompose
GET  /pipelines
GET  /attempts
POST /attempts/:id/cancel
POST /pipelines/from-work-item
POST /pipelines/from-template
POST /pipelines/:id/start
POST /pipelines/:id/run-current-stage
POST /pipelines/:id/run-devflow-cycle
POST /pipelines/:id/complete-stage
POST /pipelines/:id/pause
POST /pipelines/:id/resume
POST /pipelines/:id/terminate
GET  /checkpoints
POST /checkpoints/:id/approve
POST /checkpoints/:id/request-changes
GET  /missions
GET  /operations
GET  /proof-records
GET  /run-workpads
PATCH /run-workpads/:id
GET  /execution-locks
POST /execution-locks/:id/release
GET  /attempts/:id/timeline
POST /job-supervisor/tick
POST /missions/from-work-item
POST /operations/run
POST /orchestrator/tick
GET  /observability
GET  /local-capabilities
GET  /local-workspace-root
PUT  /local-workspace-root
GET  /llm-providers
GET  /llm-provider-selection
PUT  /llm-provider-selection
GET  /agent-definitions
GET  /github/status
GET  /github/oauth/config
PUT  /github/oauth/config
POST /github/oauth/start
POST /github/cli-login/start
GET  /auth/github/callback
GET  /github/repositories
POST /github/repo-info
POST /github/bind-repository-target
DELETE /github/repository-targets/:id
POST /github/import-issues
POST /github/create-pr
POST /github/pr-status
POST /feishu/notify
POST /feishu/review-request
POST /feishu/review-callback
```

OpenAPI 文档位于：

```text
/Users/zyong/Projects/Omega/docs/openapi.yaml
```

## 8. 当前完成度

已具备：

- Feishu-style React SPA 门户首页 + Workboard 功能页
- React Workboard
- Go Local Service
- SQLite 持久化
- GitHub OAuth config / callback
- 本机 `gh` repo 列表读取
- repository target 绑定和删除
- App 内 Requirement 创建
- Requirement -> Item 归属
- master dispatch plan
- Pipeline / Stage / Agent contract / dataFlow
- isolated workspace
- GitHub branch / commit / PR / checks / merge proof
- execution locks
- runner telemetry
- background attempt job
- Operator 面板
- 状态展示：Planning / Not started / Running / Done

仍需继续：

- 继续拆分 `App.tsx`，把 Workboard 列表、详情、inspector、operator 面板拆成更小组件。
- Pipeline Template 可编辑化。
- Agent runner registry 已接入 Codex / opencode / Claude Code 的统一分发骨架；仍需补 runner-specific 模板、timeout/cancel/retry 和 provider 映射。
- JobSupervisor：继续补 GitHub polling、Git/GitHub command timeout、远端 worker 分配和远端崩溃恢复。
- 更完整的 checkpoint timeline。
- GitHub issue comment / label 回写已落地 DevFlow 关键节点，后续继续扩展到更多 connector 和双向同步控制面。
- Feishu 审核卡片、回调与无公网 Task 审核桥已完成 Human Review 主链路；后续继续扩展常驻本地事件桥和 pipeline delivery summary 卡片。
- Shared sync control plane。
