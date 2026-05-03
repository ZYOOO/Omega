# Human Review Rework Assessment

日期：2026-04-30

## 背景

旧做法：Human Review 点 Request changes 后，系统会把人工意见写入下一轮 Attempt，但默认仍按完整 DevFlow 重新执行。这个做法虽然真实创建了 rework Attempt，但对“小改文案 / UI / CSS / 单点行为”的反馈过重，会重复经过 requirement / architect；对“需求、接口、权限、数据模型变化”的反馈又缺少明确重新规划信号。

新做法：Human Review Request changes 先进入 Rework Assessment，由 runtime 根据人工意见和上一轮 Attempt 元数据生成结构化评估，再决定下一轮入口。

## 策略

```text
Human request changes
  -> Rework Assessment
      -> fast_rework
          复用上一轮 workspace / branch / PR
          从 rework 阶段继续
          只产出本轮人工意见对应的增量 diff
      -> replan_rework
          复用上一轮 workspace / branch / PR
          从 todo / requirement 重新规划
          更新 requirement / solution plan 后再实现
      -> needs_human_info
          不启动 Agent
          Attempt 停在 waiting-human
          Workpad 说明需要补充的人工信息
```

## Runtime 数据契约

新增 `reworkAssessment` 字段，写入新旧 Attempt 和 Run Workpad：

```text
strategy
entryStageId
rationale
humanFeedback
signals
checklist
previousAttemptId
previousBranch
previousPR
workItemId
pipelineId
createdAt
```

`strategy` 当前支持：

- `fast_rework`：局部文案、样式、布局、显示状态、按钮行为等，直接从 `rework` 阶段续跑。
- `replan_rework`：涉及需求、架构、接口、权限、数据模型、跨模块流程等，从 `todo` 重新规划。
- `needs_human_info`：人工反馈为空或不明确，不启动 Agent，等待补充说明。

## Fast Rework 执行方式

旧做法：即使只是“把默认用户名改成章四”，也会把人工意见拼进 description，再让完整链路重新开始。

新做法：

- checkout 优先恢复上一轮 delivery branch，本地缺失时优先从 `origin/{branch}` 恢复。
- coding runner 以 `rework` role 在同一 repo checkout 上修改。
- rework prompt 优先消费 `reworkChecklist.prompt`，因此人工反馈、Review Agent 输出、失败原因和 PR/check 推荐动作会作为同一份执行清单进入 Agent。
- commit / push 继续使用同一 branch 和 PR。
- proof 写入 `rework-assessment.md`、`human-rework-summary.md`、`git-diff-human-rework.patch`、`test-report-human-rework.md`。
- 二次 review prompt 同时看到人工意见和本轮增量 diff。
- PR description 会补充人工意见、changed files、validation 和本轮增量 diff。
- 完成后重新进入 Human Review，等待人工确认。

## Replan Rework 执行方式

当人工意见可能改变需求或架构边界时，runtime 会把新 Attempt 入口设为 `todo`。这不是从空白分支重做，而是在继承上一轮 branch / PR / workspace 的基础上重新产出 requirement / solution plan，再继续实现和 review。

## Workpad 展示

Work Item 详情页的 Run Workpad 会优先展示 Rework Assessment 摘要：

- 标题显示 Fast rework / Replan then rework / Needs human info。
- 展开后显示评估理由、人工原始反馈和 checklist。
- Review Feedback / Retry Reason 继续保留 review agent、PR checks、human request changes 和失败原因的合并视图。
- Rework Checklist 作为新的结构化执行输入进入 Workpad；展开后可以看到来源、主因和下一轮要处理的 checklist。

## 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback|TestPrepareDevFlowHumanRequestedReworkWaitsWhenFeedbackNeedsInfo|TestAssessHumanRequestedReworkRoutesByScope|TestResetDevFlowPipelineForAttemptFromStageFallsBackWhenStageMissing'
npm run lint
npm run test -- apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx apps/web/src/components/__tests__/WorkItemDetailPanels.test.tsx --testTimeout=15000
```
