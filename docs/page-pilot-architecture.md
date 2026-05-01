# Omega Page Pilot Architecture

本文记录赛题功能二的 MVP 架构。目标是在 Omega 内打开用户正在构建的软件预览页面，让 Page Pilot 基于目标页面选区定位源码、修改真实代码、刷新预览，并在用户确认后创建 MR/PR。

当前采用三层架构：

```text
React SPA = Omega 产品 UI
Go local runtime = SQLite / runner / git / PR 执行
Electron = 桌面壳 + 目标项目内部浏览器 + 预览注入能力
```

Electron 不替代 React，也不替代 Go。开发模式不需要打包：Electron 直接加载 Vite dev server，同时保留 Go runtime。

## Desktop Shell 启动链路

2026-04-30 更新：旧做法需要用户分别启动 Go local runtime、Omega Web Vite dev server 和目标项目预览服务。新做法在 Electron main process 中加入 `process-supervisor.cjs`，先尝试复用已运行服务，缺失时再启动本地进程：

```text
npm run desktop
  -> Go local runtime: http://127.0.0.1:3888/health
  -> Omega Web: http://127.0.0.1:5174/
  -> target preview: OMEGA_PREVIEW_REPO_PATH + preview profile
  -> Electron BrowserWindow loads Omega Web
  -> Electron BrowserView loads target preview when available
```

当前 preview profile 是保守本地版：Electron 启动时必须显式传入 `OMEGA_PREVIEW_REPO_PATH` 或 `OMEGA_PAGE_PILOT_REPO_PATH`，不会从 Omega 当前目录猜目标项目。若传入 `OMEGA_PREVIEW_COMMAND`，Electron 使用该命令；否则读取目标 repo 的 `package.json` 和 lockfile，按 `dev/start/preview` 脚本生成启动计划；静态 `index.html` 项目使用 `python3 -m http.server` 兜底。

2026-04-30 更新：Page Pilot 启动器里的 `Dev server by Agent` 不再把用户输入当作直接 URL 打开。用户选择 Repository Workspace 后，Electron 会调用本地 Preview Runtime Agent：

1. 锁定所选 repository target，local target 只使用显式 path，GitHub target 使用 Omega 管理的隔离 preview workspace。
2. 读取 `package.json`、lockfile、常见框架配置、`README.md`、`index.html` 等线索。
3. 生成 Preview Runtime Profile：agent id、stage id、工作目录、dev command、preview URL、health check、reload 策略和 evidence。
4. 在该 repository workspace 内启动 dev server，并等待 health check 成功。
5. 成功后 Electron 才打开 direct pilot BrowserView；失败时把原因返回 Page Pilot 页面。

这还是 Electron 侧基础版 Preview Runtime Agent，职责和产物已经明确，但 profile / pid / stdout / stderr 仍未下沉到 Go runtime 一等 API。

这一步是打包产品的前置验证层。长期目标仍是把 Preview Runtime Profile 和 process supervisor 下沉到 Go runtime API，让启动档案、stdout/stderr、健康检查和失败诊断进入 Page Pilot run，而 Electron 只负责显示、注入和 reload。

## 当前主模式

Page Pilot 当前仍保留 Electron direct pilot 调试模式：

```text
Electron BrowserWindow
  -> 直接加载用户目标项目 URL，例如 http://127.0.0.1:5173/
  -> pilot-preload.cjs 注入 Page Pilot 浮层
  -> 用户在目标产品页面内 hover / 选择 / 批注
  -> preload 调用 Go local runtime
  -> runtime 在明确 Repository Workspace 中调用 Agent runner 修改真实源码
```

React SPA 仍然存在，但它不是功能二最终圈选体验的主入口。它负责 Omega 管理界面、Repository Workspace / Agent Profile 配置、以及浏览器调试 fallback。Go local runtime 继续负责 SQLite、repository target 解析、runner preflight、真实 patch、branch / commit / PR。

Page Pilot 背后必须有 Agent 参与，不只是一组固定命令。不同项目可能是 Vite、Next.js、Astro、Rails、Django、静态 HTML、pnpm workspace、Docker compose 或自定义脚本，Omega 不能假设所有项目都用 `npm run dev` 或固定端口。

产品层可以是一个 Page Pilot Agent。工程层把这个 Agent 拆成几个 stage/contract：

- `preview-runtime` stage：读取 repository workspace，理解项目如何安装依赖、如何启动 preview/dev server、端口和环境变量是什么，生成并维护 Preview Runtime Profile。
- `page-editing` stage：接收选区上下文、批注队列和整体修改说明，定位源码并生成真实 patch。
- `delivery` stage：在用户确认后生成语义摘要、行级 diff 摘要、branch / commit / PR。

Go runtime 负责把 Page Pilot Agent 的每个 stage 都约束在明确 repository workspace 内执行，并用 process supervisor 管理 preview server；Electron 只负责显示目标页面、注入选择浮层和在 runtime 通知后刷新页面。

当前 direct pilot 交互：

1. 右下角悬浮手指进入选择模式。
2. 鼠标 hover 时高亮目标页面元素，并展示元素类型、文本和源码映射状态。
3. 点击元素后只显示 `✓ / ×` 确认控件。
4. 点 `✓` 后打开单元素批注输入框。
5. 发送单元素批注后只加入本地批注队列，并在页面元素附近插入编号 pin。
6. 页面底部保留一个悬浮输入框，展示批注 chip，用户可以继续选择更多元素，也可以补充整体修改需求。
7. 用户点击底部输入框发送按钮后，preload 才统一调用 `/page-pilot/apply`。

`data-omega-source` 是强源码映射，不是能否选择元素的门槛。没有该元数据时仍然记录 selector、DOM context、文本快照、样式快照和用户批注，作为 DOM-only 上下文交给 Agent；有该元数据时 runtime 可以更稳定地定位文件和 symbol。

## 目标

Page Pilot 要把用户在页面上看到的元素，连接到真实源码修改与 GitHub 交付：

```text
Visible page element
  -> selection context
  -> source mapping
  -> local runtime
  -> Page Pilot Agent page-editing stage
  -> real source patch
  -> Page Pilot Agent preview-runtime stage restart/reload if needed
  -> user confirm
  -> branch / commit / PR
```

MVP 不做纯展示 UI。每次 Apply 必须落到本地 runtime，并且必须能解析到明确的 Repository Workspace；如果没有可写的本地 worktree，runtime 要拒绝执行。

## 前端 / 桌面入口

入口组件：

```text
apps/desktop/src/main.cjs
apps/desktop/src/omega-preload.cjs
apps/desktop/src/pilot-preload.cjs
apps/web/src/components/PagePilotPreview.tsx
```

挂载位置：

```text
apps/web/src/App.tsx
```

产品语义：

- Page Pilot 不应该圈选 Omega 自身的管理 UI 作为最终用户目标。
- 用户应从 Omega 的 Page Pilot 启动器选择 Repository Workspace 和预览来源，然后进入“正在构建的软件”的完整页面。
- direct pilot preload 绑定到目标项目 document，再圈选目标项目里的按钮、标题、卡片文案。
- 浏览器 SPA 中的 iframe / Overlay 只保留为开发调试 fallback；Electron BrowserView + `pilot-preload.cjs` 是功能二的主路径，用于真实目标项目预览圈选。
- 改完代码后，Go runtime 负责写源码 / 后续重启目标 dev server，Electron 负责刷新内部预览页面。

Direct pilot overlay 能力：

- 以悬浮手指按钮注入目标项目页面。
- 点击悬浮手指后进入元素圈选模式。
- 支持圈选按钮、标题、卡片文案、label、input、textarea、select、link、常见布局容器和普通文本节点；未归类元素会标成 `other`。
- 捕获稳定 selector、DOM context、文本快照、样式快照和源码映射。
- 支持多批注队列和整体说明，在用户主动发送后统一调用 `/page-pilot/apply`。
- 调用 `/page-pilot/apply` 应用真实源码变更。
- 用户确认后调用 `/page-pilot/deliver` 创建 branch / commit / PR-ready 交付。
- apply 成功后在目标页内展示结果面板，支持 Confirm / Discard / Reload / New。

Electron dev shell 能力：

- `BrowserWindow` 加载 Omega React SPA。
- `BrowserView` 加载用户目标项目 preview URL，并使用 `pilot-preload.cjs` 注入完整 direct pilot。
- `omega-preload.cjs` 暴露 `omegaDesktop.openPreview`、`omegaDesktop.reloadPreview` 和 `omegaDesktop.resolvePreviewTarget`。
- `omega-preview:open` 会把当前 Electron 已启动/复用的 Go local runtime base URL 注入给 `pilot-preload.cjs`；direct pilot 调用 `/page-pilot/apply`、`/page-pilot/deliver` 和 discard 时使用这条现有服务链路。
- Omega 首页提供 Page Pilot CTA，`#page-pilot` 可直接进入功能二页面。
- Page Pilot 页面必须先选择 Repository Workspace；Work Item 详情页的 `Open in Page Pilot` 会使用该 item 的 `repositoryTargetId` 作为目标 repo。
- Page Pilot preview source 支持三类入口：
  - Repository source：按所选 Repository Workspace 准备隔离预览工作区或显式 local path。
  - Dev server by Agent：Preview Runtime Agent 读取所选 repo，生成启动 profile，启动 dev server，通过 health check 后打开预览。
  - HTML file：例如 `/Users/demo/app/index.html`，Electron 会转成 `file://` 打开，适合无需构建服务的静态页面。
- Electron 中 `Open direct pilot` 打开 BrowserView 后不再回传给 Omega Web overlay；用户在目标页面内完成完整 Page Pilot 操作。
- 旧尝试：`preview-preload.cjs` 曾注入最小 DOM selection bridge，并把 selection context 回传给 Omega 页面。该模式现仅作为历史尝试保留，不作为产品主路径。

Electron direct pilot mode：

- `npm run desktop:pilot` 可直接打开用户目标项目 URL，默认 `http://127.0.0.1:5173/`。
- Omega 桌面壳也会通过 `omegaDesktop.openPreview` 打开同一套 direct pilot，并把当前 repo 配置传给 preload。
- `pilot-preload.cjs` 在目标项目页面内注入 Page Pilot 悬浮手指按钮、返回按钮、hover 高亮、元素信息 tooltip 和修改输入框。
- direct pilot 的本地恢复状态按 repository target 和目标 URL 分 scope；不同 repo / URL 不共享上一次 apply 结果或失败状态。
- 这个模式不经过 Chrome，也不通过 iframe；目标产品就是 Electron 主窗口内容，Page Pilot 作为 preload overlay 运行在同一个页面上下文里。
- 选择流程为：悬浮手指 -> hover 高亮 -> 点击元素 -> `✓ / ×` 确认控件 -> `✓` 后打开底部输入框。
- 圈选命中使用鼠标坐标下的元素堆叠；任何可见 DOM 元素都可以被选中。button / link / input / textarea / select / role button、动态状态提示、label、源码映射、标题、文本和卡片容器只影响排序和 tooltip 分类，不决定元素是否可选。
- 输入 comment 后先加入批注队列，并在页面上插入编号标记；用户可以选择多个元素后统一提交。
- 新增批注后会自动回到选择态，支持连续框选多个元素；用户不需要每次重新点击右下角手指。
- 批注队列默认折叠，只显示最新一条；用户可以用 `展开批注 / 收起批注` 控件查看全部历史，点击页面编号 pin 或底部批注 chip 可以回填并修改对应 comment。
- 提交给 Agent 后，历史折叠单位切换为 Page Pilot 对话轮次：一次 `/page-pilot/apply` 调用就是一轮，对话记录包含该轮所有批注、primary target、整体 instruction、运行状态和关联 run。
- 执行态和结果态使用可吸附状态栏：默认吸在底部并只留 8px 边缘热区，鼠标进入热区由 JS peek 状态滑出、离开后延迟缩回，避免边界 hover 弹跳；点击 SVG expand/minimize 图标展开/收起详情；按住 SVG move 手柄可在页面内自由移动，松手靠近顶部或底部时自动吸附并缩进边缘，否则保留浮动位置；Confirm / Discard / Reload / New 固定在状态栏右侧，避免被日志滚动区域遮住。
- 多批注提交时，Electron preload 把最新一条带 `data-omega-source` 的批注作为 primary target 传给 runtime；没有源码映射时回退到最新批注并走 DOM context / selector。
- `data-omega-source` 是强源码映射，不是批注前置条件。没有映射时仍然记录 selector、DOM context、文本/样式快照和 comment，作为 DOM-only 批注交给 Agent。
- 用户点击 `提交给 Agent` 后，preload 立即把批注编辑框切换为过程面板，展示 primary target、Agent 提交状态、changed files、Requirement / Work Item / Pipeline linkage 和 preview reload。delivery / discard 会作为下一层轻量状态动作接入，不放在初始元素确认控件里。

## Page Pilot Agent Stages

Page Pilot Agent 是产品上面对用户的单一 Agent。内部 stage 负责不同产物，避免把“启动项目”“改源码”“交付 PR”混成不可审计的一次大调用。

`preview-runtime` stage 负责回答“这个项目怎么跑起来”，并把答案变成可审计、可复用的 Preview Runtime Profile。它不能在未知目录里自由执行，必须锁定到当前 Repository Workspace。

输入：

- `projectId`
- `repositoryTargetId`
- 本地 repository path
- 用户提供的 preview intent，例如“打开登录页”或“跑本地开发预览”
- 当前机器 capabilities：node、npm、pnpm、yarn、bun、python、go、docker、git 等

允许读取的项目线索：

- `package.json` / workspace manifests
- `README.md` / `docs/*`
- lockfile 与 package manager 信息
- framework config，例如 `vite.config.*`、`next.config.*`、`astro.config.*`
- Docker / compose files
- existing `.omega/preview-runtime.json`

输出 Preview Runtime Profile：

```json
{
  "repositoryTargetId": "repo_ZYOOO_TestRepo",
  "workingDirectory": "/Users/zyong/Projects/TestRepo",
  "installCommand": "npm install",
  "devCommand": "npm run dev -- --host 127.0.0.1",
  "previewUrl": "http://127.0.0.1:5173/",
  "healthCheck": {
    "url": "http://127.0.0.1:5173/",
    "expectedStatus": 200
  },
  "reloadStrategy": "hmr-wait",
  "notes": "Static HTML project can also be served by a local static server."
}
```

运行时行为：

1. 如果 repository 没有 Preview Runtime Profile，runtime 调用 `preview-runtime` Agent 生成候选。
2. Agent 先读项目文件和 docs，再提出启动命令；命令必须记录到 profile，不能临时隐式执行。
3. runtime 对命令做 workspace path 校验和 capability preflight。
4. process supervisor 在 repository workspace 内启动 dev server，记录 pid、stdout/stderr、端口、健康检查和失败原因。
5. Electron direct pilot 打开 profile 中的 `previewUrl`。
6. `page-editing` stage 修改代码后，runtime 根据 profile 执行 HMR 等待、browser reload 或 dev server restart。
7. 如果启动失败，`preview-runtime` stage 可以基于日志进行下一轮诊断，但每轮都必须写入 run 记录。

2026-04-30 更新：Electron 基础版已经把 reload 接入 Preview Runtime Supervisor。目标页内的 `Reload` 和 Page Editing Agent apply 后的自动刷新不再直接调用浏览器刷新，而是调用 `omega-preview:reload`：

- 没有 Preview Runtime Profile 时，仅退回浏览器刷新。
- 普通源码变更按 profile 的 `reloadStrategy` 执行，默认 dev server 项目为 `hmr-wait`，静态 HTML 为 `browser-reload`。
- `package.json`、lockfile、`vite.config.*`、`next.config.*`、`astro.config.*`、Docker / env 配置等运行时相关文件变更会提升为 `server-restart`。
- Supervisor 会先做 health check；如果服务不可用或策略要求重启，会停止旧进程、重新启动 dev server、等待 health check，然后才让 Electron 刷新 BrowserView。
- Electron 只负责触发和显示，不再决定 HMR、浏览器刷新还是服务重启。

这让 Page Pilot 可以支持不同技术栈，同时避免“Agent 猜错目录然后误写/误跑其他仓库”。

`page-editing` stage 输入：

- Page Pilot selection context
- 批注队列和整体修改说明
- Preview Runtime Profile
- Repository target / local worktree
- 可选 source mapping 与 DOM-only 候选源码位置

`delivery` stage 输入：

- 已应用的 patch run
- Git diff 和 changed files
- 用户确认结果
- Repository target 的 GitHub/remote 信息

`delivery` stage 输出：

- semantic change summary
- line-level diff summary
- branch / commit / PR
- delivery proof

## 接入功能一 Requirement / Workspace

Page Pilot 可以直接接到功能一，而不是绕开 Workboard。推荐模型：

```text
Page Pilot session
  -> Requirement
  -> Work Item
  -> Page Pilot pipeline run
  -> Page Pilot Agent stages
  -> proof / branch / commit / PR
```

用户在页面里圈选多个元素并输入整体说明后，Omega 可以创建一个 Requirement：

- `source = page_pilot`
- `repositoryTargetId = 当前目标项目`
- `rawRequirement = 用户整体说明`
- `acceptanceCriteria = 选区批注转换出的可验证条目`
- `artifacts.selectionContext = selector / DOM context / style / text / source mapping`
- `previewRuntimeProfileId = 当前预览运行档案`

这样功能二不是一条独立黑箱链路，而是功能一的一个需求入口和特殊执行模板。

Workspace 关系需要严格区分：

- Omega app workspace：Omega 自己的代码仓库和运行时，不应该被 Page Pilot 改写，除非用户明确把 Omega repo 作为目标项目。
- Repository Workspace：用户正在构建的软件，也就是 Page Pilot 选区、启动预览和代码修改的目标。
- Operation workspace：功能一 DevFlow 常用的隔离执行目录，用于 branch / commit / PR 的审计闭环。

为了兼顾热更新和隔离执行，Page Pilot 支持两种执行模式：

1. `live-preview` mode：直接在绑定的 local Repository Workspace 中修改，优先保证 Electron 预览能立即 reload/HMR。适合演示和本地快速迭代，但必须加 repository lock，避免同时有其他 pipeline 写同一 worktree。
2. `isolated-devflow` mode：把 Page Pilot session 创建为 Requirement/Work Item 后，交给功能一在隔离 operation workspace 中执行。预览要么打开该 operation workspace 的 dev server，要么在用户确认后把 patch 应用回原 workspace。

MVP 优先走 `live-preview` mode，并同时创建 Requirement/Work Item 记录，保持功能一的数据审计和交付闭环。后续再把 `isolated-devflow` 做成更强的团队协作路径。

当前已落地的最小功能一接入：

- `/page-pilot/apply` 成功后创建 `source = page_pilot` 的 Requirement 和 Work Item。
- Work Item 绑定当前 `repositoryTargetId`，并记录 selection context、用户 instruction、`agentMode = single-page-pilot-agent` 和 `executionMode = live-preview`。
- runtime 创建 `templateId = page-pilot` 的轻量 pipeline run，包含 `preview_runtime`、`page_editing`、`delivery` 三个 stage。
- Page Pilot run 返回并持久化 `requirementId`、`workItemId`、`pipelineId` 和 `pipelineRunId`。
- apply / deliver / discard 会同步写入通用 Mission / Operation / Proof records，Work Item 详情页可以直接展示 Page Pilot Agent trace 和 proof。
- `/page-pilot/deliver` 会把关联 Work Item/Pipeline 推进到 delivered；discard 会标记为 blocked/discarded。
- Electron direct pilot 会在 apply 成功后自动 reload 目标页面，并从 localStorage 恢复结果面板。
- 结果面板展示 changed files、diff summary、Requirement / Work Item / Pipeline linkage，并提供 Confirm / Discard。
- Confirm 调用 `/page-pilot/deliver`，Discard 调用 `/page-pilot/runs/{id}/discard`。

源码映射第一版使用显式元数据：

```tsx
data-omega-source="apps/web/src/components/PortalHome.tsx:headline"
```

这避免一开始依赖脆弱的全自动 DOM -> source 反查。当前已在 Portal Home 的 headline、按钮和卡片文案上放置最小元数据。

## Runtime API

新增 API：

```text
POST /page-pilot/apply
POST /page-pilot/deliver
GET  /page-pilot/runs
POST /page-pilot/runs/{id}/discard
```

计划中的 Preview runtime API：

- `/page-pilot/preview-runtime/resolve`：调用或读取 Preview Runtime Agent/Profile，返回启动命令、preview URL、健康检查和风险说明。
- `/page-pilot/preview-runtime/start`：由 Go process supervisor 在明确 repository workspace 内启动目标项目 dev server。
- `/page-pilot/preview-runtime/restart`：在代码 patch 后按 profile 重新启动或刷新目标 preview。

`/page-pilot/apply` 输入：

- `projectId`
- `repositoryTargetId`
- `instruction`
- `selection`
  - `elementKind`
  - `stableSelector`
  - `textSnapshot`
  - `styleSnapshot`
  - `domContext`
  - `sourceMapping`
- `runner`

运行时行为：

1. 校验 `repositoryTargetId` 必填。
2. 从 Workboard 数据中解析 repository target。
3. 对 local target 使用 target path。
4. 旧做法：对 GitHub target，只有当前 local runtime 工作目录的 git remote 与 target 匹配时才使用当前 worktree；这个做法依赖启动目录，边界不清晰。
5. 新做法：对 GitHub target 使用 Omega 管理的隔离 Page Pilot preview workspace，默认位置为 `~/Omega/workspaces/page-pilot/<owner_repo>`；首次打开预览时由 Electron shell 按 repository target clone / prepare，Go runtime apply 阶段只消费这个隔离 workspace，不再猜用户本机其他目录。
6. 校验 `sourceMapping.file` 不逃逸 repository root，且文件真实存在。
7. 读取 Project / Repository Agent Profile，按 `coding` Agent 解析 runner。
8. 对 Codex / opencode / Claude Code 做 CLI preflight，缺失时在启动前失败。
9. 运行 Agent 修改真实 source file。Prompt 明确声明传入的 selection 是 primary target，多批注内容只作为辅助上下文。
10. 返回 changed files、diff summary、line-level diff summary 和 preview refresh 提示。
11. 将 run 记录持久化到本地 runtime，返回 `runId`，供确认交付或撤销使用。

预览刷新语义：

- Agent 不需要盯着页面热更新，但需要能理解项目启动方式。
- 代码写入后，runtime 返回 changed files 和 run 状态。
- Preview Runtime Profile 决定刷新策略：等待 HMR、Electron browser reload、或重启目标 dev server 后 reload。
- 旧做法：Electron BrowserView 只覆盖 Omega 页面右侧区域，目标页面像嵌入式浮层。
- 中间尝试：Electron BrowserView 打开后覆盖整个 content area，并通过 `preview-preload.cjs` 注入最小工具条，再把圈选结果回传到 Omega 页面。该方式虽然解决了全页预览，但改变了已验证的目标页内 direct pilot 交互，不作为主路径继续推进。
- 新做法：Omega 的 Page Pilot 页面只承担 repo / 预览来源选择和启动；`omega-preview:open` 打开完整目标页面后直接加载 `pilot-preload.cjs`，在目标页面内保留多批注、整体说明、Apply、Confirm、Discard、结果面板和刷新体验。
- `pilot-preload.cjs` 通过 Electron additional arguments 获取 `projectId`、`repositoryTargetId`、`repositoryLabel`，不再依赖固定 env/default repo；目标页内新增 `返回` 按钮关闭 BrowserView 回到 Omega。
- Electron 内部浏览器负责执行最终 reload；Go runtime 负责启动/重启目标项目进程。

`/page-pilot/deliver` 行为：

1. 确认 repository 中存在未提交的 Page Pilot 变更。
2. 创建或切换到 `omega/page-pilot-*` 分支。
3. `git add` / `git commit`。
4. GitHub target 会 push branch 并创建 PR。
5. 返回 branch、commit、PR URL、changed files 和行级 diff 摘要。
6. 使用 `runId` 更新同一条 Page Pilot 记录为 `delivered`。

`/page-pilot/runs` 行为：

- 返回本地保存的 Page Pilot apply / discard / delivery 记录。
- 使用 SQLite 一等表 `page_pilot_runs` 保存和查询记录；旧 `omega_settings` 记录只作为兼容读取来源。
- 2026-04-30 更新：run 记录新增服务端 conversation 基础字段：
  - `conversationBatch`：本轮批注、主目标、整体说明、状态和关联 run；
  - `submittedAnnotations`：提交给 Agent 的页面批注列表；
  - `primaryTarget`：当前主要修改目标，来自批注主目标或 selection fallback；
  - `processEvents`：目标页内采集、提交、刷新等过程事件。
- 2026-04-30 更新：run 记录新增 `previewRuntimeProfile`，由 Electron Preview Runtime Agent 生成，包含 preview URL、工作目录、启动命令、health check、reload strategy 和 evidence。
- 2026-04-30 更新：run 记录新增 `sourceMappingReport`，统计本轮批注的强源码映射、DOM-only 选区、缺失文件映射和覆盖率。
- 2026-04-30 更新：当覆盖率不是 `strong` 时，runtime 会先生成 `sourceLocator` 候选，按文本快照、selector token、DOM tag 和批注 token 搜索真实源码文件。
- 2026-04-30 更新：`/page-pilot/apply` 支持传入既有 `runId`。当 run 仍处于 `applied` 状态且 repository target 一致时，runtime 会复用同一个 Work Item / Pipeline / live-preview lock，递增 `roundNumber`，并追加本轮批注与过程事件。
- 2026-04-30 更新：run 记录新增 `prPreview` 和 `visualProof`。`prPreview` 提前给出语义化 PR body、changed files、diff summary 和 source context；`visualProof` 当前使用 DOM snapshot 基础证据，供 Work Item、pipeline artifact 和后续 PR body 引用。
- `buildPagePilotPrompt` 会把 `sourceMappingReport` 和 `sourceLocator` 写入 Agent prompt：
  - `strong`：优先使用明确源码映射；
  - `partial` / `dom-only`：要求 Agent 先检查候选文件，不允许凭空改无关文件；
  - 无候选时要求在 output note 中说明缺少映射，而不是制造假修改。
- `syncPagePilotRunRecords` 会把 conversation、preview runtime profile、source mapping report、source locator、PR preview 和 visual proof 写入 Page Pilot mission 和 pipeline run artifacts，Work Item 详情页后续可直接基于服务端记录展示，而不是依赖目标页 localStorage。

`/page-pilot/preview-runtime/resolve|start|restart` 行为：

- `resolve` 只解析明确的 Repository Workspace，生成可审计 Preview Runtime Profile，不启动进程。
- `start` 根据 profile 在目标 workspace 内启动 dev server；如果 health check 已通过，则复用现有服务。
- `restart` 先停止当前 runtime session，再按最新 profile 重新启动。
- session summary 会记录 profile、pid、preview URL、working directory、stdout/stderr tail、health check 和启动来源。
- 当前为 Go runtime 基础版 supervisor；跨进程恢复、持久 process table 和更完整失败诊断继续作为后续增强。

`/page-pilot/runs/{id}/discard` 行为：

- 只允许撤销 `applied` 状态的 run。
- 对记录中的 changed files 执行 git revert/clean，恢复本地 worktree。
- 将记录更新为 `discarded`，保留 diff 摘要、时间和 repository status。
- 同步把 `conversationBatch.status` 改为 `discarded`，保留原批注和过程事件供回溯。

## Runner Strategy

产品主路径使用 Agent Profile 中的 Page Pilot Agent runner。实现上可以先复用 `coding` runner，但 contract 要区分 stage：

```text
profile -> preview-runtime runner -> codex / opencode / claude-code
profile -> page-editing runner -> codex / opencode / claude-code
profile -> delivery runner -> codex / opencode / claude-code
```

`preview-runtime`、`page-editing` 和 `delivery` 可以先共用 Project Agent Profile 的 runner 配置。产品语义上它们是一个 Page Pilot Agent 的不同 stage：前者产出可执行预览运行档案，中间阶段产出源码 patch，后者产出交付摘要和 PR。

MVP 额外支持 `local-proof` / `demo-code` 的确定性文本替换路径，主要用于本地验证和单元测试。它仍然修改真实 source file、生成真实 git diff，但只接受明确的直接替换指令，例如：

```text
replace text with "New headline"
```

这个路径不是最终 AI 能力；正式演示应配置 Codex / opencode / Claude Code，并让 preflight 确认 CLI 可用。

## Safety

- Page Pilot 不会在没有 `repositoryTargetId` 时执行。
- Page Pilot Agent 的所有 stage 都必须绑定同一个 repository target 和 local worktree / operation workspace。
- Page Pilot session 创建 Requirement 时，Requirement 的 `repositoryTargetId` 必须等于当前预览页面绑定的 repository target。
- `live-preview` mode 必须持有 repository write lock；已有 DevFlow/operation 正在写同一 worktree 时要阻止启动或转入 isolated mode。
- 2026-04-30 更新：Go runtime 的 Page Pilot `apply` 会先声明 `page-pilot-live:{repositoryTargetId}:{repositoryPath}` execution lock，并持有到用户 Confirm 或 Discard。`deliver` 和 `discard` 只允许同一个 run 继续使用该锁，终态成功后释放；其他 Page Pilot run 会被拒绝，避免同一预览工作区并发写入。
- dev server 启动命令必须在 repository root 或 profile 指定的子目录内执行，不能逃逸 workspace。
- `sourceMapping.file` 必须在 repository root 内。
- 旧做法：GitHub target 不能随意 clone 到临时目录做“看不见”的修改，MVP 只接受匹配当前 runtime worktree 的 GitHub repo。
- 新做法：GitHub target 只能进入 Omega 管理的隔离 Page Pilot preview workspace；不扫描 `~/Projects` 等用户目录，不靠仓库名猜测本地路径。local target 必须来自用户显式绑定。
- Apply 阶段不 commit、不 push、不建 PR；Confirm 才进入交付。
- Discard 只处理 Page Pilot run 记录中的 changed files，不做仓库级 destructive reset。
- 缺少配置 runner CLI 时直接返回错误，不制造假的 failed runner process。

## 后续工作

- 增强 Preview Runtime supervisor：补跨进程恢复、持久 process table、日志分页和失败诊断。
- 在 direct pilot 中继续优化 patch preview / 用户确认 / deliver / discard 的浮层密度。
- 将 direct pilot 的 target URL、project id、repository target id、local worktree 绑定成一等配置，替代 env/default。
- 扩展同一 Page Pilot run 的多轮对话 UI：当前服务端已支持多轮 apply，后续继续增强目标页内的轮次切换和对比视图。
- 增加 DOM-only source locating 辅助能力：当缺少 `data-omega-source` 时，用 selector、文本、样式和 repository 搜索生成候选源码位置，但仍以显式 metadata 为优先。
- 扩展 Go target dev server start/restart API：基础 resolve/start/restart 已完成，后续补持久 session 表和更多框架检测。
- 扩展 `data-omega-source` 到 Workboard 关键可编辑区域。
- 增加 selection history 和更完整 patch preview。
- 将 Page Pilot proof 从 run JSON 和 proof 文件继续拆成可查询的一等 proof 记录。
- 在 PR body 中加入截图文件证据；当前已有 DOM snapshot 和 source context 基础证据。
