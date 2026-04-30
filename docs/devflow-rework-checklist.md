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
- 失败 GitHub check 对应的 Actions run log 基础内容；runtime 会从 check link 抽取 run id，并优先读取 `gh run view --log-failed`

## Runtime 接入点

- `failAttemptRecord`：失败 Attempt 立即生成 `reworkChecklist`。
- `markAttemptCanceled`：取消 Attempt 会把取消原因纳入 checklist，便于后续 retry。
- `prepareDevFlowAttemptRetry`：当用户未手写 retry reason 时，默认使用 checklist 的 `retryReason`；新 retry Attempt 继承 `reworkChecklist`。
- `prepareDevFlowHumanRequestedRework`：人工 request changes 后，旧 Attempt 和新 rework Attempt 都写入同一份 checklist，并合并到 `reworkAssessment.checklist`。
- `upsertRunWorkpad`：Workpad 每次刷新都会派生并展示最新 checklist。
- DevFlow rework prompt：优先使用 `reworkChecklist.prompt`，自动 rework 时会追加最新 review feedback。
- `/github/pr-status` 和 DevFlow PR cycle 会通过 `gh pr view --json comments,reviews` 读取 PR 评审/评论，写入 `reviewFeedback` / `pullRequestFeedback`，并进入下一轮 checklist。
- `/github/pr-status` 和 DevFlow PR cycle 会对 failed / error / canceled / timed out check 尝试读取 Actions run log，写入 `checkLogFeedback`，并进入下一轮 checklist 与 Workpad Review Feedback。

## 当前边界

- PR review comments / reviews 已有基础采集；thread resolved/unresolved 状态、行级 comment 上下文和 pagination 仍需后续增强。
- CI/check 失败日志已有基础采集；job/step 结构化解析、日志分页、超大日志归档和 check source drilldown 仍需后续增强。
- Run Timeline 还没有按 checklist source 过滤，后续可以让用户从 checklist 反查原始事件。
- Workpad source drilldown 当前是摘要级；后续补 source 到 Timeline event、PR comment、check log step、proof artifact 的深链。

## 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestGitHubPRStatusUsesGhViewAndChecks|TestGitHubPullRequestFeedbackFromView|TestGitHubPRStatusClassifiesFailedChecks|TestBuildReworkChecklistMergesReviewHumanAndDeliveryGateSignals|TestRunWorkpadRecordTracksAttemptRetryContext|TestPrepareDevFlowAttemptRetryLinksAttempts|TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback'
go test ./services/local-runtime/internal/omegalocal
```
