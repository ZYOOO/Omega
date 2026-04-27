# Omega Handoff

本文是当前项目交接说明。目标是让接手同事可以快速理解 Omega 当前 v0Beta 的真实状态、如何启动、如何验证、代码应该从哪里看，以及下一步优先做什么。

## 1. 当前一句话

Omega 是一个 local-first 的 AI DevFlow 产品：用户在 App 中输入 Requirement，系统将其转成可执行 Item，按 Workflow 编排 Agent，在本地隔离 workspace 中运行代码修改，并通过 GitHub branch / commit / PR / review / human gate / proof 形成可审计交付链路。

当前重点不是单点代码生成，而是：

```text
Requirement
  -> Item
  -> Pipeline
  -> Agent orchestration
  -> Local workspace execution
  -> GitHub PR delivery
  -> Human Review gate
  -> Proof / handoff
```

## 2. 当前可运行形态

### 前端

- 技术栈：TS + React + Vite SPA。
- 默认入口：门户首页。
- 功能入口：Workboard。
- 当前页面：
  - `http://localhost:5173/`
  - `http://localhost:5173/#workboard`

主要代码：

```text
apps/web/src/App.tsx                         # Workboard 主入口，仍然偏大，后续继续拆
apps/web/src/components/PortalHome.tsx       # 门户首页
apps/web/src/components/WorkItemDetailPanels.tsx # Work item detail / artifact / attempt 面板拆分
apps/web/src/styles.css                      # 门户 + Workboard 视觉样式
apps/web/src/omegaControlApiClient.ts        # Go local runtime API client
apps/web/src/workspaceApiClient.ts           # workspace state API client
apps/web/src/core/*                          # Workboard / pipeline / mission 等前端模型
```

### 本地服务

- 技术栈：Go。
- 作用：本地 API、SQLite、Workflow 编排、Attempt 后台任务、本地 runner、GitHub 交付。
- 默认端口：`127.0.0.1:3888`
- 数据库：`.omega/omega.db`

主要代码：

```text
services/local-runtime/cmd/omega-local-runtime/main.go
services/local-runtime/internal/omegalocal/server.go
services/local-runtime/internal/omegalocal/orchestrator.go
services/local-runtime/internal/omegalocal/agent_profile.go
services/local-runtime/internal/omegalocal/agent_runner.go
services/local-runtime/internal/omegalocal/workflow_template.go
services/local-runtime/internal/omegalocal/github_delivery.go
services/local-runtime/internal/omegalocal/sqlite.go
services/local-runtime/workflows/devflow-pr.md
```

### 桌面壳

```text
apps/desktop
```

当前是预留目录，还没有成为日常开发主路径。后续打包桌面 App 时，它应该负责启动 Go runtime、加载 React SPA、管理本地 workspace picker / deep link / preview webview。

## 3. 启动方式

安装依赖：

```bash
npm install
```

启动 Go local runtime：

```bash
npm run local-runtime:dev
```

另开终端启动前端：

```bash
npm run web:dev
```

访问：

```text
http://localhost:5173/
http://localhost:5173/#workboard
```

如果端口被占用，先检查已有进程，不要直接删数据目录。

## 4. 常用验证命令

前端类型检查：

```bash
npm run lint
```

前端测试：

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx
```

构建：

```bash
npm run build
```

Go 测试：

```bash
npm run go:test
```

更聚焦的 Go 测试：

```bash
go test ./services/local-runtime/internal/omegalocal
```

说明：当前前端测试里可能出现 `/api/workspace`、`/api/observability` 的 URL warning，这是测试环境 fetch fallback 的已知噪声；只要测试结果通过，不代表本次改动失败。

## 5. 推荐手动演示路径

推荐测试仓库：

```text
ZYOOO/TestRepo
```

前置条件：

- 本机已安装并登录 `gh`。
- Go local runtime 已启动。
- 前端已启动。
- App 中 GitHub connection 为 on。
- Workboard 左侧已存在或可创建 `ZYOOO/TestRepo` repository workspace。

建议流程：

1. 打开 `http://localhost:5173/#workboard`。
2. 进入 `ZYOOO/TestRepo` workspace。
3. 点击 `New requirement`。
4. 输入一个可验证的需求，例如修改 `index.html` 某个明确 UI 行为。
5. 创建后确认生成 Item。
6. 点击 Run。
7. 列表中应看到 Not started / Running / Human review / Done 等状态变化。
8. 点进 Item 详情，查看 Current attempt、Stage cards、Agent turns、Artifacts、workspace、PR。
9. 到 Human Review 阶段后，人工查看 PR、diff、review artifacts，再点击 Approve 或 Request changes。
10. Approve 后才会继续 merge / delivery。

当前正确产品语义：

- Run 不应等 HTTP 请求跑完整流程；它应该快速返回 Attempt id，后台 job 继续跑。
- Review Agent 提出 changes requested 时，不应直接终局失败；应进入 Rework，再回到 Code Review。
- Human Review 是真实阻塞点，不能自动绕过。

## 6. 当前对象模型

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

关键解释：

- `Project` 是产品/工程目标，不等于 repo。
- `Repository target` 是真实代码目标，例如 GitHub repo 或本地 repo path。
- `Requirement` 是需求源，可以来自 App、GitHub issue、未来聊天工具或共享控制面。
- `Item` 是 Omega 内部真正可运行、可排期、可审计的工作单元。
- `Pipeline run` 是某个 Item 的流程实例。
- `Attempt` 是一次 Run / AutoRun / Retry 的执行轮次。
- `Proof` 是证据，不只是文件；后续要继续结构化 diff、test、review、PR、merge 信息。

## 7. 当前 Workflow

默认 workflow：

```text
services/local-runtime/workflows/devflow-pr.md
```

当前流程大致是：

```text
Todo intake
  -> Implementation and PR
  -> Code Review Round 1
  -> Code Review Round 2
  -> Rework
  -> Human Review
  -> Merging
  -> Done
```

注意：

- Workflow 已经从 Go hardcode 中抽出为 Markdown 文件。
- Go runtime 仍负责安全边界、workspace、Attempt、runner、GitHub 操作和状态持久化。
- Workflow Template 还不是 SQLite 一等记录，也还没有 App 内编辑器。

## 8. 当前 UI 状态

已完成：

- 默认门户首页，参考工作台门户风格。
- Workboard 功能页保留原功能。
- Workboard 已改为浅色工作台视觉体系，减少之前暗色 UI 的信息噪声。
- 右侧 rail 默认折叠常态。
- GitHub Issues 已放到 Work item 分组上方，并可展开。
- 左侧 workspace 显示当前 repo 作用域和 Auto processing 开关。
- Work item 行内展示 stage strip、状态、turns/artifacts、Run/Trace。
- Item 详情页展示 Requirement source、Current attempt、Stage、Human gate、Proof。
- Settings / Workspace Config 已有 Project Agent Profile 基础 UI，可配置 Workflow、Agent runner/model、Skills、MCP、`.codex` / `.claude` 运行时策略。
- 左侧 workspace 齿轮可进入当前 repository workspace 的配置页；Auto scan 和 Delete workspace 已移入配置页。
- 顶部 `New requirement` 使用高亮渐变按钮，日夜间模式都需要继续兼顾。

刚做过的拆分：

```text
apps/web/src/components/PortalHome.tsx
apps/web/src/components/WorkItemDetailPanels.tsx
```

后续还应继续拆：

```text
Workboard list
Work item detail
Inspector panel
Operator panel
GitHub workspace panel
Pipeline stage strip
Human gate panel
```

## 9. 当前 GitHub 能力

已具备：

- 读取本机 `gh` 登录态。
- App 内 GitHub OAuth 配置和 callback。
- 读取 repositories。
- 绑定 repository target。
- 导入 GitHub issues。
- 创建 branch / commit / PR。
- 读取 PR / checks。
- Human approve 后继续 merge / delivery proof。

仍需补齐：

- GitHub issue comment / label / status 回写。
- PR lifecycle UI 更完整展示。
- checks failure 的自动修复与重试策略。
- 多用户 GitHub App / OAuth 权限模型。

## 10. 当前执行安全边界

已具备：

- Item 执行前必须有明确 `repositoryTargetId`。
- 本地 workspace root 可配置，默认 `~/Omega/workspaces`。
- workspace path 必须在 root 内。
- execution lock 防止重复认领。
- `.omega/job.json`、`.omega/prompt.md`、`.omega/agent-runtime.json` 会写入执行上下文。
- Codex runner 是独立子进程，记录 pid、exit code、duration、stdout、stderr。
- Agent Profile 已从 `omega_settings` 升级为 SQLite 一等表 `agent_profiles`，保留 settings 镜像兼容旧路径。
- Agent runner registry 基础版已接入 Codex / opencode / Claude Code / local-proof / demo-code。
- 前端 Agent Profile runner 选择已接入 `/local-capabilities`；不可用 runner 会禁选或阻止保存。
- Go runtime 在创建 attempt / operation workspace 前做 runner preflight，缺失 CLI 时返回 400，不再制造无意义的 failed runner process。

仍需补齐：

- 正式 JobSupervisor。
- heartbeat / timeout / cancel / retry / stall detection。
- 多 turn continuation。
- worker host 分配。
- runner-specific runtime 模板、provider/model 映射、timeout/cancel/retry、完整 runner registry 能力。

## 11. 文档入口

建议阅读顺序：

```text
README.md
docs/architecture.md
docs/current-product-design.md
docs/development-plan.md
docs/competition-requirements-matrix.md
docs/manual-testing-guide.md
docs/todo.md
docs/development-log.md
```

API 文档：

```text
docs/openapi.yaml
```

数据模型：

```text
docs/persistence-schema.md
docs/work-model-reference.md
```

## 12. 当前已知风险

1. `App.tsx` 仍然很大，只拆出了门户首页；Workboard 需要继续组件化。
2. Workflow Template 还没有 App 内编辑和 SQLite 持久化，当前默认从 Markdown 文件读取。
3. Agent runner registry 基础版已完成，但 Provider selection、runner-specific runtime 模板、timeout/cancel/retry 还没有完整。
4. 当前 background job 是 goroutine 基础版，还不是完整 JobSupervisor。
5. Proof 展示仍偏“记录/文件/摘要”，结构化 diff/test/review/check 预览还不够。
6. Feishu 当前只有基础文本通知，还没有 approval card callback。
7. Product Layer 还有部分 snapshot-first 数据，后续要继续 repository-first relational model。
8. 自动处理 GitHub issue 已有基础，但 issue 状态回写和冲突恢复还不完整。

## 13. 最近改动概览

最近主要做了：

- 增加门户首页，保留 Workboard 为功能页。
- 使用正式 Omega logo 资产：`apps/web/public/omega-logo.png`。
- 统一 Workboard 到浅色工作台风格，并补了夜间模式的大量可读性问题。
- 将门户首页拆成 `PortalHome.tsx`。
- 将 Work item 详情相关面板拆到 `WorkItemDetailPanels.tsx`。
- Project / Repository Workspace 配置页初步成型：workspace folder、Agent Profile、Workflow / Agents / Runtime files。
- Agent Profile 一等表、runner registry、runner capability preflight 已接入。
- 更新 README / docs / todo / feature implementation log / development log。

建议接手后先跑：

```bash
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx
npm run build
go test ./services/local-runtime/internal/omegalocal
```

再做一次 `ZYOOO/TestRepo` 的手动流程验证。

## 14. 下一步建议：转向功能二

功能一已经有本地完整闭环的基础，下一阶段建议把重点转到赛题功能二：在自建页面内注入 AI Agent 悬浮控件，支持圈选元素、对话修改、热更新预览和 PR/MR 交付。

功能二必须完成项：

1. 前端 UI 和官网：已具备 TS + React SPA、门户首页和 Workboard 功能页；后续继续 polish 首页与功能页，但不要牺牲真实执行链路。
2. 注入悬浮对话框与圈选功能：需要新增页面内 Agent Overlay，能进入 inspect/select 模式，圈选至少按钮、标题、卡片文案三类元素。
3. 圈选并定位修改：选区需要生成稳定 selector、DOM context、文字/样式快照，并映射到源文件位置；Agent 修改必须落到真实源码。
4. 热更新预览：代码修改后触发 Vite HMR 或 dev server reload，让用户在当前预览页立刻看到变化。
5. 自动创建 PR/MR 并生成摘要：用户确认后通过本地 runtime / GitHub 能力创建分支、commit、PR，并生成语义化摘要和行级 diff 摘要。

推荐第一轮实现顺序：

1. 设计 `AI Page Pilot` / `Overlay Agent` 的前端状态模型：idle、selecting、selected、chatting、applying、previewing、ready-to-pr。
2. 在 `apps/web` 新增 overlay 组件，先只在自建 SPA 中运行，不急着做浏览器扩展。
3. 为关键可编辑元素加最小 source mapping 元数据，例如 `data-omega-source="apps/web/src/components/PortalHome.tsx:headline"`，避免一开始就做脆弱的全自动源码反查。
4. 新增 Go runtime API：接收 selection context + instruction，生成 patch / apply patch / run validation / expose diff。
5. 接入 Vite dev server 热更新验证。
6. 做确认交付入口：基于当前 diff 创建 branch / commit / PR，并把摘要写到 PR body。
7. 每完成一个小功能，都更新 `docs/feature-implementation-log.md`。

功能一仍需并行维护但优先级稍后：

- 继续拆 `App.tsx`，先拆 Workboard list/detail/inspector/operator。
- 把 Workflow Template 升级成 SQLite 一等记录，并提供 API。
- 做 App 内 Workflow Template 编辑器。
- 抽正式 JobSupervisor，替代当前基础 goroutine job。
- 增强 Human Review 页面：展示 PR、changed files、Review Agent verdict、risk、checks、Approve/Request changes 的影响。
- 补 GitHub issue / PR 状态回写。

交接判断标准：

- 不要只看 UI 是否显示 Done。
- 要确认 repo target 正确、workspace 正确、branch/PR 正确、Review/Human gate 真实生效、Proof 有对应 artifacts。
- 功能二不要做假展示：圈选、源码修改、HMR、diff、PR 都要尽量落到真实数据和真实执行。
