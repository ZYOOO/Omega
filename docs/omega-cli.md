# Omega CLI

Omega CLI 是面向本地 operator 的命令行入口。它不直接读写 SQLite，也不绕过 runtime 执行代码修改；所有命令都通过 Go Local Runtime 的真实 HTTP API 完成。

## 目标

- 在不打开 Workboard UI 的情况下查看运行状态、日志、Work Item、Attempt 和 Human Review gate。
- 从命令行显式触发 Work Item 运行、Attempt retry / cancel、Checkpoint approve / request changes。
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

推荐安装成当前用户 PATH 下的 Go binary：

```bash
go install ./services/local-runtime/cmd/omega
```

安装后直接使用：

```bash
omega status
omega logs --level ERROR
omega work-items list --status Ready
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

`work-items list` 读取 `GET /workspace` 中的真实 Work Item 表。

`work-items run <id-or-key>` 的执行链路：

```text
GET /workspace
  -> 查找 Work Item
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
npm run omega -- attempts timeline <attempt-id>
npm run omega -- attempts retry <attempt-id> --reason "Retry after fixing local runner"
npm run omega -- attempts cancel <attempt-id> --reason "Operator stopped the run"
```

对应 API：

```text
GET  /attempts
GET  /attempts/{id}/timeline
POST /attempts/{id}/retry
POST /attempts/{id}/cancel
```

### Checkpoints

```bash
npm run omega -- checkpoints list
npm run omega -- checkpoints list --status pending
npm run omega -- checkpoints approve <checkpoint-id> --reviewer alice
npm run omega -- checkpoints changes <checkpoint-id> --reason "Need clearer acceptance criteria"
```

对应 API：

```text
GET  /checkpoints
POST /checkpoints/{id}/approve
POST /checkpoints/{id}/request-changes
```

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
- 增加 `omega pr status <attempt-id>`，聚合 PR lifecycle、checks 和 delivery gate。
- 增加 `omega proof open <attempt-id>`，列出并打开 proof artifacts。
- 增加机器可读输出稳定 schema，用于脚本和 CI。
