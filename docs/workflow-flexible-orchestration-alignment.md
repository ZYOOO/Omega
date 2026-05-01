# 功能一灵活编排对齐记录

日期：2026-05-01

## 对照结论

参考项目的核心优势不是某个固定流水线，而是把项目运行规则放在仓库模板里：

- workflow 文件同时包含运行配置和 Agent prompt。
- tracker 状态决定当前该执行哪一段流程。
- 每个 issue / item 使用隔离 workspace。
- Agent 可以按当前状态继续同一个 workspace，而不是每次从零开始。
- simple / complex 分类会影响计划、workpad 和验证重量。
- review / rework / human review / merging 都是状态流转，不是失败后的临时分支。

Omega 现有能力已经覆盖真实工程闭环：Repository Workspace、Attempt、runner、PR、review、human gate、merge、proof、Run Workpad、GitHub/CI 信号和 Page Pilot 联动。但功能一内核之前仍有一个明显差距：workflow contract 能配置 stage / prompt / runtime，却还不能一等描述 action graph，DevFlow 执行顺序仍偏固定。

## 已落地

本轮先落地不破坏现有闭环的基础层：

- `devflow-pr` workflow 新增 `states.actions`。
- action 支持 id、type、agent、prompt、requiresDiff、input/output artifacts、transitions 和 review verdicts。
- workflow 新增 `taskClasses`，用于记录 simple / complex 的计划与验证策略。
- Go runtime parser 解析 states/actions/taskClasses/hooks。
- Workflow validator 校验 action id/type 和 transition 目标 stage。
- Pipeline run workflow snapshot 保存 states、扁平 actions、taskClasses、hooks、executionMode。
- Agent invocation 后的 stage 推进优先读取 snapshot transitions，缺失时才回退旧固定顺序。
- 新增 `GET /attempts/{attemptId}/action-plan`，从 Pipeline workflow snapshot 生成 Attempt 当前 state、current action、state actions、transitions、retry action 和恢复策略。

## 仍未完成

这次没有直接替换 DevFlow 主链路。原因是现有链路已经承载真实 branch / commit / PR / checks / merge / proof，不能为了抽象而破坏稳定性。

下一步迁移顺序：

1. 已完成基础版：生成 Attempt 级 action plan / current action / retry action，只 dry-run，不执行写仓库动作。
2. 让 Work Item 详情页和 JobSupervisor 消费 action plan，减少 UI / supervisor 各自推断状态。
3. 先迁移 review / rework / merging 到通用 action handler。
4. 再迁移 implementation / validation / PR 创建。
5. 最后让 taskClasses 真正影响 planning、workpad 展示和 validation 策略。

## 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestDevFlowTemplateLoadsWorkflowMarkdownContract|TestWorkflowContractParsesStateActionsAndRejectsBrokenActionRoute|TestRepositoryWorkflowTemplateValidationRejectsBrokenContract|TestDevFlowReviewOutcomeRoutesChangesRequestedToRework|TestDevFlowStageStatusAfterChangesRequestedQueuesRework'
go test ./services/local-runtime/internal/omegalocal -run 'TestBuildAttemptActionPlanUsesWorkflowSnapshot|TestAttemptActionPlanAPIIncludesRetryPolicy'
go test ./services/local-runtime/internal/omegalocal
```
