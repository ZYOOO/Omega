# DevFlow Rework Checklist

## 背景

旧做法：失败原因、Review Agent 输出、人工 Request changes、PR checks / branch sync / conflict 推荐动作分散在 Attempt、Run Timeline、PR status 和 Workpad 的不同区域。Retry 或 Rework 虽然能继续执行，但 Agent 看到的输入可能只是一个简短 reason，用户也需要自己从日志里拼出“到底该改什么”。

新做法：runtime 在 Attempt 失败、取消、人工 request changes、手动 retry 和 Workpad 刷新时生成统一 `reworkChecklist`。它不是 UI 摘要，而是给下一轮 Rework Agent 和 Retry API 直接消费的结构化输入。

## 数据结构

`reworkChecklist` 写入 Attempt，并同步进入 Run Workpad：

```json
{
  "status": "needs-rework",
  "retryReason": "Review agent blocked delivery.",
  "checklist": [
    "处理 Review Agent 指出的阻塞问题：Add a loading state before merge.",
    "查看失败的 CI/check 输出，在同一分支上修复后重新验证。"
  ],
  "groups": [
    {
      "key": "src/App.tsx:44:...",
      "item": "处理未解决 PR review thread（src/App.tsx:44）：Inline thread asks for shorter loading message.",
      "count": 2,
      "kinds": ["pr-review-thread"]
    }
  ],
  "sources": [
    {
      "kind": "review",
      "label": "Review feedback",
      "message": "Add a loading state before merge."
    }
  ],
  "prompt": "Retry / rework reason...\nRework checklist...\nSource feedback..."
}
```

字段含义：

- `status`：`needs-rework` 表示有可执行修复输入；`clear` 表示当前 Attempt 没有明确 rework 信号。
- `retryReason`：Retry / Workpad 默认展示的主因。
- `checklist`：下一轮 Agent 应逐项处理的修复清单。
- `groups`：按文件行、check run 或归一化内容生成的去重分组；`checklist` 由 groups 派生，避免同一反馈重复生成多条行动项。
- `sources`：保留原始来源，便于审计是人工、review、runner、operation 还是 delivery gate 给出的信号。
- `prompt`：给 Rework Agent 的合并输入，避免每个调用点重新拼 prompt。
- UI：Workpad 的 Rework Checklist 默认只展示 action 摘要；展开后显示 `sources` drilldown，帮助用户判断 action 的依据。PR comment / review / check log source 会保留 URL、run id、state 等基础元数据，前端可在有链接时直接打开来源。

## 信号来源

当前合并这些来源：

- `humanChangeRequest`
- `failureReviewFeedback`
- `failureReason` / `statusReason` / `errorMessage` / `failureDetail`
- runner `stderrSummary`
- pipeline event 中的 rejected / changes / failed / rework 事件
- review / rework / failed operation summary
- Attempt 上已有的 PR delivery recommended actions，例如 checks failed、required checks missing、branch sync、merge conflict、PR review decision
- GitHub PR comments / reviews 的基础内容；`CHANGES_REQUESTED`、`COMMENTED` 或带正文的 review 会进入 `pullRequestFeedback`
- GitHub PR review threads 的 best-effort 结构化内容；unresolved thread 会带 `path:line` 进入 checklist，resolved thread 只保留在 `sources` 中作为证据。
- 失败 GitHub check 对应的 Actions run log 基础内容；runtime 会从 check link 抽取 run id，并优先读取 `gh run view --log-failed`，同时保存 check source drilldown 深链。

## Runtime 接入点

- `failAttemptRecord`：失败 Attempt 立即生成 `reworkChecklist`。
- `markAttemptCanceled`：取消 Attempt 会把取消原因纳入 checklist，便于后续 retry。
- `prepareDevFlowAttemptRetry`：当用户未手写 retry reason 时，默认使用 checklist 的 `retryReason`；新 retry Attempt 继承 `reworkChecklist`。
- `prepareDevFlowHumanRequestedRework`：人工 request changes 后，旧 Attempt 和新 rework Attempt 都写入同一份 checklist，并合并到 `reworkAssessment.checklist`。
- `upsertRunWorkpad`：Workpad 每次刷新都会派生并展示最新 checklist。
- DevFlow rework prompt：优先使用 `reworkChecklist.prompt`，自动 rework 时会追加最新 review feedback。
- `/github/pr-status` 和 DevFlow PR cycle 会通过 `gh pr view --json comments,reviews` 读取 PR 评审/评论，写入 `reviewFeedback` / `pullRequestFeedback`，并进入下一轮 checklist。
- `/github/pr-status` 和 DevFlow PR cycle 会对 failed / error / canceled / timed out check 尝试读取 Actions run log，写入 `checkLogFeedback`，并进入下一轮 checklist 与 Workpad Review Feedback。
- Review thread resolved/unresolved、行级上下文、check source drilldown 和 checklist 去重分组已在 runtime 侧完成；UI 和 Rework Agent 消费同一份 `reworkChecklist`。
- JobSupervisor 自动恢复分类会消费 recommended actions 和 `checkLogFeedback`：flaky CI 走验证重试，非 flaky CI 走 `rework-required`，权限失败走人工修复，不再把所有失败都当成同一种 retry。

## 当前边界

- PR review comments / reviews / review threads 已有基础采集；分页、thread 评论超过 20 条时的增量读取、以及更完整的 GitHub 行级 diff 上下文仍需后续增强。
- CI/check 失败日志已有基础采集和 source drilldown；job/step 结构化解析、日志分页、超大日志归档仍需后续增强。
- Run Timeline 还没有按 checklist source 过滤，后续可以让用户从 checklist 反查原始事件。
- Workpad source drilldown 已展示 state / path / line / URL；后续补 source 到 Timeline event、check log step、proof artifact 的跨面板联动。

## 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestGitHubPRStatusUsesGhViewAndChecks|TestGitHubPullRequestFeedbackFromView|TestGitHubPRStatusClassifiesFailedChecks|TestBuildReworkChecklistMergesReviewHumanAndDeliveryGateSignals|TestRunWorkpadRecordTracksAttemptRetryContext|TestPrepareDevFlowAttemptRetryLinksAttempts|TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback'
go test ./services/local-runtime/internal/omegalocal
```
