# Workflow Contract Executor 方案

日期：2026-04-30

状态：分阶段落地中。2026-05-01 已完成第一阶段：`states.actions` 契约解析、校验、Pipeline snapshot 持久化，以及 DevFlow 阶段推进优先读取契约 transitions；已完成第二阶段基础版：`GET /attempts/{attemptId}/action-plan` 可从 snapshot 生成当前 state、current action、state actions、可达 transitions 和 retry action。2026-05-02 已完成第三阶段基础版：review / rework / merging 的 action 识别、verdict/event 归一和下一阶段路由已迁入 `workflow-action-handler`。2026-05-02 晚间继续推进到 contract action executor：默认 `devflow-pr` 以 `states.actions` 作为运行协议，runtime 会校验 action type registry，并从 `run_review` action 派生真实 Review 轮次和 Rework 路由；Requirement、task classification、architecture、coding、validation、ensure PR、Rework、Human Review approved 后的 Merging/Done 已按 contract state runner 执行。后续治理已把 Rework 与审批后 Delivery action handler 拆入独立文件，主 adapter 不再承载这些 handler 的执行体。

## 背景

Omega 当前功能一已经具备本地交付闭环：

```text
Work Item
  -> Repository Workspace
  -> Pipeline / Attempt
  -> Agent runner
  -> branch / commit / PR
  -> review / human gate / merge / proof
```

旧做法的优点是闭环清楚、执行可靠，但核心编排仍集中在 DevFlow 专用代码里。`devflow-pr` workflow contract 已经可以定义 stages、states/actions、review rounds、runtime policy、transitions、task classes 和 prompt sections，但这些能力仍在分阶段消费：

- stage / agent / artifact 会进入 Pipeline run。
- prompt sections 会进入对应 Agent prompt。
- runtime policy 中的 review cycle、heartbeat、timeout、retry、required checks 已经有消费路径。
- transitions 已经参与运行时阶段推进；Review / Rework / Human Review / Merging 会优先按 action verdict / transition 路由。
- coding、validation、commit、push、PR 创建、Rework、Merging、handoff 已由 contract state runner 调用对应 action handler。Rework 与审批后 Delivery handler 已拆到独立文件；首次 implementation 主链路仍在 adapter 中作为下一轮体积治理对象。

目标不是立刻推翻当前闭环，而是把固定 DevFlow 逐步升级为“由 workflow contract 驱动的执行系统”。这样不同 Repository Workspace 可以拥有自己的项目交付规则，Omega runtime 负责解释规则并写入统一的 Attempt / Operation / Proof / Run Workpad。

## 目标

1. 让 workflow contract 成为项目级交付规则的主要来源，而不是只作为 stage 和 prompt 配置。
2. 保留当前 `devflow-pr` 的稳定闭环，先通过 adapter 兼容旧执行路径。
3. 新增通用 action executor，让 review、rework、merge、validation、PR 等行为可以由 contract action 描述。
4. 让 UI 只消费统一数据模型：Pipeline、Attempt、Operation、Proof、Checkpoint、Run Workpad，而不是关心某个流程是否硬编码。
5. 支持 Repository Workspace 级 workflow override，且所有执行都锁定到明确 repository target。

## 非目标

- 不在本阶段替换现有 DevFlow 主链路。
- 不在本阶段新增完整可视化 workflow builder。
- 不在本阶段引入远端 worker 调度。
- 不在本阶段让 Agent 任意执行仓库外路径。
- 不在本阶段把 Review / Human Review / Merging 简化成假状态；这些仍必须落到真实 PR、真实 checks、真实 proof。

## 当前能力盘点

### 已具备

- 默认 workflow markdown：`services/local-runtime/workflows/devflow-pr.md`。
- Repository-owned workflow：目标仓库 `.omega/WORKFLOW.md`。
- Agent Profile workflow override：Project / Repository scope 的 `workflowMarkdown`。
- Workflow parser：front matter 中解析 `stages`、`states.actions`、`taskClasses`、`hooks`、`reviewRounds`、`runtime`、`transitions`。
- Pipeline run workflow snapshot：保存 `states`、扁平 `actions`、`taskClasses`、`hooks`、`executionMode`，供 UI、JobSupervisor 和后续通用执行器消费。
- DevFlow 阶段推进：Agent invocation 后通过 `workflow-action-handler` 读取 snapshot action verdict / state transition / template transition，缺失时才回退旧固定顺序。
- Attempt Action Plan：`GET /attempts/{attemptId}/action-plan` 根据 Pipeline snapshot 和 Attempt 当前 stage 返回可解释执行计划，包含 current state、current action、state actions、transitions、taskClasses、hooks 和 retry action。
- Prompt sections：`## Prompt: requirement`、`architect`、`coding`、`testing`、`rework`、`review`、`delivery`。
- Runner registry：Codex、opencode、Claude Code 基础 runner。
- Run Workpad：结构化记录 plan、validation、blockers、PR、review feedback、retry reason、rework checklist。
- JobSupervisor：retry policy、stalled/orphan recovery、workflow metadata backfill、workspace cleanup 基础能力。

### 主要缺口

- workflow contract 已能描述 action 序列，并能生成 Attempt action plan；首次主链路和 Review 轮次会按 `states.actions` 执行，`approved` / `changes_requested` / `needs_human_info` 会按 action verdict 推进。
- action type 已接入 runtime handler registry 校验；未注册 action type 会在 workflow 校验时失败，避免配置被静默忽略。
- transitions 已能驱动 review / rework / human review / merging 的阶段路由；Rework 与审批后 Delivery 的具体副作用已经拆成 action handler 文件，首次 implementation 主链路仍复用 DevFlow adapter 的真实实现。
- Runtime 对 `devflow-pr` 有较多固定分支，新增流程需要改 Go 代码。
- `maxContinuationTurns` 没有形成统一的 continuation turn 协议。
- simple / complex 任务分类只能写进 prompt，不能改变 runtime 的执行重量。
- Workflow override 能影响 prompt 和部分 runtime，但不能完整替换 execution graph。

## 目标架构

```text
Repository Workspace
  -> Resolved Workflow Contract
      -> State graph
      -> Action graph
      -> Agent profiles
      -> Runtime policy
      -> Prompt templates
  -> Generic Workflow Executor
      -> Action handlers
      -> Stage transitions
      -> Checkpoint gates
      -> Retry / rework routing
  -> Records
      -> Pipeline
      -> Attempt
      -> Operation
      -> Proof
      -> Run Workpad
      -> Runtime logs
```

Workflow contract 只声明“应该做什么”和“成功/失败后去哪里”；executor 负责把声明落到真实命令、真实 runner、真实 PR 和真实记录。

## Contract Schema 草案

保留现有字段，并新增 `states` / `actions`。旧字段仍可继续存在，方便迁移。

```yaml
---
id: devflow-pr
name: DevFlow PR cycle

runtime:
  maxReviewCycles: 3
  runnerHeartbeatSeconds: 10
  attemptTimeoutMinutes: 30
  maxRetryAttempts: 2
  retryBackoffSeconds: 300
  maxContinuationTurns: 2
  requiredChecks: []

states:
  - id: in_progress
    title: Implementation and PR
    agents: [architect, coding, testing]
    actions:
      - id: requirement_intake
        type: write_requirement_artifact
      - id: architecture_handoff
        type: run_agent
        agent: architect
        prompt: architect
        mode: local_orchestrator_or_runner
      - id: implement_change
        type: run_agent
        agent: coding
        prompt: coding
        requiresDiff: true
      - id: validate_repo
        type: run_validation
      - id: commit_change
        type: commit_changes
      - id: publish_branch
        type: push_branch
      - id: ensure_pull_request
        type: ensure_pr
    transitions:
      passed: code_review_round_1
      failed: rework

  - id: code_review_round_1
    title: Code Review Round 1
    actions:
      - id: review_round_1
        type: run_review
        agent: review
        prompt: review
        diffSource: local_diff
        verdicts:
          approved: code_review_round_2
          changes_requested: rework
          needs_human_info: human_review

  - id: human_review
    title: Human Review
    humanGate: true
    actions:
      - id: wait_human_decision
        type: human_gate
    transitions:
      approved: merging
      changes_requested: rework

  - id: merging
    title: Merging
    actions:
      - id: refresh_checks
        type: refresh_pr_status
      - id: merge_pull_request
        type: merge_pr
    transitions:
      passed: done
      failed: rework
---
```

### Action 类型

第一批 action 建议只覆盖当前功能一已有真实能力：

| Action | 作用 | 必须写入 |
| --- | --- | --- |
| `run_agent` | 调用 runner 执行 prompt | Operation、runner process、proof |
| `run_validation` | 执行仓库验证命令 | Operation、test report、validation summary |
| `commit_changes` | 提交当前 diff | Proof、commit sha、changed files |
| `push_branch` | 推送 delivery branch | Proof、runtime log |
| `ensure_pr` | 创建或更新 PR | PR URL、proof、Run Workpad PR |
| `run_review` | 运行 review agent 并解析 verdict | Review proof、review feedback |
| `build_rework_checklist` | 汇总 review / human / PR / check 信号 | Run Workpad、Attempt retry reason |
| `human_gate` | 创建/等待 checkpoint | Checkpoint、Run Workpad blocker |
| `refresh_pr_status` | 获取 checks / branch sync / conflict | PR lifecycle、recommended actions |
| `merge_pr` | 合并 PR | merge proof、delivery output |
| `update_workpad` | 字段级写入 Workpad | field patch history |
| `classify_task` | 判断 simple / complex / needs info | Workpad plan、routing |

## 执行模型

### 1. Resolve

每次 Attempt 开始前解析 workflow，优先级保持当前做法：

```text
Repository .omega/WORKFLOW.md
  -> Repository Agent Profile workflowMarkdown
  -> Project Agent Profile workflowMarkdown
  -> Built-in default workflow
```

解析结果保存到 Pipeline run workflow snapshot，Attempt 只消费 snapshot，避免运行中 workflow 被修改导致当前 Attempt 行为漂移。

### 2. Plan

第二阶段基础版先通过只读 API 根据当前 state 和 action list 生成 action plan：

```text
Attempt currentState
  -> actions[]
  -> expected proofs
  -> possible transitions
  -> required checkpoints
```

这一步先 dry-run，只生成计划和日志，不执行 action。dry-run 是迁移期保护网。当前已落地 `GET /attempts/{attemptId}/action-plan`，返回 current state、current action、state actions、可达 transitions、taskClasses、hooks、retry action 和恢复策略。

### 3. Execute

每个 action handler 都必须满足：

- 明确 `repositoryTargetId` 和 workspace path。
- 写入 Operation。
- 产出 proof 或明确说明不产出 proof 的原因。
- 失败时返回结构化 failure reason。
- 刷新 Run Workpad。
- 写 runtime log。

### 4. Transition

Action 执行结束后由 executor 统一决定 transition：

```text
action result
  -> state local transition
  -> attempt status
  -> pipeline stage status
  -> next action or checkpoint
```

旧做法中散落在 DevFlow 执行函数里的状态更新，需要逐步迁到这个统一入口。

### 5. Continue / Retry / Rework

Continuation 不等于重跑全部流程。建议定义三种继续方式：

| 类型 | 使用场景 | 行为 |
| --- | --- | --- |
| `continue_same_state` | 同一 state 内 action 未完成 | 继续下一个 action |
| `retry_failed_action` | action 临时失败 | 复用 Workpad + failure reason 重试该 action |
| `rework_from_feedback` | review / human 要求修改 | 进入 rework state，复用 branch / PR / workspace |

`maxContinuationTurns` 应改为控制同一 Attempt 中允许多少次 continuation，而不是只作为 metadata。

## 与现有 DevFlow 的迁移关系

### 阶段 0：只补设计和测试样例

- 不改变 runtime。
- 给 workflow parser 增加 schema 测试样例。
- 文档评审后再进入下一阶段。

### 阶段 1：Action schema 只解析不执行

- [x] `workflow_template.go` 解析 `states.actions`、action verdicts / transitions、`taskClasses` 和基础 hooks。
- [x] `GET /workflow-templates` 返回 states/actions/taskClasses/hooks。
- [x] Pipeline run workflow snapshot 保存 states、扁平 actions、taskClasses、hooks、executionMode。
- [x] Workflow validator 校验 action id、action type、state/action transition 目标。
- [x] DevFlow Agent invocation 阶段推进优先读取 snapshot transitions。
- [ ] UI 展示 action plan，但不触发新执行器。
- 现有 DevFlow 仍照旧运行。

### 阶段 2：Dry-run executor

- [x] 新增 Attempt action plan API：`GET /attempts/{attemptId}/action-plan`。
- [x] 根据当前 pipeline / attempt / state 生成 action plan。
- [x] failed / stalled / canceled attempt 会生成 retry action、retry reason 和 recovery policy。
- [x] JobSupervisor recovery summary / accepted retry job 直接附带 action plan 摘要。
- [ ] Work Item 详情页直接消费 action plan。
- 不执行 git、runner、PR 命令。

### 阶段 3：先迁移 review / rework / merging

优先迁移当前问题最多、也最适合通用化的段落：

```text
run_review
build_rework_checklist
human_gate
refresh_pr_status
merge_pr
```

2026-05-02 基础版已落地：

- 新增 `workflow-action-handler`，统一从 Pipeline workflow snapshot 或 template 中解析当前 stage 的 action。
- Review Agent 的 `approved`、`changes_requested`、`needs_human_info` 先归一成 contract event，再按 action verdict 或 state transition 路由。
- Rework 的 `passed` 路由先读 state transition，支持从 rework 回到任意 contract 指定 review stage。
- Human Review approved 到 Merging、Merging passed 到 Done 会记录 action handler 路由元数据，并继续复用真实 PR merge、proof、handoff 逻辑。
- 旧 Go 固定顺序保留为 fallback，用于缺少 action graph 的历史 pipeline。

2026-05-02 增强版已落地：

- `executionMode` 从 `contract-action-plan` 升级为 `contract-action-executor`，表示 runtime 不只是展示 action plan，而是把 contract 作为默认运行协议。
- 默认 `devflow-pr.md` 是当前 DevFlow 的完整 contract；Repository / Profile override 可以修改 action verdict 和 state transition。
- Review 轮次不再只读旧 `reviewRounds` 字段，而是从 `states.actions` 中的 `run_review` action 派生；旧 `reviewRounds` 只作为 artifact/focus 兼容展示字段。
- Review approved 后会查看当前 action 的 `approved` verdict；如果目标不是另一个 Review state，就结束 Review 序列并进入后续 stage。
- Rework 会根据触发它的 review stage 读取 `changes_requested` 路由，不再固定从第一轮 Review 推断。
- 新增 action type registry，未注册的 action type 会让 workflow validation 失败。

Implementation 主链路中的 Requirement、task classification、architecture、coding、validation、ensure_pr 已通过 `runDevFlowContractState` 执行。contract 可调整这些 action 的顺序；缺失 handler 会直接返回 workflow contract action error。

### 阶段 4：迁移 implementation 主链路

把下面行为改为 action handler：

```text
run_agent(coding)
run_validation
commit_changes
push_branch
ensure_pr
```

2026-05-02 已落地：

- 新增 `devFlowContractActionStep` 和 `runDevFlowContractState`，按 active template 的 state actions 顺序调用真实 handler。
- `todo` state 使用 `write_requirement_artifact` handler 写 Requirement artifact。
- `in_progress` state 使用以下 handler 执行：
  - `classify_task`
  - `run_agent` / `architect`
  - `run_agent` / `coding`
  - `run_validation` / `testing`
  - `ensure_pr` / `delivery`
- 如果 contract action 没有对应 handler，会返回 `workflow contract action has no DevFlow runtime handler`，而不是静默跳过。
- 新增单测覆盖 contract action 执行顺序和缺失 handler 失败。

这一阶段之后，`runDevFlowCycle` 应退化为 adapter：

```text
load contract
create attempt
call generic executor
persist result
```

当前 `runDevFlowCycle` 已开始退化为 adapter：它仍承载上下文准备、workspace/proof、runner credential 和失败结果组装，但主链路动作、Rework 动作和 Human Review approved 后的 Merging/Done 动作都由 state runner 调用。旧做法是 Rework / Delivery 的 handler 执行体仍放在 `devflow_cycle.go` / `server.go`；2026-05-02 已治理为 `devflow_rework_actions.go` 和 `devflow_delivery_actions.go`，后续只剩首次 implementation 主链路继续拆文件。

#### 2026-05-02 阶段 4 增强：Rework / Merging 真实 action 化

新增已落地范围：

- `rework` state 按 contract action 执行：
  - `build_rework_checklist`
  - `apply_rework`
  - `validate_rework`
  - `update_pull_request`
- `human_review` state 在 approve 后执行 `human_gate`，只记录人工决策，不直接混入 merge。
- `merging` state 按 contract action 执行：
  - `refresh_pr_status`
  - `merge_pull_request`
- `done` state 执行 `finalize_handoff`，更新 handoff bundle / proof。
- 单测覆盖 Rework / Merging / Done 的 action 顺序，避免 contract 写了顺序但 runtime 又按旧 Go 顺序执行。

#### 2026-05-02 阶段 4 后续治理：handler 文件拆分

新增已落地范围：

- `devflow_rework_actions.go` 承载 Rework action handler：
  - `build_rework_checklist`
  - `apply_rework`
  - `validate_rework`
  - `update_pull_request`
- `devflow_delivery_actions.go` 承载 Human Review approved 后的 Delivery action handler：
  - `human_gate`
  - `refresh_pr_status`
  - `merge_pr`
  - `write_handoff`
- `devflow_cycle.go` 只负责组装 Rework handler 上下文、选择 contract stage、调用 `runDevFlowContractState`。
- `server.go` 只负责 checkpoint continuation 的记录持久化与最终 pipeline/attempt 状态更新。
- 这次治理不改变 contract 语义和外部 API，只降低 action executor 的代码耦合。

### 阶段 5：项目模板化

新增 Repository Workspace 级模板渲染能力：

```text
template variables
  -> workflow markdown
  -> validation
  -> repository profile
```

变量建议：

```text
projectName
repositoryLabel
repositoryTargetId
defaultBranch
requiredChecks
maxContinuationTurns
reviewRunner
codingRunner
validationCommand
branchPrefix
```

模板渲染结果应进入 Agent Profile 或 repo `.omega/WORKFLOW.md`，不能只存在前端内存。

## 数据模型影响

### Pipeline run.workflow

新增或扩展：

```text
workflow.states
workflow.actions
workflow.actionGraphVersion
workflow.source
workflow.validation
```

### Attempt

新增：

```text
currentStateId
currentActionId
actionCursor
continuationIndex
workflowSnapshotId
```

### Operation

新增：

```text
actionId
stateId
inputArtifactIds
outputArtifactIds
transitionEvent
```

### Run Workpad

新增：

```text
actionPlan
currentAction
lastTransition
taskClassification
continuationReason
```

## UI 影响

### Work Item 列表

列表只显示：

```text
当前 state
当前 action
进度条
右侧统一状态入口
```

不要把 action 明细塞进列表。

### Work Item 详情

详情页新增 “Execution Plan” 或复用 Run Workpad：

```text
Current state
Current action
Next transition
Blockers
Review / rework checklist
```

Action detail 使用页内弹窗浏览，不在详情页里行内撑开。

### Settings / Agent Profile

短期只展示 workflow markdown 和解析预览。后续再做模板变量 UI。

## 验证策略

### Parser 测试

- 可以解析旧 `stages`。
- 可以解析新 `states.actions`。
- 缺少 action id、未知 action type、transition 指向不存在 state 时失败。
- 旧 workflow 不包含 actions 时仍兼容。

### Executor dry-run 测试

- 给定 state，能生成正确 action plan。
- human gate state 不会执行 merge。
- rework state 能继承 branch / PR / workspace。
- failed action 能生成 retry reason 和 recovery policy。

### Runtime 集成测试

- 旧 `devflow-pr` 流程不变。
- repo `.omega/WORKFLOW.md` 覆盖 prompt 后真实进入 runner prompt。
- review changes_requested 进入 rework，不被当成系统错误。
- approved human gate 只进入 merging，由 merge action 处理。

### UI 测试

- Work Item 详情展示 workflow source、current action、Run Workpad action plan。
- 点击 action 打开页内弹窗。
- action failure 展示业务原因和来源。

## 风险与缓解

### 风险：一次迁移导致功能一闭环不稳定

缓解：先 parser 和 dry-run，再迁移 review/rework/merge，最后迁 implementation。

### 风险：contract 过度灵活，导致安全边界模糊

缓解：所有 action handler 都必须接收 resolved Repository Target 和 workspace lock，禁止 contract 直接指定任意路径。

### 风险：UI 信息更复杂

缓解：列表只展示 state/action 摘要，详情页通过 Run Workpad 和弹窗展示深层信息。

### 风险：旧 workflow 与新 schema 并存难以维护

缓解：`devflow-pr legacy adapter` 保留到 Generic executor 完成后再删除；每阶段都要求测试覆盖旧 workflow。

## 开发前检查清单

进入实现前需要确认：

- [ ] action schema 是否采用 `states.actions`，还是继续扩展 `stages.actions`。
- [ ] 第一批 action type 是否只覆盖功能一现有真实能力。
- [ ] Generic executor 是否先只做 dry-run。
- [ ] review/rework/merge 是否作为第一批迁移对象。
- [ ] template 渲染结果放在 Agent Profile 还是 repo `.omega/WORKFLOW.md`。
- [ ] `maxContinuationTurns` 的精确定义：同 Attempt continuation，还是同 workflow state continuation。

## 建议结论

建议采用“契约先行、执行器后置、分段迁移”的路线：

```text
Parser
  -> Plan
  -> Dry-run
  -> Review/Rework/Merge actions
  -> Implementation actions
  -> Project template rendering
```

这样既保留当前功能一的真实闭环，又能把后续差异化项目交付规则从 Go 固定流程中解耦出来。
