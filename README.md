# Omega AI Delivery Engine

Omega 是一个 local-first 的 AI 研发流程引擎。它把需求、任务、流程、Agent、代码仓库和交付证据串成一条可运行、可审计的 DevFlow。

当前 v0Beta 的目标是跑通比赛功能一：

```text
需求输入
  -> 需求理解与拆分
  -> Pipeline 编排
  -> 多 Agent 协作
  -> 本地隔离 workspace 执行
  -> GitHub branch / commit / pull request / checks / review / merge proof
```

## 当前架构

```text
React SPA
  -> Feishu-style Portal Home
  -> Workboard
  -> Go Local Service
      -> SQLite
      -> Pipeline / Agent orchestration
      -> background attempt jobs
      -> local git / gh / runner
      -> GitHub delivery proof
```

主要模块：

- 前端 SPA：`apps/web/src/App.tsx`、`apps/web/src/components/PortalHome.tsx`、`apps/web/src/core/*`、`apps/web/src/omegaControlApiClient.ts`、`apps/web/src/workspaceApiClient.ts`
- Desktop shell：`apps/desktop`
- 本地服务：`services/local-runtime/cmd/omega-local-runtime`
- Go 核心：`services/local-runtime/internal/omegalocal`
- 共享包预留：`packages/shared`
- 文档：`docs`
- SQLite：`.omega/omega.db`

仓库结构：

```text
apps/web                  # TS + React SPA，包含门户首页、Workboard、Pipeline、Proof UI
apps/desktop              # Electron shell 预留，用于最终桌面 App 打包
services/local-runtime    # Go local runtime，包含 API、SQLite、编排、本地 runner、GitHub 交付
packages/shared           # 共享类型和 API schema 预留
docs                      # 架构、开发计划、赛题对照和测试文档
scripts                   # 兼容脚本和 smoke 工具
```

## 核心对象

```text
Project
  -> Repository target
      -> Requirement
          -> Item
              -> Pipeline run
                  -> Attempt
                      -> Stage
                          -> Mission / Operation / Proof / Checkpoint
```

关键语义：

- `Project` 是产品或工程目标，不等于代码仓库。
- `Repository target` 是真实 GitHub repo 或本地 repo path。
- `Requirement` 是需求源，可以来自 App 手动输入、GitHub issue、飞书消息或未来共享控制面。
- `Item` 是 Omega 内部真正可执行、可排期、可审计的工作项。
- `Pipeline run` 是某个 Item 的一次流程运行。
- `Attempt` 是一次具体执行记录。点击 Run 后服务端会先创建 Attempt 并立即返回，后端后台 job 继续执行；Attempt 记录 runner、workspace、branch、PR、stage、proof 和错误。
- `Proof` 是每个阶段的证据，包括 artifact、diff、test report、review report、PR、checks、merge result。
- 默认 `devflow-pr` 流程定义在 `services/local-runtime/workflows/devflow-pr.md`，Go runtime 会从该 Markdown workflow 编译 stages、agents、artifact 和 review rounds。默认流程可以直接使用，但不是唯一写死流程。

## 快速启动

安装依赖：

```bash
npm install
```

启动 Go local runtime：

```bash
npm run local-runtime:dev
```

启动前端：

```bash
npm run web:dev
```

默认访问：

```text
http://localhost:5173/
```

默认页面是参考飞书工作台结构的门户首页，功能页入口是首页 CTA 或：

```text
http://localhost:5173/#workboard
```

Go local service 默认监听：

```text
http://127.0.0.1:3888
```

## GitHub 配置

Omega 支持两种 GitHub 路径：

1. 使用本机 `gh` 登录态读取 repositories、创建 PR、读取 checks。
2. 在 App 内配置 GitHub OAuth App，并通过本地 callback 完成授权。

OAuth callback：

```text
http://127.0.0.1:3888/auth/github/callback
```

App 内会把 OAuth 配置持久化到 `.omega/omega.db`。`.env` 只作为开发 fallback。

## 本地 workspace

执行代码变更时，Omega 会在配置的 workspace root 下创建隔离 workspace。

默认 root：

```text
~/Omega/workspaces
```

可以在 App 内修改，也可以通过 API：

```text
GET /local-workspace-root
PUT /local-workspace-root
```

每次运行会写入：

```text
.omega/job.json
.omega/prompt.md
.omega/agent-runtime.json
```

这些文件用于记录 runner、Agent、repo target、workspace path、sandbox policy 和执行上下文。

## 主要 API

```text
GET  /health
GET  /workspace
PUT  /workspace
GET  /requirements
POST /requirements/decompose
POST /work-items
PATCH /work-items/:id
GET  /pipelines
GET  /workflow-templates
GET  /attempts
POST /pipelines/from-template
POST /pipelines/:id/run-devflow-cycle
GET  /checkpoints
POST /checkpoints/:id/approve
POST /checkpoints/:id/request-changes
GET  /missions
GET  /operations
GET  /proof-records
GET  /execution-locks
POST /orchestrator/tick
GET  /agent-definitions
GET  /llm-providers
GET  /observability
GET  /github/status
GET  /github/repositories
POST /github/bind-repository-target
POST /github/import-issues
POST /github/create-pr
POST /github/pr-status
```

完整 API 文档：

```text
docs/openapi.yaml
```

## 开发命令

```bash
npm run lint
npm run test
npm run coverage
npm run build
npm run go:test
```

常用 targeted checks：

```bash
npm test -- --run src/__tests__/App.operatorView.test.tsx
go test ./services/local-runtime/internal/omegalocal
```

## 当前可验证流程

推荐测试仓库：

```text
ZYOOO/TestRepo
```

建议手动验证：

1. 启动 Go local service 和前端。
2. 打开 App。
3. 确认 GitHub 连接为 on。
4. 在 Project 页面选择并绑定 `ZYOOO/TestRepo`。
5. 进入左侧 `ZYOOO/TestRepo` workspace。
6. 在 Work items 页面创建 Requirement。
7. 点击 Run。
8. Run 请求应立即返回并显示已开始；后端后台 job 会继续执行。
9. 通过列表轮询和详情页观察 `Planning -> Running -> In Review`、Attempt、Pipeline stages 和 Agent 分配。
10. 在 Checkpoint / Operator 面板中审核 Human Review：Approve 后才会继续 merge / delivery；Reject 会回退并保留原因。
11. 检查 GitHub PR、Review Agent proof、human proof 和 merge 状态。

## 当前文档

- `docs/architecture.md`：当前架构。
- `docs/development-plan.md`：开发思路与路线。
- `docs/development-log.md`：开发日志。
- `docs/competition-requirements-matrix.md`：赛题要求对照。
- `docs/manual-testing-guide.md`：手动测试指南。
- `docs/todo.md`：任务清单。
- `HANDOFF.md`：当前接手说明、启动方式、验证路径和已知缺口。

## 当前 UI 状态

- `http://localhost:5173/` 默认进入门户首页。
- `http://localhost:5173/#workboard` 进入真实功能页。
- 门户首页已经从 `App.tsx` 拆到 `apps/web/src/components/PortalHome.tsx`。
- Workboard 保持原功能，但已统一为浅色工作台风格：左侧 workspace、GitHub Issues、Work item 列表、右侧 rail 和状态卡片保持同一视觉体系。

## 当前重点缺口

- Pipeline Template 仍需 App 内可编辑。
- Agent runner registry 仍需统一 Codex / opencode / Claude Code / demo runner。
- `run-devflow-cycle` 已改为默认异步后台 job：Run 立刻返回 `attempt`，前端靠轮询更新状态；`wait: true` 只作为测试和兼容路径。
- Human Review 已改为真实阻塞点：Review Agent 通过后停在 checkpoint，必须由用户 Approve 后才继续 merge / delivery。
- 仍需把当前 goroutine job 继续升级为正式 `AgentRunner + JobSupervisor`：heartbeat、stall detection、retry、cancel、timeout、多 turn continuation、worker host 分配和崩溃恢复。
- GitHub issue comment / label 回写仍需完成。
- PR lifecycle UI 仍需加强。
- Feishu 审核卡片和回调仍需完成。
