# Omega Handoff

更新时间：2026-05-06（Asia/Shanghai）

本文是 Omega 当前阶段的交接说明。接手者应先读本文，再读 README 和核心 docs，最后按文末验证路径启动服务、回归 DevFlow / Page Pilot / Feishu。

## 1. 当前一句话

Omega 是一个 local-first 的 AI DevFlow 产品：用户在桌面 App / Web UI 中输入 Requirement，系统将其转成 Work Item，锁定明确 Repository Workspace，在隔离 workspace 中按 workflow contract 编排 Agent，产出代码修改、branch、commit、PR、review、human gate、proof，并把过程沉淀为可审计的 Run Workpad。

当前重点已经从“跑通代码生成”进入到“稳定、可恢复、可审计、可配置、可解释地交付”。最近一轮主要围绕多 runner 启动、全局语言、Feishu 审核、GitHub Actions CI、stage Skills/MCP、SQLite read model、Work Item 详情页体验和运行态性能收缩。

## 2. 当前主线状态

### DevFlow / Workboard

已具备本地闭环：

- React SPA / Electron 壳可打开 Projects、Workboard、Page Pilot、Settings、Work Item 详情。
- Requirement 创建后会绑定明确 Repository Workspace，Agent 执行不能误写其他仓库。
- Go local runtime 负责 SQLite、workflow 编排、runner、workspace、GitHub 出站、Human Review gate、proof、Feishu 通知。
- 默认 DevFlow 由 workflow contract state runner 驱动，主要阶段通过 action executor 执行。
- Run Workpad 是一等视图：Plan、Acceptance Criteria、Validation、Review Packet、Blockers、Retry Reason、Notes、PR 等字段会被持续更新。
- Work Item 详情页优先消费后端 canonical pipeline / action plan，减少前端自行推断状态。
- Workboard 默认展示 Not Started / Running / Human Review / Blocked / Done 五个流程栏；空栏默认折叠，但用户可展开理解完整流程。
- Auto run 现在含义是自动扫描 GitHub ready issue 和 Omega 内部 Not Started Work Item；开启后会触发 orchestrator tick。
- Done / Running / Human Review 的列表显示已修过多轮，但继续测试时仍要以 pipeline stage + checkpoint 状态为准，不要只看单个 badge。

### Agent / Runner

当前支持四类 runner：

- Codex：使用本机 Codex CLI 登录态；全局 Agent Access 可保存默认模型；模型发现读取本机 Codex 配置 / `models_cache.json`。
- Claude Code：继承本机 Claude CLI 配置；旧模板里的 `gpt-5.4-mini` 会映射为继承本地默认，不再强行覆盖。
- opencode：支持用户配置 provider / model / base URL / API key；运行前生成 attempt-scoped `OPENCODE_CONFIG`。
- Trae Agent：支持用户配置 provider / model / base URL / API key；运行前生成 attempt-scoped `trae_config.yaml` 并传 `--config-file`。

Provider 支持包括 OpenAI-compatible provider、OpenRouter、DeepSeek、Qwen、Kimi / Moonshot。Kimi 默认 base URL 为 Moonshot OpenAI-compatible API。Global Agent Access 四个 runner 都有真实 Test connection；账号型 runner 会验证 provider `/models`。

重要：Workspace Agent Studio 的 stage model 字段现在可以为空，表示继承全局 runner 默认。不要再把静态 preset 当成真实可用模型列表。

### Skills / MCP

Stage 级 Skills / MCP 已不是单纯 UI 字段：

- runner 启动前会物化 `.omega/agent-capabilities.json`、`.omega/agent-capabilities.md`、`.codex/OMEGA.md` 和 `.claude/CLAUDE.md`。
- 子进程会注入 `OMEGA_AGENT_SKILLS` / `OMEGA_AGENT_MCP` 等环境变量。
- 默认安装和用途见 `docs/agent-skills-and-mcp.md`。

下一轮如果继续改 Agent Studio，要真实影响 runner launch，不要只改 UI。

### CI / GitHub Actions

GitHub Actions CI 已进入 DevFlow：

- 默认模板包含 Plan / Architect 阶段，要求输出 technical plan、functional todo list、project todo list。
- PR 创建或更新后会采集 `gh pr checks` 和失败 run log。
- CI 结果会进入 proof、Review Packet、Run Workpad 和 Rework Checklist。
- 设计与权限说明见 `docs/github-actions-ci-chain.md`。

### Feishu / Human Review

当前 Feishu 逻辑：

- Human Review approve / request changes 统一走 checkpoint decision path。
- Feishu review callback、task bridge 和 `lark-cli` 当前用户 fallback 都应同步同一条后端决策链路。
- Feishu Connections 的 `on` 表示至少有一条可用投递路由，例如 `lark-cli` current-user fallback；不代表 chat/task/webhook 全部配置。
- 飞书文本消息不使用 Markdown；只有创建飞书文档时才使用 Markdown。
- 飞书 Task 完成可以等于 approve，但必须真的完成。`status=todo` 或 `completed_at=0` 不算完成。

常见回归点：用户在飞书没有完成 task 时，Omega 不能误 approve。

### Page Pilot

Page Pilot 已具备 Electron direct pilot + Web fallback：

- Electron 内可打开目标项目页面，注入 Page Pilot 悬浮控件，圈选真实 DOM。
- 圈选会采集 selector、DOM context、style snapshot、source context、用户批注。
- Page Pilot Agent 在目标 repo 或 isolated workspace 中修改代码，修改后刷新预览。
- 用户可 Confirm / Discard；Confirm 后物化到 Work Item / Pipeline，并记录 proof / diff / linkage。
- `Repository source` 不再伪造默认 preview URL；package.json 项目会启动 Preview Runtime Agent，纯静态项目才打开 workspace `index.html`。
- Repository Workspace 删除时会同步清理 Page Pilot run、preview runtime、execution lock 和用户勾选的本地 attempt workspace。

继续测试 Page Pilot 时要覆盖：Dev server by Agent、HTML file、Confirm、Discard、PR、proof。

### Runtime / SQLite / 性能

当前数据路线：

- UI 会话恢复走 `GET /workspace?scope=session`。
- 后端 session read model 从 SQLite 规范化表组装，不应反序列化完整 workspace snapshot。
- Full workspace snapshot 只作为兼容、导出和灾难恢复层。
- Work Item create / patch / delete、repository target bind/delete/import、workflow template、Agent Profile、Page Pilot、DevFlow action、Feishu task bridge、JobSupervisor 等路径已逐步收口到规范化表和局部 writer。
- runtime logs 已去噪：成功 GET/HEAD/OPTIONS 不默认落库；高频诊断写 `.omega/logs` daily JSONL，保留约 1 天。
- Work Item 详情页 live refresh 已按当前 Work Item / Pipeline / Repository Workspace 收缩执行态读取；Attempt Timeline 按 attempt id 从规范化表组装上下文。

重要：不要把 live polling、详情页 timeline、JobSupervisor idempotency 重新接回 full `/workspace` 或 full supervisor snapshot。

## 3. 主要目录

前端：

```text
apps/web/src/App.tsx
apps/web/src/components/GlobalAgentAccessPanel.tsx
apps/web/src/components/WorkItemDetailPage.tsx
apps/web/src/components/WorkItemDetailPanels.tsx
apps/web/src/components/PagePilotPreview.tsx
apps/web/src/components/WorkspaceAgentStudio.tsx
apps/web/src/components/WorkspaceChrome.tsx
apps/web/src/i18n.tsx
apps/web/src/omegaControlApiClient.ts
apps/web/src/workspaceApiClient.ts
apps/web/src/styles.css
```

桌面壳：

```text
apps/desktop/src/main.cjs
apps/desktop/src/process-supervisor.cjs
apps/desktop/src/omega-preload.cjs
```

Go runtime：

```text
services/local-runtime/cmd/omega-local-runtime/main.go
services/local-runtime/internal/omegalocal/server.go
services/local-runtime/internal/omegalocal/server_routes.go
services/local-runtime/internal/omegalocal/devflow_cycle.go
services/local-runtime/internal/omegalocal/devflow_ci.go
services/local-runtime/internal/omegalocal/job_supervisor.go
services/local-runtime/internal/omegalocal/orchestrator.go
services/local-runtime/internal/omegalocal/agent_runner.go
services/local-runtime/internal/omegalocal/runner_credentials.go
services/local-runtime/internal/omegalocal/runner_model_discovery.go
services/local-runtime/internal/omegalocal/runner_preflight.go
services/local-runtime/internal/omegalocal/feishu_review.go
services/local-runtime/internal/omegalocal/feishu_review_task.go
services/local-runtime/internal/omegalocal/page_pilot.go
services/local-runtime/internal/omegalocal/sqlite.go
services/local-runtime/internal/omegalocal/sqlite_table_reads.go
services/local-runtime/internal/omegalocal/sqlite_migrations.go
services/local-runtime/workflows/devflow-pr.md
```

重要文档：

```text
README.md
docs/latest-architecture.md
docs/current-product-design.md
docs/development-plan.md
docs/todo.md
docs/feature-implementation-log.md
docs/development-log.md
docs/bug-log.md
docs/agent-skills-and-mcp.md
docs/data-ownership-and-read-model.md
docs/github-actions-ci-chain.md
docs/feishu-review-chain.md
docs/feishu-bot-permissions.md
docs/manual-testing-needed.md
docs/page-pilot-architecture.md
docs/new-colleague-handoff-prompt.md
```

## 4. 启动方式

开发期推荐三个进程：

```bash
npm install
npm run local-runtime:dev
npm run web:dev
npm run desktop:dev
```

默认地址：

```text
Go local runtime: http://127.0.0.1:3888
Web UI:           http://127.0.0.1:5173
Electron:         Omega AI Delivery Engine
```

如果端口被占用，先查进程：

```bash
lsof -nP -iTCP:3888 -sTCP:LISTEN
lsof -nP -iTCP:5173 -sTCP:LISTEN
```

不要直接删除 `.omega` 或 workspace 数据。当前 `.omega/omega.db` 是真实本地状态，里面有 Work Item、Pipeline、Page Pilot run、proof record、runner credential metadata 和 Feishu config。

## 5. 常用验证

轻量验证：

```bash
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=30000
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx -t "renders a workpad-first detail page" --testTimeout=60000
go test ./services/local-runtime/internal/omegalocal -run TestAttemptTimeline -count=1 -timeout=60s
git diff --check
```

前端重点验证：

```bash
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=30000
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/__tests__/desktopProcessSupervisor.test.ts --testTimeout=30000
```

Go 定向验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuReviewTaskSyncApprovesCompletedTask|TestFeishuReviewTaskSyncDoesNotApproveTodoTaskWithZeroCompletedAt|TestFeishuReviewCallbackApprovesCheckpointThroughSharedDecisionPath' -count=1
go test ./services/local-runtime/internal/omegalocal -run 'TestRunnerModelDiscoveryReadsCodexLocalModelCache|TestAgentRunnerPreflightUsesCodexSavedModelAsDefault|TestRunnerModelDiscoveryKnowsKimiBaseURL' -count=1
go test ./services/local-runtime/internal/omegalocal -run 'TestRunDevFlowPRCycle|TestDevFlowCI|TestAttemptTimeline' -count=1
```

说明：当前 `go test ./services/local-runtime/internal/omegalocal -count=1` 全量测试会被真实 runner model discovery / orchestrator 后台 job 拖慢，曾出现 10 分钟超时。下一位 Agent 应优先把 runner discovery 测试改成 mock command 或短超时，再恢复全量测试作为常规门禁。

## 6. 当前已知风险

- `App.tsx` 和 `server.go` 仍偏大；新增能力优先拆组件/模块，不要继续堆回去。
- Full workspace snapshot 仍作为兼容层存在；热路径必须继续迁到 SQLite read model。
- Runner model discovery 仍可能碰真实 CLI 命令，测试和运行时都需要更严格的 timeout / cancellation。
- `.omega/omega.db` 已经过日志去噪，但 stdout/stderr/runner details 等长期 retention 还需继续治理。
- Feishu task bridge 已修过误 approve，但需要更多真实账号场景回归。
- Page Pilot isolated-devflow mode 已能跑基础链路，但多项目、多框架、复杂 source mapping 仍需真实手测。
- Work Item list/detail 状态同步仍要继续观察，尤其是 Feishu approve 后外层列表是否快速从 Human Review / Running 转到 Merging / Done。
- UI 已加入简体中文，但仍可能有新增组件漏翻译；产品文案要按语境翻译，不要机械直译。

## 7. 下一步优先级

P0：

- 新建一个复杂 Requirement 复测 DevFlow 全闭环：Not Started 自动运行、Plan/TODO、Coding、CI、Review、Human Review、Feishu approve、Merging、Done、proof。
- 修复 runner model discovery / preflight 的命令超时和测试隔离，避免 Go 全量测试被本机真实 CLI 拖住。
- 继续观察详情页轮询：`/workspace?scope=session`、`/run-workpads`、`/pipelines`、`/attempts`、`/attempts/{id}/timeline` 不应回退到 full snapshot。
- Page Pilot isolated-devflow mode 完整手测：隔离修改、Confirm、PR、Discard、proof。

P1：

- Runtime 继续拆 `server.go` 残留 handler 和 DevFlow action handler。
- Workboard list/detail、Inspector、Settings/Agent Studio 继续拆组件。
- 为 runtime stdout / stderr / runner details 增加按天或按数量 retention。
- Workspace Agent Studio 的 workflow / prompt / agent / skills 配置继续产品化，补校验和真实 preview。

P2：

- 多端协作、shared sync、授权模型。
- 代码库语义索引。
- 更完整的 package / release / desktop auto update。

## 8. 接手建议

接手后不要先大改 UI。建议顺序：

1. 阅读本文、README、`docs/latest-architecture.md`、`docs/current-product-design.md`、`docs/development-plan.md`、`docs/todo.md`。
2. 启动 runtime / web / desktop。
3. 在 `ZYOOO/TestRepo` 跑一个 Requirement 到 Human Review，确认 repo target、workspace、branch、PR、CI、proof 都是真实的。
4. 在 Feishu 私聊里完成 Human Review task 或 Approve，确认 Omega UI、checkpoint、pipeline、attempt 同步变化。
5. 用 Page Pilot 打开同一 repo，分别测 Dev server by Agent 和 HTML file。
6. 如果发现卡顿，先看 Network payload 和 live polling scope，再查 SQLite read model，不要第一时间改 UI 动画。
