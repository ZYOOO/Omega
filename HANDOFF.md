# Omega Handoff

本文是当前项目交接说明。目标是让接手同事可以快速理解 Omega 当前 v0Beta 的真实状态、如何启动、如何验证、代码应该从哪里看，以及下一步优先做什么。

## 1. 当前一句话

Omega 是一个 local-first 的 AI DevFlow 产品：用户在 App 中输入 Requirement，系统将其转成可执行 Item，按 Workflow 编排 Agent，在本地隔离 workspace 中运行代码修改，并通过 GitHub branch / commit / PR / review / human gate / proof 形成可审计交付链路。

截至 2026-04-29，功能一已经具备本地完整闭环基础；功能二 Page Pilot 已完成 Electron 直连 MVP：可以在内置 Chromium 里打开目标项目页面，注入悬浮控件，圈选真实页面元素，收集 DOM/selector/style/source context，提交给本地 runtime / 单 Agent 链路修改目标 repo 代码，并刷新预览、生成可确认/丢弃的结果。

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
apps/web/src/components/RequirementComposer.tsx # Requirement 创建表单
apps/web/src/components/ProjectSurface.tsx   # Project / Repository Workspace 总览
apps/web/src/components/WorkspaceChrome.tsx  # Workboard 左侧导航、顶部栏和详情工具栏
apps/web/src/components/WorkItemDetailPanels.tsx # Work item detail / artifact / attempt 面板拆分
apps/web/src/components/PagePilotPreview.tsx # SPA 内 Page Pilot 预览入口
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
services/local-runtime/internal/omegalocal/work_items.go
services/local-runtime/internal/omegalocal/devflow_cycle.go
services/local-runtime/internal/omegalocal/pipeline_records.go
services/local-runtime/internal/omegalocal/job_supervisor.go
services/local-runtime/internal/omegalocal/page_pilot.go
services/local-runtime/internal/omegalocal/runtime_logs.go
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

当前已经用于功能二 MVP 的主要体验路径：Electron 打开目标项目预览页，在 Chromium 页面内通过 preload 注入 Page Pilot 悬浮控件。它还没有变成完整打包产品，但开发期可用于验证“像浏览器一样访问目标项目，同时有 Omega 悬浮 Agent”的体验。

关键文件：

```text
apps/desktop/src/pilot-preload.cjs
apps/desktop/src/main.cjs
```

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

最近验证通过：

```bash
npm run lint
npm run test -- apps/web/src/core/__tests__/manualWorkItem.test.ts apps/web/src/__tests__/App.operatorView.test.tsx --testTimeout=15000
npm run build
go test ./services/local-runtime/internal/omegalocal
```

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
- Workboard 已支持未开始 Item 的真实删除：列表最左侧出现删除按钮，只允许 `Ready` / `Backlog` 且没有执行历史的 Item 删除；删除会同步清理未共享 Requirement 和 mission state 投影。

刚做过的拆分：

```text
apps/web/src/components/PortalHome.tsx
apps/web/src/components/WorkItemDetailPanels.tsx
apps/web/src/components/RequirementComposer.tsx
apps/web/src/components/ProjectSurface.tsx
apps/web/src/components/WorkspaceChrome.tsx
apps/web/src/core/manualWorkItem.ts
services/local-runtime/internal/omegalocal/work_items.go
services/local-runtime/internal/omegalocal/devflow_cycle.go
services/local-runtime/internal/omegalocal/pipeline_records.go
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

- 多 turn continuation。
- GitHub polling heartbeat、Git/GitHub command timeout 和远端 runner 崩溃恢复。
- runner-specific runtime 模板、provider/model 映射和更完整 runner registry 能力。
- workspace archive / 压缩 / 删除整个 workspace 策略。

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

1. `App.tsx` 仍然很大，但已经拆出 Portal、Requirement 创建、Project 总览、Workspace shell、Work item detail panels；后续优先拆 Workboard list/detail、Inspector、Settings/Agent Profile。
2. Workflow Template 还没有 App 内编辑和 SQLite 持久化，当前默认从 Markdown 文件读取。
3. Agent runner registry 基础版已完成，但 Provider selection 和 runner-specific runtime 模板还没有完整。
4. JobSupervisor v1 已有常驻 tick、Attempt heartbeat、runner process heartbeat、stalled detection、retry、cancel、contract-driven timeout、workspace lock、workspace cleanup、worker host lease 和 continuation policy metadata 基础版；仍缺远端崩溃恢复和 GitHub polling。
5. Runtime log 基础版和按 Attempt 聚合的 Run Timeline 基础版已落地；还缺 cursor pagination、全文搜索、runner stdout/stderr 展开筛选和 GitHub polling 事件。
6. Proof 展示仍偏“记录/文件/摘要”，结构化 diff/test/review/check 预览还不够。
7. Feishu 当前只有基础文本通知，还没有 approval card callback。
8. Product Layer 还有部分 snapshot-first 数据，后续要继续 repository-first relational model。
9. 自动处理 GitHub issue 已有基础，但 issue 状态回写和冲突恢复还不完整。
10. Page Pilot 的单 Agent 修改链路已能跑通 MVP，但多轮对话、精确 source mapping 覆盖率、preview runtime 自动启动策略、diff 解释质量还需要继续产品化。

## 13. 最近改动概览

最近主要做了：

- 增加门户首页，保留 Workboard 为功能页。
- 使用正式 Omega logo 资产：`apps/web/public/omega-logo.png`。
- 统一 Workboard 到浅色工作台风格，并补了夜间模式的大量可读性问题。
- 将门户首页拆成 `PortalHome.tsx`。
- 将 Work item 详情相关面板拆到 `WorkItemDetailPanels.tsx`。
- Project / Repository Workspace 配置页初步成型：workspace folder、Agent Profile、Workflow / Agents / Runtime files。
- Agent Profile 一等表、runner registry、runner capability preflight 已接入。
- 功能二 Page Pilot MVP：Electron direct pilot、悬浮 FAB、元素 hover/selection、批注输入、批量提交给单 Agent、runtime 应用 patch、刷新预览、确认/丢弃。
- Page Pilot 已接入功能一记录：`source=page_pilot` 的 Requirement / Work Item / Pipeline run 会被物化。
- 功能一产品化补强：默认 workflow markdown、review/rework/human gate、runner telemetry、execution lock、基础 watcher/orchestrator。
- 代码拆分：`RequirementComposer`、`ProjectSurface`、`WorkspaceChrome`、`manualWorkItem`、`work_items.go`、`devflow_cycle.go`、`pipeline_records.go`。
- 未开始 Work Item 真实删除：`DELETE /work-items/{itemId}`，前端左侧删除按钮，runtime 防止删除已有执行历史的 Item。
- 更新 README / docs / todo / feature implementation log / development log。

建议接手后先跑：

```bash
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx
npm run build
go test ./services/local-runtime/internal/omegalocal
```

再做一次 `ZYOOO/TestRepo` 的手动流程验证。

## 14. 下一步建议

当前建议分两条线推进。

功能一产品化：

- 继续拆 `App.tsx`，先拆 Workboard list/detail/inspector/operator。
- 把 Workflow Template 升级成 SQLite 一等记录，并提供 API。
- 做 App 内 Workflow Template 编辑器。
- 继续增强 JobSupervisor：GitHub polling、远端 worker 分配、远端崩溃恢复、workspace archive/retention 高级策略。
- 增强 Human Review 页面：展示 PR、changed files、Review Agent verdict、risk、checks、Approve/Request changes 的影响。
- 补 GitHub issue / PR 状态回写。
- 补接口测试和数据分析：work item lifecycle、pipeline/attempt lifecycle、runner telemetry、failure/retry、PR delivery 统计。

功能二 Page Pilot 产品化：

- 继续提高 source mapping 覆盖率：不要只靠人工 `data-omega-source`，但也不要完全依赖脆弱自动反查；建议保留 `data-omega-source` 作为高置信入口，再做 DOM/source heuristic fallback。
- 支持一轮 Agent 对话里的多批选区记录，记录每批 annotations、用户总指令、runtime events、diff summary。
- 将 preview runtime 做成 Agent 负责的能力：根据 repo 判断 install/start/port/healthcheck/reload，而不是固定假设 Vite。
- 完善确认交付：确认后创建 branch / commit / PR，并生成语义化摘要和行级 diff 摘要。
- 把 Page Pilot run 的 process log、diff、proof、功能一 linkage 在 Workboard 详情页里展示得更清楚。

交接判断标准：

- 不要只看 UI 是否显示 Done。
- 要确认 repo target 正确、workspace 正确、branch/PR 正确、Review/Human gate 真实生效、Proof 有对应 artifacts。
- 功能二不要做假展示：圈选、源码修改、HMR、diff、PR 都要尽量落到真实数据和真实执行。
