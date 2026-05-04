# Omega Handoff

更新时间：2026-05-04（Asia/Shanghai）

本文是 Omega 当前阶段的交接说明。接手同事应先读本文，再读 README 和核心 docs，最后按文末验证路径启动服务、回归 DevFlow / Page Pilot / Feishu。

## 1. 当前一句话

Omega 是一个 local-first 的 AI DevFlow 产品：用户在桌面 App / Web UI 中输入 Requirement，系统将其转成 Work Item，锁定明确 Repository Workspace，在隔离 workspace 中按 workflow contract 编排 Agent，产出代码修改、branch、commit、PR、review、human gate、proof，并把过程沉淀为可审计的 Run Workpad。

当前重点已经从“能不能生成代码”进入到“能不能稳定、可恢复、可审计、可配置、可解释地交付”。最近一轮主要围绕 Page Pilot 可用性、Human Review/Feishu 审核、Work Item 状态一致性、UI 可读性和 runtime 性能治理。

## 2. 当前主线状态

### DevFlow / Workboard

已具备本地闭环：

- React SPA / Electron 壳可打开 Projects、Workboard、Page Pilot、Settings、Work Item 详情。
- Requirement 创建后会绑定明确 Repository Workspace，Agent 执行不能误写其他仓库。
- Go local runtime 负责 SQLite、workflow 编排、runner、workspace、GitHub 出站、Human Review gate、proof、Feishu 通知。
- 默认 DevFlow 已由 workflow contract state runner 驱动，主要阶段通过 action executor 执行。
- Run Workpad 是一等视图：Plan、Acceptance Criteria、Validation、Review Packet、Blockers、Retry Reason、Notes、PR 等字段会被持续更新。
- JobSupervisor 支持 heartbeat、stall detection、retry、cancel、timeout、workspace cleanup、worker host lease、checkpoint 恢复和 proof-backed Human Review recovery。
- Human Review 进入等待后会生成 checkpoint、review packet、PR 信息和 proof；Approve 后进入 Merging，Request changes 会归并 feedback 并进入 Rework。
- Feishu 支持 `lark-cli` 当前用户 fallback，也支持显式 chat/task/webhook 路由；review/failure 通知会走真实路由，不再只停留在 UI 绑定状态。
- Work Item 详情页优先消费后端 canonical pipeline / action plan，减少前端自行推断状态。

最近修复重点：

- 修复 Delivery flow 同时显示多个活动阶段的问题。后端把 `pipeline.run.stages` 作为当前运行态权威，读写链路都做 canonicalization，避免 Implementation / Human Review / Done 同时高亮。
- 修复 Human Review approve 后仍显示 Approve、响应慢的问题。checkpoint 决策加入互斥、idempotency、duplicate checkpoint 去重和 stale supervisor snapshot 保存保护。
- 修复 Feishu approve/task bridge、callback 与本地 checkpoint 决策路径不一致的问题。Feishu 审核统一走 shared checkpoint decision path。
- 修复 Work Item 列表状态分组和展示噪音：Human Review 独立分组，Blocked 变为黄色且前置，Page Pilot item 使用 Page Pilot 编号，UI 不再显示 `item_manual_*` 这类内部 id。
- Work Item 详情页的 proof artifacts 支持点击预览，Markdown/patch/text 走本地 runtime preview，不再把 HTML 404 当 JSON 解析。

### Page Pilot

已具备 Electron direct pilot + Web fallback 的真实链路：

- Electron 内可打开目标项目页面，注入 Page Pilot 悬浮控件，圈选真实 DOM。
- 圈选会采集 selector、DOM context、style snapshot、source context、用户批注。
- Page Pilot Agent 在目标 repo 或 isolated workspace 中修改代码，修改后刷新预览。
- 用户可 Confirm / Discard；Confirm 后物化 `source=page_pilot` 的 Requirement / Work Item / Pipeline，并记录 proof / diff / linkage。
- Preview Runtime Agent 可按 repo 推断/启动 dev server profile；`HTML file` 模式可从当前 Repository Workspace 自动寻找根目录 `index.html`。

最近修复重点：

- `Repository source` 不再伪造默认 preview URL；package.json 项目会启动 Preview Runtime Agent，纯静态项目才打开 `index.html`。
- `Dev server by Agent` 支持完整 URL path/query/hash；已有可访问 URL 会按 external-url 接入，不再强制 clone / prepare。
- Electron `openPreview` 对主 frame HTTP 404 / load fail 做校验，失败时销毁 BrowserView，避免错误页盖住 Omega 主界面。
- Web 模式恢复 iframe + overlay fallback，适合不用 Electron 时调试。
- Page Pilot 页面移除无意义 AI 浮动按钮，连接/设置入口回到主侧边栏与 Settings。
- Page Pilot recent runs、status、preview source、light/dark UI 已做基础收敛。

### Runtime / 性能

最近修复重点：

- `runtime_logs` 去噪：成功 GET/HEAD/OPTIONS 不再默认落库；高频诊断写 `.omega/logs/omega-runtime-diagnostics.YYYY-MM-DD.jsonl`，保留约 1 天。
- migration compact 旧的 `api.request` / supervisor tick / remote poll 噪音日志，避免几十万 rows 拖慢 UI。
- UI 会话恢复改走 `GET /workspace?scope=session`。
- 后端 session read model 现在从 SQLite 规范化表组装，不再反序列化完整 `workspace_snapshots.database_json` 后裁剪。
- `work_items.record_json` 保存完整 Work Item 业务记录，session 视图可保留 `source`、`repositoryTargetId`、Page Pilot 元信息。
- `/run-workpads`、`/pipelines`、`/attempts`、带 filter 的 `/operations` / `/proof-records` / `/checkpoints` 已有规范化表读取路径，减少 full snapshot fallback。

本地实测当前库：

```text
/workspace?scope=session 约 464KB / 0.26s
/workspace              约 14.7MB / 0.45s
```

## 3. 主要目录

前端：

```text
apps/web/src/App.tsx
apps/web/src/components/WorkItemDetailPage.tsx
apps/web/src/components/WorkItemDetailPanels.tsx
apps/web/src/components/PagePilotPreview.tsx
apps/web/src/components/WorkspaceChrome.tsx
apps/web/src/omegaControlApiClient.ts
apps/web/src/workspaceApiClient.ts
apps/web/src/styles.css
```

桌面壳：

```text
apps/desktop/src/main.cjs
apps/desktop/src/process-supervisor.cjs
apps/desktop/src/pilot-preload.cjs
```

Go runtime：

```text
services/local-runtime/cmd/omega-local-runtime/main.go
services/local-runtime/internal/omegalocal/server.go
services/local-runtime/internal/omegalocal/server_routes.go
services/local-runtime/internal/omegalocal/devflow_cycle.go
services/local-runtime/internal/omegalocal/job_supervisor.go
services/local-runtime/internal/omegalocal/feishu_review.go
services/local-runtime/internal/omegalocal/page_pilot_preview_runtime.go
services/local-runtime/internal/omegalocal/pipeline_records.go
services/local-runtime/internal/omegalocal/runtime_logs.go
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

不要直接删除 `.omega` 或 workspace 数据。当前 `.omega/omega.db` 是真实本地状态，里面有 Work Item、Pipeline、Page Pilot run、proof record 和 Feishu config。

## 5. 常用验证

前端：

```bash
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=30000
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/__tests__/desktopProcessSupervisor.test.ts --testTimeout=30000
npm run build
```

Go：

```bash
go test ./services/local-runtime/internal/omegalocal -count=1
```

本轮重点验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestWorkspaceSessionScopeOmitsExecutionHeavyTables|TestNormalizeDevFlowPipelineStageStatusesKeepsSingleActiveStage|TestCheckpointDecisionDeduplicatesDuplicateCheckpointIDs|TestFeishuReviewCallbackApprovesCheckpointThroughSharedDecisionPath|TestFeishuReviewTaskBridgeApprovesCheckpoint' -count=1
npm test -- --run apps/web/src/__tests__/App.operatorView.test.tsx apps/web/src/__tests__/omegaControlApiClient.test.ts
```

说明：完整 Go 测试里曾出现长运行 async settling 超时；最近复核更像高负载放大的观察窗口问题，不是稳定状态机错误。遇到失败时先看 runtime logs、attempt events、pipeline snapshot、proof 和 JobSupervisor tick，不要只看 UI。

## 6. Feishu 当前接入方式

当前优先支持本地 `lark-cli`：

- App ID / App Secret 用于拿 tenant token。
- `lark-cli` 登录用户可作为默认 review/failure 接收人。
- 如果配置了 chat id / task assignee / webhook，则优先走显式路由。
- 如果未配置显式路由，但 `lark-cli` 可以解析当前用户，review/failure fallback 到当前用户私聊。
- 侧边栏 Connections 的 Feishu `on` 表示至少有一条可用投递路由，不代表所有 chat/task/webhook 都已配置。

常见排查：

```bash
lark-cli im +messages-send --as bot --user-id <open_id> --msg-type text --content '{"text":"Omega test"}'
curl -s -X POST http://127.0.0.1:3888/feishu/test
curl -s -X POST http://127.0.0.1:3888/job-supervisor/tick -H 'content-type: application/json' -d '{"limit":10}'
```

权限说明见：

```text
docs/feishu-bot-permissions.md
docs/feishu-review-chain.md
```

## 7. Workspace / Repo 原则

- Work Item / Agent 执行必须锁定明确 Repository Workspace。
- 对真实 repo 的写入应通过隔离 workspace、branch、commit、PR、review、proof 形成审计链路。
- Page Pilot 不允许只做假 UI；圈选、源码定位、代码修改、热更新、diff、PR、review、proof 都要尽量落到真实数据。
- `HTML file` 可以从当前 Repository Workspace 自动解析，但不能跨 repo 猜路径。
- App UI 修改要同时看 light / dark。
- 新功能更新 feature implementation log / development log / todo；修 bug 更新 bug log。

## 8. 当前已知风险

- `App.tsx` 和 `server.go` 已继续拆出模块，但仍偏大；新增能力优先落到组件/模块，不要继续把逻辑堆回去。
- full snapshot 仍作为兼容层存在；热路径应继续迁到规范化 SQLite read model。
- `.omega/omega.db` 已经过日志去噪，但 stdout/stderr/runner details 等长期 retention 还需继续治理。
- Feishu 双向审批已可走当前用户 fallback / task bridge，但公网 callback、task、chat 多路由需要更多真实账号场景测试。
- Page Pilot isolated-devflow mode 已能跑基础链路，但多项目、多框架、复杂 source mapping 仍需真实手测。
- Work Item 详情页已减少前端推断，但后续还应继续让 UI 直接消费 action plan / canonical pipeline。

## 9. 下一步优先级

P0：

- 用 OMG-32 / 新 Work Item 复测 Human Review approve：Feishu 点 Approve 后 UI 应快速进入 Merging/Done，不再保留可点击 Approve。
- 继续观察 `/workspace?scope=session`、`/run-workpads`、`/pipelines` 等热路径响应体积，避免 live polling 回退到 full snapshot。
- Page Pilot isolated-devflow mode 完整手测：隔离修改、Confirm、PR、Discard、proof。
- 把 Workboard list/detail、Inspector、Settings/Agent Studio 继续拆组件。

P1：

- Runtime 继续拆 `server.go` 残留 handler 和 DevFlow action handler。
- 为 runtime stdout / stderr / runner details 增加按天或按数量 retention。
- Workspace Agent Studio 的 workflow / prompt / agent / skills 配置继续产品化，补导入模板和校验。
- Observability dashboard 增强趋势、慢阶段、最近失败、runner 使用、checkpoint 等待时长。

P2：

- 多端协作、shared sync、授权模型。
- 代码库语义索引。
- 更完整的 package / release / desktop auto update。

## 10. 新同事接手建议

接手后不要先大改 UI。建议顺序：

1. 阅读本文、README、`docs/latest-architecture.md`、`docs/current-product-design.md`、`docs/development-plan.md`、`docs/todo.md`。
2. 启动 runtime / web / desktop。
3. 在 `ZYOOO/TestRepo` 跑一个 Requirement 到 Human Review，确认 repo target、workspace、branch、PR、proof 都是真实的。
4. 在 Feishu 私聊里 Approve 一个 Human Review，确认 Omega UI、checkpoint、pipeline、attempt 同步变化。
5. 用 Page Pilot 打开同一 repo，分别测 Dev server by Agent 和 HTML file。
6. 再决定继续拆前端、补 Go runtime 模块化，还是强化 Page Pilot source mapping。

判断功能是否真的完成时，不要只看 UI 的 Done：

- repo target 是否正确。
- workspace 是否在 Omega workspace root 内。
- branch / commit / PR 是否真实存在。
- review / human gate 是否真实阻塞并可恢复。
- proof / review packet / run workpad 是否有足够证据。
- Feishu / GitHub 出站是否有日志和失败兜底。
