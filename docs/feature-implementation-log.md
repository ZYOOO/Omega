# Omega 功能实现记录

这个文档用于记录每个功能的落地方式。后续每完成一个功能，都按相同结构补一节：目标、入口、数据/API、运行时行为、验证、后续工作。

## 2026-04-27 Project Agent Profile Runtime Binding

### 目标

把前端 `Project Agent Profile` 从 localStorage 草稿推进到本地 Go runtime 可持久化、可读取、可参与执行的配置。配置需要覆盖：

- Workflow markdown 草稿
- 每个 Agent 的 runner / model / Skill / MCP / stage note
- `.omega/agent-runtime.json`
- `.codex/OMEGA.md`
- `.claude/CLAUDE.md`

### 产品入口

- Project 页面 `Project config`
- Repository Workspace 齿轮
- Repository Workspace subnav 的 `Workspace config`

Settings 页面中的 `Project Agent Profile` 现在分为三块：

- `Workflow`：编辑 workflow template 和 markdown 草稿，并展示阶段流。
- `Agents`：按 Agent role 配置 runner、model、Skills、MCP 和 stage note。
- `Runtime files`：预览并编辑 `.omega` / `.codex` / `.claude` 运行时配置。

### Go Runtime 做法

新增 `services/local-runtime/internal/omegalocal/agent_profile.go`：

- 定义 `ProjectAgentProfile` 和 `AgentProfileConfig`。
- 使用 `omega_settings` 持久化，key 规则：
  - `agent-profile:project:{projectId}`
  - `agent-profile:repository:{repositoryTargetId}`
- 新增 API：
  - `GET /agent-profile?projectId=...&repositoryTargetId=...`
  - `PUT /agent-profile`
- 解析顺序：
  1. Repository override
  2. Project profile
  3. 默认 profile

Pipeline 创建时会把 resolved profile 摘要写入 `run.agentProfile`，让一次运行可以追踪它使用了哪个配置来源。

Runner 执行时会写入：

- operation workspace 的 `.omega/agent-runtime.json`
- operation workspace 的 `.codex/OMEGA.md`
- operation workspace 的 `.claude/CLAUDE.md`
- DevFlow repository checkout 的 `.codex/OMEGA.md` / `.claude/CLAUDE.md`

Codex runner 的 prompt 会追加当前 Agent 的 policy block，review / coding / rework 阶段会读取 profile 中对应 Agent 的 model。

### 前端做法

`apps/web/src/omegaControlApiClient.ts` 新增：

- `ProjectAgentProfileInfo`
- `fetchProjectAgentProfile`
- `updateProjectAgentProfile`

`apps/web/src/App.tsx`：

- Settings 页面进入时从 Go runtime 拉取 profile。
- Save draft 会先写 localStorage 兜底，再通过 `PUT /agent-profile` 保存到本地 runtime。
- 保存成功后显示 `Saved to local runtime. New pipeline runs will use this profile.`

### API 文档

`docs/openapi.yaml` 增加 `/agent-profile` 的 GET / PUT 说明。

### 验证

已执行：

```bash
go test ./services/local-runtime/...
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx
npm run build
```

新增 Go 测试覆盖：

- profile 可以通过 API 保存/读取。
- pipeline 创建时带上 `run.agentProfile`。
- operation run 会生成 `.omega/agent-runtime.json` 和 `.codex/OMEGA.md`。

前端测试覆盖：

- Settings 中能看到 Workflow / Agents / Runtime files。
- Save draft 会走 `/agent-profile` 并显示保存到 local runtime。

### 后续工作

- Profile 已从 `omega_settings` 升级为一等 SQLite 表 `agent_profiles`，保留 settings 镜像作为旧 App / 调试工具兼容层。后续继续补校验、恢复默认、导入/导出。
- 增加正式 Workflow Template 编辑 API，把 markdown parse / validate / save 串起来。
- Runner registry 已接管 Agent 执行器选择：Codex、opencode、Claude Code 通过统一 `AgentRunner` 分发，并按 Agent profile 的 runner/model 写入 runner process。后续继续补 runner-specific 模板、timeout/cancel/retry 和 provider 映射。
- 在 Work Item detail 中展示当前 Attempt 实际使用的 profile source、Agent model、MCP、Skill 和 runtime files。

## 2026-04-27: Agent Profile 一等表与 Runner Registry

### 背景

上一版 Project Agent Profile 已经能在 UI 中编辑 workflow、Agent runner/model、Skill、MCP 和 `.codex` / `.claude` policy，也会写入 runtime bundle。但存储仍以 `omega_settings` 为主，执行器选择也主要停留在 Codex 默认路径。

### 本次实现

- 新增 SQLite 一等表 `agent_profiles`：
  - `id`
  - `scope`
  - `project_id`
  - `repository_target_id`
  - `version`
  - `profile_json`
  - `created_at`
  - `updated_at`
- `/agent-profile` 读写优先使用 `agent_profiles`，同时继续镜像到 `omega_settings` 兼容旧路径。
- 旧 settings profile 被读取时会自动迁移写入一等表。
- 新增 `AgentRunnerRegistry`：
  - `codex`
  - `opencode`
  - `claude-code` / `claude`
- `operations/run` 支持 `runner: "profile"` / `auto`，此时按当前 Agent profile 中的 runner 选择执行器。
- `run-devflow-cycle` 的 coding / rework / review Agent 开始按各自 Agent profile 选择 runner/model，而不是写死 Codex。
- local capability detection 增加 `claude-code`。
- 修复 `putOrchestratorWatcher` 返回 watcher 时与后台 goroutine 同时读写同一个 map 的并发崩溃。

### 验证

- Go 测试新增覆盖：
  - profile 保存后进入 `agent_profiles` 一等表，并继续写入 settings 兼容层。
  - `runner: "profile"` 会按 coding Agent 配置选择 `opencode` 执行器。
- `go test ./services/local-runtime/...`
- `npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx`

## 2026-04-28: Agent Runner 配置预检

### 背景

Agent Profile 已经能为不同 Agent 选择 Codex、opencode、Claude Code 等 runner。这里的产品语义应该是：前端配置决定 runner 分配，运行前做能力预检；如果本机没有安装对应 runner，应该阻止启动并提示配置或安装问题，而不是创建一次看起来已经进入执行的失败 runner process。

### 本次实现

- 前端 `Project Agent Profile -> Agents` 的 runner 选择接入 `GET /local-capabilities`：
  - 不可用 runner 在下拉项中 disabled。
  - 当前 Agent runner 下方展示 `ready` / 安装提示。
  - roster 中 runner 缺失的 Agent 会有警示样式。
- 保存 Project Agent Profile 时，如果本机 capabilities 已加载且存在不可用 runner，会展开 Agents tab、定位到第一个有问题的 Agent，并阻止保存。
- Go runtime 新增 runner preflight：
  - `local-proof` 不需要外部 CLI。
  - `demo-code` 需要 `git`。
  - `codex` 需要 `codex`。
  - `opencode` 需要 `opencode`。
  - `claude-code` 需要 `claude` 或 `claude-code`。
- `operations/run` 在创建真实运行 workspace 前检查当前 operation Agent 的有效 runner。
- `run-current-stage` 在创建 attempt 前检查当前 stage owner Agent 的有效 runner。
- `run-devflow-cycle` 和 orchestrator `autoRun` 在创建 attempt 前检查 coding / review Agent 的有效 runner。

### 验证

已执行：

```bash
go test ./services/local-runtime/...
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx
```

新增 Go 测试覆盖：

- `runner: "profile"` 指向未安装的 `opencode` 时，`/operations/run` 返回 `400 Bad Request`，不会进入真实 runner process。

前端测试更新：

- Settings / Agent Profile 保存用例补齐 `codex` capability，确保保存成功路径仍被覆盖。

### 后续工作

- 把 preflight 结果展示到 Workflow / Pipeline 启动按钮旁边，让用户在点击 Run 前就看到阻塞原因。
- 将 runner registry 补成完整 provider 映射：模型、环境变量、timeout、cancel、retry、runner-specific policy 文件。
- 在 Project Agent Profile 中增加恢复默认、导入/导出和版本历史。
