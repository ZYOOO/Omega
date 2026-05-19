# DevFlow Agent Execution Policy

本文定义 Omega DevFlow 中“应该由 AI Agent 完成”的阶段，以及仍然必须由本地确定性执行器完成的控制动作。目标是让 Work Item 详情页、proof、飞书审核和演示讲解保持一致：Agent 负责理解、计划、编码、复核、交接和受限的 Git/GitHub 故障修复；Omega runtime 负责仓库隔离、状态机、确定性校验、数据库落库和可审计 proof。

## 核心原则

- 角色型阶段必须调用对应 Agent Profile：Requirement、Master、Architect、Coding、Testing、Review、Git Recovery、Delivery。
- 本地执行器只能做确定性控制动作：clone workspace、写 machine-readable JSON、运行本地验证命令、git add/commit/push、PR happy path 创建/更新与最终校验、读取 GitHub checks、checkpoint decision、merge、状态落库。
- PR 创建 / 更新遇到 Git 拓扑、远端分支、历史不一致、PR 元数据漂移等非确定性失败时，runtime 不再直接穷举修复策略，而是把诊断、上下文和边界交给 `git_recovery` Agent；runtime 只负责触发范围、仓库锁定和修复后的 PR URL 校验。
- Agent 产出必须写入 proof 文件，且 Work Item 详情页能看到实际 runner、model、耗时、token/process 摘要。
- 本地控制动作可以作为 Agent 的输入或证据，但不能替代角色 Agent 的最终人类可读报告。
- Runtime policy、skills、MCP 会物化到隔离 workspace 的 `.omega/.codex/.claude` 等文件中；提交用户仓库时会排除这些运行时目录。
- 对齐 Symphony 的边界思路：可写能力授予隔离 attempt / repository workspace，而不是 Omega 自身工作区。非代码角色默认只读；Coding / Rework / Page Pilot 允许 `workspace-write`，但 runner final-answer proof 由 Omega 捕获并复制到 `.omega/proof`，Agent 不需要直接写 repo 外的 proof 路径。

## 角色映射

| 阶段角色 | 当前执行方式 | Agent 输出 | 本地确定性输入/动作 |
| --- | --- | --- | --- |
| Requirement | 调用 Requirement Agent | `requirement-handoff.md` | `requirement-artifact.json`、Repository boundary、Acceptance Criteria |
| Master | 调用 Master Agent | `master-dispatch.md`、`rework-assessment.md`、`rework-checklist-*.md` | `task-classification.json`、review/CI/human feedback 聚合 |
| Architect | 调用 Architect Agent | `solution-plan.md` | workflow prompt、仓库路径、需求上下文；Omega 会补 TODO guardrail |
| Coding | 调用 Coding Agent | `coding-agent-note.md`、真实 repository diff | workspace 隔离、runner policy、commit/push 由 runtime 执行 |
| Testing | 本地验证 + Testing Agent 复核 | `test-report.md`、`test-report-rework-*.md` | `npm/go/项目命令`输出、GitHub Actions checks 作为证据 |
| Review | 调用 Review Agent | `review-*.md` | PR diff、test report、CI/check logs、review focus |
| Git Recovery | 仅在 PR 发布 / 更新失败时调用 Git Recovery Agent | `git-recovery-publish_pull_request.md`、`git-recovery-update_pull_request.md` | runtime 提供失败原因、git/gh 诊断、目标 branch/base/PR body，并在 Agent 完成后重新校验 PR |
| Delivery | 调用 Delivery Agent | `delivery-handoff.md` 或 `delivery-handoff-fast-rework.md` | `handoff-bundle.json`、review packet、human gate、merge 状态 |
| Human | 人类 / 飞书 / checkpoint decision | checkpoint decision proof | runtime 统一处理 approve/request changes |

## Sandbox 与 proof capture

Omega 不把“能写 proof”理解为“Agent 可以写任意路径”。当前策略是：

- Requirement / Master / Architect / Testing / Review / Delivery 默认使用 `read-only`，只读取隔离 workspace 和证据，由 Codex final answer capture 产出 proof。
- Coding / Rework 使用 `workspace-write`，cwd 锁定到目标 repository checkout，只能编辑当前仓库；commit、push、PR happy path 仍由 runtime 执行。
- 当 Codex 的 `--output-last-message` 目标 proof 位于当前 cwd 之外时，runner 会先把 final answer 捕获到 cwd 内的 `.omega/agent-output/` 或 runtime 临时目录，再由 Omega 复制到 attempt 的 `.omega/proof/`。这样避免“要求 Agent 产出实现记录，但沙箱不允许写 proof 路径”的假失败。
- `.omega`、`.codex`、`.claude`、`.opencode`、`.trae` 等运行时目录会在 `git add` 时排除，不进入用户交付 diff。

## Git Recovery 的使用边界

Git Recovery 对齐 Symphony 的分工：Agent 可以处理 Git/GitHub 工作流里的上下文判断和修复动作，但 runtime 仍保留状态机与最终校验。该 Agent 只能在以下 action 中被触发：

| Stage | Action | 允许动作 | 禁止动作 |
| --- | --- | --- | --- |
| `in_progress` | `publish_pull_request` | `git fetch`、检查 merge-base/history、rebase/cherry-pick/format-patch、重建同名 `omega/*` 交付分支、`git push --force-with-lease`、`gh pr create` | 修改需求 / 测试 / Review / Human gate、发布不同分支、合并 PR、触碰其他仓库 |
| `rework` | `update_pull_request` | 修复现有 PR branch / metadata、重新 push 同名分支、`gh pr edit` 或补建缺失 PR | 改写产品代码逻辑、绕过 review、合并 PR、审批 human gate |

这意味着可以给 `git_recovery` 单独配置更强模型，而不用把 Requirement、Architect、Testing 全部切到高成本模型。Requirement / Architect / Testing 仍专注各自产物；PR 拓扑修复只进入 Git Recovery。

Codex runner 的普通 `workspace-write` 沙箱不能写 `.git/FETCH_HEAD`，因此 `git_recovery` 在 Codex 下会使用 `danger-full-access`。这不是扩大 DevFlow 全局权限：只有 `git_recovery` 角色、且只有上表两个 action 能拿到这个沙箱；prompt、Agent Profile policy 和 runtime PR URL 校验仍限定它只处理当前 repository workspace 的 git/gh delivery 修复。

## 为什么 Testing 里仍然会看到 local-validation

项目测试命令必须真实在 repository workspace 中运行，不能让 Agent 自己声称“测试通过”。因此 Testing 阶段采用两层结构：

1. Omega runtime 运行本地验证命令，收集 stdout/stderr 和退出码。
2. Testing Agent 读取这份输出，产出最终 `test-report.md`，说明通过/失败、覆盖范围和剩余风险。

详情页中 `process.localValidation` 是 Testing Agent 的证据源，不是替代 Testing Agent。GitHub Actions 同理：`github-actions` 是远端 CI 证据源，自动 rework 或 human gate 会继续消费它。

## 为什么 Delivery 不直接合并

Delivery Agent 负责把 PR、diff、测试、review、TODO 复核和风险整理成可交接内容。进入 Human Review 前，runtime 会把 `handoff-bundle.json` 交给 Delivery Agent，要求它只基于 Omega 已捕获的 requirement、solution plan、review packet、PR、测试 / CI 和 changed files 证据生成审核简报；简报会回填到 review packet，并同步进入飞书卡片、文档、普通文本和 Task description。

真正的 merge、checkpoint approve/request changes、GitHub API 调用仍由 runtime 执行，因为这些动作需要幂等、权限检查和可恢复状态机。Delivery Agent 不允许改写风险等级、TODO 状态、审批结论或部署状态，只能说明审核人应该重点确认什么。

PR 创建或更新属于 delivery workflow 的一部分，但错误类型比 merge / approve 更依赖上下文。例如 `branch has no history in common with main`、远端 branch 漂移、已有 PR 状态异常等情况，runtime 很难安全枚举所有修复路径。Omega 现在采用两层策略：正常路径由 runtime 快速执行；失败时只在 `publish_pull_request` / `update_pull_request` 两个 action 中启动 Git Recovery Agent，并在 Agent 结束后重新执行 PR 校验。

## 当前 proof 文件

- Requirement: `requirement-artifact.json` + `requirement-handoff.md`
- Master: `task-classification.json` + `master-dispatch.md`
- Architect: `solution-plan.md`
- Coding: `coding-prompt.md` + `coding-agent-note.md` + `git-diff.patch` + `implementation-summary.md`
- Testing: `test-report.md` / `test-report-rework-*.md`
- Review: `review-*.md`
- Git Recovery: `git-recovery-publish_pull_request.md` / `git-recovery-update_pull_request.md`
- Human Review: `human-review-request.md` + `review-packet.md` + review packet 中的 `humanReviewBrief`
- Delivery: `handoff-bundle.json` + `delivery-handoff.md`

## 验证建议

轻量回归：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestRunDevFlowPRCycleCreatesBranchPRAndMergeProof|TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback|TestProjectAgentProfilePersistsAndFeedsRuntimeBundle|TestProfileSkillsAndMCPAreMaterializedForRunnerProcess' -count=1 -timeout=120s
go test ./services/local-runtime/internal/omegalocal -run 'TestDevFlowGitHubRecovery|TestDevFlowTemplateLoadsWorkflowMarkdownContract' -count=1 -timeout=120s
```

UI/配置回归：

```bash
npm run test -- apps/web/src/components/__tests__/WorkspaceAgentStudio.test.tsx --testTimeout=30000
npm run lint
git diff --check
```
