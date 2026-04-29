# DevFlow 生产化内核

最后更新：2026-04-29

本文单独记录功能一的生产化内核，避免继续把大体量架构文档堆厚。

## 范围

功能一把一个绑定仓库的 Work Item 转成真实交付链路：

```text
Requirement / Work Item
  -> repository workspace lock
  -> workflow contract
  -> runner execution
  -> branch / commit / PR
  -> checks / review / Human Review
  -> proof / delivery
```

## 旧做法

- runtime policy 一部分写在 Go 常量、CLI flag 或请求参数里。
- runner 进度主要在 Agent invocation 边界可见，长时间子进程的 stdout/stderr 没有进入 Attempt telemetry。
- PR 状态主要返回原始 checks 和基础 delivery gate。
- manual run / retry 主要依赖 active Attempt 判断；Workboard 路径的 execution lock 覆盖弱于 issue auto-run 路径。
- workspace root、repo path、cleanup 意图和锁信息分散在 runtime spec、proof、日志和执行流程里。
- prompt 主要由 Go 字符串拼接，workflow markdown 只定义 stage/review/runtime 的一部分。
- JobSupervisor 能做基础 tick，但 worker host lease、orphan running Attempt 恢复和 workspace cleanup 没有形成闭环。

## 新做法

- `devflow-pr` workflow runtime 定义：
  - `maxReviewCycles`
  - `runnerHeartbeatSeconds`
  - `attemptTimeoutMinutes`
  - `maxRetryAttempts`
  - `retryBackoffSeconds`
  - `cleanupRetentionSeconds`
  - `maxContinuationTurns`
  - `requiredChecks`
- DevFlow 执行器消费 runtime policy：runner heartbeat、Attempt timeout、retry 上限、retry backoff、cleanup retention、continuation turns 不再只靠硬编码默认值。
- Codex / opencode / Claude Code supervised child process 会产生 stdout / stderr / process heartbeat event。
- heartbeat event 会刷新 Attempt `lastSeenAt`，追加 Attempt events，并写入 runtime DEBUG log。
- `/github/pr-status` 输出 checks summary、missing required checks、branch sync、merge conflict、delivery gate、proof records 和 recommended actions。
- manual run、retry、JobSupervisor auto run 在启动前声明 repository workspace execution lock。
- 每次 DevFlow run 写入 `.omega/workspace-lifecycle.json`，与 `.omega/agent-runtime.json` 放在同一 run workspace。
- `POST /workspaces/cleanup` 和 JobSupervisor cleanup tick 支持清理已完成 Attempt 的 repo checkout，并保留 `.omega` proof / lifecycle；失败、取消、stalled 默认保留用于排障。
- 目标仓库可提供 `.omega/WORKFLOW.md` 作为 repo-owned workflow contract；Agent Profile 中带 front matter 的 workflow markdown 可作为 Project / Repository override。
- workflow contract 加载时会校验 stage id、transition 引用、review round 引用、agent 和 runtime 非负值；失败时阻止运行。
- Workflow Markdown body 支持 `## Prompt: requirement`、`## Prompt: architect`、`## Prompt: coding`、`## Prompt: testing`、`## Prompt: rework`、`## Prompt: review`、`## Prompt: delivery`，运行时会渲染变量后交给对应 Agent 或本地 orchestrator 记录为真实阶段交接。
- 旧做法只强化 coding / rework / review，导致 requirement、architect、testing、delivery 的输出契约不够统一；当前默认 `devflow-pr` 已把所有 Agent 的交接输出固定为可被下一阶段消费的小节。
- 后台 Attempt 会记录本机 worker host lease；JobSupervisor 能把“数据库仍 running，但本机没有 job 且锁无效”的 orphan Attempt 标为 stalled，后续 retry 策略可以接管。

## 架构

Workflow contract 入口：

```text
默认模板：services/local-runtime/workflows/devflow-pr.md
Project / Repository override：Agent Profile workflow markdown
Repo-owned override：目标仓库 .omega/WORKFLOW.md
  -> workflow_template.go
  -> Pipeline run.workflow
  -> devflow_cycle.go / job_supervisor.go / github_delivery.go
```

Workspace lifecycle：

```text
Work Item + Repository Target + Pipeline + Attempt
  -> workspace root
  -> run workspace
  -> repo checkout path
  -> execution lock
  -> workspace-lifecycle.json
  -> cleanup metadata on Attempt
```

Runner telemetry：

```text
AgentRunner.RunTurn
  -> runSupervisedCommandContextWithOptions
  -> stdout / stderr / heartbeat events
  -> Attempt lastSeenAt + runtime logs
```

JobSupervisor：

```text
tick
  -> checkpoint integrity
  -> workflow contract backfill / validation summary
  -> stalled detection
  -> worker lease orphan recovery
  -> Ready Work Item scan
  -> failed / stalled retry scan
  -> workspace cleanup scan
```

## 剩余工作

- GitHub polling 在 checks pending 时还需要刷新 Attempt heartbeat。
- branch sync / conflict 目前能检测和推荐动作，自动回到 Rework 暂缓。
- workspace cleanup 当前清理 repo checkout 并保留 proof；归档、压缩、删除整个 workspace 仍需后续策略。
- workflow contract 仍需更完整的 DAG、artifact、runner policy、stage-specific timeout 校验。
- worker host 当前是本机 lease；远端 worker 分配和远端崩溃恢复仍需后续增强。
