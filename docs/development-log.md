# Omega 开发日志

本文记录当前 v0Beta 已完成的关键工程节点，方便后续整理仓库和演示材料。

## 2026-05-04

### DevFlow Delivery flow 单一活动阶段收敛

完成：

- 修复 Work Item 详情页 Delivery flow 同时高亮 Implementation、Human Review、Done 的状态混乱问题。
- 收敛 DevFlow 状态权威：`pipeline.run.stages` 表示当前运行态，`attempt.currentStageId` 表示当前执行点，`attemptActionPlan.states` 只表示 workflow contract，不再驱动主流程 UI。
- 后端对 `devflow-pr` pipeline 和 latest attempt stages 做 canonicalization；`markDevFlowStageProgress` 在写入新的 current stage 时同步清理同一 pipeline 的旧活动态。
- 终态 pipeline 不再参与 current-stage 反推，避免已完成、失败、取消、超时状态被旧 stage snapshot 覆盖成 running。
- 前端详情页只展示 backend canonical `pipeline.run.stages`，不再根据 checkpoint/currentStageId 自行重算 Delivery flow。
- 补充 Go 和 React 回归测试，锁定 OMG-32 这类 Human Review 恢复场景。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestNormalizeDevFlowPipelineStageStatusesKeepsSingleActiveStage|TestMarkDevFlowStageProgressCanonicalizesCurrentStage' -count=1
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testNamePattern "canonical backend pipeline snapshot" --testTimeout=30000
npm run lint
git diff --check
```

### Projects 页面主题与概要区收敛

完成：

- 修复 Projects 页面按钮没有完整适配 light / dark 的问题：`Project config`、disabled `Create project`、disabled `Create workspace` 不再显示黑色默认按钮。
- 将 Project summary 的三项统计从右侧高卡片改为紧凑胶囊指标，降低视觉重量。
- 移除 Projects 页中冗余的阶段标签条，避免 `Requirements / Repository Workspace / Agent Pipeline / Human Review / Delivery` 和实际 Work Item 流程展示重复。
- 手测 light / dark 两个主题，确认按钮、输入框、统计和卡片背景都跟随主题。

验证：

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=20000
npm run lint
```

### Workboard New Requirement 描述区视觉收敛

完成：

- New Requirement composer 的 Write / Preview 切换改为紧凑 segmented control，去掉 light mode 下突兀的深色竖向分隔线。
- Description textarea 与 Preview 面板改成独立圆角内容区，避免 tab 和内容边框拼接造成视觉噪声。
- Preview 面板补充 light / dark mode 专用 Markdown 颜色，标题、列表、正文和空状态更清晰。

验证：

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=20000
npm run lint
```

## 2026-05-03

### Feishu 连接状态与 Page Pilot 侧栏交互修复

完成：

- Feishu Connections 展示改为消费运行时 effective state：`lark-cli` 当前用户 fallback、显式 chat/task/webhook/reviewer 路由任一 ready 时，左侧显示 `on`，不再被旧 session 的 `off` 误导。
- Feishu Provider 面板补充 route 说明，当前用户 fallback 会显示 `current-user fallback via lark-cli`，让个人机器人审核链路和显式群/任务/webhook 路由边界更清楚。
- Page Pilot 模式点击左侧 Connections provider 会切到 Settings 并打开 Provider access，不再尝试打开被 Page Pilot 隐藏的 inspector。
- Page Pilot launcher 不再常驻右下角 `AI` 浮球；该浮球只在 Web fallback 的内嵌 iframe 预览打开后显示，Electron direct pilot 的 AI 操作继续留在目标页面内。
- 补充 operator view 回归：从 `#page-pilot` session 进入时 Feishu 显示 `on`，点击 Feishu 后能进入 Settings 并看到测试连接入口。

验证：

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=20000
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
```

### Page Pilot Repository source 打开链路修复

完成：

- 修复 Page Pilot 默认 `Repository source` 对 package.json 项目只准备 worktree、却直接打开默认 preview URL 的问题。
- Electron `resolveRepositoryPreviewTarget` 不再在没有显式配置时返回默认 `127.0.0.1` 预览地址，避免 Page Pilot 打开未启动或错误的目标页面。
- Page Pilot 启动器会根据 repo 形态选择真实路径：package.json 项目优先自动启动 Preview Runtime Agent 后再打开 Electron direct pilot；只有纯静态 `index.html` 仓库才走 file preview。
- `Dev server by Agent` 支持完整 URL 输入：完整 URL 会拆成 Preview Runtime 目标 URL 和打开路径，避免卡在错误端口的启动 / health check。
- Electron Preview Runtime 对已运行的显式 preview URL 增加 external-url fast path，不再先准备 GitHub isolated worktree；同时修复路径 query/hash 合成。
- `HTML file` 模式在输入框为空时会从当前 Repository Workspace 自动寻找根目录 `index.html`，恢复旧版“选 workspace 后直接打开”的使用感。
- 打开流程增加 busy state 和超时反馈，避免页面长时间停在 starting。
- `HTML file` 模式会忽略残留 HTTP URL，并从当前 workspace 自动解析 `index.html`；Electron 打开失败后会关闭 BrowserView，避免失败 webview 覆盖 Workboard。
- 补充 package.json repository source 单测和 process supervisor 边界测试，覆盖“不要伪造 previewUrl”。
- 同步回归 Work Item 30 相关链路：proof-backed Human Review recovery、current-user Feishu fallback、详情页 active attempt 刷新相关 targeted tests 通过。

验证：

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/__tests__/desktopProcessSupervisor.test.ts --testTimeout=15000
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
npm run lint
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilot|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget|TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestJobSupervisorRecoversOrphanedHumanReviewAttempt|TestJobSupervisorRecoversProofBackedHumanReviewAttempt|TestFeishuAutoReviewFallsBackToCurrentLarkUser|TestJobSupervisorDoesNotOverwriteFreshWaitingHumanAttempt'
```

### 飞书审核人搜索与绑定

完成：

- 新增 `POST /feishu/users/search`，Settings 中的 Feishu Reviewer lookup 可按姓名、企业邮箱或手机号查找审核人，不再要求用户手动去开放平台找 `open_id`。
- Reviewer lookup 支持 `Use current user`，通过 `lark-cli contact +get-user` 直接绑定当前登录用户，解决联系人搜索不返回自己的情况。
- 姓名搜索优先复用本机 `lark-cli` 用户登录态；邮箱 / 手机号搜索在姓名搜索不可用时用应用机器人凭据解析用户 ID，适配“只有 App ID / App Secret”的常见配置方式。
- Feishu 配置新增 `assigneeLabel`，页面保存内部审核人 ID 的同时保留展示名，后续 Human Review Task 创建继续使用强绑定 assignee。
- Provider 面板保留简化主路径：测试本机 `lark-cli`、搜索 Reviewer、保存绑定；Chat / Task / webhook 原始覆盖项继续放在高级配置中。
- 同步更新飞书审核链路、权限说明和人工测试清单，明确群机器人 / webhook 不需要个人 open_id，个人任务审核需要先解析审核人身份。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuConfigPreflightUsesLocalLarkCLIProfile|TestFeishuUserSearchUsesLocalLarkCLI|TestFeishuUserSearchCanResolveCurrentUser|TestFeishuUserSearchFallsBackToEmailIDLookup'
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=15000
```

## 2026-05-02

### Human Review 等待态信号与飞书发送原因可观测性

完成：

- Work Item 详情页不再仅因为 workflow 中有 Rework 阶段就展示 `Feedback route`；只有真实 request changes、rework assessment、retry available 或失败/停滞 attempt 才展示返工路由。
- Run Workpad 对 Human Review pending checkpoint 改为等待信号，不再染成 blocker；历史 rework checklist / retry reason 也不会在普通等待审核态误显示为当前黄色告警。
- 自动飞书审核发送在缺少 chat/task/webhook 目标时，会写入 checkpoint.`feishuReview.status = needs-configuration` 并记录 `feishu.review.needs_target`，不再静默留下空状态。
- 补齐 Current attempt 的 Action Plan 摘要样式，避免 action title / status / transition 文案挤在一行。

验证：

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuAutoReviewRecordsNeedsConfigurationWhenNoTarget|TestFeishuReviewRequestSendsInteractiveWebhookCard|TestFeishuReviewRequestUsesLarkCLIInteractiveCard|TestFeishuConfigPreflightUsesLocalLarkCLIProfile'
```

### 飞书 Provider 面板简化与 lark-cli 预检

完成：

- Settings 右侧 Feishu Provider 默认只保留本机 `lark-cli` 状态和 `Test Feishu connection`，Chat / Task / webhook 等固定投递覆盖项收进高级折叠，避免测试前出现一长串非必填字段。
- `/feishu/config/test` 改为优先检查 `lark-cli config show` 的本机 App profile；只要 App ID / App Secret 已配置即可返回 ready，不再因为未填写 chat id / assignee / webhook 直接失败。
- Feishu 连接状态会在检测到本机 `lark-cli` 后显示为 connected，符合“先配置本机 CLI，再由运行链路发起审核”的使用方式。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuConfigEncryptsSecretsAndPublicMasking|TestFeishuConfigPreflightUsesLocalLarkCLIProfile'
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=15000
npm run lint
curl -s -X POST http://127.0.0.1:3888/feishu/config/test | jq .
```

### Workflow Markdown 与 Agent Prompt 契约样例

完成：

- 参考成熟项目模板的组织方式，把 Omega 测试 fixtures 从“简短说明”升级为“可执行契约”。
- `docs/test-workflow-fixtures/prompts.md` 按 Requirement / Architect / Coding / Testing / Review / Rework / Delivery 拆成角色、输入、边界、输出契约和失败处理。
- 新增 `authoring-guide.md`，说明实际项目应准备 `WORKFLOW.md`、`PROMPTS.md`、`AGENTS.md`、`RUNTIME_POLICY.md`、`REVIEW_POLICY.md`、`DELIVERY_POLICY.md`、`REQUIREMENT_TEMPLATE.md` 和 Page Pilot 策略文件。
- 新增 requirement、runtime、review、delivery、Page Pilot policy 样例，方便后续在 Workspace Agent Studio 中逐项复制测试。

验证：

- 本次为文档与配置样例更新，不涉及运行时代码。

### Repository-first 审计表、Proof 预览与 Operation Queue 基础版

完成：

- 新增 SQLite `repository_targets` 审计表，把 Project JSON 中的 repository target 物化成可查询记录；旧 Project snapshot 仍保留为兼容来源。
- 新增 SQLite `handoff_bundles` 审计表，从真实 proof、Attempt、Pipeline、Operation 中抽取 handoff bundle、summary、PR 和 changed files。
- 新增 SQLite `operation_queue` 基础表，从 Operation 物化 queued / running / done / failed / canceled、priority、lock、attemptCount 和 queue payload。
- 新增 `GET /repository-targets`、`GET /handoff-bundles`、`GET /operation-queue` 和 `GET /proof-records/{id}/preview`。
- Proof preview 会读取本地文本、JSON、Markdown、diff 文件，限制预览大小并返回 previewType / truncated / sizeBytes，避免 UI 只能显示文件路径。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=15000
```

后续：

- Project / Pipeline run 全量拆表、旧 snapshot-first 写入清理、worker dequeue / retry mutation、二进制 proof 预览和大文件分页继续作为后续增强。
- shared sync、多端协作授权、App sync loop 和代码库语义索引不在本轮假装完成，已拆到 `docs/todo.md` 的后续计划。

### P0 产品化补齐：Action Plan / Observability / Onboarding / 飞书桥 / Page Pilot 隔离

完成：

- Work Item 详情页接入 Attempt Action Plan：Delivery flow、Attempt stage、Retry / Rework signal 优先消费 `/attempts/{attemptId}/action-plan`，旧的 stage / attempt 推断只作为兼容 fallback。
- Observability dashboard 增强：`GET /observability` 支持 `windowDays`、`groupBy`、`limit`、`from`、`to`，返回分组统计、最近失败、慢阶段 drilldown 和趋势；Views 页面增加窗口/分组切换。
- Project onboarding 基础补齐：新增 `POST /projects` 和 Projects 页面创建入口；Repository target bind 支持 `projectId`，前端绑定仓库时携带当前 Project，避免多项目时误挂到默认项目。
- 飞书 Task 本地事件桥基础版：新增 `/feishu/review-task/bridge/tick`，dry-run 可列出待同步任务；开启 `OMEGA_FEISHU_TASK_BRIDGE_ENABLED=true` 后 JobSupervisor tick 会自动触发飞书 Task 同步。
- Page Pilot isolated-devflow mode 基础版：GitHub Repository target 会解析或自动准备 Omega-managed isolated preview workspace，apply 在隔离 workspace 修改，Confirm 后从隔离 workspace branch / commit / PR，Discard 对隔离 workspace reset/clean。

验证：

```bash
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=15000
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run lint
npm run build -- --mode development
go test ./services/local-runtime/internal/omegalocal
```

### Todo 状态复核

完成：

- 逐项复核 `docs/todo.md` 中未勾选条目，把已经落地但仍停留在旧状态的任务改为完成。
- 更新范围包括 DevFlow preflight、历史参考命名清理、赛题完成度维护、Page Pilot Preview Runtime Go 一等化、Attempt retry/cancel/timeout 策略和 timeout/retry policy 持久化。
- 对仍未完成的条目补充边界说明，避免把基础版已完成和后续增强混在同一个状态里。
- 同步修正 `docs/page-pilot-architecture.md`，明确 Go Preview Runtime supervisor 当前已经记录 profile、pid、stdout/stderr tail 和 health check，跨进程恢复与持久 process table 仍是后续增强。

### Workspace Workflow Editor 目录式改版

完成：

- Workflow tab 从“左侧阶段卡片 + 右侧全量规则列表”改为“左侧目录 + 右侧单项编辑”。
- 左侧目录包含 Template、各 Stage 和 Markdown contract，视觉收敛为窄列导航，不再使用大块按钮式卡片。
- 右侧按当前选中项展示编辑内容：Template 展示模板选择和当前 contract 内容；Stage 只编辑当前 stage rule；Markdown contract 编辑完整 workflow markdown。
- Stage rule 顶部合并为横向摘要，默认规则文案只作为说明展示，不再作为可编辑 textarea 的实际内容。
- Prompts / Agents tab 也改为窄侧栏 + 右侧主编辑区：Prompts 拆成 Role instruction、Execution rules、Review notes；Agents 增加 runner 对应的 model preset、Skills/MCP chip 选择和 raw bindings 高级编辑。
- 保留 `stagePolicy` 保存链路，单个 stage 规则编辑继续序列化回 Agent Profile。

### Workspace Agent Studio 模板导入

完成：

- 新增 `POST /agent-profile/import-template`，支持从内置样例、当前 Repository target 的 `.omega` 目录、显式本地路径导入 workflow / prompt / stage policy。
- 导入会读取 `WORKFLOW.md` / `workflow.md`、`PROMPTS.md` / `prompts.md`、`STAGE_POLICY.md` / `stage-policy.md` / `stage_policy.md`。
- `PROMPTS.md` 会按 Requirement / Architect / Coding / Testing / Review / Rework / Delivery 章节合并进 workflow markdown 的 prompt sections，并同步生成 Agent Profile 摘要。
- Workspace Agent Studio 增加 `Import sample template` 和 `Import from repository .omega` 两个入口，导入后直接保存到 SQLite 一等 Agent Profile 表和兼容 settings。

路径规则：

- 新运行优先读取目标仓库 `{repository.path}/.omega/WORKFLOW.md`。
- 其次读取 Repository scoped Agent Profile，再读取 Project scoped Agent Profile。
- 再应用页面保存的 workflow template override。
- 最后回退到内置 `services/local-runtime/workflows/devflow-pr.md`。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=15000
```

验证：

```bash
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run build
```

## 2026-05-01

### 飞书 Task 审核桥接

完成：

- `POST /feishu/review-request` 支持 `mode=task`，可通过 `lark-cli task +create` 创建绑定 checkpoint 的飞书审核任务。
- 审核任务写入强绑定 token、Work Item、PR、branch、需求摘要和操作规则，降低多任务审核时串单风险。
- 可选 `OMEGA_FEISHU_REVIEW_CREATE_DOC=true` 通过 `lark-cli docs +create` 发布长 review packet 文档，并把文档链接写入任务说明。
- 新增 `/feishu/review-task/sync`：飞书任务完成后，同步为 checkpoint approved，并沿用本地 Human Review approve 决策链路。
- 新增 `/feishu/review-task/comment`：任务评论中的明确修改意见同步为 request changes；问题类评论记录为 need-info，不直接拒绝。
- 自动推送逻辑已支持 task 模式：没有 webhook/chatId 时，只要配置 task mode / assignee / tasklist，也会创建飞书审核任务。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuReviewRequestCreatesTaskReviewWithStrongBinding|TestFeishuReviewTaskSyncApprovesCompletedTask|TestFeishuReviewTaskCommentRequestsChanges|TestFeishuReviewTaskCommentNeedInfoRecordsOnly|TestFeishuReviewRequestSendsInteractiveWebhookCard|TestFeishuReviewRequestUsesLarkCLIInteractiveCard|TestFeishuReviewCallbackApprovesCheckpointThroughSharedDecisionPath|TestFeishuNotifyUsesLocalLarkCLI'
```

### GitHub / CI 出站同步增强与飞书审核卡片本地 CLI 路径

完成：

- GitHub 出站同步从 Issue comment / label 扩展到 PR comment：Attempt 有 `pullRequestUrl` 时会通过 `gh pr comment --edit-last --create-if-none` 维护结构化 review packet。
- PR comment 包含 Work Item、Pipeline、Attempt、状态、branch、review packet、diff preview、risk、recommended actions、checks、PR feedback 和失败原因。
- 新增可选 CI 触发策略：`rerun-failed` 会根据 failed check feedback 中的 run id 执行 `gh run rerun --failed`；`workflow-dispatch` 会执行 `gh workflow run`。
- `lark-cli` 已安装并确认版本为 `1.0.23`；Human Review 在没有 webhook 但有 chat id 时会优先通过 `lark-cli im +messages-send --msg-type interactive` 发送卡片。
- 文档明确：发送飞书回复 / 通知不需要公网；只有飞书云端按钮要直接调用本机 runtime 时才需要公网 callback 或后续本地事件桥。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestSyncGitHubIssueOutboundPostsCommentAndLabels|TestSyncGitHubIssueOutboundPostsPRCommentWithoutIssue|TestGitHubCITriggerRerunsFailedRuns|TestGitHubCITriggerWorkflowDispatch|TestFeishuReviewRequestUsesLarkCLIInteractiveCard'
```

## 2026-04-30

### Work Item 详情页信号卡与页内弹窗

完成：

- 旧做法：Run Workpad 卡片只像折叠容器，默认能看到的有效信息少；展开后内容直接撑开详情页，和 Agent operations 的行内展开一样，会把主视图拉得很长。
- 新做法：Run Workpad 改为紧凑信号卡，每张卡直接展示字段名、状态标题和一条真实来源摘要，例如 rework checklist、PR、validation、blocker、retry reason 或最近 operation。
- 点击 Run Workpad 卡片后打开页内弹窗浏览完整内容，详情页主布局不再因为查看长文本、来源列表或 patch history 被挤开。
- Agent operations 从行内展开改为摘要卡 + 页内弹窗，点击后在弹窗中查看 prompt、stdout、stderr、runner metadata 和执行摘要。
- 更新 `docs/work-item-detail-architecture.md`，保留旧交互说明并记录新交互边界。

验证：

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx apps/web/src/components/__tests__/WorkItemDetailPanels.test.tsx --testTimeout=15000
npm run lint
npm run build
git diff --check
```

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

## 2026-04-30 01:35 CST

### Review/Rework Feedback Sweep 运行时闭环

完成：

- 新增 `docs/devflow-rework-checklist.md`，单独记录 Rework Checklist 的数据结构、信号来源和 runtime 接入点。
- 旧做法：review feedback、人工 request changes、失败原因、runner stderr、PR/check 推荐动作分散在 Attempt、Timeline、Workpad 和 PR status 中，Retry / Rework 只能拿到片段式 reason。
- 新做法：runtime 生成 `reworkChecklist`，统一写入 Attempt 和 Run Workpad；结构包含 `retryReason`、`checklist`、`sources` 和可直接给 Agent 使用的 `prompt`。
- `failAttemptRecord` / `markAttemptCanceled` 会在失败或取消时生成 checklist。
- `prepareDevFlowAttemptRetry` 在用户没有手写 reason 时使用 checklist 主因，并让新 retry Attempt 继承 checklist。
- `prepareDevFlowHumanRequestedRework` 会把人工反馈、评估结果和 checklist 合并，旧 Attempt 与新 rework Attempt 都保留同一份可审计输入。
- DevFlow rework prompt 优先消费 `reworkChecklist.prompt`，自动 rework 时会追加最新 review feedback，避免丢失刚刚的 Review Agent 判断。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestBuildReworkChecklistMergesReviewHumanAndDeliveryGateSignals|TestRunWorkpadRecordTracksAttemptRetryContext|TestPrepareDevFlowAttemptRetryLinksAttempts|TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback'
go test ./services/local-runtime/internal/omegalocal
```

## 2026-04-30 03:35 CST

### Page Pilot 入口与 Repository Workspace 选择

完成：

- 旧做法：Page Pilot 入口只在 Workboard 左侧导航中，Electron 默认停在首页时用户不容易发现。
- 新做法：首页提供 `打开 Page Pilot` / `启动 Page Pilot`，并支持 `#page-pilot` 深链。
- Page Pilot 页面新增 Repository Workspace 选择器，用户进入功能二后先确认目标 repo，后续 apply / deliver 绑定该 repository target。
- Work Item 详情页增加 `Open in Page Pilot`，从具体 item 进入时自动带上该 item 的 `repositoryTargetId`。
- 取消 Page Pilot 默认隐藏 App chrome 的沉浸式入口，避免没有 preview 时只看到空白页和 AI 浮球。
- Electron 增加 App reload IPC，Page Pilot 顶部提供 `Reload app` 按钮，弥补桌面壳没有浏览器地址栏刷新入口的问题。
- 旧尝试：`Open preview` 曾在 Electron BrowserView 中注入简化 selection bridge，并把 selection context 回传给 Web overlay；后续已改回目标页内 direct pilot 主路径。
- 新做法：Page Pilot 页面只作为启动器，选择 repo / preview source 后调用 Electron direct pilot。

验证：

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
```

## 2026-04-30 03:20 CST

### Electron Desktop Shell 自动启动本地服务基础版

完成：

- 旧做法：需要用户手动启动 Go local runtime、Omega Web Vite dev server 和目标项目 preview server，然后再启动 Electron。
- 新做法：`apps/desktop/src/process-supervisor.cjs` 会由 Electron main process 调用，先探测已有服务，缺失时启动本地进程。
- `npm run desktop` 会加载同一套 React SPA；开发模式默认启动/复用 `http://127.0.0.1:3888/health` 和 `http://127.0.0.1:5174/`。
- 目标项目 preview 必须通过 `OMEGA_PREVIEW_REPO_PATH` 或 `OMEGA_PAGE_PILOT_REPO_PATH` 显式指定，避免从 Omega cwd 猜错项目。
- Preview 命令支持 `OMEGA_PREVIEW_COMMAND` 显式覆盖；未设置时按目标 repo 的 lockfile 和 `package.json` scripts 生成保守启动计划。

验证：

```bash
npm run test -- apps/web/src/__tests__/desktopProcessSupervisor.test.ts --testTimeout=15000
```

## 2026-04-30 15:31 CST

### Page Pilot 预览 workspace 边界修正

完成：

- 旧做法：Page Pilot 在 Electron 侧曾尝试按仓库名从用户本机常见目录推断本地 worktree；这会绕开 Repository Workspace 的明确边界，也容易打开错仓库。
- 新做法：local target 只使用用户显式绑定的 `path`；GitHub target 只使用 Omega 管理的隔离 preview workspace，默认位于 `~/Omega/workspaces/page-pilot/<owner_repo>`。
- Electron `omega-preview:resolve-target` 会为 GitHub target 准备隔离 workspace，并返回可打开的 HTML file 或 preview URL；不会扫描 `~/Projects` 等默认目录。
- Go runtime 的 Page Pilot apply 也同步收紧：GitHub target 只认同一隔离 preview workspace，避免 UI 预览和实际写入落在不同目录。
- `Open preview` 增加失败兜底和主进程日志，端口未启动时会明确显示 `ERR_CONNECTION_REFUSED` 等原因。

验证：

```bash
npm run test -- apps/web/src/__tests__/desktopProcessSupervisor.test.ts apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget'
```

## 2026-04-30 16:08 CST

### Page Pilot 目标页面全页化

完成：

- 旧做法：目标项目预览以 BrowserView 覆盖在 Omega 页面右侧，视觉上像半个浮层。
- 中间尝试：打开 preview 后 BrowserView 覆盖 Electron content area，目标 App 作为完整页面展示；`preview-preload.cjs` 注入最小控制条，并把 selection context 发送回 Omega Page Pilot 面板。
- 后续修正：全页展示保留，但主路径改为加载 `pilot-preload.cjs`，目标页面内直接完成多批注、Apply、Confirm、Discard 和返回。

验证：

```bash
npm run lint
npm run test -- apps/web/src/__tests__/desktopProcessSupervisor.test.ts apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
```

## 2026-04-30 03:05 CST

### Page Pilot 回到 Direct Pilot 主路径

完成：

- 旧尝试：Page Pilot React 页面打开 BrowserView 后只注入简化工具条，再把圈选结果回传到 Omega 页面继续操作。这改变了已验证的目标页内使用方式。
- 新做法：React 页面收敛为启动器，只做 Repository Workspace / preview source 选择；Electron 打开目标页后直接加载 `pilot-preload.cjs`，在目标页面内完成圈选、多批注、Apply、Confirm、Discard。
- `omega-preview:open` 支持传入 `projectId`、`repositoryTargetId`、`repositoryLabel`，direct pilot 不再依赖固定 env/default repo。
- 目标页内新增 `返回` 按钮，关闭 BrowserView 回到 Omega 页面。
- 保留 GitHub target 的隔离 preview workspace 解析，避免预览和写入仓库不一致。

验证：

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
```

## 2026-04-30 17:12 CST

### Page Pilot apply 失败态可恢复

完成：

- 现象：direct pilot apply 已提交到 runtime，但本机复用旧 Go runtime 时会返回旧版 workspace 解析错误；目标页顶部只显示错误文本，用户很难判断是卡住还是可恢复。
- 处理：重启 Electron / Go local runtime，确认当前 runtime health 正常且隔离 preview workspace 存在。
- UI：direct pilot 错误状态栏新增 `Reload` / `New`，失败后可以直接刷新目标页面或重新开始选择。
- 服务接入：`pilot-preload.cjs` 调用的 `/page-pilot/apply`、`/page-pilot/deliver`、`/page-pilot/runs/{id}/discard` 现在使用 Electron main process 注入的当前 Go local runtime base URL。
- 状态隔离：direct pilot 本地状态按 `repositoryTargetId + target URL` 分 scope，避免旧 repo / 旧 URL 的失败 run 被误恢复。

验证：

```bash
curl http://127.0.0.1:3888/health
node --check apps/desktop/src/main.cjs
node --check apps/desktop/src/pilot-preload.cjs
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget'
```

## 2026-04-30 17:59 CST

### Page Pilot 交付后动作状态修正

完成：

- 旧做法：direct pilot 结果栏对 `applied`、`delivered`、`discarded` 都渲染 Confirm / Discard / Reload / New，只靠 disabled 控制动作。
- 问题：delivered 后标题已经显示交付完成，但 Confirm / Discard 仍像可点击状态。
- 新做法：只有 `applied` 状态展示 Confirm / Discard；`delivered` / `discarded` 只展示 Reload / New。
- 样式：补充 disabled 视觉兜底，避免后续状态按钮再次产生误导。

验证：

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-30 01:46 CST

### PR 评审/评论并入 Rework Checklist

完成：

- 旧做法：DevFlow 运行中只读取 PR diff / checks，PR review comment 和 review state 没有进入 Attempt；request changes 后下一轮 Rework 主要依赖人工输入和 Review Agent 摘要。
- 新做法：`/github/pr-status` 和 DevFlow PR cycle 增加 `gh pr view --json comments,reviews` 基础采集，生成 `reviewFeedback` / `pullRequestFeedback`。
- `reworkChecklist` 增加 `pr-review` / `pr-comment` source，下一轮 Rework Agent prompt 会同时看到人工意见、Review Agent 结果、PR 评论和交付门禁建议。
- `attempt-run-report.md` 增加 Pull Request Feedback 小节，便于 Human Review 时审计 PR 外部反馈是否被纳入。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestGitHubPRStatusUsesGhViewAndChecks|TestGitHubPullRequestFeedbackFromView|TestBuildReworkChecklistMergesReviewHumanAndDeliveryGateSignals'
```

## 2026-04-30 02:05 CST

### Failed Check Log 进入 Rework Checklist

完成：

- 旧做法：PR checks 失败时 checklist 只能显示 `checks-failed` 推荐动作，下一轮 Rework 还需要用户自己去 GitHub Actions 找失败日志。
- 新做法：`/github/pr-status` 和 DevFlow PR cycle 会从 failed / error / canceled / timed out check 的 link 中抽取 Actions run id，优先执行 `gh run view --log-failed`，并把输出写入 `checkLogFeedback`。
- `reworkChecklist` 增加 `ci-check-log` source，Workpad Review Feedback 和 `attempt-run-report.md` 也会展示失败日志摘要。
- 如果 log 拉取失败，runtime 不阻塞 PR status 或 DevFlow 主链路；仍保留 checks summary 和 recommended action。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestGitHubPRStatusClassifiesFailedChecks|TestBuildReworkChecklistMergesReviewHumanAndDeliveryGateSignals|TestRunWorkpadRecordTracksAttemptRetryContext'
```

## 2026-04-30 02:05 CST

### Workpad Checklist Source Drilldown 基础版

完成：

- 旧做法：Workpad 只展示 Rework Checklist action，用户能看到“要做什么”，但展开后仍不容易追溯这条 action 来自人工反馈、Review Agent、PR 评论、check log 还是 delivery gate。
- 新做法：Work Item 详情页的 Rework Checklist 展开内容增加 `Checklist sources`，展示 source kind、label 和摘要，保持卡片默认短小，展开后能看到依据。
- Runtime 会保留 PR comment / review / check log source 的 URL、run id 和 state 等基础元数据；前端在存在 URL 时展示 `Open source`。
- 浅色 / 深色模式都增加对应样式，避免 source 区块重新变成难读日志墙。

验证：

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
```

## 2026-04-30 02:37 CST

### Run Workpad 字段级 Patch 审计基础版

- 目标：补齐 todo 中 Run Workpad field patch 的权限边界、source attribution 和变更历史基础能力，避免 Agent / supervisor 写入 Workpad 后无法解释来源。
- 旧做法：`PATCH /run-workpads/{id}` 只保存最终 `fieldPatches`，后续只能知道字段被覆盖，不能知道谁写入、为什么写入、来自哪个运行时事件。
- 新做法：PATCH payload 支持 `updatedBy`、`reason`、`source`；runtime 会按字段写入 `fieldPatchSources`，并追加 `fieldPatchHistory`，最多保留 100 条历史。
- 权限边界：`operator` / `human-review` 只能写人工判断与反馈字段，`job-supervisor` 可写运行门禁字段，`agent` / `review-agent` / `delivery-agent` / `test` 可写 Agent 交接字段；未知写入者或越权字段返回 400。
- UI：Work Item 详情页新增默认折叠的 Patch history 卡片，展示最近写入者、字段、来源和原因；编辑入口仍后续单独设计。
- 验证：新增 Go 单测覆盖 patch 来源、历史持久化、runtime refresh 后重新叠加，以及 operator 越权写 plan 被拒绝；前端 API client 单测覆盖 source / reason / history 字段。

## 2026-04-30 02:18 CST

### Run Workpad 字段级 Patch 基础版

完成：

- 旧做法：`runWorkpads` 每次由 runtime 派生刷新整份记录，Agent / supervisor 即使想补充 Blockers、Validation、Review Feedback，也没有稳定写入点。
- 新做法：新增 `PATCH /run-workpads/{id}`，允许写入明确字段：Plan、Acceptance Criteria、Validation、Notes、Blockers、PR、Review Feedback、Retry Reason、Rework Checklist、Rework Assessment、updatedBy。
- Patch 会保存到 `fieldPatches`；后续 runtime 刷新时先生成真实派生 Workpad，再重新叠加 `fieldPatches`，避免 heartbeat / attempt 更新冲掉 supervisor 或 Agent 写入。
- 前端 API client 增加 `patchRunWorkpad`，为后续 Operator/Agent 写入 Workpad 做准备。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPatchRunWorkpadPersistsFieldPatchesAcrossRefresh|TestRunWorkpadRecordTracksAttemptRetryContext'
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts -t 'Run Workpad' --testTimeout=15000
```

## 2026-04-30 18:20 CST

### Page Pilot 注入层低干扰收起态

完成：

- 返回控制台入口从左上角常驻按钮改为左侧透明边缘热区，默认不遮挡目标页面内容，hover / focus 时才展开。
- 顶部/底部状态条收起后完全移出视野，不再保留可见残边。
- 状态条仍保留透明热区唤回，避免用户隐藏后无法恢复操作面板。

验证：

```bash
node --check apps/desktop/src/pilot-preload.cjs
git diff --check
```

## 2026-04-30 18:45 CST

### Dev server 模式接入 Preview Runtime Agent

完成：

- Page Pilot 的 Dev server 入口不再直接打开手填 URL，改为调用 Electron `omega-preview:start-dev-server`。
- 新增本地 `preview-runtime-agent`：读取所选 repository workspace，生成 Preview Runtime Profile，并在该 workspace 内启动 dev server。
- profile 记录 stage、工作目录、dev command、preview URL、health check、reload strategy、项目 evidence 和职责说明。
- dev server health check 通过后才打开 direct pilot；失败时返回具体原因，不再表现成“点了没反应”。

验证：

```bash
npm run test -- apps/web/src/__tests__/desktopProcessSupervisor.test.ts apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
```

## 2026-04-30 19:55 CST

### Preview Runtime 接管 Page Pilot 刷新

完成：

- 目标页内 Reload 和 apply/discard 后刷新不再直接 `window.location.reload()`。
- Electron `omega-preview:reload` 会先调用 Preview Runtime Supervisor。
- Supervisor 根据 profile 和 changed files 选择 `hmr-wait`、`browser-reload` 或 `server-restart`。
- 修改运行时配置或 health check 失败时会重启 dev server，成功后再刷新 BrowserView。
- 新增策略单测，覆盖普通源码、runtime config 和静态 HTML 场景。

验证：

```bash
npm run test -- apps/web/src/__tests__/desktopProcessSupervisor.test.ts apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
npm run lint
node --check apps/desktop/src/process-supervisor.cjs
node --check apps/desktop/src/main.cjs
node --check apps/desktop/src/omega-preload.cjs
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-30 20:30 CST

### Page Pilot live-preview repository write lock

完成：

- Page Pilot `apply` 在修改源码前会声明 live-preview execution lock，scope 绑定 `repositoryTargetId` 和真实 `repositoryPath`。
- `apply` 成功后锁会关联 `pagePilotRunId`、Work Item 和 Pipeline，直到用户 Confirm 或 Discard。
- 第二个 Page Pilot run 如果试图写同一预览工作区，会在 apply 前被拒绝，不会进入 Agent 修改。
- `deliver` 允许同一个 run 继续使用锁完成交付，成功后释放；`discard` 恢复 changed files 后释放。
- `apply` 在 runner / diff / persist 失败时会自动释放锁，避免失败 run 卡住后续操作。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget'
```

## 2026-04-30 21:40 CST

### Page Pilot Recent Runs 与 Work Item 回跳

完成：

- Page Pilot 启动器新增 Recent runs 区块，读取 `/page-pilot/runs` 并展示当前 Repository Workspace 的最近 run。
- run 卡片展示真实状态、变更文件数量、Work Item、Pipeline 和更新时间。
- 有 PR 时提供 PR 跳转；有 Work Item 时回跳独立 Work Item 详情页。
- 保持 direct pilot 主路径不变，只在启动器层补可追踪入口。
- 补充 light / dark 样式，避免 run 卡片在暗色主题下读不清。

验证：

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
npm run lint
```

## 2026-04-30 22:20 CST

### Page Pilot 服务端 Run Conversation

完成：

- Electron direct pilot 在 `/page-pilot/apply` 请求中带上本轮 `conversationBatch`、`submittedAnnotations` 和 `processEvents`。
- Go runtime 把批注轮次、主目标、过程事件持久化到 Page Pilot run。
- `deliver` 会把同一 run 的 conversation 状态推进到 `delivered`。
- `discard` 会把同一 run 的 conversation 状态推进到 `discarded`。
- Page Pilot mission 和 pipeline run artifacts 同步写入 conversation，后续 Work Item 详情不需要依赖目标页 localStorage 读取批注历史。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget'
```

## 2026-04-30 22:40 CST

### Preview Runtime Profile 关联 Page Pilot Run

完成：

- Page Pilot 启动器在 Dev server / Repository source / HTML file 模式下生成或接收 Preview Runtime Profile。
- Electron direct pilot 打开目标页时把 profile 写入 preload 配置。
- `/page-pilot/apply` 会把 profile 持久化到 Page Pilot run。
- `/page-pilot/deliver` 继续保留同一 profile。
- Page Pilot mission 和 pipeline run artifacts 同步写入 `previewRuntimeProfile`，便于后续 Work Item 详情展示启动命令、健康检查和刷新策略。

边界：

- 这是 Go 一等化的持久化基础版；目标项目 dev server 的真实启动 / restart 仍由 Electron supervisor 执行。
- 后续需要把 resolve/start/restart API 和 pid/stdout/stderr/health check 统一下沉到 Go runtime。

验证：

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget'
```

## 2026-04-30 22:55 CST

### Page Pilot Source Mapping 覆盖率报告基础版

完成：

- `/page-pilot/apply` 会根据提交批注生成 `sourceMappingReport`。
- 报告记录：
  - `totalSelections`：本轮批注数量；
  - `strongSourceMappings`：带 `sourceMapping.file` 的强源码映射数量；
  - `domOnlySelections`：DOM-only 选区数量；
  - `missingFileSelections`：缺失文件映射数量；
  - `coverageRatio` 和 `status`。
- `/page-pilot/deliver` 保留同一报告。
- Page Pilot mission 和 pipeline run artifacts 同步写入报告，便于后续 Work Item 详情和失败诊断展示。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget'
```

## 2026-04-30 23:10 CST

### Source Mapping Report 接入 Agent 输入

完成：

- `sourceMappingReport` 不再只是给人看的审计数据。
- Page Pilot `apply` 会在执行 Agent 前根据覆盖率决定是否生成 `sourceLocator`。
- 当批注缺少 `sourceMapping.file` 时，runtime 会按文本快照、selector token、DOM tag 和批注 token 搜索源码候选。
- `buildPagePilotPrompt` 会把覆盖率报告和候选文件写进 Agent prompt，并要求：
  - 强映射时优先使用明确文件；
  - DOM-only / partial 时先检查候选；
  - 没有有用候选时不要凭空改无关文件。
- deterministic `local-proof` 路径也会复用候选文件，验证 DOM-only 选区可以真实落到源码替换。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotDomOnlySelectionUsesSourceLocator|TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget'
```

## 2026-04-30 23:40 CST

### 功能一 / 功能二 P0 闭环收敛

完成：

- 旧做法：GitHub 权限、PR/checks 可读性和交付前置条件主要在 PR 创建、Human Review approve 或 merge 时暴露，失败位置偏晚。
- 新做法：DevFlow preflight 增加 GitHub delivery contract 检查，运行前确认 repository target、`gh` 登录、viewer permission 和 PR/checks 元数据读取。
- 旧做法：Page Pilot 最近运行只展示摘要，用户要回看 diff、PR body、source mapping 或视觉证据需要跳到不同记录。
- 新做法：Page Pilot Recent runs 增加详情弹窗，集中展示 PR preview、diff summary、source mapping、visual proof、preview runtime、conversation 和 Work Item 回跳。
- 旧做法：Page Pilot 每次 apply 都偏向创建新 run；继续修改同一页面时容易变成多条分散记录。
- 新做法：`/page-pilot/apply` 支持 `runId`，同一 run 内追加批注 / 说明 / apply，并递增 `roundNumber`，复用同一个 Work Item / Pipeline。
- 旧做法：Preview Runtime 的启动和刷新主要由 Electron supervisor 承担，Go runtime 只保存部分 profile。
- 新做法：Go runtime 暴露 Preview Runtime `resolve/start/restart` API，锁定明确 Repository Workspace，记录 profile、pid、stdout/stderr tail 和 health check 基础信息。
- 新增固定测试脚本 `npm run test:feature-p0` 和 `docs/test-report.md`，让这批 P0 能力有统一回归入口。

验证：

```bash
npm run lint
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
node --check apps/desktop/src/process-supervisor.cjs
node --check apps/desktop/src/pilot-preload.cjs
go test ./services/local-runtime/internal/omegalocal
```

## 2026-05-01 01:55 CST

### JobSupervisor 自动恢复分类

完成：

- 旧做法：failed / stalled Attempt 只按 retry 上限和 backoff 判断，失败原因没有明确分层。
- 新做法：JobSupervisor recovery scan 会先生成 `recoveryPolicy`，再决定自动 retry、等待、人工修权限或进入 rework。
- 分类覆盖：
  - runner crash / worker host orphan：重启干净 worker 后 retry；
  - 临时网络失败：等待 backoff 后 retry；
  - GitHub API 临时失败：等待 API 恢复后 retry；
  - CI flaky failure：优先重试验证；
  - 权限失败：停止自动 retry，要求修复凭据、仓库权限或 branch policy；
  - 非 flaky CI failure：进入 rework checklist，而不是盲目重跑。
- `recoverableAttempts` 和 `acceptedRetryRuns` 都会带上 `recoveryPolicy`，Operator / UI 后续可以直接展示推荐动作。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestSupervisorRecoveryPolicyClassifiesFailureModes|TestJobSupervisorScanRecoverableAttemptsCreatesRetryJob|TestJobSupervisorRecoveryPolicyBlocksPermissionAutoRetry|TestJobSupervisorScanRecoverableAttemptsUsesWorkflowRetryPolicy|TestJobSupervisorScanRecoverableAttemptsRespectsBackoffAndLimit'
```

## 2026-05-01 02:35 CST

### Runtime Log 查询增强

完成：

- 旧做法：`GET /runtime-logs` 只能按基础字段和时间范围拉取最近日志，排查一个 Requirement 需要人工拼 Work Item / Pipeline / Attempt。
- 新做法：runtime logs 增加 `requirementId` 维度，查询时会兼容新日志字段和旧日志关联反查。
- `GET /runtime-logs` 保持默认数组返回；需要 cursor 时使用 `page=1`，返回 `items`、`nextCursor` 和 `hasMore`。
- `q` / `search` 支持全文搜索 event type、message、level 和 details JSON。
- 新增 `GET /runtime-logs/export`，支持 JSONL / CSV 导出同一组过滤结果。
- `omega logs` 增加 Requirement、全文搜索和 cursor 分页参数，CLI / UI 继续共用同一套 Runtime API。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestRuntimeLogsAPIListsAndFiltersRecords'
go test ./services/local-runtime/internal/omegacli
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=15000
```

## 2026-05-01 03:20 CST

### 数据分析指标扩展

完成：

- 旧做法：`/observability.dashboard` 只有基础健康度指标，能看 Attempt 成功率、失败原因、慢阶段和待人工队列。
- 新做法：dashboard 增加 stage 平均耗时、runner 使用次数、checkpoint 等待时长、PR 创建/合并/open 数量和按天趋势。
- stage 聚合会输出 count、averageDurationMs、maxDurationMs、latestAt，支持后续 UI 直接按“最慢 stage”排序。
- runner 聚合会输出 successCount、failureCount、activeCount 和 averageDurationMs，支持观察不同 runner 的使用与稳定性。
- checkpoint 等待时长同时给出全局和按 stage 拆分，便于发现 Human Review 或其他 gate 卡点。
- PR 指标先使用本地 Attempt / ProofRecord 中的 PR URL 和 merge proof 推断，避免为了统计再访问远端服务。
- TypeScript control API 类型补齐 dashboard 扩展字段。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestObservabilityDashboardMetrics'
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=15000
```

## 2026-05-01 03:55 CST

### Run Report / Review Packet 扩展

完成：

- 旧做法：Human Review 前只有 Markdown run report，能读但不适合 UI 和后续 Agent 稳定消费。
- 新做法：DevFlow 生成 `attempt-review-packet.json`，结构化保存 diff/test/check preview、risk level、risk reasons 和 recommended actions。
- `attempt-run-report.md` 同步增加 Diff Preview、Test Preview、Check Preview、Risk、Recommended Actions 小节，与 JSON packet 使用同一份派生结果。
- `handoff-bundle.json`、Attempt record、Run Workpad record 都写入 `reviewPacket`，后续 approve、request changes、retry、rework 可以复用同一份上下文。
- Work Item 详情页 Run Workpad 新增 `Review packet` 卡片，点击后用页内弹窗展示一页预览：diff 文件、测试状态、checks 状态、风险原因、下一步动作和 diff excerpt。
- light / dark 样式同步补齐，避免 packet 预览在不同主题下读不清。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestRunDevFlowPRCycleCreatesBranchPRAndMergeProof|TestRunWorkpadRecordTracksAttemptRetryContext'
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
npm run lint
```

## 2026-05-01 04:10 CST

### Run Workpad 字段级 patch UI

完成：

- 旧做法：Run Workpad 字段级 patch 只能通过 API / Agent / supervisor 写入，详情页只能看 `fieldPatchHistory`，不能直接补充人工判断。
- 新做法：Work Item 详情页新增 `Edit fields`，通过页内弹窗编辑 operator 允许字段。
- 支持字段：Notes、Blockers、Review Feedback、Retry Reason、Validation、Rework Checklist、Rework Assessment。
- 提交链路调用真实 `PATCH /run-workpads/{id}`，并写入 `updatedBy=operator`、`reason`、`source.kind=ui`，继续复用后端字段权限和审计历史。
- `App.tsx` 只保留 API 接线回调，表单和字段序列化留在 `WorkItemDetailPage`，降低入口文件耦合。
- 补充组件测试，验证 UI 提交会生成正确的 PATCH payload。

验证：

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
```

## 2026-05-01 04:35 CST

### Review/Rework feedback sweep 扩展

完成：

- 旧做法：PR comments / reviews 和 failed check log 已能进入 Rework Checklist，但 review thread 的 resolved 状态、行级上下文和重复信号分组不足。
- 新做法：PR feedback 采集增加 review thread best-effort GraphQL 读取，记录 resolved/unresolved、path、line、diffHunk 和 sourceUrl。
- unresolved review thread 会生成带文件行号的 rework action；resolved thread 只作为来源证据保留，不再让 Agent 重做已解决事项。
- failed check log 增加 `sourceUrl` 深链和 `logMode=failed-first`，Workpad source drilldown 可直接打开 check 来源。
- Rework Checklist 增加 `groups`，按文件行、check run 或归一化内容自动去重分组，重复信号在 checklist 中合并为一条，并标注相关信号数量。
- Work Item 详情页 source drilldown 展示 source state、path 和 line，便于人工检查 rework 来源。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestGitHubPullRequestFeedbackFromView|TestGitHubPullRequestReviewThreadFeedbackFromGraphQL|TestBuildReworkChecklistMergesReviewHumanAndDeliveryGateSignals|TestGitHubPRStatusClassifiesFailedChecks'
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
```

## 2026-05-01 04:55 CST

### GitHub / CI 出站同步

完成：

- 旧做法：GitHub Issue 可以导入，PR / checks 可以读取，但 Pipeline 状态没有真实写回 GitHub Issue。
- 新做法：DevFlow 在 attempt started、human review waiting、merge failed、attempt failed、delivery completed 等节点执行 GitHub Issue outbound sync。
- 出站同步通过 `gh issue comment` 写入结构化状态评论，通过 `gh label create --force` 和 `gh issue edit` 管理 `omega:*` 标签。
- comment 内容会包含 PR URL、changed files、checks 输出、PR feedback、failed check log、失败原因、review packet 风险等级和推荐动作。
- sync report 会写入 Attempt record，并保存到 proof JSON；失败时写 runtime log，但不阻断 PR / review / merge 主链路。
- 非 GitHub Issue 来源会 `skipped`，不会误写其他仓库或平台。
- 新增 `docs/github-outbound-sync.md` 说明生命周期、标签映射、同步内容和验证方式。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestGitHubIssueRefFromWorkItemParsesImportRefAndURL|TestSyncGitHubIssueOutboundPostsCommentAndLabels|TestSyncGitHubIssueOutboundSkipsUnlinkedWorkItem'
go test ./services/local-runtime/internal/omegalocal
```

真实 GitHub smoke：

- `gh auth status` 通过，账号具备 `repo` / `workflow` scope。
- `ZYOOO/TestRepo` 已 clone 到临时目录并 `git pull --ff-only`。
- 创建临时 issue `ZYOOO/TestRepo#36`，成功 comment、创建/切换 `omega:*` 标签，并关闭 issue。

## 2026-05-01 05:35 CST

### 飞书 Human Review 审核链路

完成：

- 旧做法：飞书只有 `POST /feishu/notify` 文本通知，Human Review 只能在 Omega Web 内 approve / request changes。
- 新做法：新增 `POST /feishu/review-request`，从 checkpoint 组装 Work Item、Requirement、Attempt、Run Workpad 和 Review Packet，生成飞书 Human Review 卡片。
- 发送优先级：飞书机器人 webhook interactive card > `lark-cli` 文本 fallback > `needs-configuration` 预览记录。
- 新增 `POST /feishu/review-callback`，飞书 approve / request changes 与 Omega Web 本地按钮复用同一个 checkpoint decision helper，避免线上线下审核状态分叉。
- DevFlow 进入 Human Review 后，如果配置了 `OMEGA_FEISHU_WEBHOOK_URL` / `FEISHU_BOT_WEBHOOK` 或 `OMEGA_FEISHU_REVIEW_CHAT_ID`，会自动推送审核通知。
- 发送结果持久化到 checkpoint.`feishuReview`，包含 provider、tool、format、message id 或 card/doc preview。
- 新增 `docs/feishu-review-chain.md` 和人工验证清单。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuReviewRequestSendsInteractiveWebhookCard|TestFeishuReviewCallbackApprovesCheckpointThroughSharedDecisionPath|TestFeishuNotifyUsesLocalLarkCLI'
```

需要人工：

- 当前本机没有 `lark-cli`。
- 真实飞书群卡片和 callback 需要配置机器人 webhook / 公网 runtime URL / 可选 token；待测项已写入 `docs/manual-testing-needed.md`。

## 2026-05-01 06:20 CST

### Workflow Action Graph 基础版

完成：

- 对照参考项目和 Projects 下模板后，确认 Omega 现有 workflow contract 已有 stages / prompts / runtime / transitions，但执行图还不够一等化。
- 默认 `devflow-pr` workflow 增加 `states.actions`，覆盖 requirement、implementation、review、rework、human review、merging、done 的真实动作序列。
- Workflow parser 新增：
  - `states.actions`
  - action `transitions`
  - review action `verdicts`
  - `taskClasses`
  - 基础 hooks snapshot
- Pipeline run workflow snapshot 新增 `states`、扁平 `actions`、`taskClasses`、`hooks`、`executionMode`。
- Workflow validator 会拒绝缺少 action id/type 或 transition 指向未知 stage 的 contract。
- DevFlow Agent invocation 的 stage 推进优先读取 snapshot transitions，降低固定 Go switch 对流程顺序的耦合。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestDevFlowTemplateLoadsWorkflowMarkdownContract|TestWorkflowContractParsesStateActionsAndRejectsBrokenActionRoute|TestRepositoryWorkflowTemplateValidationRejectsBrokenContract|TestDevFlowReviewOutcomeRoutesChangesRequestedToRework|TestDevFlowStageStatusAfterChangesRequestedQueuesRework'
```

## 2026-05-01 06:45 CST

### Attempt Action Plan 基础版

完成：

- 新增 `GET /attempts/{attemptId}/action-plan`，从 Pipeline workflow snapshot 生成 Attempt 的可解释执行计划。
- Action plan 包含 current state、current action、state actions、transitions、taskClasses、hooks、retry action 和恢复策略。
- failed / stalled / canceled attempt 会复用 JobSupervisor recovery policy，返回 retry reason、recommended action 和 failure class。
- JobSupervisor recovery summary / accepted retry job 会附带 action plan 摘要，使自动恢复决策与 workflow snapshot 对齐。
- 该 API 只做 dry-run，不执行 git、runner、PR 或 merge 命令，作为迁移通用 action executor 的保护层。
- 新增单测覆盖 workflow snapshot action plan 和 retry policy 输出。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestBuildAttemptActionPlanUsesWorkflowSnapshot|TestAttemptActionPlanAPIIncludesRetryPolicy'
go test ./services/local-runtime/internal/omegalocal -run 'TestJobSupervisorScanRecoverableAttemptsRetriesWithPolicy|TestBuildAttemptActionPlanUsesWorkflowSnapshot|TestAttemptActionPlanAPIIncludesRetryPolicy'
```

## 2026-05-01 19:14 CST

### Workspace Agent Studio 基础版

完成：

- 将设置页中的 Agent Profile 大表单拆到 `apps/web/src/components/WorkspaceAgentStudio.tsx`，降低 `App.tsx` 继续膨胀的风险。
- Workspace Config 新增工作区级共享配置体验：
  - Workflow：图形化展示 workflow stages，并显示选中阶段的 Agent、Gate、Artifacts。
  - Prompts：集中编辑每个 Agent 的 stage prompt、Codex policy、Claude policy，并保留 workflow prompt section 高级编辑。
  - Agents：继续支持 runner / model / Skills / MCP 配置和本机 capability preflight。
  - Runtime files：继续预览 `.omega` / `.codex` / `.claude` runner 文件。
- 保持保存链路不变，仍通过现有 Agent Profile API / SQLite 保存，不做仅前端生效的临时配置。
- 更新设置页测试，覆盖 Workflow graph、Prompts、Agent roster、Runtime file preview。

验证：

```bash
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run build
```

## 2026-05-01 21:40 CST

### Workspace Agent Runner 与账号配置边界修正

完成：

- 修正上一版误解：页面账号 / Key 配置只收敛到 `opencode` 和 `Trae Agent`，不是移除 Codex / Claude Code runner。
- Workspace Agent Studio 的 runner 编排恢复 Codex 优先，并继续展示 Codex / opencode / Claude Code / Trae Agent。
- Go runtime 默认 Agent Profile 和 `profile` / `auto` runner fallback 保持 Codex 优先。
- 旧本地 session 或旧 Agent Profile 中保存的 `codex` runner 会继续保留；`claude` 会归一化为 `claude-code`。
- 新增 `trae-agent` capability 和 Trae Agent runner 测试，runner 使用 `trae-cli run <prompt> --working-dir <workspace>`，并支持 `provider:model` 或 `OMEGA_TRAE_PROVIDER` / `OMEGA_TRAE_MODEL` 注入。
- `run-current-stage`、workspace operation API 和 Mission Control API 的前端类型已扩展为 Codex / opencode / Claude Code / Trae Agent，设置页保存和本机 capability preflight 使用同一套 runner 选项。

验证：

```bash
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx apps/web/src/__tests__/missionControlApiClient.test.ts --testTimeout=15000
npm run build
GOCACHE=/private/tmp/omega-go-build-cache go test ./services/local-runtime/internal/omegalocal -run 'TestTraeAgentRunnerUsesTraeCLI|TestProfileRunnerRegistrySelectsConfiguredAgentRunner|TestProfileRunnerPreflightRejectsUnavailableRunner|TestLocalCapabilitiesReportsInstalledCliTools'
```

## 2026-05-01 23:52 CST

### Workspace Config 与 Workflow Rules 编辑体验整理

完成：

- Workspace Config 的本地 workspace folder 与 repository scope 改成上下结构，去掉 scope 卡片里重复的 Workflow / Agents / Prompts / Runtime 快捷入口，避免和下方 Workspace Agent Studio 重复。
- Workflow Rules 不再只展示一段压缩 textarea，改成按阶段拆分的可编辑规则行，仍保存到 Agent Profile 的 `stagePolicy` 并由 runtime profile 消费。
- 前端和 Go runtime 的默认 `stagePolicy` 从旧的一句压缩说明扩展为 Requirement、Architecture、Coding、Testing、Review、Rework、Human Review、Delivery 八段规则。
- 清理 Workspace Agent Studio 高级 Markdown contract 区域的浅色主题残留深色背景。

验证：

```bash
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run build
go test ./services/local-runtime/internal/omegalocal -run 'TestProjectAgentProfilePersistsAndFeedsRuntimeBundle|TestProfileRunnerRegistrySelectsConfiguredAgentRunner'
```

## 2026-05-02 03:20 CST

### Runner 账号凭据加密配置

完成：

- 新增 `GET /runner-credentials` / `PUT /runner-credentials`，用于保存 opencode / Trae Agent 的本地账号配置。
- API Key 使用本机 AES-GCM 加密后写入 SQLite；接口返回只包含 configured / masked 状态，不回显明文。
- Runner 执行链路增加 `Env` 注入能力，Trae Agent 会在运行前解密账号凭据并注入 `DOUBAO_API_KEY` / `DOUBAO_BASE_URL`，不把密钥放进命令参数或 process args。
- Trae Agent model 支持从账号配置里的 EP ID 自动补齐；Agent Profile 里显式写 `provider:model` 时仍可覆盖。
- Workspace Agent Studio 的 Runtime files 页新增 opencode / Trae Agent 账号卡片，支持 provider、EP ID / model、base URL、API Key 密码框和眼睛显示开关。
- Go runtime 启动时补齐常见用户级安装目录到 PATH，避免 Desktop 环境检测不到 `~/.local/bin/trae-cli`。
- 新增单测覆盖：接口不泄漏密钥、SQLite 不保存明文、Trae 子进程拿到环境变量、命令参数不包含密钥。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestRunnerCredentialEncryptsAndInjectsTraeEnv|TestTraeAgentRunnerUsesTraeCLI|TestLocalCapabilitiesReportsInstalledCliTools'
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
```

## 2026-05-02 16:20 CST

### P0 自动回归、远端信号轮询与 Workflow Template 一等化

旧做法：

- JobSupervisor 的心跳主要来自本机 runner 进程、Attempt event 和 runtime log，远端 runner host / PR checks 不会主动刷新 `lastSeenAt`。
- PR 创建后的 CI / required checks 主要进入报告和人工审核视图；如果 checks 失败，更多依赖人工点 Retry 或后续手动 rework。
- Workflow Template 主要来自默认文件、目标仓库 `.omega/WORKFLOW.md` 或 Agent Profile 内嵌 markdown，没有独立的 SQLite 记录和编辑 API。

新做法：

- JobSupervisor tick 会扫描 running / waiting-human Attempt，对绑定 PR 轮询真实 GitHub checks / required checks，写入 `remoteSignals`，并用远端 worker host heartbeat 刷新 `lastSeenAt`。
- DevFlow PR 创建后会读取 structured checks、required checks 和失败日志；如果 CI / required checks 阻塞，会在 `maxReviewCycles` 内自动进入 Rework，再回到测试和评审，继续复用同一隔离 workspace、同一 branch 和同一 PR。
- Review Agent、PR comments/reviews、failed check log 和 required checks 统一汇入 rework input / checklist，避免 retry 时只看到底层 stderr 而看不到业务修复原因。
- 新增 SQLite 一等表 `workflow_templates`，支持 Project / Repository Workspace 覆盖；新增读取、校验、保存、恢复默认 API，运行时通过 Agent Profile 解析并消费覆盖后的 workflow markdown。
- 第四项按当前计划暂缓，保留在 todo 中，不在本轮实现。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=15000
npm run lint
npm run build
```

## 2026-05-02 18:10 CST

### 通用 Action Executor 阶段 3 基础版

完成：

- 新增 `workflow-action-handler` 路由层，统一解析 Pipeline workflow snapshot / template 中的 state actions、action verdict、state transition 和 template transition。
- Review / Rework / Merging 的下一阶段不再只靠固定 Go switch 推断：
  - Review `passed` 会按 `run_review` action verdict 归一为 `approved`。
  - Review `changes-requested` 会按 `changes_requested` verdict 路由。
  - Rework / Merging `passed` 会优先消费 state transition。
- Human Review approved 到 Merging、Merging passed 到 Done 会写入 action route 元数据，同时保留现有真实 PR merge、proof、handoff 行为。
- 旧固定顺序仍保留为 fallback，兼容历史 pipeline 或未配置 action graph 的 workflow。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestWorkflowActionRoute|TestDevFlowReviewOutcome|TestDevFlowStageStatusAfterChangesRequestedQueuesRework'
```

## 2026-05-02 22:10 CST

### DevFlow Contract Action Executor 增强

旧做法：

- 默认 DevFlow 虽然已经有 `states.actions`，但 Review 轮次仍主要从旧 `reviewRounds` 字段读取。
- Review loop 会按 Go 固定顺序跑完所有 review round，即使 contract 把第一轮 `approved` 指到 Human Review，也可能继续执行后续固定 review。
- Rework 的回环默认从第一轮 Review 推断，不能可靠表达“从哪一轮 Review 触发，就按哪一轮的 verdict 回到目标 stage”。
- action type 只校验非空，配置了 runtime 不认识的 action 时不够早暴露。

新做法：

- `executionMode` 升级为 `contract-action-executor`，默认 `devflow-pr.md` 被视为当前 DevFlow 的运行协议。
- Review 轮次从 `states.actions` 中的 `run_review` action 派生；旧 `reviewRounds` 只作为 artifact、focus、diffSource 的兼容展示补充。
- Review `approved` / `changes_requested` / `needs_human_info` 按 action verdict / transition 推进；如果 `approved` 指向非 Review stage，会结束 Review 序列。
- Rework 根据实际触发的 Review stage 读取 `changes_requested` 路由，再进入 contract 指定的 rework stage。
- 新增 action handler registry，`write_requirement_artifact`、`run_agent`、`run_validation`、`ensure_pr`、`run_review`、`build_rework_checklist`、`human_gate`、`refresh_pr_status`、`merge_pr`、`write_handoff` 等 action type 都有明确 handler 名称；未知 action type 会在 workflow validation 阶段失败。
- Agent invocation 的 process metadata 增加 action route，便于 Run Timeline / Workpad 追踪当前执行来自哪一个 contract action。
- 新增 `docs/devflow-contract.md`，说明当前默认 contract、可修改范围、handler registry 和 Review/Rework 路由规则。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestWorkflowActionRoute|TestDevFlowReviewRounds|TestWorkflowContractRejectsUnsupportedActionType|TestDevFlowTemplateLoadsWorkflowMarkdownContract|TestBuildAttemptActionPlanUsesWorkflowSnapshot'
go test ./services/local-runtime/internal/omegalocal
```

## 2026-05-02 23:05 CST

### 通用 Action Executor 阶段 4 主链路迁移

旧做法：

- Requirement、architecture、coding、validation、push、ensure PR 在 `executeDevFlowPRCycle` 中按 Go 代码顺序直接铺开。
- workflow contract 可以展示 action plan，但 implementation 主链路不能真正改变执行顺序。
- contract 中引用了 DevFlow runtime 未实现的 action 时，缺少明确的执行期错误。

新做法：

- 新增 `runDevFlowContractState`，按 active workflow template 的 `states.actions` 顺序执行真实 handler。
- `todo` state 通过 `write_requirement_artifact` 写 Requirement artifact。
- `in_progress` state 通过 action handler 执行：
  - `classify_task`
  - `run_agent` / `architect`
  - `run_agent` / `coding`
  - `run_validation` / `testing`
  - `ensure_pr` / `delivery`
- Contract 可以调整这些 action 的顺序或移除非必需 action；缺少 handler 会返回 workflow contract action error。
- 新增 `task-classification.json` proof artifact，补齐 `classify_task` 的真实输出。

实现边界：

- 主链路已经由 contract state runner 驱动。
- Rework / Merging 内部细节仍在 DevFlow adapter 文件内，后续作为代码体积治理继续拆成独立 handler 文件。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestRunDevFlowContractState|TestWorkflowActionRoute|TestDevFlowReviewRounds|TestWorkflowContractRejectsUnsupportedActionType'
go test ./services/local-runtime/internal/omegalocal
```

## 2026-05-02 23:50 CST

### 通用 Action Executor 阶段 4 增强

旧做法：

- Rework 虽然已按 Review verdict 进入对应 stage，但内部仍由 Go 固定顺序执行 build feedback、coding、validation、push 和 PR 更新。
- Human Review approve 后会直接在 approval continuation 中执行 merge / handoff，contract 只记录 action route metadata。

新做法：

- Rework 内部拆为 contract action step：
  - `build_rework_checklist`
  - `apply_rework`
  - `validate_rework`
  - `update_pull_request`
- Human Review approve 后按 state action 执行：
  - `human_gate`
  - `refresh_pr_status`
  - `merge_pr`
  - `write_handoff`
- `runDevFlowContractState` 覆盖 Rework / Merging / Done 的真实副作用顺序；contract 改顺序时 runtime 会跟随，缺少 handler 仍直接失败。
- 新增 `docs/latest-architecture.md`，记录当前功能一、功能二、runtime、workflow contract、runner、GitHub/CI、飞书和可观测性的最新版架构。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal
```

## 2026-05-02 23:58 CST

### 通用 Action Executor 后续治理

旧做法：

- Rework 与审批后 Delivery 虽然已经由 `runDevFlowContractState` 调用，但 handler 执行体仍直接写在 `devflow_cycle.go` 和 `server.go`。
- 主流程文件同时负责上下文组装、执行副作用和状态持久化，后续继续扩展 contract action 时容易再次膨胀。

新做法：

- 新增 `devflow_rework_actions.go`，承载 Rework 的四个 action handler：
  - `build_rework_checklist`
  - `apply_rework`
  - `validate_rework`
  - `update_pull_request`
- 新增 `devflow_delivery_actions.go`，承载 Human Review approved 后的四个 action handler：
  - `human_gate`
  - `refresh_pr_status`
  - `merge_pr`
  - `write_handoff`
- `devflow_cycle.go` 只组装 Rework action handler 所需上下文，并把 action steps 交给 state runner。
- `server.go` 只保留 checkpoint continuation 的状态更新、proof 归档、attempt completion 和 workpad 刷新。

验证：

```bash
go test ./services/local-runtime/internal/omegalocal
```

## 2026-05-02 继续：Migration Runner 与 Mission Control 写入收口

### 可执行增量迁移

旧做法：

- `sqlite.go` 在 `Initialize` 中创建全量 schema。
- `omega_migrations` 只记录版本，初始化完成后直接写入所有 migration metadata。
- 少量字段补齐逻辑以 ad hoc 函数散落在初始化流程里。

新做法：

- 新增 `sqlite_migrations.go`，定义 `sqliteMigration{Version, Name, Up}`。
- `Initialize` 只先保证 `omega_migrations` 表存在，然后按顺序执行未应用 migration。
- 每个 migration 的 `Up` 成功后才写入 `omega_migrations`；失败不会落表，下一次启动可重试。
- `runtime_logs_query_extensions` 迁移继续负责旧表补 `requirement_id` 和查询索引。

验证补充：

- 新增 `sqlite_migrations_test.go` 覆盖首次执行、幂等重复执行、失败不记录、旧 runtime_logs 表升级。

### Mission Control 唯一写入者

旧做法：

- 前端在 local runtime 缺失时会直接修改 Work Item 状态、优先级、创建/删除本地 Work Item，或者把 Agent Profile 保存到浏览器 localStorage。
- 这些 fallback 方便早期 demo，但会造成 UI 状态和服务端真实状态分叉。

新做法：

- 新增 `apps/web/src/missionControlWrites.ts`，所有 canonical workspace 写入在前端先检查 local runtime。
- 创建 / 删除 / 运行 Work Item、状态 / 优先级修改、Agent Profile 保存都必须通过 Go local runtime。
- 前端继续保留 theme、Page Pilot preview URL 等 UI 偏好本地状态；业务状态不再由浏览器自行写入。

### 模块化治理

- `server_routes.go` 承接 `Handler` 和 HTTP 路由表，`server.go` 不再继续承载入口注册逻辑。
- 本轮前端先拆出 Mission Control 写入守卫；Workboard list、Inspector、GitHub workspace 仍是后续拆分项。

## 2026-05-02 继续：飞书绑定与 Agent 连通性测试

### 本轮目标

设置页里之前只有静态的 Feishu provider 状态，实际配置仍主要靠环境变量；Agent Studio 也只能看到本机 capability，不能在选择前确认当前 runner + model + 账号凭据是否真的可用。

### 变更内容

- 新增 `/feishu/config` 读写接口，把 chat/task/webhook、审核人、文档目录、webhook secret、review token 和 Task bridge 开关落到 SQLite。
- webhook secret / review token 复用本地 AES-GCM 加密通道，前端只展示 masked 状态，保存时不会把星号占位写回数据库。
- Workspace Settings 的 Provider Access 面板新增 Feishu 绑定表单和 Test connection。
- DevFlow Human Review 发送飞书审核请求时会优先消费页面保存的配置，再回退环境变量。
- JobSupervisor 的 Feishu Task bridge 开关开始消费页面配置，不再只能靠 `OMEGA_FEISHU_TASK_BRIDGE_ENABLED`。
- 新增 `/agent-runner/preflight`，按 Agent Profile 的 runner/model 做真实 CLI 探测；Trae/opencode 会同时检查加密账号凭据是否存在。
- Workspace Agent Studio 的 Agents tab 展示 tested / failed / missing，并提供单个 Agent 的 Test connection。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=15000
npm run lint
npm run build
```

## 2026-05-03 继续：DevFlow 状态自愈、详情自动刷新与飞书 current-user fallback

### 本轮目标

实测 `OMG-30` 暴露了三个问题：真实失败原因不够容易定位、Human Review 后状态可能被旧 JobSupervisor 快照覆盖、飞书测试消息成功但自动审核/失败通知没有发送。

### 变更内容

- 排查 `OMG-30` 两次尝试：第一次是目标仓库 `customer-health.html` trailing whitespace 被 `git diff --check` 拦截；第二次已进入 Human Review，但 stale JobSupervisor 快照误标 stalled。
- JobSupervisor 在标记 stalled 前增加 fresh database guard，避免旧快照覆盖后台 job 已写入的 `waiting-human`。
- Integrity scan 增加 pending Human Review checkpoint 自愈，可把这类误标 stalled 的 attempt 恢复为 `waiting-human`。
- Work Item 详情页对 active attempt 的 action plan / timeline 增加独立轮询，页面不再依赖手动刷新才能看到阶段变化。
- 飞书自动审核和失败通知新增 current-user fallback：没有保存 chat/task/webhook 时，使用当前 `lark-cli auth login` 用户作为直投目标。
- 失败通知在 failed/stalled 保存后自动尝试投递，并把投递结果写入 attempt / runtime log。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
```

## 2026-05-03 继续：Page Pilot Web fallback 与预览进程恢复

### 本轮目标

用户在 Electron 中遇到 `ERR_EMPTY_RESPONSE`，并指出 HTML file 模式会退回 Work items、页面像卡住。按要求启动 Web 版手测后，复现出三个串联问题：hash route 被 session 覆盖、旧 3009 Python 静态服务只返回 empty response、历史 repository target 缺少 id。

### 变更内容

- Page Pilot Web 版在没有 Electron bridge 时不再直接报错，而是通过 local runtime API 启动选定 Repository Workspace，并以内嵌 iframe + `PagePilotOverlay` 打开。
- Vite `/page-pilot-target` 默认代理改为 `http://127.0.0.1:3009`，Web fallback 会把 3009 预览映射为同源 `/page-pilot-target/...`。
- App session 恢复阶段改为调用 `initialActiveNav(session.activeNav)`，让 `#page-pilot` 和 Work Item detail hash 优先生效。
- Go Preview Runtime 启动前如果发现当前 preview URL health check 失败，会清理 cwd 等于目标 workspace 的陈旧监听进程，再启动新的 runtime。
- Page Pilot 对旧 session 中缺失 `repositoryTarget.id` 的 GitHub target 派生 `repo_owner_repo`，避免 UI 显示已选仓库但执行链路拿不到 id。

### 本机手测

- 启动 `OMEGA_PAGE_PILOT_TARGET_URL=http://127.0.0.1:3009 npm run dev -- --host 127.0.0.1 --port 5174`。
- 访问 `http://127.0.0.1:5174/#page-pilot`，等待 workspace 加载后仍停留在 Page Pilot。
- 点击 `Open page editor` 后，坏掉的旧 3009 进程被清理，新 `python3 -m http.server` 返回 `200 OK`，页面出现 iframe。
- 打开 `AI` 浮层后，`Select`、Apply/Confirm/Discard 状态和近期 runs 能正常显示。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/__tests__/desktopProcessSupervisor.test.ts --testTimeout=15000
npm run build -- --mode development
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotPreviewRuntimeStartPersistsGoProfile|TestPagePilot|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget|TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget'
```

## 2026-05-03 继续：Electron Page Pilot 目标页状态校验

### 本轮目标

截图显示 `python3 -m http.server` 的 404 错误页浮在 Omega 左上角，同时 Page Pilot 启动器仍显示 `Target page opened...`。这说明 Electron direct pilot 把 HTTP 错误页当成了成功目标页。

### 变更内容

- `omega-preview:open` 改为先创建未挂载 BrowserView，后台加载并校验目标 URL。
- 主 frame `did-fail-load` 直接返回错误；`did-navigate` 捕获的 HTTP status `>=400` 也返回错误。
- 只有目标页校验成功后才 `setBrowserView` 并布局，避免错误页短暂或长期覆盖 Omega 主界面。
- 启动时自动恢复 preview 也复用该校验，失败时关闭 preview view 并保留主窗口。

### 验证

```bash
node --check apps/desktop/src/main.cjs
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/__tests__/desktopProcessSupervisor.test.ts --testTimeout=15000
```

## 2026-05-04：主导航 hash route 与 session 恢复收敛

### 本轮目标

排查用户反馈的刷新、进入应用、Project 页面或 Work Item 详情页偶发跳回 Work items 问题，确认是否由页面加载完成后的自动切换引起。

### 变更内容

- 将 Projects、Views、Settings 纳入显式 hash route，并统一通过 `activeNavFromHash` / `primaryNavHash` 做主导航映射。
- workspace session 加载完成后不再盲目恢复保存的 `activeNav`；当前 URL hash 优先生效。
- 侧边栏导航、Project config、Provider access、Settings 的 Open work items、Page Pilot exit 等入口都会同步 hash，避免状态和 URL 分叉。
- Work Item 详情 hash 保持一等路由，刷新后继续进入详情而不是列表。

### 本机手测

- 浏览器访问 `http://127.0.0.1:5173/#projects`，等待 workspace 加载并刷新，页面仍为 Projects。
- 浏览器打开 `OMG-30` 详情 `#/work-items/item_manual_30` 后刷新，仍显示 Work item detail。
- 浏览器访问并刷新 `#page-pilot`，仍显示 Page Pilot。

### 验证

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=20000
npm run lint
git diff --check
```

## 2026-05-04 继续：New requirement 按钮交互态修正

### 本轮目标

修复 `New requirement` 在 hover / active 后出现黑色外圈的问题，让按钮状态和 light / dark UI 都保持一致。

### 变更内容

- 保留 `.topbar-create` 原来的透明容器 + `::before` conic 光边结构。
- hover / focus-visible / active 状态只显式覆盖 `background: transparent`、`border-color` 和 `outline`，避免透明边缘吃到全局 button hover 的深色背景。
- 保持原有 light / dark mode 的填充、光边和动效，不改变按钮整体观感。

### 验证

```bash
npm run lint
git diff --check
```

本机浏览器点击并 hover `New requirement`，未再出现黑色外圈。

## 2026-05-04 继续：Dark sidebar controls 与 Connections provider 收敛

### 本轮目标

修复 dark mode 下 Workspaces / Connections 折叠按钮过亮的问题，并澄清左侧 Connections 中 `Google on` 的误导性状态。

### 变更内容

- dark theme 的 `.sidebar-section summary::after` 改为深色按钮样式，hover / focus 只轻微提亮。
- 左侧 Connections 主列表隐藏 Google，仅展示当前产品真实使用的 GitHub / Feishu / CI。
- 保留 Google provider 定义以兼容旧 workspace session 和未来身份能力，但不再作为当前工作台连接状态展示。
- 更新 sidebar GitHub provider 测试定位，避免隐藏 Google 后 provider panel 默认落到 GitHub 时产生文本歧义。

### 验证

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=20000
npm run lint
git diff --check
```

本机浏览器 dark mode 验证：折叠按钮和 sidebar 色板协调，Connections 中不再出现 Google。

## 2026-05-04 继续：Requirement composer 创建闭环与 dark UI 收敛

### 本轮目标

修复用户反馈的 `New requirement` 表单 dark mode 可读性、右侧提示卡冗余、workspace 名称截断，以及点击 `Create` 后长时间停在 `Creating...` 的问题。

### 变更内容

- `createItem()` 在 API 创建成功后立即用返回 session 更新 Projects / Requirements / Work Items / Mission State，并清理创建表单；`refreshControlPlane()` 改为后台异步刷新，避免慢观测性接口阻塞用户操作闭环。
- 新增回归测试，模拟 `/observability` 永不返回，验证 requirement 创建仍会结束 `Creating...` 并显示新 work item。
- 移除 inline requirement composer 的右侧说明卡，避免和 workspace lock status 重复。
- dark mode 下提高 title / description label 可读性，并让 `New requirement` 保持原蓝青渐变填充。
- 收紧 sidebar workspace row / config button 间距，优先给仓库名留空间。

### 验证

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testNamePattern "finishes requirement creation|shows Feishu|keeps the hash-routed Projects" --testTimeout=30000
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=30000
npm run lint
git diff --check
```

本机浏览器验证：`http://127.0.0.1:5173/#workboard` dark mode 下打开 `New requirement`，workspace 名称可完整显示，表单标签可读，右侧说明卡消失，按钮保持原渐变样式。

## 2026-05-04 继续：Delivery flow 截断与 Feishu approve 路由收敛

### 本轮目标

修复 Work Item 详情页 `Delivery flow` 状态文字只露半截的问题，并解释 / 修复飞书 Human Review 卡片在个人机器人场景点击 `Approve` 报 `200340` 的问题。

### 变更内容

- `detail-stage-grid` 的状态 `<em>` 从标题内容列中拆出，改为卡片底部整行展示，保留 ellipsis 但不再裁切文字高度。
- `feishu/review-callback` 支持飞书 card action 的嵌套 payload：`event.action.value`、`event.operator.open_id`、`event.token`，并兼容 callback URL challenge。
- 新增显式开关 `OMEGA_FEISHU_CARD_CALLBACK_ENABLED=true` / `OMEGA_FEISHU_INTERACTIVE_CALLBACK_ENABLED=true`。未启用时卡片不再渲染 `Approve` / `Request changes` 交互按钮，避免用户点击后遇到飞书 `200340`。
- `lark-cli` 当前用户 fallback 在无卡片 callback 时优先创建绑定当前用户的飞书 Task，完成任务后通过 Task bridge 或 `/feishu/review-task/sync` 同步为 Omega Human Review approve。
- 如果 Task 创建失败，会退回发送无交互按钮的卡片通知，并记录 `taskReviewError`，避免审核通知完全丢失。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuAutoReviewFallsBackToCurrentLarkUser|TestFeishuReviewRequestUsesLarkCLIInteractiveCard|TestFeishuReviewCallbackApprovesCheckpointThroughSharedDecisionPath|TestFeishuReviewCallbackAcceptsCardActionPayload|TestFeishuReviewTaskSyncApprovesCompletedTask'
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=20000
```

本机浏览器验证：`http://127.0.0.1:5173/#/work-items/item_manual_30` 的 Delivery flow 状态行完整显示。

## 2026-05-04 继续：Human Review actionability 与 dark artifacts 可读性

### 本轮目标

修复 Human Review approve 后详情页仍展示可点击审核按钮的问题，并提升 Work Item 详情页 Artifacts proof 卡片在 dark mode 下的可读性。

### 变更内容

- Work Item 详情页不再使用 pipeline 下任意 pending checkpoint 作为 Human Review 操作入口；改为筛选当前 pipeline / current attempt 的 `human_review` checkpoint，并选择最新记录。
- 新增 `checkpointActionable` 判断：只有 checkpoint pending、attempt waiting-human/currentStage human_review、pipeline waiting-human 同时成立，才渲染 `Approve delivery` / `Request changes`。
- 非 actionable checkpoint 渲染只读决策摘要，例如 `Human review approved` 和 decision note，避免 approve 后继续出现可操作按钮。
- Run Workpad blocker 逻辑同步收紧：pending Human Review 不作为 blocker；rejected checkpoint 才作为需要处理的 blocker。
- JobSupervisor integrity reconcile 增加 completed Human Review decision 回填：当历史数据里 pipeline/attempt 已 done 且 run events 有 `gate.approved`，但 checkpoint 仍 pending 时，`/checkpoints` 会自动修正为 approved。
- dark mode 下补充 `.proof-card` / `.proof-card-copy` 的 label、meta、path 颜色，修复 Artifacts 区域文字过暗。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestListCheckpointsReconcilesCompletedHumanReviewDecision|TestApproveDevFlowCheckpointCanContinueDeliveryAsync|TestFeishuReviewTaskSyncApprovesCompletedTask|TestFeishuReviewCallbackAcceptsCardActionPayload'
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=20000
npm run lint
git diff --check
```

## 2026-05-04 继续：Work Item artifacts preview 入口

### 本轮目标

把 Work Item 详情页的 Human Review proof 和 Artifacts 从“路径展示”改成真实可查看的证据入口。

### 变更内容

- `WorkItemDetailPage` 接入 `fetchProofPreview`，并传给 Current Attempt / Artifacts 区域。
- `Changed` / `Validation` proof 条目改为可点击 artifact button，直接打开 proof preview modal。
- Artifacts grid 改成整卡点击预览，卡片主标题显示短文件名，stage/kind/path 摘要保留为辅助信息。
- 新增 proof preview modal：展示完整 source path、preview content、不可用错误和 truncated 提示。
- 新增回归测试：点击 artifact card 会调用 `/proof-records/:id/preview` 对应 fetch，并展示预览内容。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=20000
npm run lint
git diff --check
```

## 2026-05-04 继续：Artifact preview runtime 路由修复

### 本轮目标

修复 artifact preview modal 把前端 HTML 当 JSON 解析的问题，并保证 stage evidence 点击时使用真实 proof record id。

### 变更内容

- 非 dev Web 默认 Mission Control API 改为 `http://127.0.0.1:3888`，覆盖 Electron/static 场景下缺少 `VITE_MISSION_CONTROL_API_URL` 的 preview 请求。
- `omegaControlApiClient` 对成功状态但非 JSON 的响应给出清晰错误，方便识别请求是否落到 SPA fallback。
- `proofCardsForWorkItem` 会按 `sourcePath` / `sourceUrl` 匹配 proof record；stage evidence 与 proof record 指向同一文件时，保留真实 proof id，避免用文件路径调用 preview endpoint。
- Go runtime 的 proof preview endpoint 增加本地路径兜底：历史 run 中只有 evidence path、没有 proof record id 的 artifact 也能打开真实文件内容。
- 增加回归测试覆盖非 JSON 响应和 stage evidence/proof record 去重后的 preview id。

### 验证

```bash
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=20000
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=20000
go test ./services/local-runtime/internal/omegalocal -run TestRepositoryAuditTablesAndProofPreview
npm run lint
git diff --check
```

## 2026-05-04 继续：Artifact preview 符号输出过滤

### 本轮目标

修复 `rollback-plan` 这类 workflow output slot 被误当成 proof record 打开后报 404 的问题。

### 变更内容

- Work Item artifact 聚合区分真实 proof evidence 和 workflow 符号输出。
- 只有匹配 proof record、URL、本地路径或带扩展名的文件引用才进入 preview card。
- `rollback-plan` 等纯符号 output artifact 不再显示为可点击 preview，避免把流程契约字段误导成真实证据文件。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=20000
npm run lint
git diff --check
```

## 2026-05-04 继续：Feishu reviewer controls 视觉适配

### 本轮目标

修复 Feishu Provider Access 中 reviewer 搜索按钮样式不适配的问题。

### 变更内容

- Reviewer input 独占一行，避免和操作按钮互相挤压。
- `Search` / `Use current user` 使用同宽局部按钮样式。
- 补齐 light / dark mode 的按钮背景、边框、文字和交互态。

### 验证

```bash
npm run lint
git diff --check
```

## 2026-05-04 继续：Workboard Human Review / Blocked 分组收敛

### 本轮目标

让 Workboard 中需要注意的 Human Review 和 Blocked 队列更容易被看到。

### 变更内容

- 新增 `Human Review` 展示状态，pipeline `waiting-human` 会进入独立分组。
- 调整 Workboard 状态顺序，`Blocked` 放到 `Done` 前面。
- `Blocked` 改为黄色/琥珀色状态点和 pill，避免和 Done / Ready 的灰色弱提示混淆。
- `Human Review` 使用独立紫色状态点和 pill。
- 补充 workboard 分组顺序回归测试。

### 验证

```bash
npm run test -- apps/web/src/core/__tests__/workboard.test.ts apps/web/src/core/__tests__/workItemProjection.test.ts --testTimeout=20000
npm run lint
git diff --check
```

## 2026-05-04 继续：Workboard Page Pilot item 展示收敛

### 本轮目标

降低 Workboard 列表元信息噪音，并让 Page Pilot 物化 Work Item 更容易识别。

### 变更内容

- 隐藏 `item_manual_*`、`page-pilot:item_*` 等内部 id 元信息。
- 新增 `page_pilot` Work Item source，Page Pilot item 使用 `Page Pilot #N` 标签。
- Page Pilot pipeline 的 `Delivery` 阶段在 Workboard 进度中显示为 `Confirm / PR`。
- 保持 Page Pilot item 在 Workboard 主列表中展示，作为统一追踪入口；Page Pilot 专页继续承担运行启动和 recent runs 管理。

### 验证

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testNamePattern "marks not-started work clearly" --testTimeout=30000
npm run lint
git diff --check
```

## 2026-05-04 继续：内部 Work Item 引用展示过滤

### 本轮目标

让 `item_manual_*` 等内部关联 id 只留在数据层，不再出现在 Workboard 详情、项目摘要和 Inspector 的普通展示文案中。

### 变更内容

- Work Item 详情页过滤内部 `sourceExternalRef` 和 requirement external ref。
- Workboard 列表元信息移除 requirement id 展示，避免 `Req item_manual_*` 这类内部关联占用行空间。
- Project workspace 的 Work items 摘要只显示真实外部引用或语义化 Work Item key。
- Workboard Inspector 的 Requirement block 同步过滤内部 requirement ref。
- 补充详情页回归测试，覆盖 `item_manual_*` / `req_item_manual_*` 不出现在可见文本里。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=30000
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testNamePattern "marks not-started work clearly" --testTimeout=30000
```

## 2026-05-04 继续：侧边栏 Agent 本地绑定状态

### 本轮目标

把 Agent runner / model / profile / account 的本机级绑定状态露出到全局侧边栏，避免用户误以为这些能力只属于当前 workspace。

### 变更内容

- 左侧 sidebar 新增 `Agents` 区块，展示当前本机 runner、model、profile readiness 和账号绑定摘要。
- Agent 状态点击后进入 Settings / Workspace Agent Studio；后续每个 stage 的 runner 分配仍保留在 workspace 级 Agent Studio 中处理。
- Agent 状态沿用 light / dark sidebar 样式，避免新增独立浮层或把本地配置混入 Workspaces 卡片。

### 验证

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testNamePattern "renders Go control-plane observability" --testTimeout=30000
npm run lint
git diff --check
```

## 2026-05-04 继续：Work Item 详情页信息架构收敛

### 本轮目标

降低 Work Item 详情页重复信息和跨功能入口噪音，保持 DevFlow 与 Page Pilot 的主路径边界清楚。

### 变更内容

- `Open in Page Pilot` 仅在 Page Pilot 物化出来的 Work Item 详情页显示，普通 Requirement / DevFlow item 不再展示该入口。
- Delivery Flow / Current Attempt 去重：Current Attempt 中的 stage/action plan 细节改为默认折叠，保留 Run Timeline、Human Review 和 PR 状态作为主要信息。
- Run Workpad 增加 Collapse / Expand 控制，便于审核时收起执行摘要。
- `Agent pending` fallback 改为按 stage 显示可理解的角色标签；`Ship` 文案改为 `Delivery`。
- 详情页深链刷新时，如果 workspace 还在加载，先展示 detail loading state，不再短暂回落到 Work items 列表。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=30000
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testNamePattern "marks not-started work clearly" --testTimeout=30000
```

## 2026-05-04 继续：DevFlow 单一活动阶段修复

### 本轮目标

修复 Work Item 详情页中 Delivery flow 同时显示多个活动阶段的问题，让 UI 只展示 pipeline 的 canonical runtime snapshot。

### 变更内容

- Go runtime 在 workspace load/normalize 时对 `devflow-pr` pipeline 做 stage canonicalization。
- canonical current stage 优先来自 pending checkpoint，其次来自 latest attempt，再回退到 run stages 中的 active status。
- 当前阶段前的线性阶段修正为 passed，当前阶段后的 stale active status 修正为 waiting；未执行的可选 `rework` 保持 waiting。
- `pipelineStatusFromRun` 增加 failed / blocked / stalled 映射，避免 failed stage 被误算成 running。
- Work Item 详情页 Delivery flow 改为优先使用 `pipeline.run.stages`，避免把 action plan states 当成主流程状态。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run TestNormalizeDevFlowPipelineStageStatusesKeepsSingleActiveStage -count=1
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testNamePattern "canonical pipeline snapshot" --testTimeout=30000
npm run lint
git diff --check
```

## 2026-05-04 继续：Feishu Human Review 无公网回调默认路由

### 本轮目标

避免没有公网 Card Request URL 时，飞书仍发送看起来可点击但无法回写 Omega 的卡片按钮。

### 变更内容

- 只有同时配置 `OMEGA_FEISHU_CARD_CALLBACK_ENABLED=true` 和 `OMEGA_PUBLIC_API_URL` 时，Feishu review card 才包含 `Approve` / `Request changes` 回调按钮。
- 没有公网回调时，如果可解析当前 `lark-cli` 用户，会优先创建 Feishu Task 审核；否则 chat 路由发送文本 fallback，不再发送不可回调的交互按钮。
- 文档和测试同步区分“公网卡片回调”和“本机 Task 审核”两条路径。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuReviewRequestUsesLarkCLITextWhenCardCallbackUnavailable|TestFeishuAutoReviewFallsBackToCurrentLarkUser|TestFeishuReviewRequestCreatesTaskReviewWithStrongBinding|TestFeishuReviewRequestSendsInteractiveWebhookCard' -count=1
```

## 2026-05-04 继续：执行状态读取性能收敛

### 本轮目标

降低 Workboard、Work Item detail 和 live execution polling 对 SQLite snapshot 与大体积 operation/proof 历史的压力，避免用户刷新、切换页面或打开详情时被全量执行历史拖慢。

### 变更内容

- 新增 SQLite 表级读取快路径：pipelines / attempts / checkpoints / operations / proof records 可按查询参数直接读取规范化表。
- 表接口支持 `limit`、`status`、`pipelineId`、`workItemId` 等过滤；未过滤 `/operations` 保留完整 snapshot 行为，兼容 runnerProcess 历史详情。
- 前端控制面刷新改成首屏限量拉取 recent operations / proof records 与 pending checkpoints。
- Work Item 详情页按当前 pipeline/work item 补拉 scoped execution records；轮询路径不再反复拉完整 artifacts。
- API client 对带 query 的新接口保留 404 fallback，兼容测试 mock 或旧 runtime。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestListOperationsSupportsFilteredFastPath|TestRunOperationPersistsRunnerProcessResult|Test.*Checkpoint|Test.*Pipeline|Test.*Operation' -count=1
go test ./services/local-runtime/internal/omegalocal -count=1
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=30000
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=30000
npm run lint
```

## 2026-05-04 继续：runtime log 噪音治理

### 本轮目标

解释并修复 `runtime_logs` 在少量 Work Item 下膨胀到 65 万条的问题，避免访问日志和周期轮询日志淹没真实执行事件。

### 变更内容

- `api.request` 持久化策略改为默认只记录失败请求和 mutating 请求；成功 GET / HEAD / OPTIONS 不再进入 `runtime_logs`。
- 增加 `OMEGA_RUNTIME_LOG_DEBUG_API=true` 开发开关，用于需要排查 API 访问链路时临时恢复成功 GET 记录。
- 被数据库跳过的成功读请求写入 `.omega/logs/omega-runtime-diagnostics.YYYY-MM-DD.jsonl`；`/health` 不写库也不写文件。
- JobSupervisor interval tick 只在产生操作者可见变化时写数据库；无变化 tick 写入 daily diagnostic file。
- 远端 PR check poll 只在 failed / missing required checks 时写数据库 `job_supervisor.remote_signals.blocked`；正常 poll 写入 daily diagnostic file。
- diagnostic file 写入时清理 24 小时以前的旧 `omega-runtime-diagnostics.*.jsonl`。
- SQLite migration 启动时 compact 历史噪音日志，保留最近少量 tail 供排查。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestAPIRequestRuntimeLogPolicySkipsSuccessfulReads|TestDiagnosticRuntimeLogsUseDailyFilesAndOneDayRetention|TestSQLiteCompactsNoisyRuntimeLogs|TestRuntimeLogsAPIListsAndFiltersRecords|TestSQLiteMigrations|TestMigrationsAndPipelineTemplates' -count=1
```

## 2026-05-04 继续：Human Review approve 幂等与 stale snapshot 防护

### 本轮目标

修复 OMG-32 approve 后仍显示 Approve、重复点击触发多次 merge/outbound sync，以及 JobSupervisor 旧 snapshot 覆盖人工审核结果的问题。

### 变更内容

- Checkpoint decision 加服务端互斥，避免同一进程内重复 approve 并发推进同一个 gate。
- 已终态 checkpoint 再次收到同类 decision 时直接返回现有结果，不重复执行 delivery continuation。
- 正常 checkpoint decision 跳过完整 integrity recovery，只有 checkpoint 缺失时才尝试恢复扫描。
- Workspace 保存时去重同 id checkpoint，终态优先于 pending，防止旧 duplicate pending 覆盖 approved。
- `appendOrReplace` 删除后续 duplicate id，避免 upsert 逻辑继续制造同 id 多条记录。
- JobSupervisor 保存前检查 workspace snapshot 是否已被其他链路更新；如果已变化，本轮 tick 跳过保存并写 diagnostic file，避免轮询覆盖人工审核或 delivery 状态。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestCheckpointDecisionDeduplicatesDuplicateCheckpointIDs|TestFeishuReviewCallbackApprovesCheckpointThroughSharedDecisionPath|TestFeishuReviewTaskBridgeApprovesCheckpoint' -count=1
```

## 2026-05-04 继续：Workspace session 轻量化与 auto-run settling 复核

### 本轮目标

拆掉 UI live polling 对完整 `/workspace` snapshot 的依赖，降低 Electron / Web 打开后对 Go runtime 的 CPU 和内存压力；同时复核此前 `DevFlow auto-run settling` 测试失败是否代表状态机混乱。

### 变更内容

- Web `fetchWorkspaceSession` 改为读取 `/workspace?scope=session`，会话恢复只消费项目、需求、Work Item、连接、UI preference 等产品状态。
- Go runtime `GET /workspace?scope=session` 改为从 SQLite 规范化表组装 session read model，不再反序列化完整 `workspace_snapshots.database_json` 后裁剪。
- `work_items` 增加 `record_json`，保存完整 Work Item 记录；session 视图可保留 `source` / `repositoryTargetId` / Page Pilot 元信息，同时继续只返回 UI 会话所需表。
- `/run-workpads` 列表读取切换到 SQLite `run_workpads` 规范化表，避免通过 snapshot fallback 拉取整库。
- Human Review approve 继续保持正常路径快速；只有 legacy checkpoint 缺失 attempt 时才触发 integrity recovery，恢复旧数据可审批能力。
- 复跑 `TestOrchestratorTickCanClaimAndRunDevFlowCycle` 单次和 5 次计数均通过；此前失败判定为异步 job settling 在高负载下被放大的超时，而不是稳定的多 active stage 状态机错误。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -count=1
curl -fsS -o /tmp/omega-workspace-session.json -w 'session HTTP=%{http_code} BYTES=%{size_download} TOTAL=%{time_total}\n' 'http://127.0.0.1:3888/workspace?scope=session'
curl -fsS -o /tmp/omega-workspace-full.json -w 'full HTTP=%{http_code} BYTES=%{size_download} TOTAL=%{time_total}\n' 'http://127.0.0.1:3888/workspace'
```

本地实测 session 约 464KB / 0.26s，full workspace 约 14.7MB / 0.45s；session scope 测试会破坏 full snapshot JSON，防止后续回退到完整镜像反序列化。
