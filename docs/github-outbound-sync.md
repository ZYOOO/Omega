# GitHub / CI 出站同步

## 目标

功能一需要把本地流水线的关键状态真实回写到 GitHub Issue 和 PR，而不是只在 Omega 内部展示。这样从 GitHub 侧也能看到当前需求是否正在执行、等待审核、CI 是否需要重跑、合并中、已完成或需要处理。

## 旧做法

- GitHub Issue 可以导入 Workboard。
- DevFlow 可以创建 branch、commit、PR，并读取 PR checks / review / failed check log。
- 但 Pipeline 状态没有真实写回 GitHub Issue。
- PR 上没有同步 review packet / risk / checks / rework 摘要。
- CI 只读取状态，不会按策略触发 rerun 或 workflow dispatch。

## 新做法

Go local runtime 增加 GitHub outbound sync：

- 通过 `gh issue comment` 写入结构化状态评论。
- 通过 `gh label create --force` 确保状态标签存在。
- 通过 `gh issue edit --add-label/--remove-label` 切换 Issue 状态标签。
- 通过 `gh pr comment --edit-last --create-if-none` 在 PR 上维护一条结构化 review packet 评论。
- 可选通过 `gh run rerun --failed` 重跑失败的 Actions run，或通过 `gh workflow run` 触发一个配置好的 workflow。
- 每次出站同步会生成 sync report，写入 Attempt record，并在 proof 目录保存 JSON 证据。
- 同步失败不会阻断 DevFlow 主链路，但会写入 runtime log 和 sync report，方便后续检查。

## 生命周期映射

| Omega 事件 | GitHub 标签 | 说明 |
| --- | --- | --- |
| `attempt.started` | `omega:running` | 仓库 workspace 已准备，流水线开始执行 |
| `human_review.waiting` | `omega:review` | PR 和审核包已准备，等待人工确认 |
| `delivery.merge_failed` | `omega:blocked` | 人工已通过，但 PR merge 失败 |
| `attempt.failed` | `omega:blocked` | 执行失败，需要 retry 或人工处理 |
| `delivery.completed` | `omega:done` | PR 已完成合并，流水线完成 |

固定标签：

- `omega:managed`
- `omega:running`
- `omega:review`
- `omega:blocked`
- `omega:merging`
- `omega:done`

## 同步内容

GitHub Issue comment 包含：

- Work item key / title
- Pipeline ID
- Attempt ID
- Event / status / stage
- Branch
- PR URL
- Changed files
- CI / checks 输出摘要
- PR feedback 和 failed check log 摘要
- 失败原因和失败详情
- Review packet 风险等级和推荐动作

GitHub PR comment 包含：

- Work item / Pipeline / Attempt
- 当前事件、状态、阶段和分支
- Review packet 摘要
- 结构化 diff preview
- 风险等级和推荐动作
- Changed files
- CI/check 输出与失败 check log 摘要
- PR feedback 和人工审核反馈
- 失败原因和需要处理的动作

## 可选 CI 触发

默认不触发远端 CI，只读取 checks 并生成推荐动作。需要自动触发时显式配置：

```bash
# 只重跑已采集到 run id 的失败 GitHub Actions run
export OMEGA_GITHUB_CI_TRIGGER=rerun-failed

# 或触发一个 workflow_dispatch workflow
export OMEGA_GITHUB_CI_TRIGGER=workflow-dispatch
export OMEGA_GITHUB_CI_WORKFLOW=ci.yml
export OMEGA_GITHUB_CI_REF=omega/OMG-1-devflow
export OMEGA_GITHUB_CI_INPUTS='{"reason":"human-review"}'
```

配置要求：

- `gh auth status` 必须能访问目标 repo。
- `rerun-failed` 依赖前序 check log feedback 中有 GitHub Actions `runId`。
- `workflow-dispatch` 要求目标 workflow 配置了 `workflow_dispatch`。
- 如需修改 workflow 文件本身，`gh` token 通常还需要 workflow 相关权限；单纯触发现有 workflow 主要依赖 repo / actions 访问权限。

## 实现边界

- Issue label/comment 只对能解析到 GitHub Issue 的 Work Item 执行：
  - `sourceExternalRef=owner/repo#number`
  - `target=https://github.com/owner/repo/issues/number`
  - 或带有 issue number 且能解析仓库的记录
- PR comment 只要 Attempt 有 `pullRequestUrl` 就可以同步，不强依赖 Issue 来源。
- 非 GitHub Issue 来源会跳过 Issue 写回，不会误写其他平台。
- label/comment 失败不会让 PR 创建、review 或 merge 主链路失败；真实失败会落到 sync report 和 runtime log。
- CI 触发默认关闭，只有显式 env 打开时才会执行真实 `gh` 写操作。

## 验证

单测：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestGitHubIssueRefFromWorkItemParsesImportRefAndURL|TestSyncGitHubIssueOutboundPostsCommentAndLabels|TestSyncGitHubIssueOutboundPostsPRCommentWithoutIssue|TestSyncGitHubIssueOutboundSkipsUnlinkedWorkItem|TestGitHubCITriggerRerunsFailedRuns|TestGitHubCITriggerWorkflowDispatch'
```

完整 runtime 测试：

```bash
go test ./services/local-runtime/internal/omegalocal
```

真实 GitHub smoke：

- 使用 `ZYOOO/TestRepo`。
- 先 clone 最新远端代码到临时目录并 `git pull --ff-only`。
- 创建临时 issue。
- 写入 Omega 同步 comment。
- 创建并切换 `omega:*` 标签。
- 关闭临时 issue。

已验证临时 issue：

- `ZYOOO/TestRepo#36`
