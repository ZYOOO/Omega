# DevFlow Contract

更新时间：2026-05-02

## 当前定位

DevFlow 的默认模式不再只是 Go runtime 里的固定流程，而是一份可编辑的 workflow contract：

- 默认 contract 文件：`services/local-runtime/workflows/devflow-pr.md`
- 仓库覆盖文件：目标仓库 `.omega/WORKFLOW.md`
- Workspace / Repository 覆盖：Workflow Template 一等记录或 Agent Profile 中的 workflow markdown

旧做法：`devflow-pr` 主要由 Go 代码里的固定顺序驱动，workflow markdown 更多承担 stage、prompt、runtime policy 的配置作用。

新做法：`devflow-pr` 的 `states.actions` 是运行协议。runtime 会保存 action snapshot，校验 action type 是否有已注册 handler，并按 action verdict / transition 推进 Review、Rework、Human Review、Merging 等状态。

## 默认 Contract 结构

当前默认 contract 保留八个阶段：

1. `todo`：写入结构化 Requirement artifact。
2. `in_progress`：任务分类、架构交接、编码、验证、创建 PR。
3. `code_review_round_1`：读取本地 diff 做第一轮 Review。
4. `code_review_round_2`：读取 PR diff / checks 做第二轮 Review。
5. `rework`：生成 rework checklist，沿用同一 workspace / branch / PR 修改并再次验证。
6. `human_review`：等待人工 approve / request changes。
7. `merging`：刷新 PR 状态并执行 merge。
8. `done`：写入 handoff bundle 和 proof。

默认 action handler registry：

| action type | handler | 作用 |
| --- | --- | --- |
| `write_requirement_artifact` | `devflow.requirement.write_artifact` | 写 Requirement artifact 和验收条件 |
| `classify_task` | `devflow.task.classify` | 生成任务分类与执行提示 |
| `run_agent` | `devflow.runner.run_agent` | 调用配置的 Agent runner |
| `run_validation` | `devflow.validation.run` | 执行仓库验证命令 |
| `ensure_pr` | `devflow.github.ensure_pr` | push branch 并创建 / 更新 PR |
| `run_review` | `devflow.review.run` | 调用 Review Agent 并解析 verdict |
| `build_rework_checklist` | `devflow.rework.build_checklist` | 聚合 Review / PR / CI / 人工反馈 |
| `human_gate` | `devflow.human_gate.wait` | 生成人工审核 checkpoint |
| `refresh_pr_status` | `devflow.github.refresh_pr_status` | 拉取 PR checks / branch sync / conflict 状态 |
| `merge_pr` | `devflow.github.merge_pr` | 执行 PR merge 并写入 proof |
| `write_handoff` | `devflow.delivery.write_handoff` | 写 handoff bundle / proof record |

如果 contract 中出现未注册的 action type，workflow 校验会失败，避免 UI 上看起来能配置、运行时却被忽略。

## 可修改范围

当前已经真实生效的修改：

- 调整 `run_review` action 的 verdict 路由，例如第一轮通过后直接到 `human_review`，或把 `changes_requested` 指到自定义 `rework` stage。
- 删除 / 增减 Review state 中的 `run_review` action，runtime 会从 `states.actions` 重新派生 Review 轮次，不再只读旧 `reviewRounds` 字段。
- 调整 `rework` state 的 `passed` transition，决定 rework 后回到哪一轮 Review。
- 调整 `human_review` 的 `approved` / `changes_requested` 路由。
- 调整 runtime policy：`maxReviewCycles`、heartbeat、timeout、required checks、cleanup retention 等。
- 调整 prompt section 和 runner profile，用同一 contract 影响 Agent 输入。

当前执行边界：

- 首次主链路的 Requirement、task classification、architecture handoff、coding、validation、ensure PR 已通过 contract state runner 执行。
- contract 可以调整这些 action 的顺序，或移除非必需 action；如果 contract 引用没有 runtime handler 的 action，会在执行前失败。
- Rework 和 Merging 的内部副作用已经按 contract 路由进入对应 stage，但 handler 代码仍在 DevFlow adapter 文件内，后续会继续拆成更小的独立 handler 文件，降低 `devflow_cycle.go` 的体积。

## Review / Rework 路由

Review Agent 必须输出：

```text
Verdict: APPROVED
```

或：

```text
Verdict: CHANGES_REQUESTED
```

或：

```text
Verdict: NEEDS_HUMAN_INFO
```

runtime 会把 verdict 映射到 action event：

- `APPROVED` -> `approved`
- `CHANGES_REQUESTED` -> `changes_requested`
- `NEEDS_HUMAN_INFO` -> `needs_human_info`

然后按以下优先级找下一阶段：

1. action `verdicts`
2. action `transitions`
3. state `transitions`
4. template `transitions`
5. 旧 DevFlow fallback

这意味着默认 DevFlow 是当前产品闭环，但不是写死流程；Repository Workspace 可以覆盖 contract，让同一 runtime 跑不同的 Review / Rework / Delivery 编排。

## 验证

本轮补充的验证：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestWorkflowActionRoute|TestDevFlowReviewRounds|TestWorkflowContractRejectsUnsupportedActionType|TestDevFlowTemplateLoadsWorkflowMarkdownContract|TestBuildAttemptActionPlanUsesWorkflowSnapshot'
go test ./services/local-runtime/internal/omegalocal
```
