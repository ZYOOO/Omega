# Omega CLI

Omega CLI 是面向本地 operator 的命令行入口。它不直接读写 SQLite，也不绕过 runtime 执行代码修改；所有命令都通过 Go Local Runtime 的真实 HTTP API 完成。

## 目标

- 在不打开 Workboard UI 的情况下查看运行状态、日志、Work Item、Attempt 和 Human Review gate。
- 从命令行显式触发 Work Item 运行、Attempt retry / cancel、Checkpoint approve / request changes。
- 查看 Agent/operation 统计、Run Workpad、proof artifact，并刷新 GitHub PR status。
- 保持 CLI 和 UI 共用同一套 Runtime API、JobSupervisor、Repository Workspace、proof 和 runtime logs。

## 启动前提

先启动 Go Local Runtime：

```bash
npm run local-runtime:dev
```

默认 API 地址：

```text
http://127.0.0.1:3888
```

可以通过参数或环境变量切换：

```bash
npm run omega -- --api-url http://127.0.0.1:3888 status
OMEGA_API_URL=http://127.0.0.1:3888 npm run omega -- status
```

## 安装与命令入口

发布包 / 演示包会把 CLI 构建到：

```bash
dist/release/bin/omega
```

桌面 App 内也会包含同一份 CLI：

```text
Omega.app/Contents/Resources/bin/omega
```

使用 release binary：

```bash
dist/release/bin/omega --api-url http://127.0.0.1:3888 status
dist/release/bin/omega work-items list
dist/release/bin/omega attempts list --status failed
```

推荐安装成当前用户 PATH 下的 Go binary：

```bash
go install ./services/local-runtime/cmd/omega
```

安装后直接使用：

```bash
omega status
omega logs --level ERROR
omega work-items list --status Ready
omega operations list --work-item OMG-21
omega workpads list --work-item item_21
omega proof preview proof_123
```

开发期也可以不安装，直接通过 npm script 包装运行：

```bash
npm run omega -- <command>
```

或者构建到项目本地目录：

```bash
go build -o bin/omega ./services/local-runtime/cmd/omega
bin/omega status
```

## 当前命令

### Health / Status

```bash
npm run omega -- health
npm run omega -- status
```

- `health` 调用 `GET /health`。
- `status` 调用 `GET /observability`，展示 work item、pipeline、attempt、checkpoint、runtime log 和 attention summary。

### Runtime Logs

```bash
npm run omega -- logs
npm run omega -- logs --level ERROR --limit 50
npm run omega -- logs --event-type job_supervisor.tick.completed
npm run omega -- logs --requirement req_item_manual_21 --search approve --page
npm run omega -- logs --cursor <nextCursor> --page
```

调用 `GET /runtime-logs`。默认展示表格；`--requirement` 会按 Requirement 维度反查 Work Item / Pipeline / Attempt 关联日志，`--search` 做全文搜索，`--page` 返回 cursor 分页并显示 `nextCursor`。全局 `--json` 可输出原始 JSON：

```bash
npm run omega -- --json logs --level ERROR
```

### Work Items

```bash
npm run omega -- work-items list
npm run omega -- work-items list --status Ready
npm run omega -- work-items run OMG-21
```

`work-items list` 读取 `GET /workspace?scope=session` 中的 session read model Work Item 表，避免为了 CLI 列表解析完整 workspace snapshot。

`work-items run <id-or-key>` 的执行链路：

```text
GET /workspace?scope=session
  -> 查找 Work Item
GET /pipelines?workItemId=<item-id>&limit=20
  -> 查找已有 devflow-pr Pipeline
  -> 如不存在，POST /pipelines/from-template
  -> POST /pipelines/{pipelineId}/run-devflow-cycle
```

支持显式运行参数：

```bash
npm run omega -- work-items run OMG-21 --wait
npm run omega -- work-items run OMG-21 --auto-approve-human
npm run omega -- work-items run OMG-21 --auto-merge
```

这些参数会直接透传给现有 DevFlow run API。

### Attempts

```bash
npm run omega -- attempts list
npm run omega -- attempts list --status failed
npm run omega -- attempts list --work-item item_21 --pipeline pipeline_21_devflow --limit 20
npm run omega -- attempts timeline <attempt-id>
npm run omega -- attempts retry <attempt-id> --reason "Retry after fixing local runner"
npm run omega -- attempts cancel <attempt-id> --reason "Operator stopped the run"
```

对应 API：

```text
GET  /attempts?compact=true
GET  /attempts/{id}/timeline
POST /attempts/{id}/retry
POST /attempts/{id}/cancel
```

`attempts list` 默认使用 compact read model，并支持 `--id`、`--status`、`--work-item`、`--pipeline`、`--repository`、`--limit`。只有显式传 `--full` 才读取完整 attempt stages/events。

### Operations / Agent Runs

```bash
npm run omega -- operations list
npm run omega -- operations list --work-item item_21
npm run omega -- operations list --pipeline pipeline_21_devflow --stage coding --limit 20
npm run omega -- operations list --agent coder --full
```

对应 API：

```text
GET /operations?compact=true
```

默认输出 Agent/operation 的 stage、agent、runner、model、duration、token totals 和 summary。Token 只显示 Runtime/runner 已真实记录的用量；CLI 不做估算。`--full` 用于排查时读取完整 prompt/record，日常列表应保持 compact，避免 stdout/stderr 或 runner detail 导致命令响应变慢。

### Checkpoints

```bash
npm run omega -- checkpoints list
npm run omega -- checkpoints list --status pending
npm run omega -- checkpoints list --attempt attempt_21 --stage human_review
npm run omega -- checkpoints approve <checkpoint-id> --reviewer alice
npm run omega -- checkpoints changes <checkpoint-id> --reason "Need clearer acceptance criteria"
```

对应 API：

```text
GET  /checkpoints
POST /checkpoints/{id}/approve
POST /checkpoints/{id}/request-changes
```

`checkpoints list` 默认传 `limit=50`，走规范化 checkpoint 表；支持 `--id`、`--status`、`--pipeline`、`--attempt`、`--stage`、`--limit`。Approve / request changes 仍统一进入后端 checkpoint decision path，CLI 不绕过 Human Review 决策链。

### Run Workpads

```bash
npm run omega -- workpads list
npm run omega -- workpads list --work-item item_21
npm run omega -- workpads list --attempt attempt_21 --full
npm run omega -- workpads show <workpad-id>
```

对应 API：

```text
GET /run-workpads?compact=true
GET /run-workpads?id=<workpad-id>&limit=1
```

`workpads list` 默认 compact，只展示身份、状态和更新时间。`workpads show` 读取单条完整 Run Workpad，用于查看 Plan、Acceptance Criteria、Validation、Review Packet、Blockers、Retry Reason、Notes 和 PR 信息。

### Proof

```bash
npm run omega -- proof list
npm run omega -- proof list --work-item item_21 --label review-packet
npm run omega -- proof preview <proof-id>
npm run omega -- proof preview /absolute/path/to/proof.md
```

对应 API：

```text
GET /proof-records
GET /proof-records/{id-or-path}/preview
```

`proof preview` 通过 Runtime 的 proof preview handler 读取 artifact，不直接猜测文件格式；支持 proof id，也支持绝对路径。

### Pull Requests

```bash
npm run omega -- pr status <attempt-id>
npm run omega -- pr status https://github.com/acme/demo/pull/12
npm run omega -- pr status 12 --repo acme/demo --required-checks lint,test
```

对应 API：

```text
POST /github/pr-status
```

如果参数是 Attempt id，CLI 会先读 `GET /attempts?id=<id>&compact=true&limit=1`，从 attempt 中取得 `pullRequestUrl` 和 `workspacePath`，再交给 Runtime 刷新 PR lifecycle、checks、review feedback 和 delivery gate。CLI 只做薄封装，不直接调用 `gh`。

### JobSupervisor

```bash
npm run omega -- supervisor tick
npm run omega -- supervisor tick --limit 20 --stale-after-seconds 900
npm run omega -- supervisor tick --auto-run-ready
npm run omega -- supervisor tick --auto-retry-failed --max-retry-attempts 2 --retry-backoff-seconds 300
```

调用 `POST /job-supervisor/tick`。

注意：`--auto-run-ready` 会显式允许 JobSupervisor 启动通过 preflight 的 Ready Work Item；`--auto-retry-failed` 会显式允许 JobSupervisor 对通过 retry policy 的 failed / stalled Attempt 创建新 retry Attempt。默认两个开关都关闭，避免命令行状态检查时意外写目标仓库。

## 架构约束

- CLI 是薄控制层，只依赖 Runtime HTTP API。
- CLI 不导入 `omegalocal` server 内部业务逻辑，不直接操作 SQLite。
- CLI 列表命令默认使用 session / compact / filtered read model；不要把 Work Item、Attempt、Run Workpad、Operation 的热路径重新接回 full workspace snapshot。
- 打包 CLI 与桌面 App 使用同一份 Go 源码构建；演示前通过 `npm run release:binaries` 或 `npm run release:prepare` 生成最新版。
- 代码位置：

```text
services/local-runtime/cmd/omega
services/local-runtime/internal/omegacli
```

- Runtime 服务入口仍然是：

```text
services/local-runtime/cmd/omega-local-runtime
```

## 后续扩展

- 增加 `omega work-items create`，但必须复用现有 Requirement / Work Item 创建 API。
- 增加 `omega page-pilot ...`，但必须走真实 Page Pilot preview runtime / isolated workspace API，不做伪造 URL。
- 增加 `omega proof open <proof-id>`，在桌面环境中打开 proof artifact。
- 增加机器可读输出稳定 schema，用于脚本和 CI。
