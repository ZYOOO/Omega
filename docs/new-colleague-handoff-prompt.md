# New Colleague Handoff Prompt

更新时间：2026-05-06（Asia/Shanghai）

下面这段可以直接发给新的 AI coding agent，让对方快速进入 Omega 当前上下文。

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
10. docs/agent-skills-and-mcp.md
11. docs/data-ownership-and-read-model.md
12. docs/github-actions-ci-chain.md
13. docs/feishu-review-chain.md
14. docs/feishu-bot-permissions.md
15. docs/manual-testing-needed.md
16. docs/page-pilot-architecture.md

当前状态：
- Omega 是 local-first AI DevFlow 产品：Requirement -> Work Item -> Pipeline -> Agent -> branch/commit/PR/checks/review/human gate/proof。
- Work Item / Agent 执行必须锁定明确 Repository Workspace，在隔离 workspace 中运行，不能误写 Omega 自身或其他仓库。
- 默认 DevFlow 由 workflow contract state runner 驱动，主要阶段通过 action executor 执行。
- Run Workpad 是一等视图，详情页优先展示 Plan、Acceptance Criteria、Validation、Review Packet、Blockers、Retry Reason、Notes、PR。
- Workboard 默认展示 Not Started / Running / Human Review / Blocked / Done；Auto run 应同时覆盖 GitHub ready issue 和 Omega 内部 Not Started Work Item。
- DevFlow 已包含 Plan/Architect 阶段、功能 TODO、项目 TODO、GitHub Actions CI、Code Review、Rework、Human Review、Merging、Done。
- Human Review approve/request changes 统一走 checkpoint decision path；Feishu callback、task bridge、lark-cli current-user fallback 都应同步同一条后端决策链路。
- 飞书 Task 完成可以等于 approve，但必须真实完成；status=todo 或 completed_at=0 不能 approve。
- Feishu Connections 的 on 表示至少有一条可用投递路由，例如 lark-cli current-user fallback；不要误解为所有 chat/task/webhook 都已配置。
- 飞书文本消息不支持 Markdown；普通消息用纯文本和合适 emoji，只有创建飞书文档时才使用 Markdown。
- Page Pilot 已接入 Electron direct pilot 和 Web fallback，可圈选真实 DOM、提交修改、刷新预览、Confirm/Discard，并物化到 Work Item / Pipeline。
- Page Pilot Repository source 不再伪造默认 URL：package.json 项目应启动 Preview Runtime Agent，纯静态项目才自动打开 workspace index.html。
- Global Agent Access 支持 Codex、Claude Code、opencode、Trae Agent。Codex/Claude 主要继承本机 CLI；opencode/Trae 支持 provider/model/base URL/API key。
- Kimi / Moonshot 已作为 OpenAI-compatible provider 支持；runner model 候选应来自真实 discovery 或用户输入，不要写死静态假列表。
- Stage 级 Skills / MCP 已真实物化到 runner workspace 的 .omega/.codex/.claude 文件和环境变量，不能只改 UI。
- UI 支持 English / 简体中文；产品页面文本要走 i18n，用户输入、Agent 输出、proof、文档内容不要强行翻译。
- UI 会话恢复走 GET /workspace?scope=session；后端 session read model 从 SQLite 规范化表组装，不应反序列化完整 workspace snapshot。
- Work Item 详情页 live refresh 已按当前 Work Item / Pipeline / Repository Workspace 收缩；Attempt Timeline 按 attempt id 读取规范化表，不要重新接回 full snapshot。

重要原则：
- 不要做假 UI。圈选、源码定位、代码修改、热更新、diff、PR、review、proof 都要尽量落到真实数据。
- 不要把无关产品名写入文档、代码或 UI。
- App.tsx 和 server.go 仍要继续拆；新增能力优先拆组件/模块。
- UI 修改要兼顾 light / dark 和中英文。
- 新功能更新 feature implementation log / development log / todo；修 bug 更新 bug log。
- full workspace snapshot 仍是兼容层，不要把 live polling、详情页 timeline、JobSupervisor idempotency 重新接回 full /workspace。
- 当前工作树很大，先读 diff 和 docs，不要盲目 revert。

建议下一步：
1. 新建一个复杂 Requirement 回归 DevFlow 全闭环：Not Started 自动运行、Plan/TODO、Coding、CI、Review、Human Review、Feishu approve、Merging、Done、proof。
2. 修复 runner model discovery / preflight 的命令超时和测试隔离，避免 Go 全量测试被本机真实 CLI 拖住。
3. 继续观察 /workspace?scope=session、/run-workpads、/pipelines、/attempts、/attempts/{id}/timeline 响应体积，避免 read model 回退到 full snapshot。
4. 对 Page Pilot isolated-devflow mode 做真实手测闭环：Dev server by Agent、HTML file、Confirm、Discard、PR、proof。
5. 继续收敛 Work Item 详情页状态，让 UI 直接消费 canonical pipeline/action plan，减少前端自推断。
6. 继续拆 App.tsx 的 Workboard list/detail、Inspector、Settings/Agent Studio。
7. 继续拆 Go runtime 中 server.go 残留 handler 和 action handler。
8. 为 stdout/stderr/runner details 等执行日志增加长期 retention，避免 .omega/omega.db 再次膨胀。

建议先跑轻量验证：
npm run test -- apps/web/src/__tests__/omegaControlApiClient.test.ts --testTimeout=30000
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx -t "renders a workpad-first detail page" --testTimeout=60000
go test ./services/local-runtime/internal/omegalocal -run TestAttemptTimeline -count=1 -timeout=60s
git diff --check

不要默认先跑 go test ./services/local-runtime/internal/omegalocal -count=1。当前全量测试会被真实 runner model discovery / orchestrator 后台 job 拖慢，曾出现 10 分钟超时；先把相关测试 mock / timeout 做好。

启动方式：
npm run local-runtime:dev
npm run web:dev
npm run desktop:dev

默认地址：
Go local runtime: http://127.0.0.1:3888
Web UI: http://127.0.0.1:5173
Electron: Omega AI Delivery Engine
```
