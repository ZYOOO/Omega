# GitHub Actions CI 链路说明

> 更新时间：2026-05-05

## 当前结论

Omega 默认支持 GitHub Actions 作为 DevFlow PR 链路中的远端 CI。当前实现不要求额外的 Omega OAuth 配置；前提是本机 `gh` 已登录目标 GitHub 账号，并具备读取仓库、PR 和 Actions 的权限。

本机检查命令：

```bash
gh auth status -h github.com
```

推荐 token scope：

- `repo`：私有仓库、PR、branch、checks 元数据。
- `workflow`：读取/操作 GitHub Actions workflow 相关信息时通常需要。

当前机器 `gh auth status` 已显示 `repo` 和 `workflow` scope，因此 Omega 可以默认通过 `gh pr checks`、`gh run view --log-failed` 获取 GitHub Actions 状态和失败日志。

## DevFlow 中的位置

默认 workflow：`services/local-runtime/workflows/devflow-pr.md`

新增一等 action：

```yaml
- id: collect_ci_results
  type: run_ci_checks
  agent: testing
  outputArtifacts: [ci-report, check-status, check-log]
```

执行顺序：

1. `implement_change`：Coding Agent 修改仓库。
2. `validate_repository`：本地测试/验证。
3. `publish_pull_request`：push branch 并创建/更新 PR。
4. `collect_ci_results`：读取 GitHub Actions checks 和失败日志。
5. `review_round_1` / `review_round_2`：Review Agent 同时消费 diff、本地验证和 CI 结果。
6. 如果 CI failed 或缺少 required checks，Omega 会将失败日志转换为 Rework feedback，进入下一轮 rework。
7. Rework 后再次执行 `collect_rework_ci_results`，确保报告和下一轮 review 使用最新 CI 状态。

## 运行时产物

每次 CI 采集会写入 proof：

- 首轮：`.omega/proof/ci-checks.md`
- 自动 rework：`.omega/proof/ci-checks-rework-N.md`
- Human request changes fast rework：`.omega/proof/ci-checks-human-rework.md`

报告包含：

- PR URL
- 刷新时间
- 总数 / passed / pending / failed / missing required
- `gh pr checks` 原始输出
- 失败 GitHub Actions run 的 `gh run view --log-failed` 摘要

这些内容会进入：

- `attempt.checkLogFeedback`
- Run Workpad 的 check preview
- Review prompt 的 `Remote checks`
- Review Packet / Human Review 报告
- Rework Checklist

## Required Checks

默认模板中的 `runtime.requiredChecks` 为空，表示不强制指定 check 名称，只根据 GitHub 返回的失败/pending 状态判断。

如果某个仓库希望强制要求特定 check，可以在仓库自定义 workflow 中配置：

```yaml
runtime:
  requiredChecks: [test, lint]
```

当 required check 不存在时，Omega 会把它标成 `missingRequired`，并进入 rework/人工处理链路。

## 权限和失败模式

无需新增 Omega 页面配置的情况：

- 本机 `gh` 已登录。
- `gh auth status` 有 `repo`，最好也有 `workflow`。
- 当前账号能访问目标 repo 和 PR。

需要用户处理的情况：

- `gh` 未登录：运行 `gh auth login`。
- token scope 缺少 `workflow`：运行 `gh auth refresh -h github.com -s workflow`。
- 无仓库写权限：DevFlow preflight 会在 push/PR 前失败。
- 非 GitHub Actions 外部 check：Omega 会保留 URL 和状态，但失败日志采集主要支持 GitHub Actions。

## 验证命令

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestDefaultDevFlowWorkflowIncludesGitHubActionsCIAction|TestDevFlowCIReport|TestRunDevFlowContractStateUsesReworkAndMergingActions' -count=1
go test ./services/local-runtime/internal/omegalocal -count=1
```

