# Omega 开发日志

本文记录当前 v0Beta 已完成的关键工程节点，方便后续整理仓库和演示材料。

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
