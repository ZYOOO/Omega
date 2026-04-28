# Omega 功能实现记录

这个文档用于记录每个功能的落地方式。后续每完成一个功能，都按相同结构补一节：目标、入口、数据/API、运行时行为、验证、后续工作。

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
apps/desktop/src/preview-preload.cjs
apps/desktop/src/pilot-main.cjs
apps/desktop/src/pilot-preload.cjs
```

开发模式下 Electron 加载现有 React/Vite URL，不需要打包。它预留目标项目 `BrowserView`，通过 preload 注入 Page Pilot selection bridge。React SPA 和 Go runtime 不被替代：React 继续做 Omega UI，Go 继续做 SQLite / runner / git / PR。

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
POST /page-pilot/preview-runtime/resolve
POST /page-pilot/preview-runtime/start
POST /page-pilot/preview-runtime/restart
POST /page-pilot/apply
POST /page-pilot/deliver
GET /page-pilot/runs
POST /page-pilot/runs/{id}/discard
```

Preview runtime API 仍为计划项。目标是让 Agent 为每个 repository workspace 生成 Preview Runtime Profile，例如 install command、dev command、preview URL、health check 和 reload strategy，再由 Go runtime 的 process supervisor 执行。

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

### 后续工作

- 在 direct pilot 中增加可视化 debug 模式，显示候选元素排名，便于排查复杂页面的遮挡和命中问题。
- 为跨 iframe / shadow DOM 场景补充更完整的 composed path / elementFromPoint 适配。

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
