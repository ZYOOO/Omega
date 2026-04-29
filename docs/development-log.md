# Omega 开发日志

本文记录当前 v0Beta 已完成的关键工程节点，方便后续整理仓库和演示材料。

## 2026-04-29

### Work Item 详情页 / Run Workpad Record 基础版

完成：

- 新增 `docs/work-item-detail-architecture.md`，把 Work Item 详情页的旧做法、新做法、路由边界、Run Workpad 数据来源和后续演进单独记录，避免继续把设计正文堆进大日志。
- 旧做法：Work Item 详情页大量 JSX 和派生数据直接内嵌在 `App.tsx`，Requirement、Attempt、Agent trace、Proof、PR/checks、Retry reason 分散展示，信息密度低且入口文件耦合重。
- 新做法：新增独立 `WorkItemDetailPage` 和 `#/work-items/{itemId}` 路由，`App.tsx` 只负责状态编排和数据传入，详情页自身负责 Workpad、Requirement、Flow、Operation、Artifact 的展示组织。
- Work Item 详情页新增 Run Workpad UI 基础版，按 Plan、Acceptance criteria、Validation status、Notes、Blockers、PR、Review Feedback、Retry Reason 聚合真实 Requirement / Pipeline / Attempt / Operation / Proof / Checkpoint / PR status 记录。
- Requirement source 限制高度并内部滚动，修复浅色模式下 markdown 正文对比度过低的问题。
- Delivery flow 从长条列表改为紧凑网格，正在运行的阶段增加更明显的动画反馈。
- Agent operations 和 Artifacts 改为可展开记录，可直接查看 prompt、stdout、stderr、runner metadata、proof path 和 proof URL，不再只是占位卡片。
- Agent operations 折叠态继续压缩为小摘要卡，Artifacts 区域限制高度并内部滚动，避免详情页被低密度执行记录和 proof 列表撑得过长。
- Delivery flow 前置到详情页顶部；Run Workpad 和 Run Timeline 改为默认折叠，只展示摘要，展开后再查看长列表和事件明细。
- Runtime 新增 `runWorkpads` 记录和 `GET /run-workpads` 查询接口，Attempt 创建、Agent invocation、完成/失败/取消、retry、Human Review approve 后进入 merging 时会刷新结构化 Workpad。
- Workpad record 绑定明确 Attempt，字段覆盖 Plan、Acceptance Criteria、Validation、Notes、Blockers、PR、Review Feedback、Retry Reason，前端详情页优先消费 record，缺失时才用真实执行记录兜底派生。

验证：

```bash
npm run lint
npm run test -- apps/web/src/core/__tests__/manualWorkItem.test.ts apps/web/src/__tests__/App.operatorView.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx apps/web/src/components/__tests__/WorkItemDetailPanels.test.tsx apps/web/src/__tests__/attemptRetryReason.test.ts --testTimeout=15000
npm run build
go test ./services/local-runtime/internal/omegalocal -run 'TestRunWorkpadRecordTracksAttemptRetryContext|TestApproveDevFlowCheckpointCanContinueDeliveryAsync|TestRunCurrentPipelineStagePersistsOperationProofAndCheckpoint'
```

浏览器验证：

- `#/work-items/item_manual_21` 可直接打开详情页。
- Requirement source 在浅色模式下可读，并在内容过长时内部滚动。
- Agent operation 可展开查看 Prompt、runner metadata 和执行输出。

### DevFlow Production Core P1

完成：

- Workflow runtime 开始驱动 runner heartbeat、Attempt timeout、retry 上限、retry backoff 和 required checks。
- runner process stdout / stderr / heartbeat event 会刷新 Attempt `lastSeenAt`，并写入 Attempt events 与 runtime DEBUG logs。
- `/github/pr-status` 增加 check summary、missing required checks、branch sync、merge conflict 和 recommended actions。
- DevFlow manual run、retry、JobSupervisor auto run 增加 workspace execution lock，并写入 `.omega/workspace-lifecycle.json`。
- 新增 workspace cleanup 策略：已完成 Attempt 可清理 repo checkout 并保留 `.omega` proof/lifecycle；清理结果写回 Attempt metadata。
- 新增 repo-owned workflow contract：目标仓库 `.omega/WORKFLOW.md` 和 Agent Profile workflow override 可覆盖默认模板，并在运行前校验。
- Workflow Markdown body 支持 coding / rework / review prompt sections，运行时按变量渲染到 Agent prompt。
- JobSupervisor 增加本机 worker host lease、continuation policy metadata 和 orphan running Attempt 恢复基础版。
- 新增 `docs/devflow-production-core.md`，将功能一生产化内核说明从大文档中拆出维护。

验证：

```bash
go test ./services/local-runtime/...
```

### Runtime Logs 基础版

完成：

- Go local runtime 新增 append-only `runtime_logs` SQLite 表和 `GET /runtime-logs` 查询接口。
- HTTP handler 自动为每个 request 写入结构化日志，并返回 `x-omega-request-id`。
- DevFlow job、agent invocation、checkpoint decision、Page Pilot apply/deliver/discard、PR merge 等关键路径补充 INFO / DEBUG / ERROR 日志。
- `/observability` 增加 `recentErrors` 和 runtime log 计数，Operator 视图新增 Runtime logs 区块。
- Human Review approve 兼容旧 pipeline 缺失 attempt 的状态，并记录 ERROR 日志；DevFlow 完成/失败路径会 backfill 缺失 attempt，减少状态不一致。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestRuntimeLogsAPIListsAndFiltersRecords|TestCompleteDevFlowCycleBackfillsMissingAttempt|TestDevFlowCheckpointApprovalToleratesMissingAttempt'
npm run lint
```

### JobSupervisor Integrity Tick 基础版

完成：

- 新增 `POST /job-supervisor/tick`，先落地 JobSupervisor 的状态一致性修复能力。
- Pending Human Review checkpoint 会绑定具体 `attemptId`，不再只依赖 pipeline/stage 反查“最近一次 attempt”。
- `GET /checkpoints` 和 checkpoint approve / reject 前会执行轻量 integrity reconciliation，兼容旧数据。
- 对缺失 attempt 的 pending gate，supervisor 会 backfill 一条 `supervisor-repair` attempt，并写入 runtime ERROR 日志。
- DevFlow completion 在写 checkpoint 前先确保 attempt 存在，避免 checkpoint 已创建但 attempt 还没落库的断链窗口。
- 修复 orchestrator auto-run 测试偶发碰真实 Codex runner 的问题，改为 fake runner，避免本机环境差异导致全量 Go 测试波动。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestJobSupervisorTickBackfillsPendingCheckpointAttemptLink|TestDevFlowCheckpointApprovalToleratesMissingAttempt|TestCompleteDevFlowCycleBackfillsMissingAttempt|TestRunDevFlowPRCycleCreatesBranchPRAndMergeProof'
npm run lint
```

### JobSupervisor Heartbeat / Stalled Detection 基础版

完成：

- Attempt 创建、Agent invocation 持久化、Attempt 完成/失败都会维护 `lastSeenAt`。
- `POST /job-supervisor/tick` 支持可选 `staleAfterSeconds`。
- Supervisor tick 会扫描 running attempts，超过阈值未刷新 `lastSeenAt` 时标记为 `stalled`。
- Stalled attempt 会同步更新 Pipeline 为 `stalled`、Work Item 为 `Blocked`，并写入 Attempt event 和 runtime ERROR log。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestJobSupervisorTickBackfillsPendingCheckpointAttemptLink|TestJobSupervisorTickMarksStalledRunningAttempt|TestCompleteDevFlowCycleBackfillsMissingAttempt'
npm run lint
```

### Runner Context Cancel / Attempt Cancel API 基础版

完成：

- Codex / opencode / Claude Code runner 改为 context-aware supervisor。
- `runSupervisedCommandContext` 能在 deadline/cancel 时终止子进程，并返回 `timed-out` / `canceled` runner process status。
- DevFlow background job 注册 attempt cancel func，结束时注销。
- 新增 `POST /attempts/{id}/cancel`，向本机运行中的 job 发送 cancel signal，并落库 Attempt / Pipeline / Work Item 的 canceled 状态。
- Pending checkpoint 若绑定被取消 attempt，会同步标记为 `canceled`，避免取消后仍显示可审批。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestCancelAttemptAPIUpdatesStateAndSignalsRunningJob|TestRunSupervisedCommandContextTimesOutProcess|TestJobSupervisorTickMarksStalledRunningAttempt'
npm run lint
```

### Page Pilot Proof Records

完成：

- Page Pilot apply 成功后，会把同一条 run 同步成通用 Mission / Operation / Proof records。
- Work Item 详情页可以通过现有 Agent trace / Proof 面板看到 Page Pilot 的 prompt、diff、summary、runner process 和后续 PR URL，不再只依赖 `/page-pilot/runs` 私有记录。
- Page Pilot pipeline stage 会同步写入 evidence：`page_editing` 关联 source patch proof，`delivery` 关联 commit / PR proof，discard 会保留丢弃状态。
- 修正文档中 Page Pilot 计划 API 和旧文件名造成的状态偏差，避免把尚未落地的 Preview Runtime API 写成已存在接口。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget
```

### Requirement Create / Work Item Runtime Refactor

完成：

- 从 `apps/web/src/App.tsx` 拆出 Requirement 创建 UI：`apps/web/src/components/RequirementComposer.tsx`。
- 从 `App.tsx` 拆出 Projects / Repository Workspace 总览 UI：`apps/web/src/components/ProjectSurface.tsx`。
- 从 `App.tsx` 拆出 Workboard 左侧导航、workspace 切换、顶部搜索和详情页工具栏：`apps/web/src/components/WorkspaceChrome.tsx`。
- 从 `App.tsx` 拆出手动 Work Item 生成与 markdown title 兜底逻辑：`apps/web/src/core/manualWorkItem.ts`。
- 从 `services/local-runtime/internal/omegalocal/server.go` 拆出 Work Item 写入、唯一编号、Requirement 映射逻辑：`services/local-runtime/internal/omegalocal/work_items.go`。
- 从 `server.go` 拆出 DevFlow PR 长流程执行器：`services/local-runtime/internal/omegalocal/devflow_cycle.go`。
- 从 `server.go` 拆出 Pipeline record / template materialization：`services/local-runtime/internal/omegalocal/pipeline_records.go`。
- 为未开始的 Work Item 增加真实删除链路：前端列表左侧删除按钮、`DELETE /work-items/{itemId}`、本地 workspace snapshot 同步删除。
- 为前端 helper 增加独立单测，并保留 Workboard 创建 Requirement 的重复提交回归测试。
- 保留 Go runtime 的 Work Item 创建 / 初始化 / 重复编号回归测试。

效果：

- `App.tsx` 从约 4180 行降到约 3811 行，创建 Requirement、Projects 总览和 Workboard shell 不再继续压在主组件里。
- `server.go` 从约 4809 行降到约 2905 行，Work Item lifecycle、DevFlow cycle、Pipeline record 构造都有了独立文件，后续补 retry、queue、接口测试会更容易定位。

验证：

```bash
npm run test -- apps/web/src/core/__tests__/manualWorkItem.test.ts apps/web/src/__tests__/App.operatorView.test.tsx -t "manual work item helpers|creates app requirements" --testTimeout=15000
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run lint
npm run build
go test ./services/local-runtime/internal/omegalocal
```

## 2026-04-29 16:10 CST

### Omega CLI 基础版

完成：

- 新增 `omega` operator CLI，可通过 `go install ./services/local-runtime/cmd/omega` 安装后直接使用 `omega status`；开发期也可通过 `npm run omega -- <command>` 使用。
- CLI 只调用 Go Local Runtime HTTP API，不直接读写 SQLite，不复制执行编排。
- 覆盖状态、runtime logs、Work Item 运行、Attempt retry/cancel/timeline、Checkpoint approve/request changes、JobSupervisor tick。
- 新增独立文档 `docs/omega-cli.md`，说明命令、API 映射和架构约束。

验证：

```bash
go test ./services/local-runtime/internal/omegacli ./services/local-runtime/cmd/omega
```

## 2026-04-29 16:35 CST

### P1 Observability Dashboard 基础版

完成：

- `/observability` 保留旧 summary，同时新增 dashboard data。
- Dashboard 聚合 Attempt 成功率、失败原因、慢阶段、待人工队列、活跃运行和推荐动作。
- `omega status` 展示 dashboard attempts 和 recommended actions，仍只调用 Runtime API。

验证：

```bash
go test ./services/local-runtime/internal/omegacli ./services/local-runtime/internal/omegalocal -run 'TestStatusPrintsObservabilitySummary|TestObservabilityDashboardMetrics|TestObservabilitySummary|TestRuntimeLogsAPIListsAndFiltersRecords'
```

## 2026-04-29 17:05 CST

### 正式 JobSupervisor v1

完成：

- JobSupervisor tick 统一扫描 waiting-human gate、workflow contract、running attempt、Ready Work Item、failed/stalled Attempt。
- 增加 failed/stalled recovery scan，支持 `autoRetryFailed`、`maxRetryAttempts`、`retryBackoffSeconds`。
- 自动 retry 复用真实 Attempt retry 链路，创建新 Attempt 并交给后台 DevFlow job，不复制执行逻辑。
- Pipeline run workflow metadata 补齐 runtime / transitions；JobSupervisor 会回填旧 pipeline 缺失的 workflow contract metadata。
- CLI / API / daemon 参数已接入自动 retry 策略。

安全约束：

- 默认不自动启动 Ready Work Item。
- 默认不自动 retry failed/stalled Attempt。
- 任何会写目标仓库的后台动作都需要显式开启策略开关。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestJobSupervisorTickBackfillsWorkflowContractMetadata|TestJobSupervisorScanRecoverableAttempts|TestJobSupervisorTickReportsRunnableReadyWork|TestPrepareDevFlowAttemptRetryLinksAttempts|TestWorkflowTemplateLoadsDevFlowMarkdown'
```

## 2026-04-27

### 门户首页与功能页 SPA 结构

完成：

- React SPA 增加门户首页，默认从 `http://localhost:5173/` 进入。
- Workboard 保留为真实功能页，可通过首页 CTA 或 `#workboard` 打开。
- 首页参考飞书工作台门户：顶部导航、左侧应用入口、中部应用宫格和模板推荐、右侧信息卡，文案聚焦 Requirement -> Pipeline -> Agent -> GitHub PR -> Human Review。
- 功能页保留现有本地执行、GitHub workspace、Agent trace、human gate 和 proof 展示能力。
- 门户首页从 `App.tsx` 拆分为 `apps/web/src/components/PortalHome.tsx`，为后续继续拆 Workboard 子模块做准备。
- Workboard 视觉系统更新为浅色工作台风格，保持原功能不变：左侧 workspace、GitHub Issues、Work item 分组、状态 pill、右侧 rail 和主要卡片统一到白色 / 浅蓝体系。

验证：

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx
npm run lint
npm run build
```

## 2026-04-26

### Workflow 状态机与 Rework 回流

完成：

- 默认 `devflow-pr` workflow 增加 `Rework` stage、review outcome transitions 和 `maxReviewCycles`。
- Go workflow loader 增加 runtime / transitions / review outcome 解析。
- `CHANGES_REQUESTED` 不再直接把 Attempt 标为失败，而是记录 review artifact，进入 Rework，再回到 Code Review。
- DevFlow run 改为每个 Item 复用稳定 workspace、稳定 branch 和同一个 PR，Attempt 只记录一次执行轮次。
- 抽出 `AgentRunner` 接口和 Codex CLI 默认实现，为后续 opencode / Claude Code / 长期 session runner 做接口边界。
- Work item 列表把单纯 proof 数字弱化为 `Turns N` / `Artifacts N`，详情页继续展示 Agent orchestration。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal
npm run lint
```

### Review Agent 与真实 Human Gate

完成：

- 移除 `devflow-pr` 中为了表面跑通而默认生成的人工审批 / merge proof。
- Implementation 后会启动两轮只读 Review Agent，审查 PR diff、changed files、测试结果和验收标准。
- Review Agent 必须输出明确 verdict；`APPROVED` 才能进入人工审核，`CHANGES_REQUESTED` 会进入 Rework 并回到 Code Review，blocked / failed 才会让 Attempt 失败并保留错误。
- `human_review` 变成真实 checkpoint：Pipeline 停在 `waiting-human`，Work item 进入 `In Review`，不会自动 merge。
- 用户调用 checkpoint Approve 后，后端继续执行 merge / delivery，生成 `human-review.md`、`merge.md`，更新 `handoff-bundle.json` 并将 Pipeline / Attempt / Item 标记完成。
- 前端 Run / AutoRun 默认发送 `autoApproveHuman=false`、`autoMerge=false`。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal
npm test -- --run apps/web/src/__tests__/App.operatorView.test.tsx -t "DevFlow PR cycle|runs repository work item buttons"
```

### Default Workflow Template 抽离

完成：

- 新增默认 Workflow 文件：`services/local-runtime/workflows/devflow-pr.md`。
- Go local runtime 增加 workflow loader，支持读取 Markdown front matter 和 prompt body。
- `devflow-pr` 的 stage、agents、output artifacts、review rounds 从 Markdown workflow 编译进入 Pipeline run。
- 新增 `GET /workflow-templates`，返回当前 workflow-backed templates 的 source、markdown、stages、review rounds。
- Pipeline run 保存 workflow metadata，方便详情页和后续编辑器展示。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestDevFlowTemplateLoadsWorkflowMarkdownContract|TestPipelineTemplateIncludesAgentContractsDependenciesAndDataFlow|TestMigrationsAndPipelineTemplates|TestRunDevFlowPRCycleCreatesBranchPRAndMergeProof'
```

### Monorepo 目录结构迁移

完成：

- React SPA 从根目录 `src` 迁移到 `apps/web/src`。
- Vite / Vitest 配置迁移到 `apps/web/vite.config.ts` 和 `apps/web/vitest.setup.ts`。
- Go local runtime 从 `cmd` / `internal` 迁移到 `services/local-runtime`。
- 预留 `apps/desktop` 作为 Electron shell 目录，后续用于打包 React SPA 和 Go runtime。
- 预留 `packages/shared`，后续承载 web、desktop、preview agent 共享类型和 API schema。
- 根目录脚本更新为 `web:dev`、`local-runtime:dev`、`web:build`，并保留 `mission-control:api` 兼容别名。

验证：

```bash
npm run lint
npm test -- --run src/__tests__/App.operatorView.test.tsx -t "runs repository work item buttons"
npm run build
go test ./...
```

### DevFlow Run 异步化

完成：

- `POST /pipelines/:id/run-devflow-cycle` 默认改为异步执行：先创建 Attempt，立即返回 `202 Accepted`、`pipeline` 和 `attempt`。
- 后端启动 background job 继续执行 DevFlow PR cycle，不再让产品主路径等待一个 HTTP 请求跑完整流程。
- `wait: true` 保留为测试和兼容路径。
- `orchestrator/tick` 的 `autoRun` 同步改为创建 Attempt 后交给后台 job，execution lock 在 job 完成或失败后释放。
- 前端 `Run` 识别 `accepted` 状态，提示 Pipeline 已启动，并通过轮询刷新 Work item / Pipeline / Attempt / Proof 进度。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestRunDevFlowPRCycleCreatesBranchPRAndMergeProof|TestOrchestratorTickCanClaimAndRunDevFlowCycle'
npm test -- --run src/__tests__/App.operatorView.test.tsx -t "runs repository work item buttons"
npm run lint
```

剩余：

- 当前是 goroutine background job，已经符合 v0Beta 的异步体验；下一步需要抽象正式 `AgentRunner` 与 `JobSupervisor`，补齐 heartbeat、stall retry、cancel、timeout、多 turn continuation、worker host 分配和崩溃恢复。

## 2026-04-25

### Attempt 运行记录与详情页

完成：

- 新增一等 `attempts` 记录，用于串联一次 Run 的 Item、Pipeline、repository target、runner、workspace、branch、PR、stage snapshot、错误与耗时。
- Go local service 增加 `GET /attempts`。
- `run-current-stage`、`run-devflow-cycle` 和 `orchestrator/tick autoRun` 会创建并更新 Attempt。
- Work item 详情页增加 Current attempt、Proof 和 Attempt history，展示 stage、Agent、artifact/proof、workspace、branch 和 PR。
- Done 的 Item 在列表禁用 Run，详情页提供显式 Rerun；失败状态保留错误原因并展示 Retry 入口。
- AutoRun 入口放在左侧 Repository workspace 下，运行时扫描可执行 issue、claim、创建 Pipeline、执行本地闭环并释放 execution lock。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestOrchestratorTickCanClaimAndRunDevFlowCycle|TestRunCurrentPipelineStagePersistsOperationProofAndCheckpoint|TestRunDevFlowPRCycleCreatesBranchPRAndMergeProof'
npm test -- --run src/__tests__/App.operatorView.test.tsx -t "not-started work clearly"
npm run lint
```

### 状态展示与执行过程可见性

完成：

- 增加 `Planning` 状态，用于创建后准备编排、创建 Pipeline、分配 Agent 的阶段。
- `Ready` 在 UI 中显示为 `Not started`，避免被误解成完成。
- 完成的 Item / Pipeline 禁用 Run，按钮显示 `Completed`。
- Work items 列表行内展示 Pipeline stages 与 Agent 分配。
- Detail 页 Delivery flow 优先展示真实 Pipeline stages。
- 状态筛选和 inspector 状态选择同步支持 `Planning`。

验证：

```bash
npm run lint
npm test -- --run src/__tests__/App.operatorView.test.tsx -t "DevFlow PR cycle|not-started work clearly"
npm test -- --run src/core/__tests__/workboard.test.ts src/core/__tests__/workspacePersistence.test.ts
```

说明：当前产品文档和 UI 使用 Omega 自己的 Workboard 模型。

### v0Beta 数据清理与文档入口

完成：

- 清理本地测试数据：work items、requirements、pipelines、missions、operations、proof、checkpoints 归零。
- 保留 GitHub 登录状态与 `ZYOOO/TestRepo` repository target。
- 备份 SQLite 和 workspace snapshot。
- 建立 `docs/README.md` 作为当前文档入口。

### Requirement -> Item -> DevFlow PR 真实闭环

完成：

- 在 `ZYOOO/TestRepo` 完成 App 内 Requirement 到 PR / merge 的真实闭环。
- Requirement 创建时由 `master` 生成结构化需求、dispatch plan、suggested work items。
- Item 继承当前 repository target。
- DevFlow run 生成 isolated workspace、branch、commit、PR、review proof、human proof、merge proof。

## 2026-04-24

### 主 Agent 与 Pipeline Contract

完成：

- 增加 `master` Agent。
- Pipeline run 物化 Agent contracts。
- Stage 增加 `dependsOn`、`inputArtifacts`、`outputArtifacts`。
- Pipeline 增加 `dataFlow`。
- 旧 Requirement / Pipeline 数据加载时自动 backfill 缺失字段。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestCreateWorkItemStoresMasterRequirementDispatch|TestPipelineTemplateIncludesAgentContractsDependenciesAndDataFlow|TestEnsureTablesBackfillsLegacyRequirementAndPipelineExecutionMetadata'
```

### GitHub PR 与 Checks

完成：

- `POST /github/create-pr`
- `POST /github/pr-status`
- DevFlow cycle 可读取 PR 和 checks 状态，并生成 delivery gate 相关 proof。

## 2026-04-23

### Repository Workspace 与 GitHub 登录

完成：

- GitHub OAuth 配置从 `.env` 迁移到 App 内填写并持久化。
- 支持本机 `gh` 登录态读取 repositories。
- Project 页面选择 repo 后绑定 repository target。
- 左侧 Workspaces 展示 repository workspace。
- 进入 workspace 后 Work items 默认只显示当前 repo 下的 Items。
- repository target 支持删除，并真实写入本地状态。

### App 内 Requirement 创建

完成：

- 用户可以在 Repository workspace 内直接创建需求。
- 创建时不要求从 GitHub issue 开始。
- 创建后的 Requirement / Item 持久化到 Go local service。
- 新 Item 会继承当前 repository target，避免手填 owner/repo。

## 2026-04-22

### Go Local Service 主路径

完成：

- Go local service 成为主要本地服务端。
- SQLite 保存 workspace 数据和 runtime settings。
- 提供 Workboard、Pipeline、Checkpoint、Mission、Operation、Proof 基础 API。
- 前端从本地 API 读取数据，不再依赖浏览器临时状态。
- 增加 OpenAPI 文档。

### Local Runner 与 Workspace Safety

完成：

- workspace root 可配置，默认 `~/Omega/workspaces`。
- 每次 operation 创建隔离 workspace。
- workspace path 安全校验必须在 root 内。
- 写入 `.omega/job.json`、`.omega/prompt.md`、`.omega/agent-runtime.json`。
- 增加 Codex runner process supervisor 基础版。

## 2026-04-21

### Workboard UI 与右侧栏

完成：

- 修复右侧 inspector 折叠行为。
- 增加固定右侧 rail。
- Details 页面可从 Issue 列表进入。
- Inspector 默认收起，点击 Item 后打开详情。
- 空状态创建入口优化。
- 搜索、筛选、列表密度做了多轮调整。

## 当前风险

- `devflow-pr` 已抽成默认 Markdown workflow，但尚未提供 App 内模板编辑器和 workspace 级覆盖。
- Codex / opencode / Claude Code 的 runner registry 尚未统一。
- retry / cancel / timeout 尚未完整持久化。
- GitHub issue comment / label 回写还未完成。
- PR lifecycle UI 仍需加强。
- Feishu 审核卡片和回调还未完成。

## 当前可演示能力

可以演示：

```text
App 创建 Requirement
  -> master dispatch
  -> Item
  -> Planning
  -> Pipeline stages / Agent assignment
  -> isolated workspace
  -> branch / commit
  -> PR
  -> proof
  -> Done
```

推荐测试仓库：

```text
ZYOOO/TestRepo
```

## 2026-04-29 14:53 CST

### 功能一 Run Timeline 基础版

完成：

- 新增 `GET /attempts/{id}/timeline`，按具体 Attempt 聚合 attempt events、pipeline events、stage snapshots、operation、proof、checkpoint 和 runtime logs。
- Work Item 详情页 Current attempt 增加 Run timeline，展示真实执行事件，辅助 Human Review 和失败排障。
- OpenAPI、当前产品设计、架构和 Todo 已同步记录。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestAttemptTimelineAggregatesRunRecords|TestCancelAttemptAPIUpdatesStateAndSignalsRunningJob|TestRuntimeLogsAPIListsAndFiltersRecords'
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx -t "shows review packet details" --testTimeout=15000
```

后续：

- Run Timeline 继续补 runner stdout/stderr 摘要展开、GitHub checks/rebase/conflict 事件和按 stage/agent 过滤。

## 2026-04-29 15:45 CST

### P0 JobSupervisor / Preflight / Run Report 基础版

完成：

- 新增 JobSupervisor daemon，Go Local Runtime 启动后会周期性执行安全维护 tick。
- `POST /job-supervisor/tick` 增加 Ready Work Item runnable scan 和显式 `autoRunReady` 接受运行能力。
- DevFlow manual run、Attempt retry、JobSupervisor scan 复用统一 preflight，避免运行中才发现 repo / runner / workspace 问题。
- DevFlow 进入 Human Review 前生成 `attempt-run-report.md`，把需求、PR、changed files、验证、checks、review 和 artifact 汇总到 proof。

约束：

- daemon 默认不自动启动 Ready item，避免后台隐式写目标仓库；自动启动必须显式开启。
- 当时旧做法：正式 JobSupervisor 的失败自动恢复、CI/rebase/conflict 策略仍保留在 Todo 中，未标为完成。2026-04-29 后续 P1 已补 JobSupervisor v1、基础 CI/checks、branch sync/conflict 检测和 contract-driven retry；剩余项见 `docs/devflow-production-core.md`。

验证：

```bash
npm run lint
npm run test -- apps/web/src/core/__tests__/manualWorkItem.test.ts apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run build
go test ./services/local-runtime/internal/omegalocal
```

## 2026-04-29 19:05 CST

### Workboard 状态一致性与 Retry 可诊断性

完成：

- Human Review thread 的头像与评论区样式从旧 Human Gate 通用选择器里解耦，light / dark 下保持居中和对齐。
- Page Pilot Work Item 在列表里改为优先显示 pipeline 事实状态，避免旧数据里 `Done + waiting-human` 的矛盾展示。
- Page Pilot discard 后刷新 Workboard 控制面数据，并在后端测试中校验 discarded 会落到 Work Item `Blocked` 与 pipeline `discarded`。
- 移除 Retry context 卡片，改为运行时写入 `failureReason` / `failureStageId` / `failureAgentId` / `failureReviewFeedback`；前端失败报告优先展示明确失败原因和 review feedback，同时保留 runner stderr 作为执行证据，不做环境日志过滤。
- Work Item 详情触发 Retry 时，会把失败原因、失败阶段、失败 agent、review feedback / failure detail / stderr summary 一并写入 retry reason，便于下一轮 agent 直接看到为什么需要重试。

验证：

```bash
npm run lint
npm run test -- apps/web/src/core/__tests__/manualWorkItem.test.ts apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run build
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget'
```

## 2026-04-29 20:10 CST

### Human Review Approve 轻量化

完成：

- 旧做法：Approve 请求同步执行 checkpoint 保存、PR merge、proof 更新和完整控制面刷新，点击后会被交付链路和大量观测接口阻塞。
- 新做法：前端对 Human Review Approve 发送 `asyncDelivery`，后端先保存人工审批、标记 Merging running 并快速返回，后续 merge / proof / done 状态由后台交付继续推进。
- 新增轻量执行态刷新，只拉取 workspace session、pipeline、attempt 和 checkpoint；Approve、Request changes 与运行中轮询不再触发 GitHub status、capability、operation、runtime log 等重接口。
- 保留未传 `asyncDelivery` 的同步路径，兼容现有 API 语义和测试。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestApproveDevFlowCheckpointCanContinueDeliveryAsync|TestApproveDevFlowCheckpointIgnoresBranchCleanupFailure'
npm run lint
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run build
```

## 2026-04-29 20:25 CST

### Review / Rework 交接契约增强

完成：

- 旧做法：`devflow-pr` 的 Review Prompt 主要要求输出一个 verdict line，`CHANGES_REQUESTED` 时缺少强制的 blocking finding、validation gap 和 rework instruction，下一轮 rework / retry 容易只拿到泛化原因。
- 新做法：Review Prompt 必须输出 Summary、Blocking findings、Validation gaps、Rework instructions、Residual risks；`CHANGES_REQUESTED` 至少要包含一个 blocking finding 或 rework instruction。
- Rework / Coding Prompt 的 completion note 增加固定小节，方便 Human Review、Retry 和后续 run report 读取。
- 编排层把提炼后的 review summary 传给 rework，不再把整篇原始 review Markdown 直接塞进下一轮输入；runner stderr 仍保留为执行证据。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestDevFlowReviewOutcomeRoutesChangesRequestedToRework|TestFailAttemptRecordPersistsFailureReasonContract'
```

## 2026-04-29 20:45 CST

### 全 Agent 交接契约补齐

完成：

- 旧做法：默认 `devflow-pr` 主要强化 coding / rework / review prompt，requirement、architect、testing、delivery 虽然有 artifact，但 prompt section 和 Agent output contract 不够统一。
- 新做法：默认 workflow 增加 `Prompt: requirement`、`Prompt: architect`、`Prompt: testing`、`Prompt: delivery`，并保留 coding / rework / review；每个 Agent 都有固定交接小节。
- Go runtime 的 Agent definitions 同步更新 output contract，避免 UI contract 与真实 prompt 口径不一致。
- DevFlow 编排层会用 workflow prompt section 记录 requirement、architect、testing、delivery 的真实阶段 prompt；testing report 和 human review request 也改成更稳定的结构化交接内容。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestWorkflowTemplateLoadsDevFlowMarkdown|TestAgentDefinitionsExposeStructuredHandoffContracts|TestDevFlowReviewOutcomeRoutesChangesRequestedToRework'
npm run lint
```

## 2026-04-29 23:43 CST

### Work Item 详情折叠与 Human Review Rework 链路

完成：

- Work Item 详情页保持 Delivery Flow 前置，Run Workpad 每个区块默认折叠，并修复摘要行宽度和 `+` 按钮错位。
- Run Timeline 默认折叠，只展示最新事件摘要；展开后展示当前 attempt timeline API 返回的完整事件列表。
- Human Review 的 Request changes 不再只写 rejected 事件：后端会创建新的 `human-request-changes` Attempt，并把人工反馈写入 `humanChangeRequest` / `retryReason` / Workpad Review Feedback。
- DevFlow prompt 注入最新人工反馈，下一轮 Agent 能真实看到用户要求，例如“改成章四”。
- 旧 Attempt 会标记为 `changes-requested`，保留 review feedback 和 retry link，方便详情页和 Attempt history 解释为什么发生下一轮。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback|TestPrepareDevFlowAttemptRetryLinksAttempts|TestApproveDevFlowCheckpointCanContinueDeliveryAsync'
go test ./services/local-runtime/internal/omegalocal
npm run lint
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx apps/web/src/components/__tests__/WorkItemDetailPanels.test.tsx apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run build
```

## 2026-04-30 00:21 CST

### Human Review Rework 体验和续写语义

完成：

- Human Review 卡片去掉重复的产品名前缀，header 和内容区改为更紧凑的左右结构。
- Delivery Flow passed/done 不再整卡大面积绿色，改为轻量通过信号，减少和当前执行态混淆。
- Human-requested rework Attempt 继承上一轮 delivery branch、PR URL 和 workspace path。
- 隔离 workspace 丢失时，DevFlow 会优先从远端同名 delivery branch 恢复，确保人工修改基于第一次已完成版本继续。
- Review prompt 会带上人工意见，并要求二次 review 按本轮增量 diff 核对人工反馈。
- PR description 在 rework 后按需更新，补充人工意见、增量 diff、changed files 和 validation。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback|TestEnsureDevFlowRepositoryWorkspaceRestoresRemoteDeliveryBranch|TestWorkflowTemplateLoadsDevFlowMarkdown'
npm run lint
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx apps/web/src/components/__tests__/WorkItemDetailPanels.test.tsx --testTimeout=15000
```

## 2026-04-30 00:55 CST

### Human Review Rework Assessment

完成：

- 新增 `docs/human-review-rework-assessment.md`，单独记录 Human Review Request changes 后的评估策略、runtime 数据契约、Workpad 展示和验证方式，避免继续膨胀大文档。
- 旧做法：人工 request changes 后虽然会创建 rework Attempt，但默认仍偏向完整重跑，缺少“局部修改直接续跑”和“架构变化先重新规划”的显式判断。
- 新做法：runtime 在创建 `human-request-changes` Attempt 前生成 `reworkAssessment`，并写入 Attempt / Run Workpad；策略分为 `fast_rework`、`replan_rework`、`needs_human_info`。
- `fast_rework` 直接从 `rework` stage 续跑，复用上一轮 workspace、branch 和 PR，生成本轮增量 diff、test report、review proof，并重新进入 Human Review。
- `replan_rework` 从 `todo` 重新规划，但仍继承上一轮 delivery branch / PR / workspace，避免丢失第一版成果。
- `needs_human_info` 不启动 Agent，Attempt 停在 `waiting-human`，防止人工意见不明确时让 Agent 猜需求。
- Work Item 详情页 Run Workpad 增加 Rework Assessment 折叠区块，先展示策略摘要，展开后查看理由、原始人工意见和 checklist。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback|TestPrepareDevFlowHumanRequestedReworkWaitsWhenFeedbackNeedsInfo|TestAssessHumanRequestedReworkRoutesByScope|TestResetDevFlowPipelineForAttemptFromStageFallsBackWhenStageMissing'
npm run lint
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx apps/web/src/components/__tests__/WorkItemDetailPanels.test.tsx --testTimeout=15000
```
