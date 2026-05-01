# Omega 功能实现记录

这个文档用于记录每个功能的落地方式。后续每完成一个功能，都按相同结构补一节：目标、入口、数据/API、运行时行为、验证、后续工作。

## 2026-05-01: GitHub / CI 出站同步增强与飞书审核卡片本地 CLI 路径

### 目标

把功能一的外部协作从“能在本地看见状态”推进到“外部系统也能看见可操作上下文”。旧做法已经能回写 GitHub Issue、读取 PR/checks，并通过飞书 webhook 发送审核卡片；但 PR 本身缺少结构化 review packet 评论，CI 不能按策略触发，`lark-cli` 路径也只保留文本降级。

### 数据/API

- GitHub outbound sync 继续使用现有 DevFlow report 写入路径，无新增 HTTP API。
- PR 出站同步新增 `gh pr comment --edit-last --create-if-none`，在同一 PR 上维护结构化 review packet 评论。
- CI 触发通过显式环境变量启用：
  - `OMEGA_GITHUB_CI_TRIGGER=rerun-failed`
  - `OMEGA_GITHUB_CI_TRIGGER=workflow-dispatch`
  - `OMEGA_GITHUB_CI_WORKFLOW`
  - `OMEGA_GITHUB_CI_REF`
  - `OMEGA_GITHUB_CI_INPUTS`
- 飞书 `POST /feishu/review-request` 在无 webhook 且有 `chatId` 时优先走 `lark-cli im +messages-send --msg-type interactive --content ...`。

### 运行时行为

- Work Item 没有 GitHub Issue 也不影响 PR comment 同步；只要 Attempt 有 `pullRequestUrl`，PR review packet 就会尝试写回。
- CI 自动触发默认关闭，避免本地演示误触远端 Actions；只有显式配置时才执行真实 `gh` 写操作。
- 发送飞书回复 / 通知本身不需要公网；公网只用于飞书云端按钮直接回调本机 runtime。没有公网时，卡片仍可发送，审核人可以通过 `Open review` 回到 Omega Web 操作。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestSyncGitHubIssueOutboundPostsCommentAndLabels|TestSyncGitHubIssueOutboundPostsPRCommentWithoutIssue|TestGitHubCITriggerRerunsFailedRuns|TestGitHubCITriggerWorkflowDispatch|TestFeishuReviewRequestUsesLarkCLIInteractiveCard'
```

### 后续工作

- 本地事件桥：用 `lark-cli event consume` 消费飞书交互事件，再调用本机 runtime，避免必须暴露公网 callback。
- 飞书长文档：把当前 review doc preview 通过飞书文档 API 发布为正式文档，并在卡片中附链接。
- CI 触发策略：把 rerun / dispatch 配置接入 Project / Repository settings，而不是只靠 env。

## 2026-04-30: Page Pilot 产品入口与 Repository Workspace 选择

### 目标

解决功能二入口不明显、进入后依赖默认 Repository Workspace 的问题。旧做法把 Page Pilot 藏在 Workboard 左侧导航里，用户从 Electron 首页启动后不容易发现；进入 Page Pilot 后也没有显式选择目标 repo 的步骤，容易误以为仍是固定项目演示。

### 入口

- 首页增加 `打开 Page Pilot` / `启动 Page Pilot` 按钮。
- 支持 `#page-pilot` 深链，Electron 或浏览器可直接打开功能二页面。
- Work Item 详情页增加 `Open in Page Pilot`，使用当前 Work Item 的 `repositoryTargetId` 作为 Page Pilot 目标。
- Page Pilot 页面保留普通 App chrome，不再默认进入会隐藏导航的沉浸式空白预览状态。
- Electron preload 暴露 `window.omegaDesktop.reloadApp()`；Page Pilot 顶部提供 `Reload app`，用于替代浏览器地址栏刷新按钮。
- 旧尝试：`Open preview` 曾在 Omega 页面里打开 BrowserView，并用 `preview-preload.cjs` 注入最小工具条，再把圈选结果回传到 Omega 页面的 overlay。这个做法改变了已验证的 direct pilot 体验，现不作为主路径继续推进。
- 新做法：Page Pilot 页面只作为启动器，负责选择 Repository Workspace、预览来源和打开目标页面；打开后 Electron BrowserView 使用已验证的 `pilot-preload.cjs`，在目标页面内完成圈选、多批注、Apply、Confirm 和 Discard。
- Preview source 保留三种显式模式：Repository source、Dev server URL、HTML file。Repository source 会通过 Electron bridge 准备明确 repo 的预览来源；HTML file 会转为 `file://` 交给 Electron 打开；Dev server URL 适合 Vite / Next / Astro 等需要端口的项目。

### 运行时行为

- `PagePilotPreview` 顶部新增 Repository Workspace 选择区，但不再承载目标页面内的圈选和编辑交互。
- 用户选择 repo 后，前端更新当前 `activeRepositoryWorkspaceTargetId`，后续 apply / deliver 都使用该 repo target。
- 未选择 repo 时，页面明确显示 `Choose a target repository`，不再静默使用模糊默认值。
- Work Item 详情页按钮会先绑定该 item 的 repo，再切换到 Page Pilot。
- Electron `omega-preview:open` 支持把 `projectId`、`repositoryTargetId`、`repositoryLabel` 随目标 URL 传给 `pilot-preload.cjs`，旧 direct pilot 不再依赖固定 env/default repo。
- direct pilot 目标页内新增 `返回` 按钮，用于关闭 BrowserView 回到 Omega 页面。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
```

### 后续工作

- Electron service state 和 preview URL 还需要接到 Page Pilot 页面，让用户能从 UI 启动/重启目标项目 preview。
- Repository Workspace 选择后应展示该 repo 的 Preview Runtime Profile、启动状态和最近 Page Pilot runs。

## 2026-04-30: Electron Desktop Shell 自动启动本地服务基础版

### 目标

让功能二尽早进入接近打包产品的启动方式。旧做法需要手动分别启动 Go local runtime、Omega Web Vite dev server 和目标项目 preview server；新做法由 Electron main process 统一启动或复用这些本地服务，减少 Page Pilot 集成验证时的手动前置步骤。

### 入口

```bash
npm run desktop
```

可选目标项目预览：

```bash
OMEGA_PREVIEW_REPO_PATH=/Users/zyong/Projects/TestRepo npm run desktop
OMEGA_PREVIEW_REPO_PATH=/Users/zyong/Projects/TestRepo OMEGA_PREVIEW_COMMAND="npm run dev -- --host 127.0.0.1 --port 5173" npm run desktop
```

### 数据/API

新增 Electron 侧 supervisor 模块：

```text
apps/desktop/src/process-supervisor.cjs
```

新增 preload 查询能力：

```text
window.omegaDesktop.getServices()
```

当前没有新增 Go API。Preview Runtime Profile API 仍作为后续正式运行档案能力推进。

### 运行时行为

- Electron 启动后先探测 `http://127.0.0.1:3888/health`，存在则复用，不存在则启动 `go run ./services/local-runtime/cmd/omega-local-runtime --host 127.0.0.1 --port 3888`。
- Electron 探测 `http://127.0.0.1:5174/`，存在则复用，不存在则启动 `npm run web:dev -- --host 127.0.0.1 --port 5174`。
- 如果设置 `OMEGA_PREVIEW_REPO_PATH` 或 `OMEGA_PAGE_PILOT_REPO_PATH`，Electron 会在该明确 repo 内启动目标 preview server；没有显式路径时不会猜测目标项目，避免误跑 Omega 自身目录。
- Preview 命令优先使用 `OMEGA_PREVIEW_COMMAND`；否则读取目标 repo `package.json`，按 `dev/start/preview` 脚本和 lockfile 推断包管理器；静态 `index.html` 用 `python3 -m http.server` 兜底。
- Electron 退出时会尽力停止由本次启动创建的子进程；已存在的外部服务不会被关闭。

### 验证

```bash
npm run test -- apps/web/src/__tests__/desktopProcessSupervisor.test.ts --testTimeout=15000
```

### 后续工作

- 将 preview process supervisor 下沉到 Go runtime API，并把 pid、stdout/stderr、健康检查和失败诊断写入 Page Pilot run。
- 从 Repository Workspace UI 选择目标 repo 和 preview profile，替代 env 配置。
- 打包模式下将 Go runtime 二进制和 Web 静态资源纳入正式 app bundle。

## 2026-04-30: PR 评审反馈进入 Rework Checklist

### 目标

让 request changes、PR 评论、PR review state、Review Agent 输出和交付门禁汇成同一份可执行 rework 输入。旧做法已经能创建真实 Rework Attempt，但 PR 侧评论没有进入 Attempt，用户看到“已拒绝”后不容易判断下一轮 Agent 到底会依据什么修改。

### 数据/API

- `/github/pr-status` 在读取 PR 基础状态和 checks 后，额外读取 `comments,reviews`，返回 `reviewFeedback`。
- DevFlow PR cycle 在创建或更新 PR 后同步读取 PR feedback，并把 `pullRequestFeedback` 写入 Attempt result。
- `/github/pr-status` 和 DevFlow PR cycle 会从 failed check link 抽取 Actions run id，优先读取 `gh run view --log-failed`，返回 / 写入 `checkLogFeedback`。
- `reworkChecklist.sources` 新增 `pr-review`、`pr-comment`、`ci-check-log` 类型，和人工反馈、Review Agent 反馈、checks / branch sync / conflict 建议一起生成 checklist。

### 运行时行为

- Human Review request changes 后，新 Rework Attempt 会继续复用上一轮 workspace / branch / PR，并消费当前 Attempt 上的 `reworkChecklist.prompt`。
- 自动 review / rework loop 在 review prompt 中附带 PR feedback，避免 PR 上的 comment 与本地 Review Agent 结果脱节。
- 自动 review / rework loop 在 review prompt 中附带 failed check log，避免只告诉 Agent “CI 失败”却不给失败输出。
- `attempt-run-report.md` 增加 Pull Request Feedback 和 Failed Check Logs 小节，Human Review 可看到 PR 评论和 CI 失败输出是否已经进入运行上下文。
- Workpad 的 Rework Checklist 展开后会显示 source drilldown，用户可以看到每条 action 背后的 human / review / PR comment / check log / gate 来源摘要。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestGitHubPRStatusUsesGhViewAndChecks|TestGitHubPullRequestFeedbackFromView|TestGitHubPRStatusClassifiesFailedChecks|TestBuildReworkChecklistMergesReviewHumanAndDeliveryGateSignals|TestRunWorkpadRecordTracksAttemptRetryContext'
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
```

### 后续工作

- PR review thread 的 resolved/unresolved 状态和行级 diff 上下文仍需后续接入。
- CI/check 日志分页、job/step 结构化解析和 source drilldown 仍需后续接入。
- Workpad source drilldown 当前是摘要级；后续补从 source 反跳到具体 Timeline event、PR comment、check log step 或 artifact 的深链。

## 2026-04-29: Work Item 详情页 / Run Workpad Record 基础版

### 目标

降低 Work Item 详情页和 `App.tsx` 的耦合，同时把用户真正需要排障和评审的信息前置。旧做法把 Requirement、Delivery flow、Agent orchestration、Artifacts、Retry reason 分散在多个大区块里，很多卡片只能看不能点；新做法以 Run Workpad record 汇总当前 Attempt 的计划、验收、验证、阻塞、PR、Review feedback 和 Retry reason。

### 产品入口

新增独立详情路由：

```text
#/work-items/{itemId}
```

新增前端模块：

```text
apps/web/src/workItemRoutes.ts
apps/web/src/components/WorkItemDetailPage.tsx
apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx
services/local-runtime/internal/omegalocal/run_workpads.go
services/local-runtime/internal/omegalocal/run_workpads_test.go
```

增强既有详情面板：

```text
apps/web/src/components/WorkItemDetailPanels.tsx
```

### 数据/API

新增服务端记录：

```text
runWorkpads
```

新增 API：

```text
GET /run-workpads
PATCH /run-workpads/{id}
```

支持按 `attemptId`、`pipelineId`、`workItemId`、`repositoryTargetId`、`status` 过滤。

Workpad 的每个字段都从现有真实记录生成：

- Plan：Pipeline stage 和当前 Attempt。
- Acceptance criteria：Requirement / Work Item 验收条件。
- Validation status：测试 proof、checks summary 和 Attempt 状态。
- Notes：Attempt events、timeline 和 proof 摘要。
- Blockers：pipeline failure、attempt failure、checkpoint、PR checks / conflict / branch sync。
- PR：proof PR URL 和 `/github/pr-status` 返回的检查结果。
- Review Feedback：review agent 结果、checkpoint decision、human request changes 相关记录。
- Retry Reason：runtime 归并出的失败和重试依据；前端缺失 record 时才使用本地兜底。
- Field patches：Agent / supervisor 可通过 `PATCH /run-workpads/{id}` 写入 Plan、Validation、Blockers、Review Feedback、Retry Reason 等字段。旧做法每次 runtime 刷新都会覆盖整份 Workpad；新做法把 patch 保存为 `fieldPatches`，刷新时先派生真实状态，再叠加字段级 patch。
- Field patch audit：2026-04-30 新增 `fieldPatchSources` / `fieldPatchHistory` 和 `updatedBy` 写入边界。旧做法只能看到最终覆盖值；新做法可以追踪每个被覆盖字段的来源、原因、写入者和时间，并在 Work Item 详情页以默认折叠的 Patch history 卡片展示。

### 运行时行为

- Runtime 在 Attempt 创建、Agent invocation 持久化、Attempt complete / fail / cancel、retry 创建、Human Review approve 后进入 merging 时刷新 `runWorkpads`。
- `App.tsx` 继续负责读取 Requirement / Pipeline / Attempt / Operation / Proof / Checkpoint / PR status，并额外读取 `runWorkpads`，把当前 Work Item 的上下文传给独立详情组件。
- `WorkItemDetailPage` 负责 Workpad-first 布局、Requirement 内部滚动、紧凑 Delivery flow、可展开 Agent operations 和可展开 Artifacts；Workpad 内容优先使用 `runWorkpads`，缺失时用真实执行记录兜底。
- 点击 Work Item 或 Human Review 入口会进入独立 hash 路由，刷新页面后仍可恢复到同一个 item 详情。
- Agent operation 现在可以展开查看 prompt / stdout / stderr / runner metadata，便于判断失败是 agent 输出、runner 环境、验证命令还是 delivery 步骤导致。
- Artifact 卡片可以展开查看 URL / path，避免 proof 只作为不可操作的展示块。

### 验证

```bash
npm run lint
npm run test -- apps/web/src/core/__tests__/manualWorkItem.test.ts apps/web/src/__tests__/App.operatorView.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx apps/web/src/components/__tests__/WorkItemDetailPanels.test.tsx apps/web/src/__tests__/attemptRetryReason.test.ts --testTimeout=15000
npm run build
go test ./services/local-runtime/internal/omegalocal -run 'TestRunWorkpadRecordTracksAttemptRetryContext|TestApproveDevFlowCheckpointCanContinueDeliveryAsync|TestRunCurrentPipelineStagePersistsOperationProofAndCheckpoint'
```

浏览器验证：

- `http://127.0.0.1:5173/#/work-items/item_manual_21` 可直接打开。
- 浅色模式下 Requirement markdown 内容可读，长内容会在卡片内部滚动。
- Delivery flow 为多列紧凑布局，active stage 有明显运行态反馈。
- Agent operation 可展开查看 Prompt 和执行输出。

### 后续工作

- 把当前 runtime 刷新的 Run Workpad 升级为按字段 patch，允许 Agent / supervisor 直接写入 Plan、Validation、Blockers、Review Feedback、Retry Reason。
- 已完成基础版：review agent 结果、PR comments / reviews、human request changes 和 checks / branch sync / conflict 推荐动作会进入 rework checklist，并让 Rework Agent 和 Retry API 消费；后续继续补 thread resolved 状态和 CI/check 详细日志。
- Approve 只推进 checkpoint approved；merge 作为单独 job 运行并展示 checks、branch sync、conflict、merge output 和失败原因。

## 2026-04-29: Runtime Logs 基础版

### 目标

把本地 runtime 的关键行为写成可查询的 append-only 日志，优先解决本地联调时“API failed 但不知道真实后端原因”的问题。

### 产品入口

- Operator `Views` 页面新增 `Runtime logs` 区块，展示最近 INFO / DEBUG / ERROR 事件、时间和关联 work item / pipeline / attempt / request id。
- `/observability` 返回 `recentErrors` 和 runtime log 计数，Operator 可直接看到最近失败。

### 数据/API

新增 SQLite 表：

```text
runtime_logs
```

新增 API：

```text
GET /runtime-logs
```

支持按 `level`、`eventType`、`entityType`、`entityId`、`projectId`、`repositoryTargetId`、`workItemId`、`pipelineId`、`attemptId`、`stageId`、`agentId`、`requestId`、`createdAfter`、`createdBefore` 和 `limit` 查询。

2026-05-01 更新：

- 旧做法保留：基础查询仍默认返回数组，兼容 Operator 面板和旧 CLI。
- 新增 `requirementId` 维度：新日志可以直接写入 `requirementId`；旧日志会通过当前 workspace snapshot 反查同 Requirement 下的 Work Item / Pipeline / Attempt。
- 新增 cursor pagination：`GET /runtime-logs?page=1&limit=N&cursor=...` 返回 `items / nextCursor / hasMore`。
- 新增全文搜索：`q` / `search` 会匹配 level、event type、message 和 details JSON。
- 新增导出：`GET /runtime-logs/export?format=jsonl|csv` 复用同一批过滤条件，默认导出 JSONL。
- CLI `omega logs` 增加 `--requirement`、`--search`、`--page`、`--cursor`。

### 运行时行为

- API request 自动记录 request id、method、path、status、bytes、duration。
- DevFlow job 记录 start / complete / fail、attempt backfill、agent invocation、checkpoint decision、PR merge 相关事件。
- Page Pilot apply / deliver / discard 记录 requested、invalid、preflight、runner、persist、sync、applied/delivered/discarded 等事件。
- checkpoint approve 兼容旧数据缺少 attempt 的情况，并把缺失 attempt 写入 ERROR 日志，避免 UI 只显示泛化失败。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestRuntimeLogsAPIListsAndFiltersRecords|TestCompleteDevFlowCycleBackfillsMissingAttempt|TestDevFlowCheckpointApprovalToleratesMissingAttempt'
npm run lint
```

### 后续工作

- 补齐 cursor pagination、Requirement 维度、全文搜索和导出。
- 把 runtime log、attempt events、stage timeline、runner process telemetry 聚合为单个 Run Timeline。
- 继续把 git operation、checks polling、rebase/conflict/retry 细节写入日志。

## 2026-04-29: JobSupervisor Integrity Tick

### 目标

修复 Human Review checkpoint 和具体 Attempt 之间的断链风险。Human Review 不是一个独立的 Pipeline 状态按钮，它必须指向一次真实执行记录，才能继续使用 workspace、branch、PR 和 proof 做交付动作。

### 数据/API

新增 API：

```text
POST /job-supervisor/tick
```

Checkpoint 记录新增兼容字段：

```text
attemptId
```

### 运行时行为

- 创建 pending checkpoint 时写入当前 pipeline 的 latest attempt id。
- `GET /checkpoints` 和 checkpoint decision 前执行轻量 integrity reconciliation。
- 如果 pending checkpoint 没有 attempt link，优先链接同 pipeline 的 latest attempt。
- 如果同 pipeline 没有 attempt，但仍是可修复的 DevFlow human gate，则 backfill `supervisor-repair` attempt，并记录 `attempt.recovered` event。
- approve 时优先使用 checkpoint 的 `attemptId`，不再只依赖 pipeline 反查最近 attempt。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestJobSupervisorTickBackfillsPendingCheckpointAttemptLink|TestDevFlowCheckpointApprovalToleratesMissingAttempt|TestCompleteDevFlowCycleBackfillsMissingAttempt'
npm run lint
```

### 后续工作

- 把 JobSupervisor 从手动 tick 推进到常驻扫描。
- 增加 runner process stream heartbeat、retry、cancel 和 timeout。
- 把 Run Timeline 建在 checkpoint/attempt/runtime log 的统一视图上。

## 2026-04-29: JobSupervisor Heartbeat / Stalled Detection 基础版

### 目标

让运行中的 Attempt 有可查询的健康信号，并让长期没有心跳的运行进入明确的 `stalled` 状态，而不是一直停留在 `running`。

### 数据/API

沿用：

```text
POST /job-supervisor/tick
```

请求可选：

```json
{
  "staleAfterSeconds": 900
}
```

Attempt 新增动态字段：

```text
lastSeenAt
stalledAt
statusReason
```

### 运行时行为

- `makeAttemptRecord` 创建 Attempt 时写入 `lastSeenAt`。
- Agent invocation 持久化时刷新 Attempt `lastSeenAt`。
- Attempt complete / fail 时同步刷新 `lastSeenAt`。
- `job-supervisor/tick` 扫描 `status = running` 的 Attempt；如果 `lastSeenAt` 超过阈值未更新，则：
  - Attempt 标记为 `stalled`。
  - Pipeline 标记为 `stalled`。
  - Work Item 标记为 `Blocked`。
  - Attempt events 写入 `attempt.stalled`。
  - runtime logs 写入 `job_supervisor.attempt.stalled` ERROR。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestJobSupervisorTickBackfillsPendingCheckpointAttemptLink|TestJobSupervisorTickMarksStalledRunningAttempt|TestCompleteDevFlowCycleBackfillsMissingAttempt'
npm run lint
```

### 后续工作

- runner process stdout/stderr 流刷新 heartbeat。
- stalled attempt 的 retry / resume / cancel / timeout 策略。
- Operator Run Timeline 中展示 heartbeat age 和 stalled 原因。

## 2026-04-29: Runner Context Cancel / Attempt Cancel API 基础版

### 目标

补齐 cancel / timeout 的第一层真实执行能力：Agent runner 子进程必须能被 context deadline 或 operator cancel 终止，Attempt cancel 不能只是 UI 状态变化。

### 数据/API

新增 API：

```text
POST /attempts/{id}/cancel
```

请求可选：

```json
{
  "reason": "Canceled by operator."
}
```

返回：

```json
{
  "attempt": {},
  "cancelSignalSent": true
}
```

### 运行时行为

- Codex / opencode / Claude Code runner 改用 `runSupervisedCommandContext`。
- context deadline exceeded 时，runner process 返回 `status = timed-out`。
- context canceled 时，runner process 返回 `status = canceled`。
- DevFlow background job 会注册 `attemptId -> cancel func`。
- `POST /attempts/{id}/cancel` 会调用注册 cancel func；若当前进程仍在本机运行，`cancelSignalSent = true`。
- Attempt 标记为 `canceled`，Pipeline 标记为 `canceled`，Work Item 标记为 `Blocked`。
- Pending checkpoint 若绑定该 attempt，会同步标记为 `canceled`，避免取消后的 Human Review 继续误导用户。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestCancelAttemptAPIUpdatesStateAndSignalsRunningJob|TestRunSupervisedCommandContextTimesOutProcess|TestJobSupervisorTickMarksStalledRunningAttempt'
npm run lint
```

### 后续工作

- Git / GitHub command 也要接入 context-aware execution。
- Runner stdout/stderr streaming heartbeat。
- Retry / resume / cancel reason taxonomy。
- Operator UI 暴露 cancel 按钮和 canceled/stalled recovery actions。

## 2026-04-28: Page Pilot MVP

### 目标

启动赛题功能二的真实链路 MVP：在当前 React SPA 内提供 Page Pilot preview surface，让用户打开正在构建的软件页面，圈选目标项目页面元素、采集上下文和源码映射，再交给 Go local runtime 在明确 Repository Workspace 中应用真实源码 patch。用户确认后，runtime 可以创建 branch / commit，并在 GitHub target 上继续创建 PR。

### 产品入口

新增组件：

```text
apps/web/src/components/PagePilotPreview.tsx
apps/web/src/components/PagePilotOverlay.tsx
```

挂载位置：

```text
apps/web/src/App.tsx
```

当前 `Page Pilot` 作为 Workboard 内的独立 nav 页面。用户先输入目标项目 preview URL，Omega 在页面内打开目标项目，再用右下角 `AI` 悬浮按钮圈选 preview 中的元素。浏览器模式下只有 same-origin iframe 可直接 inspect；跨 origin 本地预览需要 Electron webview bridge，UI 会明确提示。

为便于本地浏览器调试，Vite dev server 增加 `/page-pilot-target` 代理。开发者可以把目标项目跑在 `127.0.0.1:5173`，再在 Omega 中打开 `http://127.0.0.1:5174/page-pilot-target/`，让 preview iframe 保持与 Omega 同源，从而验证真实 DOM 圈选、source mapping 采集和 apply API。这个代理仅作为 dev/test bridge；产品主路径仍然是 Electron 内部浏览器 + preload bridge。

交互修正：Page Pilot 页面进入 immersive preview mode，隐藏 Workboard sidebar / topbar / inspector rail，让用户产品 iframe 占满 Omega 主工作区。Overlay 不再在 preview 不可 inspect 时回退圈选 Omega 自身；本地 `/page-pilot-target` URL 会自动归一为相对路径，避免 `localhost` 与 `127.0.0.1` host 不一致造成跨源。

新增 Electron dev shell 基础版：

```text
apps/desktop/src/main.cjs
apps/desktop/src/omega-preload.cjs
apps/desktop/src/pilot-main.cjs
apps/desktop/src/pilot-preload.cjs
```

开发模式下 Electron 加载现有 React/Vite URL，不需要打包。它预留目标项目 `BrowserView`，通过 preload 注入 Page Pilot selection bridge。React SPA 和 Go runtime 不被替代：React 继续做 Omega UI，Go 继续做 SQLite / runner / git / PR。

后续修正：`preview-preload.cjs` 的最小 selection bridge 属于中间尝试；产品主路径改为 BrowserView 直接加载 `pilot-preload.cjs`，保留已验证的目标页内 direct pilot 体验。

新增 `desktop:pilot` 直接体验模式：Electron 主窗口直接打开用户产品 URL（默认 `http://127.0.0.1:5173/`），`pilot-preload.cjs` 在目标页面内注入悬浮手指、hover 高亮、tooltip 和修改输入框。这个模式用于还原赛题中的页面内 Agent 体验：用户看到的就是自己的产品页面，Page Pilot 浮层直接跑在页面上，而不是 Chrome 或 Workboard iframe。

Electron direct pilot 的交互收敛为：

1. 右下角悬浮手指进入选择模式。
2. hover 高亮当前元素并展示元素 tooltip。
3. 点击元素后只出现 `✓ / ×` 选择确认控件。
4. 点击 `✓` 后出现底部悬浮输入框。
5. 输入要求并发送后先加入页面批注队列，并在对应元素附近插入编号标记。
6. 底部保留一个悬浮输入框，展示批注 chip，用户可以继续补充整体要求，也可以继续选择更多元素。
7. 用户点击悬浮输入框的发送按钮后再调用 `/page-pilot/apply`，写入真实源码并产生真实 diff。

`data-omega-source` 不再作为“能否批注”的前置条件。它表示强源码映射：有映射时 runtime/Agent 可以直接定位文件和 symbol；没有映射时仍然收集 selector、DOM context、文本/样式快照和用户 comment，作为 DOM-only 批注交给 Agent 辅助定位。

当前功能二主模式：

- Electron direct pilot 是产品主路径：窗口直接加载用户目标项目 URL，Page Pilot 由 preload 注入到目标页面。
- React SPA 的 Page Pilot preview / `/page-pilot-target` 代理保留为开发调试 fallback，不作为最终页面内 Agent 体验的主路径。
- Go local runtime 继续作为唯一真实执行面：接收 selection context 和用户指令，在明确 repository target 中执行 runner、写源码、记录 run，并负责后续 branch / commit / PR。
- 当前 direct pilot 支持多元素批注队列：单个 comment 只创建编号 pin，底部悬浮输入框统一收集整体修改说明，用户主动发送后才调用 `/page-pilot/apply`。
- Page Pilot 背后产品上是一个 Page Pilot Agent；工程上拆成 `preview-runtime`、`page-editing`、`delivery` stages。`preview-runtime` 理解不同项目的安装、启动、端口、健康检查和刷新方式；`page-editing` 根据选区上下文和批注修改源码；`delivery` 负责摘要、行级 diff、branch / commit / PR。
- Page Pilot session 可以创建功能一的 Requirement / Work Item：`source = page_pilot`，绑定同一个 `repositoryTargetId`，把选区上下文作为 artifact，并进入 Page Pilot pipeline run。

单一 Page Pilot Agent 模式已开始落地：

- `/page-pilot/apply` 成功应用真实 patch 后，会创建 `source = page_pilot` 的 Requirement 和 Work Item。
- Work Item 绑定当前 `repositoryTargetId`，并把 selection context、整体 instruction、agent mode、execution mode 写入 artifacts / description。
- runtime 同时创建 `templateId = page-pilot` 的轻量 pipeline run，stage 包含 `preview_runtime`、`page_editing`、`delivery`。
- Page Pilot run 记录会带回 `requirementId`、`workItemId`、`pipelineId`、`pipelineRunId`、`agentMode = single-page-pilot-agent`、`executionMode = live-preview`。
- 用户 deliver 后，对应 Work Item/Pipeline 会更新为 delivered；discard 后会更新为 blocked/discarded。

Electron direct pilot 的交付闭环基础版已接入：

- apply 成功后，preload 把 run 存入 localStorage，并自动 reload 当前目标页面，让用户看到修改后的预览。
- reload 后 Page Pilot 浮层会恢复结果面板，展示状态、changed files、diff summary、Requirement / Work Item / Pipeline linkage。
- 结果面板提供 `Confirm`、`Discard`、`Reload`、`New`。
- `Confirm` 调用 `/page-pilot/deliver`，创建 branch / commit，并在 GitHub target 上继续创建 PR。
- `Discard` 调用 `/page-pilot/runs/{id}/discard`，撤销本地源码变更后 reload 预览。

当前缺口：

- apply 成功后 Electron direct pilot 已能 browser reload；还没有 Preview Runtime Agent/Profile 驱动的项目 dev server restart。
- `/page-pilot/deliver` 后端能力和 direct pilot 基础 Confirm/Discard 已接通；还缺更完整的 patch preview、语义摘要编辑和 Work Item 详情回跳。
- target URL、project id、repository target id 和 local worktree 仍偏开发态，后续需要做成一等配置，避免误写其他仓库。
- DOM-only 批注已经可采集，但缺少自动源码候选定位；强定位仍依赖 `data-omega-source`。
- 多轮对话还只是多批注 + 一次整体说明，不是同一 run 上的持续 Agent conversation。
- Preview Runtime Agent/Profile 还未落地；当前 TestRepo 仍由手工启动静态服务器和 env/default URL 支撑。后续需要由 Agent 读取 repo 文件、生成可审计启动档案，并由 Go process supervisor 启动/重启。
- Page Pilot -> Requirement -> Work Item 的基础接入已落地，但 direct pilot UI 还没有展示这些功能一记录，也还没有从 Requirement detail 反向打开对应 Page Pilot run。
- live-preview mode 和 isolated-devflow mode 还未分层；MVP 应优先 live-preview + repository lock，后续再支持把预览指向隔离 operation workspace。

Overlay 支持：

- `Select` 进入页面元素圈选模式。
- 圈选按钮、标题、卡片文案、label、input、textarea、select、link、常见布局容器和普通文本；其他元素标记为 `other`。
- 展示元素类型、文本快照和 `data-omega-source`。
- `Apply` 调用本地 runtime 修改真实源码。
- `Confirm` 调用本地 runtime 生成 branch / commit / PR-ready 交付。

Portal Home 已加入最小源码元数据：

```text
apps/web/src/components/PortalHome.tsx:headline
apps/web/src/components/PortalHome.tsx:welcome-copy
apps/web/src/components/PortalHome.tsx:primary-workboard-button
apps/web/src/components/PortalHome.tsx:open-workboard-button
apps/web/src/components/PortalHome.tsx:template-card-{index}
```

### 数据/API

新增 API：

```text
POST /page-pilot/apply
POST /page-pilot/deliver
GET /page-pilot/runs
POST /page-pilot/runs/{id}/discard
```

Preview runtime API 仍为计划项，尚未在 OpenAPI / Go runtime 中作为真实接口暴露。目标是让 Agent 为每个 repository workspace 生成 Preview Runtime Profile，例如 install command、dev command、preview URL、health check 和 reload strategy，再由 Go runtime 的 process supervisor 执行。

`/page-pilot/apply` 的返回值新增功能一 linkage：

- `requirementId`
- `workItemId`
- `pipelineId`
- `pipelineRunId`
- `agentMode`
- `executionMode`

前端 client：

```text
applyPagePilotInstruction
deliverPagePilotChange
```

selection context 包含：

- `elementKind`
- `stableSelector`
- `textSnapshot`
- `styleSnapshot`
- `domContext`
- `sourceMapping`

### 运行时行为

`/page-pilot/apply`：

1. 校验 `repositoryTargetId`、用户指令和 source mapping。
2. 解析 Workboard 中的 repository target。
3. local target 直接使用本地 path；GitHub target 只在 local runtime 当前 worktree 的 remote 与 target 匹配时使用，用于保证 Vite HMR / dev server reload 能看到修改。
4. 校验 source file 位于 repo root 内且真实存在。
5. 读取 Agent Profile，按 `coding` Agent runner 做 capability preflight。
6. 可用时调用 Codex / opencode / Claude Code 修改真实源码。
7. MVP 也提供 `local-proof` / `demo-code` 的直接文本替换路径，用于可重复测试；它仍然写真实 source file 并产生真实 git diff。
8. 返回 changed files、diff summary、行级 diff summary、proof files 和 HMR 提示。
9. 使用 SQLite 一等表 `page_pilot_runs` 保存 Page Pilot run 记录，返回 `runId`，用于后续确认或撤销。

`/page-pilot/deliver`：

1. 确认 repo 中有未提交变更。
2. 创建或切换到 Page Pilot 分支。
3. 提交 commit。
4. GitHub target 会 push branch 并创建 PR。
5. 返回 branch、commit、PR URL 和 diff 摘要。
6. 如果请求带 `runId`，会把同一条 Page Pilot run 更新为 `delivered`。

`/page-pilot/runs`：

- 返回本地 runtime 中的 Page Pilot run 记录。
- 从 SQLite 一等表 `page_pilot_runs` 返回本地 Page Pilot run 记录，Overlay 会显示最近 runs。

`/page-pilot/runs/{id}/discard`：

- 仅允许撤销 `applied` 状态。
- 按 run 记录中的 changed files 执行 git revert/clean，不做整仓 reset。
- 更新 run 为 `discarded`，保留可审计记录。

### 验证

新增/更新测试：

- Go: Page Pilot 使用 local repository target 修改真实 source file，将 run 写入 `page_pilot_runs`，列出 run，撤销 source patch，然后再次 apply 并创建 branch / commit。
- TS: omega control API client 覆盖 `/page-pilot/apply`、`/page-pilot/deliver`、`/page-pilot/runs` 与 discard 请求。

计划执行：

```bash
go test ./services/local-runtime/internal/omegalocal
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx
npm run lint
npm run build
```

### 后续工作

- 增加 Page Pilot Agent stage contracts：`preview-runtime`、`page-editing`、`delivery`。
- 增加 Preview Runtime Profile，让 Omega 自动理解不同项目的启动方式。
- 增加 preview runtime process supervisor，管理目标项目 dev server 的 pid、日志、健康检查、重启和失败诊断。
- 在 UI 中展示 Page Pilot session 对应的 Requirement / Work Item / Page Pilot pipeline run，并支持从 Work Item 回到 Page Pilot run。
- 将 direct pilot 结果面板升级为完整 patch preview：可展开文件 diff、语义摘要、行级摘要和 PR body 预览。
- 增加 live-preview repository lock，避免 Page Pilot 和 DevFlow 同时写同一 worktree。
- 在 Electron shell 中把 preview webview、repository target 和 local worktree 绑定成明确配置。
- 将 `/page-pilot-target` dev proxy 的调试能力收敛到 Electron preview bridge 或显式开发开关。
- 增加更完整 patch preview 和 proof UI。
- 将 Page Pilot proof 从文件和 run JSON 继续拆成可查询的一等 proof 记录。
- 扩展 Workboard 关键区域的 `data-omega-source`。
- 将 PR body 扩展为语义化变更摘要、DOM context 摘要和截图证据。

## 2026-04-27 Project Agent Profile Runtime Binding

### 目标

把前端 `Project Agent Profile` 从 localStorage 草稿推进到本地 Go runtime 可持久化、可读取、可参与执行的配置。配置需要覆盖：

- Workflow markdown 草稿
- 每个 Agent 的 runner / model / Skill / MCP / stage note
- `.omega/agent-runtime.json`
- `.codex/OMEGA.md`
- `.claude/CLAUDE.md`

### 产品入口

- Project 页面 `Project config`
- Repository Workspace 齿轮
- Repository Workspace subnav 的 `Workspace config`

Settings 页面中的 `Project Agent Profile` 现在分为三块：

- `Workflow`：编辑 workflow template 和 markdown 草稿，并展示阶段流。
- `Agents`：按 Agent role 配置 runner、model、Skills、MCP 和 stage note。
- `Runtime files`：预览并编辑 `.omega` / `.codex` / `.claude` 运行时配置。

### Go Runtime 做法

新增 `services/local-runtime/internal/omegalocal/agent_profile.go`：

- 定义 `ProjectAgentProfile` 和 `AgentProfileConfig`。
- 使用 `omega_settings` 持久化，key 规则：
  - `agent-profile:project:{projectId}`
  - `agent-profile:repository:{repositoryTargetId}`
- 新增 API：
  - `GET /agent-profile?projectId=...&repositoryTargetId=...`
  - `PUT /agent-profile`
- 解析顺序：
  1. Repository override
  2. Project profile
  3. 默认 profile

Pipeline 创建时会把 resolved profile 摘要写入 `run.agentProfile`，让一次运行可以追踪它使用了哪个配置来源。

Runner 执行时会写入：

- operation workspace 的 `.omega/agent-runtime.json`
- operation workspace 的 `.codex/OMEGA.md`
- operation workspace 的 `.claude/CLAUDE.md`
- DevFlow repository checkout 的 `.codex/OMEGA.md` / `.claude/CLAUDE.md`

Codex runner 的 prompt 会追加当前 Agent 的 policy block，review / coding / rework 阶段会读取 profile 中对应 Agent 的 model。

### 前端做法

`apps/web/src/omegaControlApiClient.ts` 新增：

- `ProjectAgentProfileInfo`
- `fetchProjectAgentProfile`
- `updateProjectAgentProfile`

`apps/web/src/App.tsx`：

- Settings 页面进入时从 Go runtime 拉取 profile。
- Save draft 会先写 localStorage 兜底，再通过 `PUT /agent-profile` 保存到本地 runtime。
- 保存成功后显示 `Saved to local runtime. New pipeline runs will use this profile.`

### API 文档

`docs/openapi.yaml` 增加 `/agent-profile` 的 GET / PUT 说明。

### 验证

已执行：

```bash
go test ./services/local-runtime/...
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx
npm run build
```

新增 Go 测试覆盖：

- profile 可以通过 API 保存/读取。
- pipeline 创建时带上 `run.agentProfile`。
- operation run 会生成 `.omega/agent-runtime.json` 和 `.codex/OMEGA.md`。

前端测试覆盖：

- Settings 中能看到 Workflow / Agents / Runtime files。
- Save draft 会走 `/agent-profile` 并显示保存到 local runtime。

### 后续工作

- Profile 已从 `omega_settings` 升级为一等 SQLite 表 `agent_profiles`，保留 settings 镜像作为旧 App / 调试工具兼容层。后续继续补校验、恢复默认、导入/导出。
- 增加正式 Workflow Template 编辑 API，把 markdown parse / validate / save 串起来。
- Runner registry 已接管 Agent 执行器选择：Codex、opencode、Claude Code 通过统一 `AgentRunner` 分发，并按 Agent profile 的 runner/model 写入 runner process。后续继续补 runner-specific 模板、timeout/cancel/retry 和 provider 映射。
- 在 Work Item detail 中展示当前 Attempt 实际使用的 profile source、Agent model、MCP、Skill 和 runtime files。

## 2026-04-27: Agent Profile 一等表与 Runner Registry

### 背景

上一版 Project Agent Profile 已经能在 UI 中编辑 workflow、Agent runner/model、Skill、MCP 和 `.codex` / `.claude` policy，也会写入 runtime bundle。但存储仍以 `omega_settings` 为主，执行器选择也主要停留在 Codex 默认路径。

### 本次实现

- 新增 SQLite 一等表 `agent_profiles`：
  - `id`
  - `scope`
  - `project_id`
  - `repository_target_id`
  - `version`
  - `profile_json`
  - `created_at`
  - `updated_at`
- `/agent-profile` 读写优先使用 `agent_profiles`，同时继续镜像到 `omega_settings` 兼容旧路径。
- 旧 settings profile 被读取时会自动迁移写入一等表。
- 新增 `AgentRunnerRegistry`：
  - `codex`
  - `opencode`
  - `claude-code` / `claude`
- `operations/run` 支持 `runner: "profile"` / `auto`，此时按当前 Agent profile 中的 runner 选择执行器。
- `run-devflow-cycle` 的 coding / rework / review Agent 开始按各自 Agent profile 选择 runner/model，而不是写死 Codex。
- local capability detection 增加 `claude-code`。
- 修复 `putOrchestratorWatcher` 返回 watcher 时与后台 goroutine 同时读写同一个 map 的并发崩溃。

### 验证

- Go 测试新增覆盖：
  - profile 保存后进入 `agent_profiles` 一等表，并继续写入 settings 兼容层。
  - `runner: "profile"` 会按 coding Agent 配置选择 `opencode` 执行器。
- `go test ./services/local-runtime/...`
- `npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx`

## 2026-04-28: Agent Runner 配置预检

### 背景

Agent Profile 已经能为不同 Agent 选择 Codex、opencode、Claude Code 等 runner。这里的产品语义应该是：前端配置决定 runner 分配，运行前做能力预检；如果本机没有安装对应 runner，应该阻止启动并提示配置或安装问题，而不是创建一次看起来已经进入执行的失败 runner process。

### 本次实现

- 前端 `Project Agent Profile -> Agents` 的 runner 选择接入 `GET /local-capabilities`：
  - 不可用 runner 在下拉项中 disabled。
  - 当前 Agent runner 下方展示 `ready` / 安装提示。
  - roster 中 runner 缺失的 Agent 会有警示样式。
- 保存 Project Agent Profile 时，如果本机 capabilities 已加载且存在不可用 runner，会展开 Agents tab、定位到第一个有问题的 Agent，并阻止保存。
- Go runtime 新增 runner preflight：
  - `local-proof` 不需要外部 CLI。
  - `demo-code` 需要 `git`。
  - `codex` 需要 `codex`。
  - `opencode` 需要 `opencode`。
  - `claude-code` 需要 `claude` 或 `claude-code`。
- `operations/run` 在创建真实运行 workspace 前检查当前 operation Agent 的有效 runner。
- `run-current-stage` 在创建 attempt 前检查当前 stage owner Agent 的有效 runner。
- `run-devflow-cycle` 和 orchestrator `autoRun` 在创建 attempt 前检查 coding / review Agent 的有效 runner。

### 验证

已执行：

```bash
go test ./services/local-runtime/...
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx
```

新增 Go 测试覆盖：

- `runner: "profile"` 指向未安装的 `opencode` 时，`/operations/run` 返回 `400 Bad Request`，不会进入真实 runner process。

前端测试更新：

- Settings / Agent Profile 保存用例补齐 `codex` capability，确保保存成功路径仍被覆盖。

### 后续工作

- 把 preflight 结果展示到 Workflow / Pipeline 启动按钮旁边，让用户在点击 Run 前就看到阻塞原因。
- 将 runner registry 补成完整 provider 映射：模型、环境变量、timeout、cancel、retry、runner-specific policy 文件。
- 在 Project Agent Profile 中增加恢复默认、导入/导出和版本历史。

## 2026-04-28: Page Pilot 提交过程面板与主目标修正

### 目标

补齐 Electron direct pilot 提交后的过程可见性，并修复多批注场景下 Agent 使用错误选区作为主目标的问题。

### 入口

- Electron direct pilot：`npm run desktop:pilot`
- 目标项目 URL：`OMEGA_PAGE_PILOT_URL`
- 运行时代码：`apps/desktop/src/pilot-preload.cjs`
- Go runtime apply prompt：`services/local-runtime/internal/omegalocal/page_pilot.go`

### 数据 / API

- `/page-pilot/apply` 仍接收单个 primary `selection` 和完整 `instruction`。
- Electron overlay 会把本次所有批注写入 `instruction`，同时选取最新一条带 `sourceMapping.file` 的批注作为 primary `selection`。
- 本地 UI 持久化的 Page Pilot run 增加 `submittedAnnotations` 和 `processEvents`，用于刷新后恢复结果面板中的过程信息。

### 运行时行为

- 用户提交后，底部批注编辑框立即切换为过程面板，展示：
  - 本次捕获了几条批注；
  - 哪一条是 primary target；
  - Agent 正在提交、应用源码变更、关联 Work Item / Pipeline、刷新预览；
  - changed files、diff summary、Requirement / Work Item / Pipeline linkage。
- 批注历史默认折叠，只显示最新一条；点击 `^` 可以展开全部，避免遮挡目标页面。
- Confirm / Discard 会保留过程事件，并继续调用 `/page-pilot/deliver` 或 `/page-pilot/runs/{id}/discard`。
- Go prompt 明确声明 `Selected element context` 是 primary target，多批注只作为辅助上下文，避免 Agent 被早期批注带偏。

### 验证

已执行：

```bash
node --check apps/desktop/src/pilot-preload.cjs
go test ./services/local-runtime/internal/omegalocal -run TestPagePilot
```

### 后续工作

- 把 `submittedAnnotations` / `processEvents` 从 Electron localStorage 升级为 Page Pilot run 的服务端字段。
- 增加完整 patch preview / PR body preview / Work Item 回跳。
- 为 DOM-only 批注增加源码候选定位能力，减少目标项目需要手写 `data-omega-source` 的范围。

## 2026-04-28: Page Pilot 精确元素命中

### 目标

让 Electron direct pilot 能稳定圈选小型链接、按钮和表单字段，例如登录页中的 `忘记密码？`、`立即注册`、`登录` 按钮，而不是被父级容器或卡片抢走。

### 入口

- `apps/desktop/src/pilot-preload.cjs`

### 数据 / API

不改变 runtime API。变化只发生在 selection context 采集前的候选元素选择阶段。

### 运行时行为

- hover / click 时使用 `document.elementsFromPoint` 获取鼠标坐标下的元素堆叠。
- 候选元素按交互精度排序：按钮、链接、输入框、下拉框、textarea、`role=button` 优先，然后才是 label、`data-omega-source`、标题、文本和卡片容器。
- selection 的 `elementKind` 增加 `link`、`field`、`label`，让 Agent 更清楚用户选中的是链接、表单字段还是普通文案。

### 验证

已执行：

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-30: Preview Runtime Profile 关联 Page Pilot Run 基础版

### 背景

- Electron 已能理解目标项目并启动 dev server，但 Go runtime 的 Page Pilot run 只保存修改结果，缺少“当时预览是怎么启动和刷新的”。
- 这会影响后续审计、失败诊断和 Work Item 详情页回看。

### 新做法

- Page Pilot 启动器在打开 direct pilot 时携带 Preview Runtime Profile。
- `pilot-preload.cjs` 在 `/page-pilot/apply` 和 `/page-pilot/deliver` 中提交 `previewRuntimeProfile`。
- Go runtime 把 profile 写入 Page Pilot run，并同步到 mission / pipeline artifacts。
- profile 记录 preview URL、工作目录、dev command、health check、reload strategy 和 evidence。

### 当前边界

- 这是持久化基础版，真实 dev server 启动和刷新仍由 Electron supervisor 执行。
- 后续需要把 Preview Runtime resolve/start/restart API 和 pid/stdout/stderr/health check 下沉到 Go runtime。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget'
```

## 2026-04-30: Page Pilot Source Mapping 覆盖率报告基础版

### 背景

- Page Pilot 的圈选定位质量会直接影响 Agent 修改的可靠性。
- 旧记录只保存 selection context，缺少对“本轮有多少强源码映射、多少 DOM-only”的结构化判断。

### 新做法

- `/page-pilot/apply` 基于本轮批注生成 `sourceMappingReport`。
- 报告包含批注总数、强源码映射数量、DOM-only 数量、缺失文件映射数量、覆盖率和状态。
- `/page-pilot/deliver` 保留同一报告。
- mission 和 pipeline artifacts 同步记录该报告，后续 UI 可以直接展示定位可信度。

### 当前边界

- 这是单 run 基础版统计。
- 按页面 / 组件聚合、定位失败原因分类和 DOM-only 源码候选能力仍需后续扩展。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget'
```

## 2026-04-30: Source Mapping Report 驱动 Agent 输入

### 背景

- 单纯记录覆盖率只能帮助人排查，不能直接提升 Agent 修改质量。
- Page Pilot 需要把定位质量变成执行链路的一部分：强映射走明确文件，DOM-only 先定位候选源码。

### 新做法

- `apply` 在 Agent 执行前生成 `sourceMappingReport`。
- 如果报告不是 `strong`，runtime 会生成 `sourceLocator`：
  - 使用文本快照、selector token、DOM tag 和批注 token 搜索仓库源码；
  - 跳过 `.git`、`.omega`、`node_modules`、`dist`、`build` 等目录；
  - 每条候选记录文件、分数、命中依据和预览行。
- Agent prompt 新增覆盖率报告和源码候选区块。
- prompt 规则要求低覆盖率时先检查候选文件；没有候选时说明缺少映射，不做无关大范围修改。
- `local-proof` 验证路径也会复用候选文件，DOM-only 批注可以在测试中真实修改源码。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotDomOnlySelectionUsesSourceLocator|TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget'
```

## 2026-04-30: Page Pilot live-preview 写锁基础版

### 背景

- Page Pilot `apply` 会直接修改目标预览工作区，用户确认前这些改动既不能被其他 Page Pilot run 覆盖，也不能被误当成另一个运行的上下文。
- 旧做法只有文档要求 live-preview 需要 repository write lock，但 Go runtime 的 apply / deliver / discard 没有真正声明和释放锁。

### 新做法

- `apply` 解析出明确 repository workspace 后，先声明 `page-pilot-live:{repositoryTargetId}:{repositoryPath}` execution lock。
- 锁会写入 `ownerType = page-pilot`、`pagePilotRunId`、`repositoryTargetId`、`repositoryPath`、`workItemId`、`pipelineId` 和保留策略。
- 同一个 run 的 `deliver` 可以继续使用这把锁完成 branch / commit / PR 交付。
- `discard` 和 `deliver` 成功后释放锁；`apply` 失败会自动释放锁，避免卡住后续操作。
- 第二个 Page Pilot run 在锁未释放时会被拒绝，前端可以直接展示“预览工作区正在被某个 Page Pilot run 持有”。

### 当前边界

- 这是 Go runtime 内的 Page Pilot live-preview 锁基础版，覆盖 Page Pilot run 之间的并发写入。
- Preview Runtime Profile 和 process supervisor 进入 Go 一等化后，还需要把 dev server 进程、刷新动作和锁状态统一关联到 Page Pilot run。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget'
```

## 2026-04-30: Page Pilot Recent Runs 与 Work Item 回跳基础版

### 目标

让功能二不再只是“打开目标页面做一次操作”。用户回到 Page Pilot 启动器后，应该能看到最近 run 的真实状态，并能跳回对应 Work Item 查看 pipeline、operation、proof 和后续交付记录。

### 新做法

- `PagePilotPreview` 消费已有 `GET /page-pilot/runs`，按当前 Repository Workspace 过滤后展示最近 8 条 run。
- 每条 run 展示状态、变更文件数量、Work Item ID、更新时间和 Pipeline ID。
- 存在 `pullRequestUrl` 时展示 PR 跳转。
- 存在 `workItemId` 时展示 Work Item 回跳，链接到独立 Work Item 详情路由。
- 该区块只消费真实 runtime 记录，不在前端伪造 run。

### 当前边界

- 这是 Page Pilot run 可追踪 UI 基础版。
- 同一 run 多轮追加批注 / 重新 apply 仍未完成，后续需要在服务端 conversation 基础上继续迭代。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
npm run lint
```

### 后续工作

- 在 direct pilot 中增加可视化 debug 模式，显示候选元素排名，便于排查复杂页面的遮挡和命中问题。
- 为跨 iframe / shadow DOM 场景补充更完整的 composed path / elementFromPoint 适配。

## 2026-04-30: Page Pilot 服务端 Run Conversation 基础版

### 背景

- 旧做法：Electron direct pilot 的批注轮次、primary target 和过程事件主要存在目标页 localStorage。
- 这会导致刷新、换页面或从 Work Item 回看时，服务端 run 只知道最终 diff，缺少“用户选了什么、如何提交、Agent 如何处理”的上下文。

### 实现

- `/page-pilot/apply` 接收可选 `conversationBatch`、`submittedAnnotations`、`processEvents`。
- Go runtime 在 Page Pilot run 中持久化：
  - `conversationBatch`：本轮批注、整体说明、主目标、runId、状态；
  - `submittedAnnotations`：提交给 Agent 的批注列表；
  - `primaryTarget`：服务端可查询的主目标摘要；
  - `processEvents`：目标页内的关键过程事件。
- `/page-pilot/deliver` 会把 conversation 状态同步为 `delivered`。
- `/page-pilot/runs/{id}/discard` 会把 conversation 状态同步为 `discarded`。
- `syncPagePilotRunRecords` 把 conversation 写入 mission 和 pipeline run artifacts，后续 Work Item 详情可以直接读取服务端记录。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget'
```

### 2026-04-28 补充

圈选分类不再作为可选门槛。所有可见 DOM 元素都可以进入候选，分类只用于排序、tooltip 和 Agent 上下文；未知类型仍会作为 `other` 元素采集 selector、文本、样式和 DOM context。

## 2026-04-28: Page Pilot 动态状态提示圈选

### 目标

支持用户圈选页面交互后动态出现的提示文案，例如登录按钮提交后由 JS 写入的 `role=status` 消息。

### 入口

- `apps/desktop/src/pilot-preload.cjs`

### 数据 / API

不改变 runtime API。动态状态元素没有强制要求 `data-omega-source`，selection context 会通过稳定 selector（例如 `#message`）、DOM context、文本和样式快照传给 Agent。

### 运行时行为

- 每次 hover/click 都从当前 live DOM 读取候选元素，不缓存启动时 DOM。
- `role=status`、`aria-live`、`.message`、`.alert`、toast / notice / error / success 类元素获得高优先级。
- `elementKind` 增加 `status`，让 Agent 知道用户选中的是运行时反馈文案。

### 验证

已执行：

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

### 后续工作

- 为运行时生成的 DOM-only 状态元素增加源码候选提示：例如通过 id、事件监听器、临近 submit handler、文本写入语句辅助定位源码。

## 2026-04-28: Page Pilot 当前选区 Esc 取消

### 目标

用户在框选元素后，如果还没有点击 `✓` 或正在填写当前批注，可以按 `Esc` 取消这一轮选区，不影响已经添加好的批注队列。

### 入口

- `apps/desktop/src/pilot-preload.cjs`

### 运行时行为

- `Esc` 会清除当前 hover highlight、`✓ / ×` 确认控件、当前批注输入框和临时 `selected`。
- 已经 `✓` 后发送进入队列的批注、页面 pin 和底部历史列表不会被删除。
- 清空全部批注仍然需要点击底部 `清空`。

### 验证

已执行：

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot 批注编辑与过程视觉优化

### 目标

改善批注输入体验：编号更清晰、序号按真实批注顺序显示、已保存批注可回点编辑，并让 Agent 执行过程有更明确的视觉反馈。

### 入口

- `apps/desktop/src/pilot-preload.cjs`

### 运行时行为

- 选区编号 pin 和 composer badge 使用更明确的渐变圆形样式。
- 新批注 composer 的编号来自 `annotations.length + 1`，第二条批注显示 `2`，不再固定为 `1`。
- 点击页面上的编号 pin 或底部批注 chip，会打开同一条批注的 composer，并回填原 comment；提交后更新原批注，不改变顺序。
- 批注折叠控件从小符号改为 `展开批注 / 收起批注` 文字按钮。
- Page Pilot process panel 在执行中显示旋转 loading 指示，减少“黑箱等待”感。
- 单条批注 composer 的提交按钮从图标箭头改为 `添加 / 保存`。
- 当前批注队列的整批提交按钮改为无文字图标主按钮，并和输入框、清空按钮统一垂直对齐。
- Agent 执行面板的折叠单位从单条批注改为“一轮 Page Pilot 对话 / 一次 Agent 调用”。每轮对话包含该次提交的所有批注、primary target、整体 instruction 和状态。
- 新增批注保存后自动回到选择态，用户可以连续框选多个元素，不需要每次点击右下角手指。

### 验证

已执行：

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot 顶层状态栏与轻量输入栏

### 目标

减少 Page Pilot 执行态和结果态遮挡目标页面，让用户能继续看预览效果，同时把交付操作固定在顶层可见位置。

### 入口

- `apps/desktop/src/pilot-preload.cjs`

### 运行时行为

- Page Pilot process/result 从底部大卡片改为顶部状态栏。
- 默认只显示一条薄状态：标题、当前对话轮次摘要、pin/展开控件、右侧操作按钮。
- hover 或点击 pin 会展开详情，展示对话轮次、diff summary 和 process events。
- Confirm / Discard / Reload / New 固定在顶部状态栏右侧，按钮高度降为 34px，不再混在可滚动日志底部。
- 底部批注输入栏高度压缩到 42px，提交和清空按钮同高对齐，减少遮挡。
- 取消 hover 自动展开，避免用户只是经过状态栏就打开详情。
- 状态栏默认吸附在底部，点击 pin 展开/收起；点击 `↑/↓` 可以在底部和顶部之间切换，并用 localStorage 记住吸附位置。
- 状态栏吸附按钮升级为 SVG move 拖动手柄：按住可在页面内自由拖动；松手时靠近顶部或底部会自动吸附，否则保持浮动位置并持久化。
- 展开按钮改为 SVG expand/minimize 图标，避免使用文字括号。
- 收起且吸附在顶部/底部时，状态栏会缩进窗口边缘，只露出 8px hover 热区；鼠标移到边缘由 JS peek 状态稳定滑出，离开后延迟缩回，避免纯 CSS hover 在边界反复弹跳。展开态不会自动缩进。

### 验证

已执行：

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot FAB 视觉对齐 New Requirement

### 目标

让 Electron direct pilot 右下角手指按钮与 Workboard `New requirement` 主按钮保持一致的品牌动效和颜色。

### 入口

- `apps/desktop/src/pilot-preload.cjs`

### 运行时行为

- 右下角 FAB 使用同一套蓝青渐变：`#4f8cff -> #3478ff -> #20c9f3 -> #5eead4`。
- 外圈使用 conic-gradient 旋转描边，节奏与 `New requirement` 的 orbit 动画一致。
- 按钮内容包一层 `span`，避免伪元素覆盖图标；选择态仍切换为 `×`。

### 验证

已执行：

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot 飞书风格浅色浮层与更严格吸附阈值

### 目标

把 Electron direct pilot 的悬浮窗口从深色面板改为更接近飞书的浅色工作台风格，并降低拖拽吸附触发范围，避免状态栏还没靠近窗口边缘就缩进去。

### 入口

- `apps/desktop/src/pilot-preload.cjs`

### 运行时行为

- Page Pilot tooltip、批注输入栏、Agent 执行状态栏、结果详情、确认浮层统一改为白底、浅灰边框、蓝色主按钮、柔和投影。
- 主操作使用飞书蓝视觉，危险操作使用浅红底和红色文本，辅助 chip 使用浅灰/浅蓝 hover。
- 输入框、批注 chip、执行日志行使用浅色 surface，降低对目标页面的遮挡感。
- 状态栏拖拽吸附阈值从 42px 降为 16px；只有真正贴近顶部或底部边缘时才会吸附并进入可缩进状态。

### 验证

已执行：

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-29: Requirement 创建链路拆分

### 目标

降低 `App.tsx` 和 Go `server.go` 的维护压力，先把最近反复调试的 Requirement 创建 / Work Item 写入链路拆成独立模块，方便后续补接口测试、重试和数据分析。

### 入口

- `apps/web/src/components/RequirementComposer.tsx`
- `apps/web/src/components/ProjectSurface.tsx`
- `apps/web/src/components/WorkspaceChrome.tsx`
- `apps/web/src/core/manualWorkItem.ts`
- `services/local-runtime/internal/omegalocal/work_items.go`
- `services/local-runtime/internal/omegalocal/devflow_cycle.go`
- `services/local-runtime/internal/omegalocal/pipeline_records.go`

### 运行时行为

- 前端创建 Requirement 的交互不变：仍从 Workboard 内填写标题/描述并写入当前 Repository Workspace。
- `App.tsx` 只保留状态编排和提交动作，创建表单 UI 交给 `RequirementComposer`。
- Project / Repository Workspace 总览交给 `ProjectSurface`，App 只传入当前项目、仓库列表和操作回调。
- Workboard shell 交给 `WorkspaceChrome`，包括左侧导航、workspace 切换、顶部搜索、主题切换和详情页运行工具栏。
- 手动 Work Item 的 id/key/title/default acceptance criteria 生成集中在 `manualWorkItem.ts`。
- Go runtime 的 Work Item append、unique id/key、Requirement link/enrich 逻辑集中在 `work_items.go`，接口行为保持不变。
- 未开始 Work Item 支持真实删除：列表左侧按钮调用 `DELETE /work-items/{itemId}`，runtime 只允许 `Ready` / `Backlog` 且没有执行历史的 item 删除，并同步移除未共享 Requirement。
- DevFlow PR 长流程执行集中在 `devflow_cycle.go`，包括 attempt reset、后台 job、agent invocation、review/rework/human gate。
- Pipeline record / workflow template 物化集中在 `pipeline_records.go`，包括 `devflow-pr` stage、agent contract、mission/operation record 构造。

### 验证

```bash
npm run test -- apps/web/src/core/__tests__/manualWorkItem.test.ts apps/web/src/__tests__/App.operatorView.test.tsx -t "manual work item helpers|creates app requirements" --testTimeout=15000
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run lint
npm run build
go test ./services/local-runtime/internal/omegalocal
```

### 后续工作

- 继续拆 `App.tsx` 中 Work item list/detail、GitHub issue sync、runtime settings 三块。
- 继续拆 `server.go` 中 HTTP handler 注册、delivery/PR API、workspace cleanup API。

## 2026-04-29: Page Pilot Proof Records

### 目标

把 Page Pilot 的 apply / deliver / discard 结果从私有 run JSON 和 `.omega/page-pilot` 文件，补齐到 Omega 通用的 Mission / Operation / Proof records。这样 Page Pilot 创建出来的 Work Item 在详情页、Proof 面板和 Agent trace 中都能看到同一套执行证据，而不是只能回到 Electron 浮层或 `/page-pilot/runs` 查询。

### 入口

- `POST /page-pilot/apply`
- `POST /page-pilot/deliver`
- `POST /page-pilot/runs/{id}/discard`
- Work item detail 的 Agent trace / Proof 面板

### 运行时行为

- apply 成功后，为对应 `page-pilot` Pipeline 写入 `Page Pilot Agent` Mission、`page_editing` Operation 和 proof records。
- proof records 指向 `page-pilot-prompt.md`、`page-pilot-agent-note.md`、`page-pilot.diff`、`page-pilot-summary.md` 等真实 proof 文件。
- deliver 成功后补写 `delivery` Operation，并把 Pipeline 状态更新为 `delivered`，PR URL 会作为 proof URL 进入通用 proof stream。
- discard 后同样记录 `delivery` 阶段失败/丢弃事件，保留可审计状态。
- Pipeline stage 会同步写入 `evidence`，因此现有详情页无需 Page Pilot 专用逻辑即可展示 artifact。

### 验证

已执行：

```bash
go test ./services/local-runtime/internal/omegalocal -run TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget
```

## 2026-04-29 14:53 CST: Run Timeline API / Work Item Detail 基础版

### 目标

给功能一补一条按 Attempt 聚合的真实运行时间线，让 Human Review、失败排障和本地联调不再只依赖分散的 proof 文件或前端快照。

### 入口

- `GET /attempts/{id}/timeline`
- Work Item detail -> Current attempt -> Run timeline

### 实现架构

- Go runtime 新增 `AttemptTimelineResponse` / `AttemptTimelineItem`，`attempt_timeline.go` 只读 SQLite 中已持久化的 Attempt、Pipeline run、Checkpoint、Operation、Proof 和 Runtime log。
- 时间线按 `attemptId` 和 `pipelineId` 归集记录，去重后按事件时间排序；不存在的记录不会生成占位数据。
- 前端 `omegaControlApiClient` 增加 `fetchAttemptTimeline`，`App.tsx` 在打开 Work Item 详情时按当前 attempt 拉取时间线。
- `WorkItemAttemptPanel` 增加 `RunTimeline` 区块，展示最近真实事件的 source、event type、stage、agent 和时间，支持 light / dark。

### 验证

已执行：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestAttemptTimelineAggregatesRunRecords|TestCancelAttemptAPIUpdatesStateAndSignalsRunningJob|TestRuntimeLogsAPIListsAndFiltersRecords'
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx -t "shows review packet details" --testTimeout=15000
```

后续仍需在全量验证中覆盖 lint、build 和完整 Go 测试。

## 2026-04-29: Attempt Retry API 基础版

### 旧做法

Workboard 列表上的 Retry 主要按 Work Item / Pipeline 重新触发运行，旧 Attempt 会保留，但新旧 Attempt 之间没有明确 retry 关系，后续排障需要人工对时间和 pipeline 记录。

### 新做法

- 新增 `POST /attempts/{id}/retry`，只允许 `failed` / `stalled` / `canceled` Attempt 重试。
- 新 Attempt 继承同一个 Work Item、Pipeline、Repository Workspace，并写入 `retryOfAttemptId`、`retryRootAttemptId`、`retryIndex`、`retryReason`。
- 旧 Attempt 写入 `retryAttemptId` 和 `attempt.retry.requested` event，保留完整审计链。
- Work Item detail 对可重试 Attempt 展示 `Retry attempt`，调用真实 API 后刷新 control plane。

### 验证

已新增：

```bash
go test ./services/local-runtime/internal/omegalocal -run TestPrepareDevFlowAttemptRetryLinksAttempts
```

## 2026-04-29: PR Lifecycle Detail 基础版

### 旧做法

Work Item detail 和 Human Review packet 主要展示 PR URL、branch 和 proof 文件；checks、review decision、mergeable、delivery gate 虽然已有 `/github/pr-status` API，但没有进入详情页主视图。

### 新做法

- 当当前 Attempt 有真实 `pullRequestUrl` 时，前端调用 `/github/pr-status`。
- API 成功后才渲染 PR lifecycle 卡片，展示 review decision、mergeable、head/base branch、delivery gate 和 check 列表。
- API 失败或没有 PR URL 时不展示该卡片，避免假状态。

### 验证

已新增/更新前端详情页测试覆盖，随常用验证执行：

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx -t "shows review packet details" --testTimeout=15000
```

## 2026-04-29: P0 JobSupervisor / Preflight / Run Report 基础版

### 旧做法

- `job-supervisor/tick` 主要做 checkpoint integrity 和 stalled detection，不扫描 Ready Work Item。
- DevFlow 运行前的检查分散在 handler、Agent runner preflight 和运行中错误里。
- Human Review 材料分散在 proof 文件、PR URL、timeline 和 runtime log 中，没有单页报告。

### 新做法

- 新增 JobSupervisor daemon：Go Local Runtime 启动时默认运行后台维护 tick，启动后立即执行一次，之后按固定 interval 执行。
- daemon 默认只做安全维护：checkpoint integrity、running attempt stalled detection、Ready item preflight scan；不会隐式启动 Ready item，除非显式传入 `job-supervisor-auto-run-ready`。
- `job-supervisor/tick` 增加 runnable scan：扫描 `Ready` 且绑定 repository target 的 Work Item，统一执行 DevFlow preflight；显式传 `autoRunReady=true` 时创建 Attempt 并交给后台 job。
- 新增 `devflow_preflight.go`：集中检查 repository target、workspace root、git/gh、runner availability、local dirty state，并被 manual run、Attempt retry、JobSupervisor scan 复用。
- 新增 `devflow_report.go`：DevFlow 进入 Human Review 前生成 `attempt-run-report.md`，聚合需求、PR、changed files、测试、remote checks、review verdict 和 artifact 清单。

### 验证

已新增/更新：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestJobSupervisorTickReportsRunnableReadyWork|TestRunDevFlowPRCycleCreatesBranchPRAndMergeProof|TestPrepareDevFlowAttemptRetryLinksAttempts'
go test ./services/local-runtime/internal/omegalocal -run 'TestJobSupervisorTickBackfillsPendingCheckpointAttemptLink|TestJobSupervisorTickMarksStalledRunningAttempt|TestJobSupervisorLoopRunsMaintenanceTicks|TestJobSupervisorTickReportsRunnableReadyWork|TestCancelAttemptAPIUpdatesStateAndSignalsRunningJob'
```

## 2026-04-29: Omega CLI 基础版

### 旧做法

只能通过 Workboard UI 或直接 curl Runtime API 做 operator 操作；已有命令入口只负责启动 Go Local Runtime，不是可直接使用的控制 CLI。

### 新做法

- 新增 `services/local-runtime/cmd/omega` 作为命令行入口，可通过 `go install ./services/local-runtime/cmd/omega` 安装成 `omega` 命令。
- 新增 `services/local-runtime/internal/omegacli`，作为薄 HTTP client 调用 Go Local Runtime API。
- 支持 `health`、`status`、`logs`、`work-items list/run`、`attempts list/timeline/retry/cancel`、`checkpoints list/approve/changes`、`supervisor tick`。
- `work-items run` 会读取真实 workspace，查找或创建 `devflow-pr` Pipeline，再调用现有 DevFlow run API；CLI 不直接读写 SQLite，也不复制 server 编排逻辑。
- 新增独立说明文档：`docs/omega-cli.md`。

### 验证

```bash
go test ./services/local-runtime/internal/omegacli ./services/local-runtime/cmd/omega
npm run omega -- status
```

## 2026-04-29: P1 Observability Dashboard 基础版

### 旧做法

`/observability` 主要返回 counts、pipeline/checkpoint/operation/work item status、attention 和 recent errors。它适合确认当前状态，但不直接回答“哪些 run 最慢、失败集中在哪里、哪些 human gate 等待最久、operator 下一步该看哪里”。

### 新做法

- 保留旧 summary 字段，向后兼容 UI 和 CLI。
- 新增 `counts.attempts` 和 `attemptStatus`，把 Attempt 也纳入 summary。
- 新增 `dashboard`：
  - `attempts`：Attempt total / terminal / active / successRate / status。
  - `failureReasons`：按失败、stalled、canceled Attempt 的错误原因聚合。
  - `slowStages`：从 Attempt stage snapshot 计算最慢阶段。
  - `waitingHumanQueue`：按 pending checkpoint 生成待人工队列。
  - `activeRuns`：展示 running / waiting-human Attempt 的 lastSeen age。
  - `recommendedActions`：根据失败、stalled、pending gate、blocked item 生成 operator 动作建议。
- `omega status` 消费 dashboard attempts 和 recommended actions，但仍只通过 Runtime API 读取真实数据。

### 验证

```bash
go test ./services/local-runtime/internal/omegacli ./services/local-runtime/internal/omegalocal -run 'TestStatusPrintsObservabilitySummary|TestObservabilityDashboardMetrics|TestObservabilitySummary|TestRuntimeLogsAPIListsAndFiltersRecords'
```

## 2026-04-29: 正式 JobSupervisor v1

### 旧做法

P0 基础版已经有 daemon、checkpoint integrity、stalled detection、Ready item preflight scan、显式 `autoRunReady` 和手动 Attempt retry，但 JobSupervisor 还没有把 failed/stalled recovery、retry policy、workflow contract 状态纳入同一个调度 tick。因此文档中保留了“正式 JobSupervisor”未完成项。

### 新做法

- `POST /job-supervisor/tick` 在同一次 tick 中处理：
  - pending human gate integrity repair / attempt backfill。
  - workflow contract metadata 校验和回填。
  - running Attempt heartbeat stalled detection。
  - Ready + repository target Work Item preflight scan。
  - failed / stalled Attempt recovery scan。
- 自动写仓库仍是显式策略：
  - `autoRunReady=true` 才会启动 Ready Work Item。
  - `autoRetryFailed=true` 才会对 failed / stalled Attempt 创建 retry Attempt。
- 自动恢复策略基础版：
  - `maxRetryAttempts` 控制每个 retry root 的最大 retry 次数。
  - `retryBackoffSeconds` 控制失败/卡住后多久才可重试。
  - retry 仍调用 `prepareDevFlowAttemptRetry`，复用 repository target、preflight、retry metadata 和后台 job。
- Workflow contract 基础消费：
  - Pipeline run metadata 保存 workflow source、runtime、review rounds、transitions。
  - JobSupervisor tick 会校验/回填 devflow pipeline 的 workflow contract metadata。
  - summary/log 暴露 workflow contract pipeline 数、缺失数和 contract 摘要。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestJobSupervisorTickBackfillsWorkflowContractMetadata|TestJobSupervisorScanRecoverableAttempts|TestJobSupervisorTickReportsRunnableReadyWork|TestPrepareDevFlowAttemptRetryLinksAttempts|TestWorkflowTemplateLoadsDevFlowMarkdown'
```

## 2026-04-29: DevFlow Production Core P1

### 旧做法

- Workflow runtime 只驱动 review cycle，Attempt timeout、runner heartbeat、retry 上限和 backoff 仍主要来自 Go 常量、CLI flag 或 API payload。
- 长时间 runner 子进程没有 stdout/stderr 级别的 Attempt heartbeat，排障主要看最终 process result。
- PR status 读取 checks，但没有缺失 required checks、branch sync、merge conflict 和推荐动作的统一输出。
- Workboard manual run / retry 的并发保护主要依赖 active Attempt；execution lock 覆盖不如 issue auto-run 路径完整。
- Workspace root、repo path、cleanup 意图和锁信息分散在运行逻辑和 proof 中。

### 新做法

- `devflow-pr` workflow runtime 增加 `runnerHeartbeatSeconds`、`attemptTimeoutMinutes`、`maxRetryAttempts`、`retryBackoffSeconds`、`requiredChecks`。
- DevFlow 执行器消费 runtime policy：runner heartbeat、Attempt timeout、JobSupervisor retry 上限和 backoff 不再只靠硬编码默认值。
- supervised runner command 输出 stdout / stderr / heartbeat event；Attempt `lastSeenAt`、Attempt events 和 runtime DEBUG log 会随子进程输出刷新。
- `/github/pr-status` 输出 `checkSummary`、`branchSync`、`recommendedActions`，可区分 failed / pending / missing required checks、behind 和 conflict。
- 新增 `workspace_lifecycle.go`：manual run、retry、JobSupervisor auto run 会声明 workspace execution lock；运行时写入 `.omega/workspace-lifecycle.json`。
- 新增 `docs/devflow-production-core.md`，把功能一生产化内核从大文档中拆出来维护。

### 验证

```bash
go test ./services/local-runtime/...
```

## 2026-04-29: DevFlow Contract / Cleanup / Worker Lease P1

### 旧做法

- workspace lifecycle 已能写入 spec 和 execution lock，但没有和数据库状态联动的 cleanup 策略。
- workflow contract 主要来自默认 Markdown 模板；目标仓库自己的运行协议还没有被 runtime 优先消费。
- coding / review / rework prompt 仍主要由 Go 字符串拼接，workflow markdown 不能直接定义 Agent prompt section。
- JobSupervisor 可以扫描 stalled / retry，但没有明确记录本机 worker host lease；runtime 重启或 job 丢失后，只能依赖 heartbeat 超时兜底。

### 新做法

- 新增 `POST /workspaces/cleanup` 和 JobSupervisor workspace cleanup scan：
  - 已完成 Attempt 满足 retention 后可清理 `repo` checkout。
  - `.omega` proof / lifecycle 默认保留。
  - 失败、取消、stalled Attempt 默认保留，避免丢失排障材料。
  - cleanup 结果写回 Attempt `workspaceCleanup` metadata，并追加 `workspace.cleaned` event。
- 新增 repo-owned workflow contract：
  - 目标仓库 `.omega/WORKFLOW.md` 优先于默认模板。
  - Agent Profile 中带 front matter 的 workflow markdown 可作为 Project / Repository override。
  - 加载时校验 stage、transition、review round、agent 和 runtime，失败时阻止运行。
- Workflow Markdown body 支持 prompt sections：
  - `## Prompt: coding`
  - `## Prompt: rework`
  - `## Prompt: review`
  - runtime 会渲染变量后传给对应 Agent，并继续追加 Agent Profile policy。
- JobSupervisor / DevFlow 增加 worker lease：
  - background Attempt 会记录本机 worker host。
  - execution lock 会记录 worker host 和 runner process state。
  - JobSupervisor tick 会把没有本机 job 且锁无效的 running Attempt 标记为 `stalled`，后续 retry 策略可以接管。

### 验证

```bash
go test ./services/local-runtime/...
```

## 2026-04-29: Human Review Request Changes 进入真实 Rework

### 旧做法

- Human Review 点 Request changes 后，只把 checkpoint 写成 `rejected`，并把 pipeline stage 重置为 ready / waiting。
- 人工反馈只出现在 run event 中，不会创建新的 Attempt，也不会稳定进入下一轮 Agent prompt。
- 前端点击后只能看到 timeline 多了 rejected 事件，容易误判链路是假的。

### 新做法

- `POST /checkpoints/{id}/request-changes` 对 DevFlow Human Review checkpoint 会创建新的 `human-request-changes` Attempt。
- 旧 Attempt 会标记为 `changes-requested`，并保存 `failureReviewFeedback`、`statusReason`、`retryAttemptId`。
- 新 Attempt 会保存 `humanChangeRequest`、`retryReason`、`retryOfAttemptId`、`retryRootAttemptId` 和 retry index。
- Pipeline run event 增加 `human.rework.requested` / `human.rework.started`，Run Workpad 的 Review Feedback / Retry Reason 会直接展示人工反馈。
- DevFlow prompt 会把最新人工反馈追加到 requirement description，并写入 `reviewFeedback` 变量，下一轮 Agent 可以真实消费“需要改什么”。
- Request changes 后前端立即做轻量刷新，并延迟再刷新一次，减少用户看到“没反应”的窗口。

### 当前边界

- 当前实现会启动新的完整 DevFlow Attempt，但复用同一 Work Item workspace 和分支名；下一步应继续收敛为只从 Rework 阶段续跑，减少不必要的 requirement / architect 重跑。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback|TestPrepareDevFlowAttemptRetryLinksAttempts|TestApproveDevFlowCheckpointCanContinueDeliveryAsync'
go test ./services/local-runtime/internal/omegalocal
npm run lint
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx apps/web/src/components/__tests__/WorkItemDetailPanels.test.tsx apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run build
```

## 2026-04-30: Human Review Rework 续写同一 PR

### 旧做法

- Human Review Request changes 会创建新 Attempt，但 UI 还没有明确说明它是否基于第一次版本继续修改。
- 如果本地隔离 workspace 不存在，delivery branch 恢复会退回 base branch，存在把人工 rework 当作新实现跑的风险。
- 二次 review 只看当前 diff 和 requirement，没有明确提示“这是人工要求修改后的增量 diff”。
- PR description 创建后不随人工 rework 更新，GitHub PR 可能缺少人工意见和本轮增量 diff。

### 新做法

- Human-requested rework Attempt 继承上一轮 `branchName`、`pullRequestUrl`、`workspacePath`。
- Runtime checkout 时优先恢复本地同名 branch；本地 branch 不存在时优先从 `origin/{branch}` 恢复，再退回 base branch。
- Review prompt 增加人工 / 上轮 review feedback，并提示 reviewer 按“上一版到本轮的增量 diff”核对人工意见。
- PR description 在人工 rework 或自动 rework 后按需更新，加入人工意见、changed files、validation 和本轮增量 diff。
- Human Review UI 去掉重复产品名前缀，PR/Changed/Validation/Artifacts 改为左右紧凑布局；Delivery Flow 的 passed/done 视觉改为轻量通过信号。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback|TestEnsureDevFlowRepositoryWorkspaceRestoresRemoteDeliveryBranch|TestWorkflowTemplateLoadsDevFlowMarkdown'
npm run lint
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx apps/web/src/components/__tests__/WorkItemDetailPanels.test.tsx --testTimeout=15000
```

## 2026-04-30: Human Review Request Changes 评估与分流

### 旧做法

- Request changes 会创建真实 `human-request-changes` Attempt，但默认更像“带人工意见的完整重跑”。
- 局部 UI / 文案改动没有直接从 Rework 阶段续跑，导致响应慢、执行成本高。
- 架构、接口、权限、数据模型变化没有显式重规划标记，review / retry 只能从事件和 artifact 里反推原因。
- 人工意见为空或不明确时，系统仍可能继续启动 Agent，存在猜测需求的风险。

### 新做法

- 新增 Rework Assessment，详见 `docs/human-review-rework-assessment.md`。
- Runtime 会根据人工意见生成 `reworkAssessment`，写入旧 Attempt、新 Attempt 和 Run Workpad。
- `fast_rework`：直接进入 `rework` stage，复用上一轮 workspace / branch / PR，只生成本轮增量 diff、validation、review proof 和更新后的 PR description。
- `replan_rework`：从 `todo` 重新整理 requirement / solution plan，再继续实现，但仍基于上一轮 delivery branch / PR 推进。
- `needs_human_info`：不启动 Agent，Attempt 保持 `waiting-human`，Workpad 解释需要补充的人工信息。
- Work Item 详情页 Run Workpad 新增 Rework Assessment 摘要区块，默认折叠，展开后显示 rationale、human feedback 和 checklist。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback|TestPrepareDevFlowHumanRequestedReworkWaitsWhenFeedbackNeedsInfo|TestAssessHumanRequestedReworkRoutesByScope|TestResetDevFlowPipelineForAttemptFromStageFallsBackWhenStageMissing'
npm run lint
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx apps/web/src/components/__tests__/WorkItemDetailPanels.test.tsx --testTimeout=15000
```

## 2026-04-30: DevFlow Rework Checklist

### 旧做法

- Retry Reason、Review Feedback、PR/check 推荐动作和人工 request changes 虽然都能显示，但来源分散。
- Rework Agent prompt 主要消费当轮 feedback 字符串；失败重试时需要调用方重新拼接上下文。
- 用户看到 “Retry” 时仍需要打开 timeline、artifact 或 PR status 自己判断下一轮该改什么。

### 新做法

- 新增 `services/local-runtime/internal/omegalocal/rework_checklist.go`，集中生成 `reworkChecklist`。
- Checklist 来源包括人工反馈、Review Agent 输出、失败原因、runner stderr、operation/event、checks/rebase/conflict 推荐动作。
- Attempt 失败、取消、Human Review request changes、手动 retry、Run Workpad 刷新都会生成或继承 checklist。
- Retry API 在没有显式 reason 时会使用 checklist 的 `retryReason`。
- Rework prompt 优先使用 checklist 的 `prompt`，并在自动 review rework 时追加最新 review feedback。
- Run Workpad 新增 `reworkChecklist` 字段，后续 UI 可以直接展开来源和执行清单。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestBuildReworkChecklistMergesReviewHumanAndDeliveryGateSignals|TestRunWorkpadRecordTracksAttemptRetryContext|TestPrepareDevFlowAttemptRetryLinksAttempts|TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback'
go test ./services/local-runtime/internal/omegalocal
```

## 2026-04-30: Page Pilot 隔离 Preview Workspace

### 旧做法

- Page Pilot 打开目标项目预览时，Electron 侧曾尝试从用户本机常见目录推断 GitHub target 对应的本地 worktree。
- Go runtime 对 GitHub target 只在当前启动目录刚好是目标仓库时才能 apply。
- 这两者都会让“看到的预览”和“实际写入的仓库”存在不一致风险。

### 新做法

- local target：只使用用户显式绑定的本地 `path`。
- GitHub target：Electron shell 准备 Omega 管理的隔离 preview workspace，默认路径为 `~/Omega/workspaces/page-pilot/<owner_repo>`。
- `omega-preview:resolve-target` 返回真实 `repoPath`、可用 `index.html` 或 preview URL；不会扫描 `~/Projects` 等用户目录。
- Go runtime Page Pilot apply 同步改为只认显式 local target 或同一个隔离 preview workspace。
- Open preview 失败会回传 Electron `loadURL` 错误并记录主进程日志，方便判断是端口未启动、URL 错误还是页面自身为空。

### 验证

```bash
npm run test -- apps/web/src/__tests__/desktopProcessSupervisor.test.ts apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget'
```

## 2026-04-30: Page Pilot 全页预览模式

### 旧做法

- Electron BrowserView 只覆盖 Omega 页面右侧区域，看起来像一块浮层。
- 用户进入目标页面后仍能看到 Omega 的左侧栏和配置区，容易误解为“预览嵌在控制台里”，也不利于检查真实页面布局。

### 中间尝试

- `omega-preview:open` 打开后 BrowserView 覆盖整个 Electron content area，用户看到的是目标 App 完整页面。
- 目标页面通过 `preview-preload.cjs` 注入最小 Page Pilot 控制条：圈选元素、刷新、返回。
- 圈选元素后 BrowserView 自动关闭，并把 selection context 发送回 Omega Page Pilot 面板，继续展示指令输入和 apply/deliver 链路。

### 新做法

- 保留 BrowserView 全页打开目标 App。
- BrowserView 使用 `pilot-preload.cjs`，目标页面内直接完成圈选、多批注、整体说明、Apply、Confirm、Discard 和返回。
- Page Pilot React 页面只做 repo / preview source 启动器，不再承载目标页内圈选结果。

### 验证

```bash
npm run lint
npm run test -- apps/web/src/__tests__/desktopProcessSupervisor.test.ts apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
```

## 2026-04-30: Page Pilot Dev Server Preview Runtime Agent

### 旧做法

- Page Pilot 的 Dev server 模式把输入框内容当作 URL 直接交给 Electron 打开。
- 如果目标项目服务没有启动，用户只能看到连接失败或空白页。
- Electron 虽然有启动时的保守 preview supervisor，但它依赖环境变量，不会在用户选择 Repository Workspace 后动态理解并启动目标项目。

### 新做法

- Dev server 模式改为 `Dev server by Agent`。
- 用户选择 Repository Workspace 后，前端调用 `omegaDesktop.startPreviewDevServer`。
- Electron 本地 Preview Runtime Agent 锁定所选 repository target：
  - local target 只使用显式绑定路径；
  - GitHub target 使用 Omega 管理的隔离 preview workspace。
- Agent 读取项目线索，生成 Preview Runtime Profile，包含 `agentId`、`stageId`、工作目录、dev command、preview URL、health check、reload strategy 和 evidence。
- supervisor 在对应 repository workspace 内启动 dev server，并等待 health check 成功。
- 只有启动成功后才打开 direct pilot；失败原因直接展示在 Page Pilot 启动器里。

### 当前边界

- 这是 Electron 侧基础版 Preview Runtime Agent，已能真实启动服务。
- profile / pid / stdout / stderr 还没有进入 Go runtime 一等记录，后续继续做 Go Preview Runtime Profile API 和 process supervisor。

### 验证

```bash
npm run test -- apps/web/src/__tests__/desktopProcessSupervisor.test.ts apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
```

## 2026-04-30: Page Pilot Preview Runtime Reload Supervisor

### 旧做法

- 目标页内 `Reload` 和 Page Editing Agent apply 后的刷新直接调用浏览器 reload。
- Electron 不知道当前项目是否需要 HMR 等待、浏览器刷新还是 dev server restart。
- 如果 Agent 修改了 `package.json`、lockfile、框架配置或服务已挂掉，单纯 reload 不能保证用户看到最新页面。

### 新做法

- `pilot-preload.cjs` 的 Reload / apply / discard 刷新统一调用 Electron `omega-preview:reload`。
- Electron 主进程把刷新交给 Preview Runtime Supervisor。
- Supervisor 根据 Preview Runtime Profile 和 changed files 选择策略：
  - 普通源码变更：按 profile 执行 `hmr-wait` 或 `browser-reload`；
  - 运行时配置变更：提升为 `server-restart`；
  - 服务 health check 失败：自动 restart 后再刷新。
- Electron 只负责触发和刷新 BrowserView，不再决定具体技术策略。

### 当前边界

- 这是 Electron 侧基础版 reload supervisor。
- 后续 Go runtime 一等化后，需要把 refresh action、restart pid、stdout/stderr、health check 结果写入 Page Pilot run 记录。

### 验证

```bash
npm run test -- apps/web/src/__tests__/desktopProcessSupervisor.test.ts apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
npm run lint
node --check apps/desktop/src/process-supervisor.cjs
node --check apps/desktop/src/main.cjs
node --check apps/desktop/src/omega-preload.cjs
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-30: 功能一 / 功能二 P0 测试闭环

### 目标

把最近拆出的 8 个高优先级验收项收敛成可重复验证的产品能力，而不是只停留在 UI 或局部 API：

1. 功能一 GitHub delivery contract preflight。
2. 功能一 Reject -> Rework 可视化。
3. 固定标准测试脚本和测试报告。
4. Go runtime 全量测试可靠性。
5. 功能二 Go Preview Runtime supervisor/API。
6. Page Pilot 结果面板：diff summary、PR preview、Work Item 回跳。
7. 同一 Page Pilot run 多轮追加 apply。
8. Page Pilot visual proof。

### 实现

- DevFlow preflight 新增 GitHub delivery contract 检查：`gh auth status`、`gh repo view`、viewer permission 和 PR/checks 元数据读取都在运行前完成。
- Work Item 详情页新增 rework return signal，把人工拒绝、rework assessment、rework checklist 和回流状态放在 Delivery flow 附近展示。
- 新增 `npm run test:feature-p0`，把前端关键测试、desktop preload 语法检查和 Go runtime 全量测试串成固定回归命令。
- Go runtime 新增 `/page-pilot/preview-runtime/resolve|start|restart`，由明确 Repository Workspace 生成 Preview Runtime Profile，启动目标 dev server，并记录 pid/stdout/stderr/health check 基础信息。
- Page Pilot apply 支持传入既有 `runId`，同一 run 可继续追加批注和说明，`roundNumber` 递增，同时沿用同一个 Work Item / Pipeline。
- Page Pilot run 新增 `prPreview` 和 `visualProof`，并同步到 mission / pipeline artifacts；Recent runs 的详情弹窗可查看 PR preview、diff summary、source mapping、visual proof、preview runtime 和 conversation。
- 新增 `docs/test-report.md` 记录测试口径；需要 Electron / 真实目标页验证的项目继续放在 `docs/manual-testing-needed.md`。

### 验证

```bash
npm run lint
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
node --check apps/desktop/src/process-supervisor.cjs
node --check apps/desktop/src/pilot-preload.cjs
go test ./services/local-runtime/internal/omegalocal
```

### 后续

- Go Preview Runtime 当前是进程托管基础版，后续继续补跨进程恢复、持久 process table 和更完整的失败诊断。
- Visual proof 当前以 DOM snapshot 为基础证据，后续可增加前后截图和更细的行级 diff 证据。

## 2026-05-01: JobSupervisor 自动恢复分类策略

### 旧做法

- JobSupervisor 已能扫描 failed / stalled Attempt，并按 `autoRetryFailed`、`maxRetryAttempts`、`retryBackoffSeconds` 决定是否创建 retry Attempt。
- 但失败原因没有分层：runner crash、临时网络失败、GitHub API 临时失败、CI flaky failure、权限失败都会进入近似相同的 retry 判断。
- 权限类失败容易被自动重试浪费时间；非 flaky CI 失败也可能被当成普通 retry，而不是进入 rework checklist。

### 新做法

- 新增 `recovery_policy.go`，集中生成 `supervisorRecoveryPolicy`：
  - `runner_crash` -> `retry-with-clean-worker`，允许自动 retry；
  - `transient_network` -> `wait-and-retry`，允许自动 retry；
  - `github_api_transient` -> `wait-and-retry`，允许自动 retry；
  - `ci_flaky_failure` -> `retry-validation`，允许自动 retry；
  - `permission_failure` -> `manual-fix-permission`，不自动 retry；
  - `ci_failure` -> `rework-required`，不自动 retry；
  - `unknown_failure` -> `retry-with-context`，低置信度自动 retry，并保留失败上下文。
- `scanRecoverableAttempts` 会把 `recoveryPolicy` 写入 recoverable record 和 accepted retry run。
- JobSupervisor summary 新增：
  - `manualRecoveryRequired`
  - `recoveryClassCounts`
- 自动 retry reason 会带上 `[failureClass/recoveryAction]`，下一轮 Attempt 能看到为什么被重试。
- 权限失败和非 flaky CI 失败会进入 manual/rework 决策，不再被 `autoRetryFailed=true` 盲目重跑。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestSupervisorRecoveryPolicyClassifiesFailureModes|TestJobSupervisorScanRecoverableAttemptsCreatesRetryJob|TestJobSupervisorRecoveryPolicyBlocksPermissionAutoRetry|TestJobSupervisorScanRecoverableAttemptsUsesWorkflowRetryPolicy|TestJobSupervisorScanRecoverableAttemptsRespectsBackoffAndLimit'
```

## 2026-05-01: 数据分析指标扩展版

### 旧做法

`/observability.dashboard` 已有基础指标：Attempt 成功率、失败原因分布、慢阶段、待人工队列、活跃运行和推荐动作。这一版适合看当前运行是否异常，但还不能直接回答“哪个 stage 平均最慢、哪个 runner 用得最多、checkpoint 等待多久、PR 创建/合并趋势如何”。

### 新做法

- `stageAverageDurations`：按 stage 聚合运行次数、平均耗时、最大耗时和最近更新时间。
- `runnerUsage`：按 runner 聚合使用次数、成功数、失败数、活跃数和平均耗时。
- `checkpointWaitTimes`：聚合 checkpoint 总数、已处理数、待处理数、平均等待秒数、最大等待秒数，并按 stage 拆分。
- `pullRequests`：统计本地交付记录中可见的 PR created / merged / open 数量。
- `trends`：按天聚合 Attempt started / completed / done / failed、PR created / merged、checkpoint created / resolved。
- 前端 control API 类型同步补齐这些 dashboard 字段，后续 Operator UI 或 CLI 可以直接消费。

### 实现说明

当前 PR 指标以本地 SQLite 中的 Attempt `pullRequestUrl` 和 ProofRecord PR 证据为来源，不额外访问远端平台；这样可以在离线或权限不足时仍给出本地交付视角。后续如果引入一等 PR 表，可以把 created/merged/open 从事件推断升级为记录级统计。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestObservabilityDashboardMetrics'
```

## 2026-05-01: Run Report / Review Packet 扩展版

### 旧做法

DevFlow 进入 Human Review 前会生成 `attempt-run-report.md`，聚合需求、PR、changed files、测试、checks、review 和 artifacts。这个基础版适合人工阅读，但结构化程度不足：前端只能从 Workpad、proof 和 PR 状态里拼信息，无法稳定展示“diff/test/check preview、风险等级、下一步动作”。

### 新做法

- Runtime 在 Human Review 前生成 `attempt-review-packet.json`，并把同一份 `reviewPacket` 写入：
  - `handoff-bundle.json`
  - Attempt record
  - Run Workpad record
- `attempt-run-report.md` 继续保留，同时新增 Diff Preview、Test Preview、Check Preview、Risk、Recommended Actions 小节，来源和 JSON packet 保持一致。
- `reviewPacket` 结构包含：
  - `diffPreview`：changed files、文件数、增删行统计和 diff excerpt；
  - `testPreview`：validation status、summary、output excerpt；
  - `checkPreview`：remote check status、summary、output excerpt、PR/comment/check feedback；
  - `risk`：`low` / `medium` / `high` 和原因列表；
  - `recommendedActions`：下一步动作，例如打开 PR、补验证、查看 checks、按高风险处理。
- Work Item 详情页 Run Workpad 新增 `Review packet` 卡片，默认只展示风险和文件数；点击后打开页内弹窗的一页预览，不把长 diff 直接塞进主页面。

### 实现说明

风险分级先基于本地可验证信号：changed files、validation output、remote check output、PR feedback 和 failed check log。它不是替代 Human Review，而是把“审核时应该先看什么”结构化，让人工和后续自动 rework 都消费同一份 packet。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestRunDevFlowPRCycleCreatesBranchPRAndMergeProof|TestRunWorkpadRecordTracksAttemptRetryContext'
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
npm run lint
```

## 2026-05-01: Run Workpad 字段级编辑入口

### 旧做法

Run Workpad 已是一等记录，并且后端支持 `PATCH /run-workpads/{id}`、字段权限、来源归因和 patch history。但详情页只能展示字段和历史，operator 发现 Retry Reason、Blockers、Review Feedback 或 Validation 需要补充时，仍要绕到 API 或等待 Agent / supervisor 写入。

### 新做法

- Work Item 详情页 Run Workpad header 新增 `Edit fields`。
- 编辑入口使用页内弹窗，不把长字段直接展开到主页面。
- 提交时调用真实 `PATCH /run-workpads/{id}`，不会只改本地 UI 状态。
- UI 只开放 operator 允许字段：
  - Notes
  - Blockers
  - Review Feedback
  - Retry Reason
  - Validation
  - Rework Checklist
  - Rework Assessment
- 每次提交固定带上 `updatedBy=operator`、`reason` 和 `source.kind=ui`，后端继续记录 `fieldPatchSources` / `fieldPatchHistory`。

### 实现说明

`App.tsx` 只新增 `updateRunWorkpadPatch` 回调和 `patchRunWorkpad` API 接线；字段选择、表单状态、payload 生成和错误展示放在 `WorkItemDetailPage`，避免继续扩大主入口耦合。数组类字段按“一行一条”写入；结构化字段保留原对象并覆盖 summary / checklist / reason。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
```

## 2026-05-01: Review/Rework Feedback Sweep 扩展版

### 旧做法

PR comments / reviews 和 failed check log 已经能进入 Attempt / Run Workpad / Rework Checklist，但来源粒度还偏粗：PR review thread 是否已 resolved 不清楚，inline comment 缺少文件和行号，check log 只有 run 链接，重复的 review / comment / thread 容易在 checklist 里生成多条相似待办。

### 新做法

- GitHub PR feedback 采集在 `gh pr view --json comments,reviews` 基础上，增加 GraphQL review thread best-effort 采集。
- PR review thread source 会记录：
  - `state=resolved/unresolved`
  - `resolved`
  - `path`
  - `line`
  - `originalLine`
  - `diffHunk`
  - `sourceUrl`
- Rework Checklist 生成时：
  - unresolved thread 会转成带 `path:line` 的行动项；
  - resolved thread 只保留在 sources 中，不再制造新的 rework action；
  - 同一文件行、同一内容或同一 check run 的重复信号会进入 `groups`，checklist 只展示合并后的行动项，并标注相关信号数量。
- failed check log source 增加 `sourceUrl` 深链和 `logMode=failed-first`，Workpad source drilldown 优先打开具体 check 来源。
- Work Item 详情页的 Rework Checklist source 列表会展示 source state、文件和行号，让用户不用打开全部日志也能先定位来源。

### 实现说明

GraphQL review thread 采集是 best-effort：如果当前 `gh` 权限或 GraphQL schema 不支持对应字段，会静默退回 comments / reviews 基础采集，不阻断 PR status 或 DevFlow。Rework Checklist 的去重分组发生在 runtime 侧，因此 Rework Agent、Retry API 和 UI 读取到的是同一份结构化结果。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestGitHubPullRequestFeedbackFromView|TestGitHubPullRequestReviewThreadFeedbackFromGraphQL|TestBuildReworkChecklistMergesReviewHumanAndDeliveryGateSignals|TestGitHubPRStatusClassifiesFailedChecks'
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
```

## 2026-05-01: GitHub / CI 真实出站同步

### 旧做法

GitHub Issue 已经能导入 Workboard，DevFlow 也能创建 PR、读取 checks 和 failed check log，但 Pipeline 状态只保存在 Omega 本地。GitHub Issue 侧没有自动 comment / label 回写，外部协作者无法从 Issue 页面判断当前是否在执行、等待审核、已完成或需要处理。

### 新做法

- Go local runtime 新增 GitHub Issue outbound sync。
- 对能解析 GitHub Issue 的 Work Item，DevFlow 会在关键节点写回：
  - `attempt.started`
  - `human_review.waiting`
  - `delivery.merge_failed`
  - `attempt.failed`
  - `delivery.completed`
- 每次同步会：
  - 通过 `gh issue comment` 写入结构化状态评论；
  - 通过 `gh label create --force` 确保 `omega:*` 标签存在；
  - 通过 `gh issue edit` 切换 `omega:running/review/blocked/merging/done` 状态标签；
  - 把 sync report 写入 Attempt record 和 proof JSON。
- GitHub Issue comment 会包含 PR URL、changed files、CI/checks 输出、failed check log / PR feedback 摘要、失败原因，以及 review packet 的风险等级和推荐动作。

### 实现说明

出站同步是主链路的 best-effort 旁路：GitHub comment / label 失败不会让 PR 创建、review 或 merge 主链路失败，但失败详情会进入 runtime log 和 sync report。非 GitHub Issue 来源会返回 `skipped`，避免误写其他仓库或平台。

详细设计见 `docs/github-outbound-sync.md`。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestGitHubIssueRefFromWorkItemParsesImportRefAndURL|TestSyncGitHubIssueOutboundPostsCommentAndLabels|TestSyncGitHubIssueOutboundSkipsUnlinkedWorkItem'
go test ./services/local-runtime/internal/omegalocal
```

真实 GitHub smoke：

- 使用 `ZYOOO/TestRepo`。
- 先 clone 最新远端代码并 `git pull --ff-only`。
- 创建临时 issue，执行 comment / label 出站同步后关闭。
- 已验证临时 issue：`ZYOOO/TestRepo#36`。

## 2026-05-01: 飞书 Human Review 审核链路

### 旧做法

Go local runtime 只有基础文本通知能力，且依赖本机 `lark-cli`。DevFlow 到 Human Review 后只能在 Omega Web 内操作，飞书侧没有结构化卡片、没有长内容承载、也没有回调到 checkpoint 状态迁移。

### 新做法

- 新增 `POST /feishu/review-request`。
  - 读取 checkpoint、Work Item、Requirement、Attempt、Run Workpad 和 Review Packet。
  - 生成 Human Review interactive card。
  - 优先通过飞书机器人 webhook 发送。
  - 未配置 webhook 时，如果有 `lark-cli + chatId`，发送文本 fallback。
  - 两者都没有时返回 `needs-configuration`，并把 card/doc preview 写入 checkpoint.`feishuReview`。
- 新增 `POST /feishu/review-callback`。
  - 飞书侧 approve / request changes 与 Omega Web 本地按钮使用同一个 checkpoint decision helper。
  - approve 会继续原有 merging / delivery。
  - request changes 会继续原有 rework assessment / rework attempt。
- DevFlow 进入 Human Review 后会按配置自动发送飞书审核通知，不阻塞主链路。

### 实现说明

飞书卡片只放审核所需摘要：Work Item、需求摘要、PR、风险等级、Review Packet 摘要和操作按钮。长需求和长 diff 会生成 Markdown review doc preview，后续可以接入飞书文档 API 发布为正式文档。发送结果持久化到 checkpoint.`feishuReview`，用于本地回溯和后续一等 Connector Sync Report 演进。

详细说明见 `docs/feishu-review-chain.md`。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuReviewRequestSendsInteractiveWebhookCard|TestFeishuReviewCallbackApprovesCheckpointThroughSharedDecisionPath|TestFeishuNotifyUsesLocalLarkCLI'
```

当前本机没有 `lark-cli`，真实飞书群体验需要配置机器人 webhook 或安装并登录 `lark-cli`。人工验证项已记录到 `docs/manual-testing-needed.md`。

## 2026-05-01: Workflow Action Graph 基础版

### 旧做法

`devflow-pr` 已经能从 Markdown workflow 读取 stages、review rounds、runtime、transitions 和 prompt sections，但执行图仍主要由 Go 中固定 DevFlow 顺序表达。新增或调整流程时，需要改 Go 代码，workflow contract 只能影响局部参数。

### 新做法

- `devflow-pr` workflow front matter 新增 `states.actions`，把每个 state 下要执行的动作声明出来。
- action 支持：
  - `id` / `type` / `agent` / `prompt`；
  - `requiresDiff`；
  - `inputArtifacts` / `outputArtifacts`；
  - `transitions`；
  - review action 的 `verdicts`。
- workflow contract 新增 `taskClasses`，记录 simple / complex 任务的 workpad、planning、validation 策略。
- Pipeline run workflow snapshot 会保存：
  - `states`
  - 扁平化 `actions`
  - `taskClasses`
  - `hooks`
  - `executionMode`
- Workflow validator 会检查 action id、action type、state/action transition 指向的 stage。
- Agent invocation 后的 next stage 推进优先读取 Pipeline snapshot transitions；没有配置时才回退旧 DevFlow 固定顺序。

### 实现说明

本阶段没有推翻现有 DevFlow 主链路，而是先把“执行图”变成可解析、可校验、可持久化、可路由消费的契约。后续通用 action executor 可以从同一个 snapshot 生成 action plan，并逐步接管 review / rework / merging。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestDevFlowTemplateLoadsWorkflowMarkdownContract|TestWorkflowContractParsesStateActionsAndRejectsBrokenActionRoute|TestRepositoryWorkflowTemplateValidationRejectsBrokenContract|TestDevFlowReviewOutcomeRoutesChangesRequestedToRework|TestDevFlowStageStatusAfterChangesRequestedQueuesRework'
```

## 2026-05-01: Attempt Action Plan 基础版

### 旧做法

Workflow contract 已能声明 action graph，但 Attempt 层还没有一个统一、可查询的“当前该执行什么”的视图。UI、JobSupervisor、retry 提示仍容易各自从 stage status、attempt status 和日志里推断状态。

### 新做法

- 新增 `GET /attempts/{attemptId}/action-plan`。
- API 直接读取 Pipeline run workflow snapshot，不重新解析运行中的 workflow，避免 Attempt 过程中规则漂移。
- 返回内容包含：
  - current state；
  - current action；
  - 当前 state 下的 action 列表；
  - 当前 state 的可达 transitions；
  - taskClasses / hooks snapshot；
  - retry action、retry reason 和 recovery policy。
- failed / stalled / canceled attempt 会把 retry action 标出来，并复用 JobSupervisor 的恢复策略分类。
- JobSupervisor recovery summary / accepted retry job 会附带 action plan 摘要，让自动恢复不再只依赖 attempt status 和错误文本。

### 实现说明

本阶段仍是 dry-run executor：只生成可解释执行计划，不执行 git、runner、PR 或 merge 命令。后续 UI 和 JobSupervisor 可以先消费这个计划，再逐步把 review / rework / merging 迁到通用 action handler。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestBuildAttemptActionPlanUsesWorkflowSnapshot|TestAttemptActionPlanAPIIncludesRetryPolicy'
go test ./services/local-runtime/internal/omegalocal -run 'TestJobSupervisorScanRecoverableAttemptsRetriesWithPolicy|TestBuildAttemptActionPlanUsesWorkflowSnapshot|TestAttemptActionPlanAPIIncludesRetryPolicy'
```
