# Omega Bug Log

本文记录开发过程中遇到并修复的实现问题。产品功能记录继续写入 `docs/feature-implementation-log.md`；这里专门保留 bug、原因、修复和验证。

## 2026-05-04: DevFlow Delivery flow 同时显示多个活动阶段

### 现象

Work Item 处于 Human Review 时，详情页 Delivery flow 同时把 Implementation、Human Review、Done 等多个 stage 画成活动状态，顶部状态、Run Workpad 和阶段卡片互相矛盾。

### 原因

- 前端 Delivery flow 优先消费 `attemptActionPlan.states`。该字段表示 workflow contract / action plan 的状态集合，不是 pipeline 的唯一运行快照；当历史或可转移状态带有 runtime status 时，会被误当成主流程状态展示。
- 后端 DevFlow 恢复和异步推进逻辑会标记当前 stage，但没有统一清理同一 pipeline 中旧的 `running` / `needs-human` 活动态。历史异常恢复后，run snapshot 可能残留多个 active stage。
- `pipelineStatusFromRun` 只识别 `needs-human` 和 all-passed，没有把 failed / blocked / stalled stage 映射回 pipeline failed 状态，放大了状态不一致风险。

### 修复

- 后端读取 workspace 时对 `devflow-pr` pipeline 做 stage canonicalization：按 pending checkpoint / latest attempt / active stage 推导单一当前阶段，当前阶段之前的线性阶段归并为 passed，之后的活动残留归为 waiting；可选 `rework` 未执行时继续保持 waiting。
- 同步归一化 latest attempt 自身保存的 `stages`，避免 `/attempts` 返回旧 snapshot 后又被前端或 Run Workpad 误用。
- 后端写入链路收敛为 `pipeline.run.stages` 是当前运行态权威：`markDevFlowStageProgress` 在进入新的 current stage 时清理同一 pipeline 内其他活动态，防止恢复链路、agent invocation 增量写入和 human review checkpoint 产生漂移。
- 归一化只处理仍在推进中的 DevFlow pipeline；`done` / `failed` / `blocked` / `stalled` / `canceled` / `terminated` 等终态不再被 stage 推导覆盖，避免取消、超时、已完成审批在读取时被误写回 running。
- `pipelineStatusFromRun` 增加 failed / blocked / stalled 映射。
- 前端 Work Item 详情页 Delivery flow 只展示 backend canonical `pipeline.run.stages`，只有没有 pipeline snapshot 时才回退到 action plan states；前端不再根据 checkpoint/currentStageId 自行重算主流程状态。
- 补充 Go 和 React 回归测试，覆盖 stale `running` + stale future `needs-human` 同时存在时只显示 Human Review 一个活动阶段。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestNormalizeDevFlowPipelineStageStatusesKeepsSingleActiveStage|TestMarkDevFlowStageProgressCanonicalizesCurrentStage' -count=1
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testNamePattern "canonical backend pipeline snapshot" --testTimeout=30000
npm run lint
git diff --check
```

## 2026-05-04: Projects 页面 light/dark 按钮与右侧概要区不协调

### 现象

Projects 页面在 light mode 下仍有 `Project config`、`Create project`、`Create workspace` 等按钮落回黑色默认样式；右侧三张统计卡片又高又窄，和页面主体不协调。项目阶段胶囊 `Requirements / Repository Workspace / Agent Pipeline / Human Review / Delivery` 在该页信息重复，占用空间。

### 修复

- 为 `Project config` 和 disabled primary action 增加基础态、light mode、dark mode 样式，避免按钮落回全局黑色。
- 将项目统计从右侧竖向卡片改成标题区下方的紧凑指标胶囊，减少视觉重量。
- 移除 Projects 页面阶段胶囊条，保留 Work Item / Pipeline 详情页中的真实流程状态展示。
- 同步检查 light / dark mode 视觉，确保 Projects 表单、按钮、统计指标都跟随主题。

### 验证

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=20000
npm run lint
```

## 2026-05-04: New Requirement 描述区 tab 分隔线和 Preview 可读性问题

### 现象

New Requirement 展开后，Write / Preview tab 中间出现一条很硬的深色竖线，在 light mode 下像输入光标一样突兀。切到 Preview 后，预览区域与输入框层级接近，正文和 Markdown 内容不够清晰。

### 修复

- 将描述区 tab 改成紧凑 segmented control，去掉深色 `border-right` 分隔线。
- textarea 与 preview 改为独立圆角内容区，不再依赖 tab 下边框拼接。
- 为 light / dark mode 分别补充 description preview 的背景、正文、标题和空状态颜色，提升 Markdown 预览可读性。
- New Requirement 主按钮覆盖浏览器默认黑色 focus outline，hover / focus / active 状态改用蓝青同色系光晕。

### 验证

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=20000
npm run lint
```

## 2026-05-03: Page Pilot 中点击 Feishu 连接无响应且状态误显 off

### 现象

飞书已经通过本机 `lark-cli` 当前用户 fallback 可用于 Human Review 通知，但左侧 Connections 仍显示 Feishu 灰色 `off`。在 Page Pilot 页面点击 Feishu 行时，看起来没有任何反应，用户会误以为绑定没有生效或侧边栏丢失。

### 原因

- Connections 只消费 session 中持久化的 `connections` 状态，旧 session 里的 Feishu `off` 会覆盖运行时重新读取到的 `lark-cli` ready / reviewer route ready 状态。
- Page Pilot 模式为了给预览区让空间隐藏了右侧 inspector rail，但连接行点击仍然只尝试在 inspector 里打开 Provider 面板，所以在 Page Pilot 页面上表现为“点了没用”。
- 测试中的部分 `/feishu/config` mock 只返回局部字段，状态判断直接 `.trim()` 会放大成运行时脆弱点。

### 修复

- 新增 Feishu effective connection 判断：只要 `lark-cli` 当前用户 fallback、chat、task assignee、tasklist 或 webhook 任一路由 ready，侧边栏就显示 Feishu `on`。
- Provider 面板显示更具体的 Feishu route 说明，例如 `current-user fallback via lark-cli`，区分“连接可用”和“具体投递目标类型”。
- 在 Page Pilot 中点击 Connections provider 行会切到 Settings 并打开对应 Provider access 面板，避免被 Page Pilot 隐藏的 inspector 吃掉交互。
- Feishu route 判断改为 nil-safe，兼容局部 config 返回。

### 验证

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=20000
npm run lint
```

## 2026-05-03: Page Pilot Repository source 没有启动目标项目 dev server

### 现象

最近更新后，用户在 Page Pilot 选择 Repository Workspace，保持默认 `Repository source` 后点击 `Open page editor`，Electron 无法正常打开目标页面，表现为连接失败、空白页或停在错误的预览地址。

### 原因

`Repository source` 链路只调用 Electron 的 `resolvePreviewTarget` 准备隔离 worktree。该接口即使没有显式预览地址，也会返回默认 `http://127.0.0.1:5173/`。前端看到 `previewUrl` 后直接调用 `openPreview`，但目标项目的 dev server 并没有启动；真实使用时这会打开一个不存在或不属于目标仓库的地址。

这个问题没有被原测试覆盖，因为测试只覆盖了静态 `index.html` 直开和显式 `Dev server by Agent` 模式，没有覆盖“Repository source + package.json 项目”的默认路径。

### 修复

- `resolveRepositoryPreviewTarget` 不再凭空返回默认 preview URL；只有显式配置 `OMEGA_PREVIEW_URL` / `OMEGA_PAGE_PILOT_URL` 时才返回预览地址。
- Page Pilot 默认 `Repository source` 现在按仓库真实形态分流：
  - 有 `package.json` 时优先自动调用 Preview Runtime Agent 启动 dev server，再把返回的真实 URL 交给 Electron direct pilot。Vite / React 等项目即使根目录也有 `index.html`，也不会再被当成静态文件打开。
  - 没有 `package.json`、但有根目录 `index.html` 时，才直接打开 `file://` 静态预览。
  - 两者都没有时停留在启动器并展示明确状态，不再打开假地址。
- `Dev server by Agent` 输入框现在识别完整 URL：`http://127.0.0.1:3009/dashboard` 会把 `http://127.0.0.1:3009/` 作为 Preview Runtime health / launch URL，把 `/dashboard` 作为打开路径，不再把完整 URL 误当成普通 intent。
- 如果显式 preview URL 已经可访问，Electron Preview Runtime 会按 `external-url` 直接接入，不再先 clone / prepare GitHub isolated worktree，避免用户已有 dev server 时仍卡在 starting。
- URL path / query / hash 会按浏览器 URL 语义合成，例如 `/dashboard?tab=customers#health` 不会被错误编码进 pathname。
- `HTML file` 模式恢复 workspace-first 行为：输入框为空时会从当前选中的 Repository Workspace 自动解析根目录 `index.html`；只有找不到时才要求用户手动输入路径。
- `HTML file` 模式不再把上一次残留的 `http://127.0.0.1:3009/` 当作 HTML 文件路径；遇到 HTTP URL 会忽略该输入并从当前 Repository Workspace 自动寻找根目录 `index.html`。
- Page Pilot 打开流程增加前端 busy state 和超时保护，避免 Electron IPC 或 Preview Runtime 长时间无响应时按钮和状态一直卡在 starting。
- Electron `openPreview` 在 `loadURL` 失败后会关闭刚创建的 BrowserView，避免失败页面覆盖 Omega UI、看起来退回 Work items 且卡住。
- 补充前端和 Electron process supervisor 单测，覆盖 package.json 默认路径和“不伪造 previewUrl”的边界。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/__tests__/desktopProcessSupervisor.test.ts --testTimeout=15000
node --check apps/desktop/src/main.cjs && node --check apps/desktop/src/pilot-preload.cjs && node --check apps/desktop/src/omega-preload.cjs && node --check apps/desktop/src/process-supervisor.cjs
```

## 2026-05-02: Work Item 详情页误把等待审核显示成返工/阻塞信号

### 现象

Work Item 进入 Human Review 后，详情页 Delivery flow 下方出现 `Feedback route: rework`，Run Workpad 中 `Rework checklist`、`Blockers`、`Retry reason` 等卡片也被染成黄色。用户看到后会误以为当前已经进入返工或失败重试，但实际状态只是等待人工审核。

同一轮 Human Review 没有发送飞书消息，checkpoint 的 `feishuReview` 为空。

### 原因

- 前端只要发现 workflow contract 中存在 `rework` 阶段，就展示 `Feedback route`。这把“系统具备返工路线”误显示成“当前已经有反馈要返工”。
- Run Workpad 会直接消费历史 `reworkChecklist` / `retryReason` / pending checkpoint，导致历史捕获或等待人工审核被当成当前阻塞。
- 后端进入 Human Review 后，如果没有 chat、task、webhook 目标，会在自动发送前直接 `feishu.review.skipped`，没有把 `needs-configuration` 写回 checkpoint。`lark-cli` ready 只代表本机凭证可用，不代表已经知道要发给哪个群或任务负责人。

### 修复

- `Feedback route` 只在真实存在 request changes、rework assessment、retry available、failed/stalled/canceled attempt 时展示。
- Run Workpad 不再把 pending Human Review checkpoint 当作 blocker；历史 rework checklist 只有在当前确实需要返工或重试时才展示为黄色。
- 自动飞书审核发送取消静默跳过：缺少投递目标时会把 `needs-configuration` 结果写入 checkpoint，并记录 `feishu.review.needs_target`，方便页面和日志解释原因。
- 修复 `Action plan` 摘要缺 CSS 导致文字挤在一起的问题。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuAutoReviewRecordsNeedsConfigurationWhenNoTarget|TestFeishuReviewRequestSendsInteractiveWebhookCard|TestFeishuReviewRequestUsesLarkCLIInteractiveCard|TestFeishuConfigPreflightUsesLocalLarkCLIProfile'
```

## 2026-04-30: Page Pilot 集成路径偏离已验证 direct pilot 体验

### 现象

Page Pilot 入口增加 repo 选择和 preview source 后，打开目标页面时进入了 Omega 中转式 BrowserView：目标页只出现简化工具条，圈选结果回传到 Omega 页面继续填写指令。这和此前已验证的 Electron direct pilot 不一致，旧版目标页内的多批注、整体说明、Apply、Confirm、Discard 和结果面板没有作为主交互出现。

### 原因

为了快速接入 Repository Workspace 选择、隔离 preview workspace 和 preview URL 解析，曾把 Page Pilot 拆成“Omega 页面配置 + 目标页简化圈选 + 回传 Omega overlay”的两段式流程。该流程复用了部分能力，但本质上重写了用户操作路径，破坏了功能二“在真实目标页面内完成选择和修改”的设计。

### 修复

- `omega-preview:open` 改回加载 `pilot-preload.cjs`，沿用已验证的 direct pilot 目标页内交互。
- Page Pilot React 页面收敛为启动器，只负责选择 repo、选择预览来源和打开 direct pilot。
- Electron 打开目标页时通过 additional arguments 注入 `projectId`、`repositoryTargetId`、`repositoryLabel`，旧 direct pilot 不再依赖固定 env/default repo。
- 目标页内新增 `返回` 按钮，可关闭 BrowserView 回到 Omega 页面。
- 保留隔离 preview workspace 解析能力，不再保留简化 `preview-preload.cjs` 作为主路径。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
```

## 2026-04-30: Page Pilot apply 复用旧 runtime 导致 workspace 解析失败

### 现象

Direct pilot 进入目标页面后可以正常圈选和提交批注，但 apply 失败并显示：

```text
Page Pilot needs a local repository workspace for HMR; bind a local repository target or run the local runtime from the matching GitHub worktree
```

同时目标 GitHub repo 的隔离 preview workspace 已经存在。

### 原因

Electron 桌面壳会优先复用 `http://127.0.0.1:3888/health` 上已有的 Go local runtime。前面已经把 Go runtime 的 Page Pilot repo 解析改成支持 Omega 管理的隔离 preview workspace，但本机端口上仍运行着旧进程，导致 apply 仍走旧错误分支。

### 修复

- 重启 Go local runtime 和 Electron 桌面壳，让当前 `page_pilot.go` 生效。
- Direct pilot 的错误状态栏新增 `Reload` / `New` 操作；遇到后端修复或 repo 重新准备后，可以在目标页面里直接刷新或重新开始一轮选择，不再像卡死。
- Direct pilot 的 `runtimeUrl` 改为由 Electron main process 从已启动/复用的 Go local runtime service 注入，不再只依赖 `pilot-preload.cjs` 默认值或 env。
- Direct pilot 的 localStorage 状态按 `repositoryTargetId + target URL` 分作用域，避免旧目标页或旧 repo 的失败状态污染新一轮 Page Pilot。

### 验证

```bash
curl http://127.0.0.1:3888/health
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget'
node --check apps/desktop/src/pilot-preload.cjs
node --check apps/desktop/src/main.cjs
```

## 2026-04-30: Page Pilot delivered 后仍显示确认/撤销动作

### 现象

Direct pilot 点击 `Confirm` 并完成 delivery 后，顶部标题已经显示 `Page Pilot Delivered`，但右侧仍显示 `Confirm` / `Discard` 按钮，看起来仍可继续确认或撤销。

### 原因

旧版结果栏无论 run status 是 `applied`、`delivered` 还是 `discarded` 都渲染同一组动作，只是用 `disabled` 尝试阻止非 applied 状态。但样式没有明显 disabled 视觉，导致 delivered 后仍像可点击状态。

### 修复

- 只有 `status = applied` 时渲染 `Confirm` / `Discard`。
- `delivered` / `discarded` 状态只保留 `Reload` / `New`。
- 为状态栏按钮补充 disabled 视觉兜底。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-30: Page Pilot 入口不明显且缺少目标 repo 确认

### 现象

Electron 启动后默认展示首页，功能二入口只存在于 Workboard 左侧导航中的 `Page Pilot`。用户无法从首页直接发现功能二；进入 Page Pilot 后也没有显式 Repository Workspace 选择步骤，容易把功能二理解成固定项目演示。

后续验证中还发现：Page Pilot 默认 immersive 样式会隐藏 App chrome，并让 preview 区域占满窗口；在没有可用 preview URL 或 Electron HMR 没刷新到最新代码时，页面只剩右下角 AI 浮球，看起来像 Electron 白屏。Electron 窗口也没有浏览器地址栏式刷新按钮，用户难以主动刷新。

### 原因

功能二底层已有 Electron preview / apply / deliver 能力，但产品入口仍停留在调试阶段：导航项存在，缺少首页 CTA、深链和从 Work Item 详情进入的上下文入口。

### 修复

- 首页增加 `打开 Page Pilot` / `启动 Page Pilot`。
- 支持 `#page-pilot` 深链。
- Page Pilot 页面新增 Repository Workspace 选择器。
- Work Item 详情页增加 `Open in Page Pilot`，自动携带当前 item 的 `repositoryTargetId`。
- 取消 Page Pilot 默认全屏沉浸式入口，保留左侧导航和普通页面布局，避免没有 preview 时出现空白页。
- Electron 增加 `omega-app:reload` IPC 和 `window.omegaDesktop.reloadApp()`，Page Pilot 页面提供 `Reload app` 按钮。
- `Open preview` 在 Electron 中改为调用 BrowserView bridge，避免只更新 iframe 导致跨端口页面空白或无法 inspect。
- Preview source 增加 Dev server URL、HTML file、Omega proxy，避免把所有目标页面都假设成“已有端口服务”。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
```

## 2026-04-29: Orchestrator auto-run test cleaned TempDir before background job fully released

### 现象

全量 Go runtime 测试偶发失败：

```text
TestOrchestratorTickCanClaimAndRunDevFlowCycle
TempDir RemoveAll cleanup: directory not empty
```

失败时还可能看到 runtime log 写入失败，因为测试已经进入 cleanup，但后台 auto-run goroutine 仍在释放 execution lock 或写最后的日志。

### 原因

测试只等待 Attempt 从 `running` 变为非 running，就允许 `t.TempDir()` cleanup 执行。但 DevFlow background job 在更新 Attempt 后还有尾声动作：写 runtime log、release execution lock、保存 lock 状态。这个窗口里 cleanup 会删除 sqlite / workspace 临时目录，造成目录清理和后台写入竞争。

### 修复

- 测试中为 auto-run 增加 fake Codex runner，避免本机真实 CLI 造成不可控执行时间。
- 等待 Attempt settle 后，继续等待 execution lock 变为 `released`，确认 background job 已完成尾声动作，再允许测试结束。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestOrchestratorTickCanClaimAndRunDevFlowCycle|TestJobSupervisorTickMarksStalledRunningAttempt'
```

## 2026-04-29: Human Review approve failed on legacy pipeline without attempt

### 现象

Workboard 中进入 Human Review 后，Approve 操作返回 API failed。后端真实错误为：

```text
devflow approval cannot continue: attempt not found
```

### 原因

该 Work Item 已有 pipeline、checkpoint、operation 和 proof 记录，但缺少对应 attempt 记录。Approve 后端原先强依赖 attempt 提供 workspace、branch 和 PR 信息，因此遇到旧状态不一致时直接 500。

同时前端只显示泛化 API failed，缺少后端 `error` 详情，排障成本高。

### 修复

- Human Review 入口在列表和详情页都可见，缺少 attempt 时仍展示 checkpoint 审批卡片。
- checkpoint approve 兼容缺少 attempt 或 attempt 缺少交付信息的旧数据：记录审批事件后不再 500。
- DevFlow 完成 / 失败 / agent invocation 持久化路径会 backfill 缺失 attempt。
- 前端 API client 会展示后端 JSON `error` 详情。
- 新增 runtime log 基础版，把缺失 attempt 等状态不一致写入 ERROR 记录。
- 后续补强：pending checkpoint 写入并修复 `attemptId`，`POST /job-supervisor/tick` 可以扫描 Human Review gate 并 backfill 可审计 attempt，避免人工审批入口和真实执行记录断链。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestDevFlowCheckpointApprovalToleratesMissingAttempt|TestCompleteDevFlowCycleBackfillsMissingAttempt|TestRuntimeLogsAPIListsAndFiltersRecords'
npm run lint
```

## 2026-04-28: Page Pilot pure SPA preview could not satisfy target-project selection

### 现象

功能二最初在 React SPA 中用 iframe 承载目标项目 preview，并把 Overlay 绑定到 iframe document。这个方案只能在 same-origin iframe 中工作；真实用户项目通常跑在另一个 localhost origin，浏览器同源策略会阻止 Omega 读取 DOM、注入圈选脚本和采集 selector/context。

更重要的是，Page Pilot 的产品语义不是圈选 Omega 自己的管理 UI，而是圈选用户正在构建的软件页面。纯 SPA iframe 只能验证管线，不能稳定满足赛题要求。

### 原因

- 浏览器同源策略限制跨 origin iframe DOM 访问。
- 用户项目 preview、Omega SPA、Go runtime 通常会运行在不同端口。
- 赛题需要“内部浏览器”式能力：打开目标项目页面、注入选择逻辑、刷新预览。

### 修复 / 架构选择

启用 Electron 作为桌面壳，但不替代现有 React SPA 和 Go runtime：

```text
Electron
  -> BrowserWindow loads React SPA
  -> BrowserView loads target project preview
  -> preview preload injects Page Pilot selection bridge
  -> Go runtime keeps SQLite / runner / git / PR execution
```

开发模式不需要打包：

```bash
npm run local-runtime:dev
npm run web:dev -- --host 127.0.0.1 --port 5174
npm run desktop:dev
```

### 验证

本次新增 Electron dev shell 和 preload bridge，并继续通过：

```bash
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx
go test ./services/local-runtime/...
npm run build
```

## 2026-04-28: Electron install exposed dependency and Node stdio typing issues

### 现象

安装 Electron dev shell 依赖后：

- `npm audit` 报 Electron 38 存在 high severity advisory。
- `npm run lint` 报旧本地 Node 脚本中的 `child.stdout` / `child.stderr` / `child.stdin` 可能为 `null`。

### 原因

- 初始 Electron 版本落在 npm audit 标记的 vulnerable range。
- Electron 带来的 Node 类型解析让 `ChildProcess` stdio nullable 类型更严格地进入当前 TypeScript build。

### 修复

- 升级 Electron 到 `^41.3.0`，`npm audit` 归零。
- 在本地 runner / sqlite / gh CLI helper 和相关测试中显式检查 stdio streams，不再假设 spawn 总是返回 pipe。

### 验证

```bash
npm audit --json
npm run lint
```

## 2026-04-28: Page Pilot overlay blocked element selection

### 现象

浏览器模式下打开 Page Pilot 后，完整浮窗会遮挡目标 preview，进入 Select 后仍然挡住用户要圈选的区域。hover 解析也不像检查器一样随鼠标持续更新。Apply / Confirm / Discard 的禁用条件虽然正确，但 UI 没解释启用条件，容易被误解为按钮不可用。

### 原因

- Select 模式复用了完整对话面板，没有切换成轻量 inspector control。
- hover 只监听 `pointerover`，对 iframe 内部连续移动反馈不够及时。
- 可选元素匹配范围偏窄，主要覆盖 Omega 自身组件类名，没有把目标项目常见 `.card` / `.hero` / `data-omega-source` 作为优先候选。
- 按钮禁用状态缺少 inline hint。

### 修复

- Select 模式下把浮窗移动到左下并压缩成 compact inspector，只显示 Cancel、Clear、当前 hover 元素类型、文本和 source mapping。
- 增加 `pointermove` 监听，实时更新当前鼠标指向元素和高亮框。
- 选择候选优先匹配 `[data-omega-source]`，并覆盖按钮、标题、正文、`.card`、`.hero`、article 等目标项目常见结构。
- Apply / Confirm / Discard 增加 title 和 inline 状态提示：Apply 需要 selection + instruction，Confirm / Discard 需要先 Apply 成功。

### 验证

```bash
npm run lint
```

## 2026-04-28: Page Pilot selected Omega chrome instead of target product

### 现象

用户在 Page Pilot 中打开目标产品后，页面显示 `Needs preview bridge`，但 Select 仍然允许启动。结果 hover 高亮看起来落在目标产品区域，解析出来的文本却是 Omega Page Pilot 自己的说明文案，体验不像之前的页面检查器。

### 原因

- Overlay 在 `targetDocument` 不可用时回退到了父页面 `document`，导致圈选对象变成 Omega chrome。
- 本地调试时用户可能用 `localhost:5174` 打开 Omega，却在 preview URL 中填 `127.0.0.1:5174/page-pilot-target/`。浏览器把这两个 host 当成不同 origin，iframe 不可 inspect。
- Page Pilot 仍嵌在 Workboard shell 中，左侧导航、顶栏和右侧 rail 抢占空间，没有形成“在 Omega 内直接打开用户产品”的工作区。

### 修复

- Select 模式只允许绑定到目标 preview document；如果 target iframe 不可 inspect，立即阻止选择，不再回退检查 Omega 页面。
- 对 `/page-pilot-target` dev proxy URL 做本地 host 归一：同端口的 `localhost` / `127.0.0.1` 会转成相对路径，保持 iframe 与 Omega 同源。
- Page Pilot nav 进入 immersive preview mode：隐藏 Workboard sidebar、topbar、inspector rail，让目标产品 iframe 占满 Omega 主工作区；只保留浮层 URL 控制和 Page Pilot overlay。

### 验证

```bash
npm run lint
```

## 2026-04-28: Chrome fallback could not match the Page Pilot interaction model

### 现象

用户期望的功能二体验是：Omega 像浏览器一样直接打开用户产品页面，例如 `http://127.0.0.1:5173/`，页面右下角有一个悬浮手指按钮；点击后进入选择模式，hover 时高亮当前元素，点击元素后在页面上弹出修改输入框。此前 Chrome / iframe fallback 虽能验证 API，但体验不像真正的页面内 Agent。

### 原因

- Chrome 普通网页不能让 Omega 稳定向任意目标 origin 注入控制层。
- iframe fallback 需要同源代理，容易把用户注意力放在 Workboard chrome 和 proxy URL 上。
- 赛题核心更接近“内置浏览器 + preload 注入”，Electron 天然能提供这个边界。

### 修复 / 架构选择

- 新增 `npm run desktop:pilot`。
- Electron 主窗口直接加载目标产品 URL，默认 `http://127.0.0.1:5173/`。
- `pilot-preload.cjs` 注入悬浮手指、hover 高亮、元素 tooltip、修改输入框，并直接调用 Go runtime 的 Page Pilot API。
- Chrome / iframe 路径保留为开发 fallback，不再作为主体验判断标准。

### 验证

```bash
node --check apps/desktop/src/pilot-main.cjs
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot exposed source mapping internals to users

### 现象

Electron Page Pilot tooltip 显示 `No data-omega-source`，让用户误以为没有 source mapping 的元素不能选择。输入 comment 后也会直接触发 apply，不适合同时选择多个元素并批量描述修改。

### 原因

- 第一版把 `data-omega-source` 当成用户可见状态，而不是内部强源码映射。
- Composer 的发送按钮直接调用 `/page-pilot/apply`，没有批注队列。
- 选择候选偏向按钮、标题、卡片，普通 DOM 元素虽然能被浏览器看到，但没有进入 Page Pilot 批注体验。

### 修复

- Tooltip 改为显示源码映射或 `DOM context captured`，不再向用户暴露 `No data-omega-source`。
- Electron direct pilot 增加批注队列：发送 comment 只添加批注和编号 pin，不立刻提交。
- 增加底部悬浮输入框，显示批注 chip 和整体补充说明输入区；用户点击发送后才统一调用 runtime。
- 选择候选扩展到 label、input、textarea、select、link 以及普通布局 DOM，未识别类型标为 `other`。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot batch annotations lacked a persistent prompt box

### 现象

Electron direct pilot 可以在元素上添加批注 pin，但批注完成后页面只显示“批注中 / 批注数量”状态，没有保留一个可继续输入整体需求的悬浮输入框。用户想一次圈选多个元素，再像聊天输入框一样补充整体修改意图，最后统一提交给 Agent。

### 原因

- 前一版把“单个元素 comment composer”和“批注队列状态”拆成两个阶段，但队列阶段只展示状态，没有继续输入框。
- 这会让用户误以为添加 comment 后已经进入提交流程，也无法对多个选区写统一需求。

### 修复

- 批注发送后只加入本地 annotation queue，并在选中元素附近插入编号 pin。
- 页面底部保留悬浮输入框，展示最近批注 chip、批注数量和整体补充说明 textarea。
- 用户可以继续点击右下角手指选择更多元素；只有点击底部输入框的发送按钮时，才把批注集合和整体说明提交给 `/page-pilot/apply`。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Full Go runtime test suite has watcher TempDir cleanup flake

### 现象

在验证 Page Pilot 单一 Agent 模式时，局部测试通过：

```bash
go test ./services/local-runtime/internal/omegalocal -run TestPagePilot
```

但两次运行全量 runtime 测试时，失败点出现在 watcher/orchestrator 相关测试的临时目录清理：

```text
TempDir RemoveAll cleanup: unlinkat ... directory not empty
```

两次失败分别落在：

- `TestOrchestratorTickCanClaimAndRunDevFlowCycle`
- `TestOrchestratorWatcherPersistsAndScansReadyIssues`

### 判断

这不是 Page Pilot 断言失败，而是 watcher/background process 或 goroutine 在测试结束时仍可能持有或写入临时目录，导致 Go test 的 `t.TempDir()` cleanup 失败。

### 后续修复方向

- 给 orchestrator watcher 测试增加显式 stop/cleanup。
- 确保 watcher goroutine 和外部 fake command 进程在测试返回前完全退出。
- 将 watcher 测试与长跑 DevFlow cycle 的临时 workspace 生命周期隔离。

## 2026-04-28: Page Pilot selector helper TypeScript inference failure

### 现象

新增 `PagePilotOverlay` 后，`npm run lint` 报错：

```text
PagePilotOverlay.tsx: parent implicitly has type any
PagePilotOverlay.tsx: child is of type unknown
```

### 原因

`selectorFor` 中遍历 DOM ancestor 时，`cursor` 会在循环内变化；TypeScript 对 `cursor.parentElement` 和 `Array.from(parent.children)` 的类型收窄不稳定，导致 `parent` 被推断成隐式 `any`，children 被推断为 `unknown`。

### 修复

- 显式声明 `parentElement: Element | null`。
- 保存 `currentTagName`，避免 filter callback 捕获变化中的 `cursor`。
- 给 `filter` callback 参数标注 `Element`。

### 验证

```bash
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx
```

## 2026-04-28: Page Pilot discard route parsed the wrong id

### 现象

新增 `POST /page-pilot/runs/{id}/discard` 后，Go 测试返回：

```text
Page Pilot run runs not found
```

### 原因

已有 `pathID` helper 只适用于 `/resource/{id}` 形态，会固定返回第三段路径。`/page-pilot/runs/{id}/discard` 的第三段是 `runs`，导致 runtime 用错误 id 查询记录。

### 修复

为 Page Pilot discard route 使用专用解析：

```text
strings.TrimSuffix(strings.TrimPrefix(path, "/page-pilot/runs/"), "/discard")
```

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run TestPagePilot
```

## 2026-04-28: Page Pilot nav was missing from persisted Workboard session type

### 现象

把 Page Pilot 做成 Workboard 内独立 nav 后，`npm run lint` 报错：

```text
Type '"Page Pilot"' is not assignable to type 'PrimaryNavPersistence'
```

### 原因

`App.tsx` 的 `PrimaryNav` 已经加入 `Page Pilot`，但 `workspacePersistence.ts` 中用于 local/session 持久化的 `PrimaryNavPersistence` 仍只允许 `Projects | Views | Issues`。

### 修复

将 `PrimaryNavPersistence` 扩展为：

```text
Projects | Views | Issues | Page Pilot
```

### 验证

```bash
npm run lint
```

## 2026-04-28: Page Pilot multi-annotation apply used the wrong primary target

### 现象

Electron direct pilot 中同时选择多条批注后，用户把第三条 `login-submit` 登录按钮标注为“改成红蓝绿渐变”，但实际改到了页面顶部的两个 `.brand-mark` 图案。

### 原因

提交批注时，preload 代码使用第一条带 `sourceMapping.file` 的批注作为 `/page-pilot/apply` 的 `selection`：

```text
annotations.find(...)
```

多选场景下，用户最新选中的元素通常才是当前主目标。旧逻辑把第一条 `login-title` 当作主目标传给 Agent，后端 prompt 又强调优先使用 source mapping，导致 Agent 在错误源码区域附近落点。

### 修复

- Electron direct pilot 改为选择“最新一条带源码映射的批注”作为 primary target；如果没有源码映射，则回退到最新批注，让 Agent 走 DOM context / selector。
- 提交给 Agent 的 instruction 明确写入“主目标是第 N 条批注”。
- Go runtime 的 Page Pilot prompt 增加约束：`Selected element context` 是 primary target，多批注只作为辅助上下文。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
go test ./services/local-runtime/internal/omegalocal -run TestPagePilot
```

## 2026-04-28: Page Pilot submit kept stale annotation controls visible

### 现象

用户点击提交后，底部仍显示上一轮批注 chip 和输入框，看不到 Agent 正在做什么，也容易误以为旧批注还处于可编辑待提交状态。

### 原因

Electron direct pilot 只有 toast 状态提示，`/page-pilot/apply` 请求期间仍保持批注编辑 tray；apply 成功后虽然会 reload，但网络/runner 执行期间没有持久的过程信息面板。

### 修复

- 提交后立即把编辑 tray 切换为 Page Pilot process panel。
- process panel 展示本次提交的批注、primary target、Agent 提交流程、已修改文件、功能一 Work Item / Pipeline linkage 和预览刷新步骤。
- 批注历史默认折叠，只显示上一条；点击 `^` 展开全部，避免挡住目标页面。
- 成功、失败、Confirm、Discard 后都保留过程事件，用户能看到上一轮发生了什么。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot could miss small link/button targets

### 现象

Electron direct pilot 中，用户想圈选登录页里的 `忘记密码？`、`立即注册` 或某个具体按钮时，hover/点击有时命中父级行容器、卡片或其它带源码映射的祖先元素，而不是鼠标下的小型链接/按钮。

### 原因

旧逻辑基于 `event.target.closest(...)` 从当前事件目标向祖先查找候选元素。这个方式只看 DOM ancestor，不看鼠标坐标下的完整堆叠元素；当用户点在链接周边空白、文字行高区域、内部 span，或父级容器先命中时，小链接/按钮会被更大的祖先元素吞掉。`kindFor` 也没有把 `a`、`input`、`label` 独立分类，导致 tooltip 更像泛化的 `other` / `card-copy`。

### 修复

- 圈选候选从 `closest(...)` 改为 `document.elementsFromPoint(x, y)`。
- 按元素类型打分：`button/a/input/textarea/select/[role=button]` 优先，其次 `label`、`data-omega-source`、标题、文本、卡片、普通容器。
- `kindFor` 增加 `link`、`field`、`label` 类型。
- 分类不再作为可选门槛；所有可见 DOM 元素都可以被选中，未知类型保留为 `other` 并继续采集上下文。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot could miss dynamic status messages

### 现象

用户点击登录后，页面动态写入 `这是静态登录页，暂未接入 API。` 状态提示。该提示可见，但进入 Page Pilot 选择模式后不容易被高亮选中。

### 原因

这类动态提示通常是空 `div` 后续通过 JS 写入 `textContent`，没有 `data-omega-source`，也不是按钮、链接、标题或卡片。旧候选排序会把它当成普通 `div`，优先级低，容易被周围 form/card/container 抢走。

这不是因为 Page Pilot 缓存了旧 DOM。Electron preload 在每次 hover/click 时读取当前 live DOM；日夜模式切换、表单校验提示、hash 路由切换后的元素都应该按最新页面状态读取。问题在于动态状态元素没有被识别成高价值候选。

### 修复

- `role="status"`、`aria-live`、`.message`、`.alert`、`toast/notice/error/success` 类元素提升为高优先级候选。
- `elementKind` 增加 `status` 类型。
- 每次进入选择模式先清除旧 highlight，避免用户看到上一次 selection 的残影。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot composer badge always showed 1

### 现象

添加第二条批注时，底部 composer 左侧 badge 仍显示 `1`，导致用户无法确认当前输入的是第几条批注。

### 原因

`showComposer` 中 badge 文案写死为 `1`，没有读取当前批注队列长度。

### 修复

- 新批注 badge 使用 `annotations.length + 1`。
- 编辑已有批注时 badge 使用原批注 id。
- 页面 pin 和底部 chip 支持点击编辑 comment，不改变原顺序。
- 执行过程面板增加 loading spinner，批注折叠按钮改为明确文字。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-29: Requirement create could duplicate Work Items during slow submit

### 现象

在 Workboard 里创建 Requirement 时，如果本地 runtime 响应稍慢，用户会看到 Create 后短时间没有明显反馈。连续点击后，列表里会出现多条同名 Work Item，而且部分条目的展示编号也一样，例如多个 `Work item 21`。

### 原因

前端只依赖 React state 更新禁用按钮，不能挡住同一轮事件里的快速重复点击。后端虽然会给重复的 client `id` 加后缀，但没有同步保证 `key` 唯一，导致 UI 展示编号重复。

### 修复

- Create 入口增加同步 `useRef` in-flight 锁，请求未结束前直接忽略后续提交。
- Create 按钮进入 `Creating...` 状态，并给用户显示创建中的过程信息。
- Go local runtime 在写入 Work Item 时同时保证 `id` 和 `key` 唯一；重复 `OMG-N` 会自动分配下一个可用编号。
- 新增回归测试：快速点击 Create 两次只会向 `/work-items` 发送一次请求。

### 验证

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx -t "creates app requirements" --testTimeout=15000
go test ./services/local-runtime/internal/omegalocal -run 'TestCreateWorkItemAllocatesUniqueIDForDuplicateClientID|TestPagePilot'
```

## 2026-04-29 14:53 CST: Run Timeline handler passed pointer to value builder

### 现象

新增 Run Timeline 后，目标 Go 测试第一次编译失败：

```text
cannot use database (variable of type *WorkspaceDatabase) as WorkspaceDatabase value
```

### 原因

`Repo.Load` 返回 `*WorkspaceDatabase`，而 timeline 聚合函数接收不可变值类型 `WorkspaceDatabase`，handler 调用时少了显式解引用。

### 修复

- `attemptTimeline` 调用 `buildAttemptTimelineItems` 时传入 `*database`。
- 保持聚合函数只读数据库快照，避免在构造时间线时误改运行状态。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestAttemptTimelineAggregatesRunRecords|TestCancelAttemptAPIUpdatesStateAndSignalsRunningJob|TestRuntimeLogsAPIListsAndFiltersRecords'
```

## 2026-04-29 14:56 CST: Work Item detail crashed on malformed timeline response

### 现象

全量前端测试中，部分旧 mock 对新增 `/attempts/{id}/timeline` 路径返回空对象，Work Item detail 渲染时报错：

```text
Cannot read properties of null (reading 'items')
```

### 原因

详情页只对 `activeAttemptTimeline` 做了可选链保护，但直接读取 `activeAttemptTimeline.attempt.id`。当测试或旧服务返回非完整 timeline payload 时，`attempt` 缺失会导致渲染崩溃。

### 修复

- 渲染 `WorkItemAttemptPanel` 时要求 `activeAttemptTimeline` 和 `activeDetailAttempt.id` 同时存在，再读取 timeline items。
- `timelineItems` 兜底为空数组，保证缺失或旧版本响应不会影响 Work Item 详情主路径。

### 验证

已执行：

```bash
npm run test -- apps/web/src/core/__tests__/manualWorkItem.test.ts apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
```

## 2026-04-29: Attempt retry index counted the new retry attempt

### 现象

新增 Attempt retry 基础版时，针对性测试发现第一次 retry 的 `retryIndex` 被写成 `2`。

### 原因

实现先调用 `beginDevFlowAttempt` 把新 Attempt 追加进数据库，再调用 `nextRetryIndex` 统计已有 retry，因此把刚创建的新 Attempt 也算进去了。

### 修复

- 在追加新 Attempt 前先计算 `retryIndex`。
- 新 Attempt 仍写入 `retryOfAttemptId`、`retryRootAttemptId`、`retryReason`，旧 Attempt 仍写入 `retryAttemptId`。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPrepareDevFlowAttemptRetryLinksAttempts|TestCancelAttemptAPIUpdatesStateAndSignalsRunningJob|TestAttemptTimelineAggregatesRunRecords'
```

## 2026-04-29: PR lifecycle effect read GitHub-only fields before type narrowing

### 现象

PR lifecycle 卡片接入时，`npm run lint` 报错：`RepositoryTarget` 可能是 local target，不能直接读取 `owner` / `repo`。

### 原因

effect 参数对象里用三元表达式读取 GitHub-only 字段，TypeScript 没有稳定缩窄到 GitHub target。

### 修复

- 先派生 `activeDetailGitHubRepositoryTarget`。
- `/github/pr-status` 参数只从该 GitHub target 读取 owner / repo；local target 或无 target 时只传 PR URL。

### 验证

```bash
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx -t "shows review packet details" --testTimeout=15000
```

## 2026-04-29: Run report markdown template used raw string backticks

### 现象

新增 `attempt-run-report.md` 生成器时，Go 格式化/编译失败，提示 `missing ',' in argument list`。

### 原因

报告模板使用 Go raw string literal，同时 Markdown 内容里包含反引号。反引号提前结束了 raw string。

### 修复

- 报告模板去掉 inline backtick 包裹，保留普通 Markdown 文本。
- 代码块由 helper 使用普通字符串拼接生成。

### 验证

```bash
gofmt -w services/local-runtime/internal/omegalocal/devflow_report.go
go test ./services/local-runtime/internal/omegalocal -run 'TestJobSupervisorTickReportsRunnableReadyWork|TestRunDevFlowPRCycleCreatesBranchPRAndMergeProof|TestPrepareDevFlowAttemptRetryLinksAttempts'
```

## 2026-04-29: Human Review Approve 把 PR 合并后的分支清理失败当成交付失败

### 现象

用户在 Human Review 详情页点击 Approve delivery 后，页面提示 `Omega control API failed: /checkpoints/.../approve`。Run Timeline 里同时出现 `github.pr.merge_failed` 和 `checkpoint.approve.failed`，错误内容显示 `gh pr merge ... --delete-branch` 已经 fast-forward 更新了目标仓库，但随后因为本地分支 `omega/OMG-22-devflow` 不存在，`failed to delete local branch` 导致命令退出 1。

### 原因

Approve 链路把“合并 PR”和“清理本地/远端分支”绑定成同一个 `gh pr merge --delete-branch` 命令。实际执行目录是 Attempt 隔离 workspace，对应目标 Repository Workspace 的 checkout；这个 workspace 不一定持有与 PR head 同名的本地分支。PR 合并已经完成时，分支清理失败不应该反向把 Human Review 判成失败。

### 修复

- Approve 合并 PR 时不再使用 `--delete-branch`。
- `gh pr merge` 失败后会查询 PR state；如果 PR 已经是 `MERGED`，按成功处理并记录 `github.pr.merge_already_done`。
- 本地分支删除和远端分支删除拆成 best-effort cleanup，失败只写 DEBUG runtime log，不阻断 checkpoint approve。
- Timeline 默认只返回最近事件，前端只渲染最近事件，降低详情页在大量 runtime event 下的渲染开销。
- 前端详情时间统一按北京时间展示。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestApproveDevFlowCheckpointIgnoresBranchCleanupFailure|TestRunDevFlowPRCycleCreatesBranchPRAndMergeProof|TestAttemptTimelineAPIAggregatesRunEvidence'
go test ./services/local-runtime/...
npm run test -- apps/web/src/core/__tests__/manualWorkItem.test.ts apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run lint
npm run build
```

## 2026-04-29: Review UI 与 Page Pilot 状态不一致

### 现象

- Human Review 详情页里的 Ω / H 头像文字偏移，评论输入区左侧头像与输入框视觉对不齐。
- 已 discard 或仍在等待确认的 Page Pilot Work Item 会出现在 Done 分组，但卡片内部 pipeline 又显示 `waiting-human`，列表状态与真实交付状态矛盾。

### 原因

- 旧的 `.human-gate-card span` 选择器范围过宽，影响了新 Human Review thread 中的头像和文本样式。
- Page Pilot 的 Work Item 列表分组只看 `workItem.status`，没有优先使用 Page Pilot pipeline 的事实状态；discard 后父页面也没有主动刷新 Workboard。

### 修复

- 移除 Human Review thread 里的装饰头像，保留标题、PR、Changed、Validation、Artifacts、评论和审批按钮。
- Workboard 对 Page Pilot item 增加运行时状态投影：`waiting-human` 显示为 Running，`discarded/failed` 显示为 Blocked，`delivered/done` 显示为 Done。
- Page Pilot discard API 返回后主动刷新控制面数据，避免 UI 保留旧分组。
- 移除 Retry context 卡片；失败主因改由 Attempt 的 `failureReason` / `failureStageId` / `failureAgentId` / `failureReviewFeedback` 事实字段驱动，并在失败报告和 Run Timeline 中展示。
- runner stderr 保留为执行证据，不做环境日志过滤；当 review agent 判断不通过或 rework 失败时，Retry 请求会携带 review feedback，避免只把 stderr 当成 retry 原因。
- 失败报告 light 主题改为浅底深字，确保 stderr、review feedback 和失败主因都可读。
- Page Pilot discard 后端测试补充 Work Item = Blocked、Pipeline = discarded 的断言。

### 验证

```bash
npm run lint
npm run test -- apps/web/src/core/__tests__/manualWorkItem.test.ts apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run build
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget'
go test ./services/local-runtime/...
```

## 2026-04-29: Human Review Approve 反馈延迟过长

### 现象

用户在 Human Review 里点击 Approve delivery 后，需要等待数秒才看到页面反馈。运行日志显示 `/checkpoints/.../approve` 曾耗时 8 秒以上；同一轮前端刷新里，`/github/status`、`/operations`、`/attempts`、`/requirements` 等接口也会一起请求，其中 GitHub status 可能耗时 3 秒以上，operations 返回体可达到数 MB。

### 原因

旧做法：Approve API 在同一个请求内同步完成 checkpoint 保存、PR merge、branch cleanup、proof 更新和数据库写回；前端成功后又调用完整控制面刷新，重新拉取设置、capability、GitHub 状态、operation、runtime log 等所有数据。这个设计保证了返回后状态完整，但把交付重活和 UI 刷新重活都压在一次点击反馈路径上。

### 修复

- 前端 Approve 增加 `asyncDelivery` 参数，Human Review 点击后只要求后端先保存人工审批决定。
- 后端对可继续交付的 DevFlow checkpoint 支持异步交付：同步请求内标记 Human Review passed、Merging running，并立即返回；PR merge、proof 更新和最终状态写入放入后台 goroutine 继续执行。
- 前端新增轻量执行态刷新，只拉取 workspace session、pipelines、attempts、checkpoints；Approve、Request changes 和运行中轮询不再触发完整控制面刷新。
- 保留旧同步路径：没有传 `asyncDelivery` 或 checkpoint 不满足 DevFlow 交付条件时，仍按原同步逻辑执行，避免破坏已有测试和外部调用语义。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestApproveDevFlowCheckpointCanContinueDeliveryAsync|TestApproveDevFlowCheckpointIgnoresBranchCleanupFailure'
npm run lint
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run build
```

## 2026-04-29: Human Review Request Changes 后没有继续执行

### 现象

用户在 Human Review 中输入修改意见并点击 Request changes 后，Run Timeline 只出现 `checkpoint.rejected` / `gate.rejected` 事件，页面没有明显进入新的执行状态，也没有新的 Agent 依据人工反馈继续修改。

### 原因

旧链路把 Request changes 当成 checkpoint 决策保存：只更新 checkpoint 和 pipeline stage 状态，没有创建新的 Attempt、没有 workspace lock、没有后台 job，也没有把人工反馈写入下一轮 Agent prompt。也就是说，状态记录是真的，但 rework 执行链路缺失。

### 修复

- DevFlow Human Review Request changes 会创建新的 `human-request-changes` Attempt。
- 旧 Attempt 标记为 `changes-requested`，并保存人工反馈和 retry link。
- 新 Attempt 记录 `humanChangeRequest` / `retryReason`，并进入同一 repository workspace 的后台 job。
- Pipeline event 和 Run Workpad 同步记录人工反馈；DevFlow prompt 会注入最新人工反馈，确保 Agent 不是凭空重跑。
- 前端 Request changes 后做轻量刷新和延迟刷新，让用户更快看到 queued / running 变化。

### 当前边界

当前修复先保证链路真实生效，会启动新的 DevFlow Attempt；后续需要继续把人工修改收敛为 Rework-only 续跑，减少重复的前置阶段。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback'
go test ./services/local-runtime/internal/omegalocal
npm run lint
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx apps/web/src/components/__tests__/WorkItemDetailPanels.test.tsx apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run build
```

## 2026-04-30: Page Pilot Open preview 看起来没有反应

### 现象

用户在 Page Pilot 选择 repo 后点击 Open preview，页面下方仍为空白；输入 `127.0.0.1:3009` 或 `http://127.0.0.1:3009/` 时也不容易判断按钮是否真的打到 Electron 后台。

### 原因

- Electron 主进程的 `omega-preview:open` 没有捕获 `loadURL` 失败，目标端口未启动时 renderer 侧只看到像“无反应”的状态。
- 前端 URL 规范化把无协议的 `127.0.0.1:3009` 当成相对路径，导致实际打开 `http://localhost:3000/127.0.0.1:3009`。
- 曾尝试按仓库名从用户本机常见目录推断 worktree，这和 Repository Workspace 必须明确绑定的设计不一致。

### 修复

- Electron `omega-preview:open` 增加成功/失败日志，并把失败原因返回给 UI。
- Page Pilot 前端会把无协议的 localhost / 127.0.0.1 地址规范成 `http://...`。
- 移除默认目录猜测：local target 只使用显式 path；GitHub target 走 Omega 管理的隔离 preview workspace。
- Go runtime apply 同步只认显式 local target 或隔离 preview workspace，避免预览和写入目录不一致。

### 验证

```bash
npm run test -- apps/web/src/__tests__/desktopProcessSupervisor.test.ts apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget'
```

## 2026-04-30: Page Pilot 注入控件遮挡目标页面观察

### 现象

在目标项目页面里打开 Page Pilot 后，返回控制台按钮固定占用左上角；顶部/底部控制条点击隐藏后仍保留一小段可见残边，会干扰用户检查自己的页面布局。

### 原因

旧注入层把返回入口作为常驻按钮渲染，并通过 `translate(... - 8px)` 的方式保留状态条边缘用于唤回。这个设计方便发现入口，但不符合 Page Pilot 需要尽量不影响目标页面观察的原则。

### 修复

- 返回入口改为左侧透明边缘热区，默认不显示文字和按钮底色；鼠标移入或键盘聚焦时才展开为“返回”按钮。
- 状态条收起后整体移出视野，不再露出 8px 残边。
- 状态条通过透明热区唤回，保留可恢复性，同时避免视觉遮挡。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
git diff --check
```

## 2026-04-30: GitHub delivery preflight 扩展后测试假 `gh` 合约不完整

### 现象

新增 GitHub delivery contract preflight 后，局部测试通过，但运行完整 Go runtime 测试时，DevFlow PR 周期用例在 preflight 阶段失败。

### 原因

测试中的 fake `gh` 只覆盖了旧链路需要的 `pr create`、`pr checks`、`pr merge` 等命令。新 preflight 会先调用 `gh auth status`、`gh repo view --json nameWithOwner,viewerPermission,defaultBranchRef` 和 `gh pr list --json number`，fake `gh` 对这些命令返回了不符合 JSON 合约的内容，导致运行前检查误判失败。

### 修复

- 扩展测试 fake `gh`，补齐 auth、repo view、pr list 的最小可信返回。
- 新增 `TestGitHubDeliveryContractPreflightChecksPermissions`，专门验证权限不足时会在 preflight 阶段阻止运行。
- 完整回归 `go test ./services/local-runtime/internal/omegalocal`，确保新 preflight 不破坏既有 DevFlow PR 周期。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal
```

## 2026-05-01: Page Pilot 启动器重复入口和实现文案干扰用户

### 现象

Page Pilot 页面顶部同时出现 `Workboard`、`Reload app` 和 `Open direct pilot`，但左侧导航已经能回到 Workboard，下面的预览来源表单也有打开入口。预览来源卡片还展示“沿用旧版目标页内操作体验”等实现说明，用户测试时会看到与当前任务无关的信息。

### 原因

旧版启动器保留了开发期调试入口和内部迁移说明，没有按最终用户任务重新收敛页面信息层级，导致同一操作出现两次，并暴露实现细节。

### 修复

- 移除顶部 direct pilot launcher 区块。
- 移除 `Workboard` / `Reload app` / 顶部 `Open direct pilot` 重复按钮。
- 预览来源卡片只保留 source selector、输入框和一个打开按钮。
- 删除“沿用旧版目标页内操作体验”等内部说明和多余状态 chip；只有真实启动或错误状态才显示反馈。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
npm run lint
```

## 2026-05-01: Page Pilot 打开按钮暴露内部叫法

### 现象

Page Pilot 启动器按钮使用 `Open direct pilot`，状态文案也出现 `Direct Page Pilot`。这个词来自开发期区分不同预览实现的内部命名，用户不需要理解，也容易被误认为是过时功能。

### 修复

- 按钮文案改为“打开页面编辑”。
- Electron 缺失和打开中的状态文案改为“页面编辑需要 Electron 桌面壳”“正在打开目标页面...”。
- 功能测试清单中同步使用“页面编辑模式”。

### 后续修正

Page Pilot 启动器页面整体是英文 UI，因此按钮和状态文案最终统一为英文：

- `Open page editor`
- `Page editing requires the Electron desktop shell.`
- `Opening target page...`

相关 placeholder、仓库说明和成功状态也统一成英文，避免同一页面中英混杂。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
npm run lint
```

## 2026-05-01: Page Pilot 结果栏 Reload 与自动刷新重复

### 现象

目标页内 Page Pilot Apply 后，顶部结果栏同时显示 `Confirm`、`Discard`、`Reload`、`New`。由于 apply / discard / refresh 已经接入 Preview Runtime reload supervisor，常驻 `Reload` 容易让用户误以为需要手动刷新。

### 修复

- 保留 `refreshLivePreviewFromButton`、`omega-preview:reload` 和相关事件绑定逻辑。
- 给结果栏中的 Reload 按钮增加专用 class，并通过样式隐藏。
- `New`、`Confirm`、`Discard` 仍然可见。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-05-03: OMG-30 DevFlow 两次失败与 Human Review 状态漂移

### 现象

测试 Work Item `OMG-30` 时连续失败两次。第一次进入 failed，第二次已经创建 PR 并进入 Human Review 相关日志，但页面最后显示为 stalled / blocked。Work Item 详情页的 Delivery flow 也不会自动更新，需要手动刷新页面才能看到最新阶段。

### 原因

- 第一次失败是真实仓库校验失败：`git diff --check HEAD~1 HEAD` 检测到 `customer-health.html` 第 1177 行 trailing whitespace。
- 第二次不是 Agent 业务失败，而是 JobSupervisor 在 DevFlow job 已经写入 `waiting-human` 后，用旧内存快照继续扫描 running attempt，并把它误判为 `No active local worker host lease for running attempt.`。
- Work Item 详情页只跟随 Workboard 主轮询，`action-plan` / `timeline` 只在进入详情时加载一次；阶段变化后没有独立刷新。

### 修复

- JobSupervisor 在标记 stalled 前会重新读取数据库；如果 attempt 的状态或 `updatedAt` 已经被后台 job 更新，就刷新本地快照并跳过本轮 stale 写入。
- Integrity scan 增加 Human Review checkpoint 自愈：如果 pending `human_review` checkpoint 存在，而 attempt 被 stale supervisor 标为 orphaned stalled，会恢复为 `waiting-human`，pipeline 恢复为 `waiting-human`，Work Item 恢复为 `In Review`。
- Work Item 详情页对 active attempt 的 `action-plan` / `timeline` 增加 2.5 秒轮询，并改为 `Promise.allSettled`，避免 action plan 单接口失败时把 timeline 一起清空。
- stalled / failed attempt 会标记 `feishuFailureNotifyPending`，保存后触发飞书失败通知。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
```

## 2026-05-03: 飞书测试消息成功但 Human Review / 失败通知未自动发送

### 现象

设置页里的飞书测试消息可以成功发到当前用户，但 DevFlow 到 Human Review 或失败时没有自动给飞书发消息。

### 原因

测试消息走的是一次性 `lark-cli im +messages-send --as bot --user-id <current-user>`。而自动 Human Review 发送只消费已保存的 chat / task / webhook 配置；当前数据库配置为空，没有 chat id、assignee、tasklist 或 webhook，因此自动链路记录为 `needs-configuration`。

### 修复

- 自动 Human Review 发送在没有显式 chat/task/webhook 路由时，会通过 `lark-cli contact +get-user --as user` 读取当前登录用户的 `open_id`，再走 bot direct message fallback。
- 失败通知复用同一 current-user fallback；如果没有显式路由但 `lark-cli` 已登录，仍能把 failed / stalled 信息发给当前用户。
- `feishuReview` 写回中增加 `route=direct-user`、`fallback=current-user` 和 `userId`，便于从 checkpoint 里追踪实际投递路径。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuAutoReviewRecordsNeedsConfigurationWhenNoTarget|TestFeishuAutoReviewFallsBackToCurrentLarkUser'
```

## 2026-05-03: Page Pilot Web 打开后退回 Work items / 3009 空响应

### 现象

浏览器访问 `/#page-pilot` 首屏能看到 Page Pilot，但 workspace session 加载完成后会回到 Work items。点击 `Open page editor` 时，如果 3009 上残留一个坏掉的旧 `python3 -m http.server`，Electron 会报 `ERR_EMPTY_RESPONSE`，Web 模式也会等待到 health check 失败。部分旧 session 的 repository target 缺少 `id`，UI 显示已选仓库但按钮仍提示需要选择 Repository Workspace。

### 修复

- session 恢复时尊重当前 URL hash，不再用保存的 `activeNav` 覆盖 `#page-pilot`。
- Page Pilot 在无 Electron bridge 的 Web 模式恢复 iframe + overlay fallback，并通过 Go Preview Runtime API 启动所选 workspace。
- `/page-pilot-target` 默认代理到 `127.0.0.1:3009`，Web fallback 对 3009 预览使用同源代理，便于 iframe inspection。
- Go Preview Runtime 在 health check 失败时，会清理同端口、同 workspace cwd 的陈旧 Page Pilot 监听进程后再启动。
- Page Pilot 对旧 workspace session 缺失 `repositoryTarget.id` 的情况派生稳定 id，例如 `repo_ZYOOO_TestRepo`。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/__tests__/desktopProcessSupervisor.test.ts --testTimeout=15000
npm run build -- --mode development
go test ./services/local-runtime/internal/omegalocal -run 'TestPagePilotPreviewRuntimeStartPersistsGoProfile|TestPagePilot|TestPagePilotApplyUsesIsolatedWorkspaceForGitHubTarget|TestPagePilotApplyAndDeliverUsesLocalRepositoryTarget'
```

## 2026-05-03: Electron Page Pilot 404 页面浮在 Omega 主界面上

### 现象

Electron 中点击 `Open page editor` 后，Omega 页面仍显示 `Target page opened...`，但左上角浮出 Python `http.server` 的 `Error response / Error code: 404 / File not found` 页面，视觉上像目标页错位覆盖在 Omega 导航上。

### 原因

Electron `BrowserView.webContents.loadURL()` 对 HTTP 404 也会 resolve，旧逻辑只要 loadURL 没抛异常就立即返回成功并挂载 BrowserView。静态 preview server 只服务真实文件路径；如果 URL 带了不存在的 path、被错误编码的 hash，或预览进程处在旧状态，就会返回 404，但 Omega 仍把它当成打开成功。

### 修复

- Electron preview 先在未挂载的 BrowserView 中后台加载目标页。
- 监听主 frame `did-navigate` / `did-fail-load`，HTTP status `>=400` 或主 frame load fail 都返回错误。
- 只有校验通过后才把 BrowserView 挂到主窗口；失败时销毁 preview view，让 Omega 页面保持正常，并把错误显示到 Page Pilot 状态区。
- App 启动时的 initial preview 也走同样校验；失败只记录 warning，不再把坏页挂上去。

### 验证

```bash
node --check apps/desktop/src/main.cjs
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/__tests__/desktopProcessSupervisor.test.ts --testTimeout=15000
```

## 2026-05-04: 刷新或 workspace session 加载后跳回 Work items

### 现象

在 Projects、Page Pilot 或 Work Item 详情页刷新，或等待应用进入并完成 workspace session 加载后，页面会短暂显示当前视图，然后自动跳回 Work items。用户在 Project 页面和已进入的 Work Item 详情页都能遇到该问题。

### 原因

主导航只有 `#workboard` / `#page-pilot` / Work Item detail 等少量 hash。Projects、Views、Settings 等视图仍复用 `#workboard`，而 workspace session 异步加载完成后会恢复保存的 `activeNav`。当旧 session 记录为 `Issues` 时，当前页面缺少明确 URL 语义，加载完成就被覆盖成 Work items。

### 修复

- 为主导航补齐稳定 hash：`#projects`、`#views`、`#settings`、`#page-pilot`、`#workboard`。
- session 恢复时先读取当前 hash，并让 hash 优先于保存的 `activeNav`。
- 侧边栏导航、Project config、Open work items、Page Pilot exit、Provider access 等入口统一写入对应 hash。
- GitHub issue import / workspace open 等显式进入 Work items 的动作会清空 detail route 并写回 `#workboard`。

### 验证

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=20000
npm run lint
git diff --check
```

本机浏览器验证：

- `http://127.0.0.1:5173/#projects` 等待加载并刷新后仍停留在 Projects。
- `http://127.0.0.1:5173/#/work-items/item_manual_30` 刷新后仍停留在 Work Item detail。
- `http://127.0.0.1:5173/#page-pilot` 刷新后仍停留在 Page Pilot。

## 2026-05-04: New requirement hover / active 状态出现黑色外圈

### 现象

鼠标指向或点击 `New requirement` 后，按钮右侧和底部出现深色外圈，和当前 light UI 不协调。

### 原因

按钮本体原本是透明背景，视觉边框依赖 `::before` 的 conic-gradient 装饰层；进入 hover / active 时全局 `button:hover` 的深色背景会落到 `.topbar-create` 上，透明边框段就会露出黑色。

### 修复

- 保留原有 `.topbar-create` 的透明容器 + `::before` conic 光边结构。
- hover / focus-visible / active 只显式覆盖全局 button hover 背景为 `transparent`，不再透出深色底。
- 保持原有 light / dark mode 的按钮填充和动效，不改整体视觉。

### 验证

```bash
npm run lint
git diff --check
```

本机浏览器验证：`http://127.0.0.1:5173/#workboard` 中点击并 hover `New requirement`，按钮保持原有 conic 光边，无黑色描边。

## 2026-05-04: Dark mode sidebar 折叠按钮过亮，Google 连接状态误导

### 现象

Dark mode 下 Workspaces / Connections 的折叠按钮是白色小方块，和深色 sidebar 不协调。Connections 中的 `Google` 显示为 `on`，但当前产品主链路并没有 Google 登录、Drive 或 Workspace 功能，容易让用户误以为已有 Google 集成。

### 原因

- dark theme 的 `.sidebar-section summary::after` 继承了 light mode 的浅色按钮样式。
- `Google` 是早期身份 provider 占位，`createInitialConnectionState()` 默认把它标为 connected；当前左侧 Connections 仍把它作为可见 provider 渲染。

### 修复

- dark theme 下 sidebar section 的折叠按钮改为深色底、低对比边框，hover / focus 时只轻微提亮。
- 左侧 Connections 主列表隐藏 Google，只保留当前真实链路使用的 GitHub / Feishu / CI；Google provider 类型保留用于旧数据兼容和后续身份能力扩展。

### 验证

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=20000
npm run lint
git diff --check
```

本机浏览器验证：dark mode 下两个折叠按钮不再是白色硬块，Connections 只显示 GitHub / Feishu / CI。

## 2026-05-04: Requirement 创建卡住与 dark mode composer 可读性问题

### 现象

打开 `New requirement` 后，dark mode 下 `Add a title *` / `Add a description` 标签对比度过低；右侧说明卡与表单不协调且信息重复。点击 `Create` 后偶发长时间停在 `Creating...`，刷新后刚创建的内容看起来消失。

### 原因

- 创建成功后仍同步等待 `refreshControlPlane()`，当观测性 / 控制面接口慢或无响应时，按钮状态和表单清理都会被阻塞。
- inline composer 的右侧说明卡复述 repository workspace 锁定信息，但页面下方已有 workspace lock status，形成重复且在 dark mode 下显得突兀。
- Workspaces 卡片的 product-shell 覆盖样式仍保留较大 padding 和较高 row 高度，导致窄 sidebar 下仓库名被过早截断。
- dark theme 单独把 `New requirement` 的填充改成近黑色，覆盖了 light mode 的蓝青渐变观感。

### 修复

- Work item / requirement 创建成功后立即更新本地 session、关闭 `Creating...` 状态并清理表单；控制面刷新改为后台异步执行。
- 移除 inline composer 右侧说明卡，保留下方真实 workspace lock status 作为唯一上下文。
- 提升 dark mode composer 标签对比度，并压缩 workspace entry / row / config button 间距，让 `ZYOOO/TestRepo` 在常规 sidebar 宽度下完整显示。
- dark mode 的 `New requirement` 继续使用原蓝青渐变填充，只保留深色模式的边缘光效参数。

### 验证

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testNamePattern "finishes requirement creation|shows Feishu|keeps the hash-routed Projects" --testTimeout=30000
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=30000
npm run lint
git diff --check
```

本机浏览器验证：`http://127.0.0.1:5173/#workboard` reload 后打开 `New requirement`，dark mode 下标签可读、右侧说明卡不再渲染、workspace 名称完整显示，按钮保持蓝青渐变。

## 2026-05-04: Delivery flow 状态文字被截断，飞书卡片 Approve 报 200340

### 现象

Work Item 详情页 `Delivery flow` 卡片底部状态文字只显示上半截，例如 `Ready` / `Waiting` 被裁成 `Rea...` / `Wai...`。飞书 Human Review 卡片点击 `Approve` 时手机端提示“出错了，请稍后重试 code: 200340”。

### 原因

- `detail-stage-grid` 的阶段状态 `<em>` 和标题区域共用两列 grid，且状态文本继承 `overflow: hidden`，在 card 高度和行高叠加后被裁切。
- 飞书交互卡片发送成功只说明 bot 有发消息权限；按钮点击还要求飞书应用启用卡片回传交互、订阅 `card.action.trigger`，并配置可被飞书云端访问的 Card Request URL。当前个人 `lark-cli` 直投场景没有公网 callback，旧卡片仍渲染 `Approve` / `Request changes`，点击就会被飞书客户端报 200340。

### 修复

- `Delivery flow` 阶段状态行独占整张卡片底部一行，补足 line-height 和 padding，不再被标题两列布局裁切。
- `feishu/review-callback` 兼容飞书真实 card action 嵌套 payload，并支持 callback URL challenge 响应。
- 没有显式启用 `OMEGA_FEISHU_CARD_CALLBACK_ENABLED=true` 时，卡片不再渲染会失败的 approve/request changes 按钮。
- 当前用户 `lark-cli` fallback 在无卡片回调时优先创建绑定当前用户的飞书 Task；完成任务后可通过 Task bridge 或 `/feishu/review-task/sync` 同步为 Omega approve。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuAutoReviewFallsBackToCurrentLarkUser|TestFeishuReviewRequestUsesLarkCLIInteractiveCard|TestFeishuReviewCallbackApprovesCheckpointThroughSharedDecisionPath|TestFeishuReviewCallbackAcceptsCardActionPayload|TestFeishuReviewTaskSyncApprovesCompletedTask'
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=20000
```

本机浏览器验证：`http://127.0.0.1:5173/#/work-items/item_manual_30` 的 Delivery flow 状态文字完整显示。

## 2026-05-04: Human Review 已 Approve 后仍显示审核按钮，dark artifacts 文字对比不足

### 现象

Work Item 详情页完成 Human Review approve 后，Timeline 已出现 `gate.approved` / `devflow.cycle.completed`，但 Human Review 区域仍显示 `Approve delivery` / `Request changes`。同一详情页的 Artifacts 卡片在 dark mode 下 `REVIEW`、文件名等文字过暗，看起来像被禁用。

### 原因

- 详情页只按 pipeline 找第一个 `pending` checkpoint，没有把 checkpoint 绑定到当前 attempt、`human_review` stage、pipeline/attempt 的 `waiting-human` 状态。
- Human Review 卡片没有区分“可操作 checkpoint”和“已决策 checkpoint”，所以已 approved 的运行仍可能被旧 pending checkpoint 驱动出审核按钮。
- proof card 的文字颜色被 dark mode 通用容器样式覆盖，缺少针对 `.proof-card` 文本层级的深色模式显式颜色。

### 修复

- Work Item 详情页先筛选当前 pipeline/current attempt 的 Human Review checkpoints，并按更新时间选最新记录。
- 只有 checkpoint 仍为 `pending`、attempt 仍处于 `waiting-human` 且 current stage 为 `human_review`、pipeline 仍处于 `waiting-human` 时才显示审核操作。
- 已 approved/rejected 或非当前可操作 checkpoint 改为只读决策摘要，避免重复点击。
- `/checkpoints` 读取时复用 JobSupervisor integrity reconcile：如果 pipeline/attempt 已完成且 run events 已有 `gate.approved`，但 checkpoint 仍残留 `pending`，会自动回填为 `approved` 并记录 decision note。
- dark mode 下为 proof/artifact 卡片的 kind、label、meta、path 文本补充明确对比色。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestListCheckpointsReconcilesCompletedHumanReviewDecision|TestApproveDevFlowCheckpointCanContinueDeliveryAsync|TestFeishuReviewTaskSyncApprovesCompletedTask|TestFeishuReviewCallbackAcceptsCardActionPayload'
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=20000
npm run lint
git diff --check
```

## 2026-05-04: Work Item 详情页 artifacts 只显示不可操作路径

### 现象

Human Review 区域的 `Changed` / `Validation` proof 只能看到文件名和截断路径，不能点击查看内容；下方 Artifacts 卡片也只展示被截断的本地绝对路径，用户无法判断这些证据是否有价值。

### 原因

- 后端已有 `/proof-records/:id/preview`，但 Work Item 详情页没有把 proof card 接到 preview endpoint。
- UI 把 source path 当作主要展示内容，导致真实有用的文件名、stage、proof kind 被路径截断噪音淹没。

### 修复

- `Changed` / `Validation` 中的 proof 条目改为可点击 artifact button，点击后打开 proof preview modal。
- 下方 Artifacts 从不可操作 details/path 展示改为整卡可点击 preview，主标题显示 artifact 文件名，stage/kind 作为辅助信息，完整路径放到 modal 中。
- preview modal 复用 `/proof-records/:id/preview`，展示 markdown/json/diff/text 内容、完整 source path、不可用错误和 truncation 状态。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=20000
npm run lint
git diff --check
```

## 2026-05-04: Artifact preview 打开后提示 `<!doctype` 不是 JSON

### 现象

点击 Work Item 详情页 artifact preview 后，modal 显示 `Unexpected token '<', "<!doctype "... is not valid JSON`，路径类似 `.omega/proof/human-review-request.md`。

### 原因

- Electron/static Web 环境没有显式 `VITE_MISSION_CONTROL_API_URL` 时，proof preview 请求可能落到前端 SPA fallback，浏览器拿到的是 `index.html`，而不是 Go runtime 的 JSON。
- Work Item proof card 先从 stage evidence 去重，遇到同一路径的 proof record 时会保留文件路径作为 id，导致点击预览没有使用真实 `/proof-records/:id/preview` id。

### 修复

- Web 默认 runtime API 在非 dev 环境指向 `http://127.0.0.1:3888`，避免 proof preview 请求误打到 SPA HTML。
- `fetchJson` 对 200 但非 JSON 的响应给出明确 runtime route 错误。
- stage evidence 与 proof record 同源路径匹配时，artifact card 使用真实 proof record id，再进行去重。
- `/proof-records/:id/preview` 兼容历史 evidence 直接传本地 proof 文件路径的情况；即使 encoded leading slash 被 Go HTTP path 规整掉，也会在本地文件存在时恢复为绝对路径并返回 JSON preview。

### 验证

```bash
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=20000
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=20000
go test ./services/local-runtime/internal/omegalocal -run TestRepositoryAuditTablesAndProofPreview
npm run lint
git diff --check
```

## 2026-05-04: Artifact preview 对 `rollback-plan` 等符号产物报 404

### 现象

Work Item 详情页点击 `rollback-plan` 等 artifact 后，preview modal 报 `Omega control API failed: /proof-records/rollback-plan/preview 404: proof record not found`。

### 原因

`rollback-plan` 是 workflow contract 里的预期输出物名称，不是 proof record id，也不是可读取的本地 proof 文件。前端把 stage `outputArtifacts` 中的符号名也做成了 preview 入口，导致后端按 proof id 查找失败。

### 修复

- Work Item artifact 聚合只把真实 proof record、URL、绝对/相对文件路径或带文件扩展名的 evidence 做成 preview card。
- `rollback-plan`、`proof-records` 等 workflow output slot 名称不再作为可点击 artifact preview 渲染。
- 增加回归测试覆盖同一 stage 同时包含真实 proof 路径和 `rollback-plan` 符号输出时，只真实 proof 可打开。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=20000
npm run lint
git diff --check
```

## 2026-05-04: Feishu reviewer 操作按钮吃到默认黑色样式

### 现象

Settings / Provider Access 的 Feishu reviewer 区域中，`Search` 和 `Use current user` 两个按钮在 light mode 下显示为黑色默认按钮，并和 reviewer 输入框挤在同一行，视觉上不适配当前卡片。

### 修复

- Reviewer 搜索输入框改为独占一行。
- `Search` / `Use current user` 改为同宽局部操作按钮，不再吃全局黑色 button 样式。
- 为 light / dark mode 分别补充按钮背景、边框、文字和 hover/focus 状态。

### 验证

```bash
npm run lint
git diff --check
```

## 2026-05-04: Workboard Blocked 分组不醒目且排在 Done 后面

### 现象

Workboard 中 `Blocked` 分组使用灰色状态点，并排在 `Done` 后面。大量 Done item 存在时，Blocked 队列很容易被忽略；等待人工审核的 item 仍混在 `Running` / `In Review` 里，不利于定位 Human Review 队列。

### 修复

- 新增 `Human Review` Work Item 展示状态，并将 pipeline `waiting-human` 映射到该分组。
- Workboard 分组顺序调整为 `Planning / Not started / Running / Human Review / Backlog / Blocked / Done`，确保 `Blocked` 位于 `Done` 前。
- `Blocked` 状态点和状态 pill 改为黄色/琥珀色系，light / dark mode 均可见。
- `Human Review` 使用独立紫色状态点和 pill，和 Running / Blocked 区分。

### 验证

```bash
npm run test -- apps/web/src/core/__tests__/workboard.test.ts apps/web/src/core/__tests__/workItemProjection.test.ts --testTimeout=20000
npm run lint
git diff --check
```

## 2026-05-04: Workboard item 元信息噪音与 Page Pilot 编号缺失

### 现象

Workboard 列表中 manual work item 会展示 `Req item_manual_23` 这类内部 id，占用标题下方空间。Page Pilot 物化出来的 Work Item 只显示 `Work item`，没有稳定编号；对应 pipeline 阶段显示 `Delivery`，对 Page Pilot 用户不够直观。

### 修复

- manual / Page Pilot 的内部 id 不再进入 Workboard 元信息行；只保留 repository 和有用来源信息。
- Page Pilot Work Item 支持 `page_pilot` source，并用 `PP-*` key 展示为 `Page Pilot #N`。
- Page Pilot 的 delivery 阶段在列表进度中显示为 `Confirm / PR`，表达“确认并交付 PR”的用户动作。
- Page Pilot item 仍保留在 Workboard 主列表中，作为 Page Pilot run 到 Work Item / Pipeline / PR / proof 的统一追踪入口。

### 验证

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testNamePattern "marks not-started work clearly" --testTimeout=30000
npm run lint
git diff --check
```

## 2026-05-04: Work Item 内部 id 仍在详情/项目摘要中暴露

### 现象

Workboard 主列表已隐藏 `item_manual_*` / `page-pilot:item_*` 这类内部 id，但 Work Item 详情页、Requirement source、Project workspace 摘要和右侧 Inspector 仍会直接展示 `sourceExternalRef`，导致用户在详情链路里还能看到 `item_manual_23` 这类实现细节。

### 修复

- 新增展示层过滤规则：`item_manual_*`、`item_page_pilot_*`、`page-pilot:item_*`、`req_item_manual_*`、`pipeline_item_*` 仅作为内部关联 id 保留，不进入普通用户可读元信息。
- Workboard 列表元信息不再展示 requirement id；这类关联只用于内部追踪，避免出现 `Req item_manual_*` 并占用标题下方空间。
- Work Item 详情页的标题元信息和 Requirement source 改为只显示真实外部引用，例如 GitHub issue 编号。
- Project workspace 摘要和 Inspector requirement 说明复用同一过滤逻辑，避免切换页面后再次泄露内部 id。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=30000
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testNamePattern "marks not-started work clearly" --testTimeout=30000
```

## 2026-05-04: 详情页重复流程信息与 Feishu 无公网卡片误导

### 现象

普通 Requirement Work Item 详情页展示 `Open in Page Pilot`，容易让功能一 DevFlow 与功能二 Page Pilot 的入口混在一起。详情页中 Delivery Flow 与 Current Attempt 都铺开 stage 卡片，Run Workpad 不能收起；刷新 Work Item detail 深链时会短暂回到 Work items 列表再切回来。Feishu 在没有公网 Card Request URL 时仍可能发送交互卡片按钮，用户点击后无法 approve。

### 修复

- Page Pilot 入口只对 Page Pilot Work Item 显示。
- Current Attempt 的 stage/action plan 细节默认折叠，Run Workpad 可折叠；`Agent pending` / `Ship` 文案改为更清晰的角色标签。
- Detail route 加载期间展示 detail loading state，避免刷新时闪到 Work items 列表。
- Feishu card 回调按钮要求公网 API 与 callback flag 同时 ready；无公网时优先走 Feishu Task 审核，chat 路由降级为文本通知。

### 验证

```bash
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=30000
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testNamePattern "marks not-started work clearly" --testTimeout=30000
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuReviewRequestUsesLarkCLITextWhenCardCallbackUnavailable|TestFeishuAutoReviewFallsBackToCurrentLarkUser|TestFeishuReviewRequestCreatesTaskReviewWithStrongBinding|TestFeishuReviewRequestSendsInteractiveWebhookCard' -count=1
```

## 2026-05-04: Workboard / 详情页刷新时全量读取执行历史导致 UI 变慢

### 现象

当前本地数据库约 400MB，`workspace_snapshots.database_json` 约 14MB，`runtime_logs` 超过 65 万条；Workboard 首屏和 live execution 轮询会同时读取 pipelines / attempts / checkpoints / operations / proof records，其中 `/operations` 单次响应可达 6MB 以上。后端旧实现每个表接口都会重新读取并反序列化整份 workspace snapshot，导致刷新或进入详情页时 UI 明显卡顿。

### 修复

- Go runtime 为 pipelines / attempts / checkpoints / operations / proof records 增加 SQLite 规范化表快读路径，并支持 `status` / `pipelineId` / `workItemId` / `limit` 等过滤。
- 保留未过滤 `/operations` 的完整 snapshot fallback，避免 runnerProcess 等历史详情字段被破坏；过滤查询走轻量 SQL。
- Web 首屏只读取最近 operations / proof records 和 pending checkpoints；Work Item 详情页打开后按当前 pipeline/work item 追加加载 scoped operations / proof records / checkpoints。
- live execution 轮询只刷新执行状态和 pending checkpoint，不再周期性拉完整 proof/operation 历史。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestListOperationsSupportsFilteredFastPath|TestRunOperationPersistsRunnerProcessResult|Test.*Checkpoint|Test.*Pipeline|Test.*Operation' -count=1
go test ./services/local-runtime/internal/omegalocal -count=1
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=30000
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=30000
npm run lint
```

## 2026-05-04: runtime_logs 被成功 GET 轮询访问日志淹没

### 现象

本地 `runtime_logs` 达到 658,335 条，但当前只有约 30 个 Work Item。数据库统计显示其中 624,267 条是 `api.request`，主要来自 `/pipelines`、`/workspace`、`/attempts`、`/checkpoints`、`/run-workpads` 等前端轮询接口；另外还有 18,395 条 `job_supervisor.remote_signals.polled` 和 12,858 条 `job_supervisor.tick.completed`。

### 根因

Go runtime 的 route wrapper 会把每一个 HTTP 请求都写入 `runtime_logs`。成功 GET 被标成 DEBUG，但 DEBUG 仍然持久化；Workboard / detail / live execution 的轮询频率叠加后，访问日志远高于真实业务事件。JobSupervisor interval tick 和远端 PR check poll 也在无变化时持续写入日志，进一步放大数据库体积和列表查询压力。

### 修复

- 默认不再持久化成功 GET / HEAD / OPTIONS 访问日志；失败请求和 mutating 请求仍写入 runtime log。
- 默认把被数据库跳过的成功读请求写入 `.omega/logs/omega-runtime-diagnostics.YYYY-MM-DD.jsonl`，用于偶发排查；`/health` 始终不落库也不写文件。
- 保留 `OMEGA_RUNTIME_LOG_DEBUG_API=true` 开关，用于开发调试时临时恢复成功 GET 数据库记录。
- JobSupervisor interval tick 只有出现 stalled / retry / cleanup / blocked remote checks / workflow missing 等操作者可见变化时才写入。
- JobSupervisor 无变化 interval tick 和正常远端 PR checks poll 写入 daily diagnostic file；只有 failed / missing required checks 时写数据库 `remote_signals.blocked`。
- diagnostic file 写入时自动清理 24 小时以前的 `omega-runtime-diagnostics.*.jsonl`，避免 `.omega/logs` 长期增长。
- 新增 SQLite migration `compact_noisy_runtime_logs`，启动时清理历史噪音：保留最近 1000 条 `api.request` DEBUG、最近 200 条 supervisor tick、最近 200 条 remote poll。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestAPIRequestRuntimeLogPolicySkipsSuccessfulReads|TestDiagnosticRuntimeLogsUseDailyFilesAndOneDayRetention|TestSQLiteCompactsNoisyRuntimeLogs|TestRuntimeLogsAPIListsAndFiltersRecords|TestSQLiteMigrations|TestMigrationsAndPipelineTemplates' -count=1
```

## 2026-05-04: OMG-32 approve 后仍显示 Approve 且响应慢

### 现象

OMG-32 Human Review 点击 `Approve delivery` 后按钮仍然保留，用户可以重复点击。日志显示多次 `POST /checkpoints/pipeline_item_manual_32:human_review/approve -> 200`，单次耗时约 6.9s 到 17.9s；同一 PR merge / GitHub outbound sync 被重复执行。

### 原因

- OMG-32 workspace snapshot 中存在两个相同 id 的 checkpoint：`pipeline_item_manual_32:human_review`。Approve 更新其中一个为 `approved` 后，SQLite 规范化表保存时后面的旧 duplicate `pending` 记录又覆盖同 id，UI 读取到的仍是 pending。
- Checkpoint decision 没有服务端幂等保护；重复点击或并发请求会在第一个 approve 完成前再次进入 delivery continuation。
- JobSupervisor 轮询 remote checks 时基于旧 snapshot 做 load-mutate-save；如果人工审核期间 workspace 已经被 approve 写入，旧 tick 仍可能后写回 pending，覆盖人工审核结果。
- 虽然前端已传 `asyncDelivery=true`，但 repeated POST / SQLite 大 snapshot 保存 / 并发 GitHub delivery 仍让按钮反馈显得很慢。

### 修复

- Checkpoint decision 增加服务端互斥和幂等：已 `approved` / `rejected` / `done` / `passed` 的 checkpoint 再次提交会直接返回现有结果，不重复触发 merge / outbound sync。
- Checkpoint decision 不再每次点击都先跑完整 integrity recovery；只有 checkpoint 找不到时才做恢复扫描，降低正常审核点击的响应时间。
- Workspace 保存前去重同 id checkpoint；对于 duplicate checkpoint，优先保留 approved/rejected/done/passed 等终态，再比较 `updatedAt`。
- `appendOrReplace` 现在替换同 id 第一条记录并丢弃后续重复记录，避免 `upsertPendingCheckpoint` 继续留下 duplicate。
- JobSupervisor 保存前检查 workspace snapshot `savedAt` 是否已经变化；如果本轮基于旧 snapshot，就跳过保存并写入 diagnostic file，避免轮询状态覆盖人工审核结果。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestCheckpointDecisionDeduplicatesDuplicateCheckpointIDs|TestFeishuReviewCallbackApprovesCheckpointThroughSharedDecisionPath|TestFeishuReviewTaskBridgeApprovesCheckpoint' -count=1
```

## 2026-05-04: 后台 CPU 飙高与 DevFlow auto-run settling 偶发超时

### 现象

Electron / Web 页面打开后，`omega-local-runtime` 可持续接近 100% CPU，占用约 860MB RSS。diagnostic log 显示 live execution 轮询每 2.5s 拉取一次 `/workspace`，单次响应约 14.7MB，同时还会拉取 `/run-workpads` 约 920KB、`/pipelines` 约 704KB、`/attempts` 约 541KB。此前整包测试中 `TestOrchestratorTickCanClaimAndRunDevFlowCycle` 偶发报 `auto-run attempt did not settle before cleanup`。

### 根因

- `/workspace` 是完整持久化快照接口，包含 missions / operations / proof records / run workpads 等执行明细；当前 snapshot 中 missions 约 4.9MB、operations 约 6.3MB。
- 前端 live execution 轮询把 `/workspace` 当成轻量状态接口使用，导致后端频繁反序列化和序列化大 JSON，前端也反复解析大对象。
- `/run-workpads` 旧路径也从完整 snapshot fallback 读取，即使只有十几条 workpad，也会反序列化整份 snapshot。
- `DevFlow auto-run settling` 的失败不是稳定状态机断言失败，而是在后台异步 job 需要把 attempt 从 `running` 推到终态时，测试 10s 等待窗口内没有及时观察到终态；高 CPU / 大 JSON 轮询会放大这类异步 settling 超时。

### 修复

- `fetchWorkspaceSession` 改为读取 `/workspace?scope=session`，只返回 Projects / Requirements / Work Items / MissionControlState / Connections / UI preferences，不再把 execution-heavy tables 发给 UI 会话恢复。
- Go runtime 的 `/workspace?scope=session` 改为从 SQLite 规范化表拼 session read model，不再先反序列化 `workspace_snapshots.database_json` 再裁剪；`work_items.record_json` 保存完整 Work Item 记录，避免丢失 `source` / `repositoryTargetId` / Page Pilot 元信息。
- `/run-workpads` 增加 SQLite 规范化表读取路径，避免列表刷新时读取完整 snapshot。
- Checkpoint approve 的正常路径继续跳过完整 integrity scan；若遇到 legacy human-review checkpoint 缺失 attempt，则只在该异常路径触发恢复，保持旧数据可审批。

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -count=1
curl -fsS -o /tmp/omega-workspace-session.json -w 'session HTTP=%{http_code} BYTES=%{size_download} TOTAL=%{time_total}\n' 'http://127.0.0.1:3888/workspace?scope=session'
curl -fsS -o /tmp/omega-workspace-full.json -w 'full HTTP=%{http_code} BYTES=%{size_download} TOTAL=%{time_total}\n' 'http://127.0.0.1:3888/workspace'
```

本地实测：session 约 464KB / 0.26s，full workspace 约 14.7MB / 0.45s；`TestWorkspaceSessionScopeOmitsExecutionHeavyTables` 会故意破坏 full snapshot JSON，确认 session scope 不依赖完整镜像反序列化。
