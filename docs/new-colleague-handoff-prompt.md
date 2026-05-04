# New Colleague Handoff Prompt

更新时间：2026-05-04（Asia/Shanghai）

下面这段可以直接发给新同事或新的 AI coding agent，让对方快速进入 Omega 当前上下文。

```text
你现在接手 Omega 项目，工作目录是 /Users/zyong/Projects/Omega。

请先阅读：
1. HANDOFF.md
2. README.md
3. docs/latest-architecture.md
4. docs/current-product-design.md
5. docs/development-plan.md
6. docs/todo.md
7. docs/feature-implementation-log.md
8. docs/development-log.md
9. docs/bug-log.md
10. docs/feishu-review-chain.md
11. docs/feishu-bot-permissions.md
12. docs/manual-testing-needed.md
13. docs/page-pilot-architecture.md

当前状态：
- 功能一 DevFlow / Workboard 已进入产品化核心阶段：默认链路由 workflow contract state runner 驱动，主要阶段通过 action executor 执行。
- Requirement / Work Item 必须锁定明确 Repository Workspace，在隔离 workspace 中跑 Agent，生成 branch / commit / PR / review / human gate / proof。
- Run Workpad 是一等视图，详情页优先展示 Plan、Acceptance Criteria、Validation、Review Packet、Blockers、Retry Reason、Notes、PR。
- JobSupervisor 支持 heartbeat、stall detection、retry/cancel/timeout、checkpoint 恢复和 proof-backed Human Review recovery。
- Human Review approve/request changes 统一走 checkpoint decision path；Feishu review callback、task bridge 和 lark-cli 当前用户 fallback 都应同步同一条后端决策链路。
- Feishu Connections 的 on 表示至少有一条可用投递路由，例如 lark-cli current-user fallback；不要误解为所有 chat/task/webhook 都已配置。
- Page Pilot 已接入 Electron direct pilot 和 Web fallback，可圈选真实 DOM、提交修改、刷新预览、Confirm/Discard，并物化到 Work Item / Pipeline。
- Page Pilot Repository source 不再伪造默认 URL：package.json 项目应启动 Preview Runtime Agent，纯静态项目才自动打开 workspace index.html。
- Workspace Agent Studio 已支持 workflow、prompt、agent runner/model、skills、MCP、runtime files 的基础配置和样例导入。
- UI 会话恢复走 GET /workspace?scope=session；后端 session read model 从 SQLite 规范化表组装，不应反序列化完整 workspace snapshot。
- runtime logs 已去噪：成功 GET/HEAD/OPTIONS 不默认落库，高频诊断写 .omega/logs daily JSONL，保留约 1 天。

重要原则：
- 不要做假 UI。圈选、源码定位、代码修改、热更新、diff、PR、review、proof 都要尽量落到真实数据。
- Work Item / Agent 执行必须锁定明确 Repository Workspace，不能误写其他仓库。
- 不要把无关产品名写入文档、代码或 UI。
- App.tsx 和 server.go 仍要继续拆；新增能力优先拆组件/模块。
- UI 修改要兼顾 light / dark。
- 新功能更新 feature implementation log / development log / todo；修 bug 更新 bug log。
- full workspace snapshot 仍是兼容层，不要把 live polling 或详情页热路径重新接回 full /workspace。

建议下一步：
1. 用 OMG-32 或新 Work Item 回归 Feishu Approve：点 Approve 后 Omega UI 应快速进入 Merging/Done，不应继续显示可点击 Approve。
2. 继续观察 /workspace?scope=session、/run-workpads、/pipelines、/attempts 响应体积，避免 session read model 回退到 full snapshot。
3. 对 Page Pilot isolated-devflow mode 做真实手测闭环：Dev server by Agent、HTML file、Confirm、Discard、PR、proof。
4. 继续收敛 Work Item 详情页状态，让 UI 直接消费 canonical pipeline/action plan，减少前端自推断。
5. 继续拆 App.tsx 的 Workboard list/detail、Inspector、Settings/Agent Studio。
6. 继续拆 Go runtime 中 server.go 残留 handler 和 action handler。
7. 为 stdout/stderr/runner details 等执行日志增加长期 retention，避免 .omega/omega.db 再次膨胀。

建议先跑验证：
go test ./services/local-runtime/internal/omegalocal -count=1
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=30000
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/__tests__/desktopProcessSupervisor.test.ts --testTimeout=30000
git diff --check

启动方式：
npm run local-runtime:dev
npm run web:dev
npm run desktop:dev

默认地址：
Go local runtime: http://127.0.0.1:3888
Web UI: http://127.0.0.1:5173
Electron: Omega AI Delivery Engine
```
