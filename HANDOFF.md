# Omega Handoff

更新时间：2026-05-03（Asia/Shanghai）

本文是 Omega 当前阶段的交接说明。接手同事应先读本文，再按文末的验证路径启动服务和做手动测试。

## 1. 当前一句话

Omega 是一个 local-first 的 AI DevFlow 产品：用户在桌面 App / Web UI 中输入 Requirement，系统将其转成 Work Item，锁定明确 Repository Workspace，在隔离 workspace 中按可配置 workflow 编排 Agent，产出代码修改、branch、commit、PR、review、human gate、proof，并把过程沉淀为可审计的 Run Workpad。

当前重点已经从“能不能生成代码”进入到“能不能稳定、可恢复、可审计、可配置地交付”。

## 2. 两条主线状态

### 功能一：DevFlow / Workboard

已具备本地闭环：

- React SPA / Electron 壳可打开 Workboard、Settings、Work Item 详情。
- Requirement 创建后会绑定明确 Repository Workspace，不能误写其他仓库。
- Go local runtime 负责 SQLite、workflow 编排、runner、workspace、GitHub 出站、Human Review gate、proof。
- Workflow 已由 contract state runner 驱动，默认 DevFlow contract 可在 Workspace Agent Studio 中查看和编辑。
- 通用 action executor 已接管默认链路的主要阶段，包括 review / rework / human review / merging / delivery。
- Run Workpad 已是一等视图：Plan、Acceptance Criteria、Validation、Review Packet、Blockers、Retry Reason、Notes、PR 等字段会被持续更新。
- JobSupervisor 已支持 heartbeat、stall detection、retry、cancel、timeout、workspace cleanup、worker host lease、checkpoint 恢复和 proof-backed Human Review recovery。
- Human Review 进入等待后会生成 checkpoint、review packet、PR 信息和 proof；Approve 后进入 Merging，Request changes 会归并 feedback 并进入 Rework。
- Feishu 已支持通过 `lark-cli` 给当前登录用户发送 review / failure 通知；如果显式配置 chat/task 路由，也可走对应路由。
- Work Item 详情页对 active attempt 的 action plan / timeline 做轮询刷新，避免必须手动刷新才能看到阶段推进。
- GitHub 侧支持真实 branch / commit / PR / merge；CI/checks、PR comments、review feedback 已有基础采集和 rework checklist 聚合。

最近重点修复：

- Work Item 30 两次失败排查：
  - 第一次失败来自目标 repo 的 `git diff --check`，目标文件存在 trailing whitespace。
  - 第二次失败发生在 proof / review 已完成、PR 已创建后，worker orphan 标记早于 checkpoint 恢复，导致 UI 看到 stalled。
  - 已新增 proof-backed Human Review recovery：JobSupervisor 可从 `.omega/proof/human-review-request.md`、`handoff-bundle.json`、`attempt-review-packet.json` 恢复 checkpoint、PR、workspace、review packet，并重新发送 Feishu review 通知。
- Feishu 测试消息成功但 workflow 不发送的问题：已补 review/failure 自动发送路径和当前 `lark-cli` 用户 fallback。
- Work Item 详情页进度不会自动刷新的问题：已补 active attempt 轮询。

### 功能二：Page Pilot

已具备 Electron direct pilot MVP：

- 在 Electron 内打开目标项目页面。
- 注入 Page Pilot 悬浮控件，支持隐藏、展开、圈选真实 DOM 元素。
- 圈选会采集 selector、DOM context、style snapshot、source context、用户批注。
- 提交给 Go runtime 的 Page Pilot Agent 链路后，在目标 repo 或 isolated workspace 中修改代码。
- 修改后刷新预览，用户可 Confirm / Discard。
- Confirm 后会物化 `source=page_pilot` 的 Requirement / Work Item / Pipeline，并记录 proof / diff / linkage。
- Preview Runtime Agent 已有基础：可按 repo 推断/启动 dev server profile，减少固定端口假设。

仍需重点手动测：

- Electron 内 Page Pilot 打开真实 repo preview 的稳定性。
- isolated-devflow mode 下确认后回写目标仓库的边界。
- 多批选区、多轮对话、source mapping 覆盖率。

## 3. 主要目录

前端：

```text
apps/web/src/App.tsx
apps/web/src/components/WorkItemDetailPage.tsx
apps/web/src/components/WorkspaceAgentStudio.tsx
apps/web/src/components/PagePilotPreview.tsx
apps/web/src/omegaControlApiClient.ts
apps/web/src/missionControlWrites.ts
apps/web/src/styles.css
```

桌面壳：

```text
apps/desktop/src/main.cjs
apps/desktop/src/pilot-preload.cjs
```

Go runtime：

```text
services/local-runtime/cmd/omega-local-runtime/main.go
services/local-runtime/internal/omegalocal/server.go
services/local-runtime/internal/omegalocal/server_routes.go
services/local-runtime/internal/omegalocal/devflow_cycle.go
services/local-runtime/internal/omegalocal/devflow_delivery_actions.go
services/local-runtime/internal/omegalocal/devflow_rework_actions.go
services/local-runtime/internal/omegalocal/job_supervisor.go
services/local-runtime/internal/omegalocal/feishu.go
services/local-runtime/internal/omegalocal/feishu_review.go
services/local-runtime/internal/omegalocal/feishu_config.go
services/local-runtime/internal/omegalocal/runner_preflight.go
services/local-runtime/internal/omegalocal/sqlite.go
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
docs/workflow-contract-executor-plan.md
```

## 4. 启动方式

推荐在开发阶段使用三个进程：

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

如果端口被占用，先用 `lsof -nP -iTCP:<port> -sTCP:LISTEN` 查进程，不要直接删除 `.omega` 或 workspace 数据。

## 5. 常用验证

前端：

```bash
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
npm run build
```

Go：

```bash
go test ./services/local-runtime/internal/omegalocal
```

本轮新增/重点验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestJobSupervisorRecoversOrphanedHumanReviewAttempt|TestJobSupervisorRecoversProofBackedHumanReviewAttempt|TestFeishuAutoReviewFallsBackToCurrentLarkUser'
```

说明：完整 Go 测试中仍可能出现长运行异步测试超时，需要继续拆分为更稳定的接口级测试；不要把单次 async timeout 直接等同于功能链路失败，要结合 runtime logs、attempt events 和 proof 判断。

## 6. Feishu 当前接入方式

当前优先支持本地 `lark-cli`：

- App ID / App Secret 用于拿 tenant token。
- `lark-cli` 登录用户可作为默认 review/failure 接收人。
- 如果配置了 chat id / task assignee / webhook，则优先走显式路由。
- 如果未配置显式路由，但 `lark-cli` 可以解析当前用户，review/failure 会 fallback 到当前用户私聊。

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

## 7. Workspace / Repo 目录原则

面向用户的目录划分建议：

- 应用安装目录：存放 Omega 自己的 app、runtime、全局 DB、日志。
- Omega workspace root：默认 `~/Omega/workspaces`，存放每个 Work Item / Page Pilot 的隔离执行 workspace。
- Repository target：用户真实项目仓库，必须明确绑定。
- `.omega`：每个 workspace 和必要 repo target 内都可以有项目级配置、proof、runtime metadata。

原则：

- Work Item / Agent 执行必须锁定 Repository Workspace。
- 默认可以提供 `.omega` 配置，不要求用户一开始手写配置。
- 对真实 repo 的写入应通过隔离 workspace、branch、commit、PR 形成审计链路。

## 8. 当前已知风险

- `App.tsx` 已拆出一批组件和写入 API，但仍偏大，Workboard list/detail、Inspector、Settings 还应继续拆。
- `server.go` 已拆出 routes / action handler / migration 等文件，但仍有旧兼容逻辑，需要继续收敛。
- Workflow contract 已可驱动默认链路，但复杂自定义 workflow 的 UI 校验、版本化和回滚还不够完整。
- Feishu 当前可发送 review/failure，但双向审核同步仍以本地 sync / task bridge 为主，长连接事件桥还需要更多真实账号场景测试。
- Page Pilot 已接回旧版可用体验，但 isolated-devflow 和 preview runtime 的自动识别还需要更多真实项目覆盖。
- 完整 Go 测试存在少数长运行异步用例超时，需要继续拆成更确定的单元测试和接口测试。

## 9. 下一步优先级

P0：

- 用 Work Item 30 验证 proof-backed Human Review recovery：运行 JobSupervisor tick 后应恢复 checkpoint，并发送 Feishu review 通知。
- 完成 Work Item 详情页 action plan 消费收敛，减少 UI 自己推断状态。
- 把 Feishu review/failure 真实链路加入手动测试文档。
- 继续稳定 full Go test 中的长运行 async 用例。

P1：

- Page Pilot isolated-devflow mode 完整回归：隔离修改、确认回写、discard 清理、PR 摘要。
- Workspace Agent Studio 的 workflow / prompt / agent / skills 配置继续产品化，补导入模板和校验。
- Observability dashboard 继续做趋势、慢阶段、最近失败、runner 使用和 checkpoint 等待时长。

P2：

- 多端协作、shared sync、授权模型。
- 代码库语义索引。
- 更完整的 package / release / desktop auto update。

## 10. 新同事接手建议

接手后不要先大改 UI。建议顺序：

1. 阅读本文和 `docs/latest-architecture.md`。
2. 启动 runtime / web / desktop。
3. 对 `ZYOOO/TestRepo` 跑一次 Requirement 到 Human Review。
4. 对 Work Item 30 跑一次 `job-supervisor/tick`，确认 checkpoint recovery 和 Feishu 通知。
5. 再决定是否继续拆前端或补 Go runtime 模块化。

判断功能是否真的完成时，不要只看 UI 的 Done：

- repo target 是否正确。
- workspace 是否在 Omega workspace root 内。
- branch / commit / PR 是否真实存在。
- review / human gate 是否真实阻塞并可恢复。
- proof / review packet / run workpad 是否有足够证据。
- Feishu / GitHub 出站是否有日志和失败兜底。
