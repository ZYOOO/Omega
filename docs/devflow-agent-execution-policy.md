# DevFlow Agent Execution Policy

本文定义 Omega DevFlow 中“应该由 AI Agent 完成”的阶段，以及仍然必须由本地确定性执行器完成的控制动作。目标是让 Work Item 详情页、proof、飞书审核和演示讲解保持一致：Agent 负责理解、计划、编码、复核、交接；Omega runtime 负责仓库隔离、状态机、GitHub 操作、数据库落库和可审计 proof。

## 核心原则

- 角色型阶段必须调用对应 Agent Profile：Requirement、Master、Architect、Coding、Testing、Review、Delivery。
- 本地执行器只能做确定性控制动作：clone workspace、写 machine-readable JSON、运行本地验证命令、git add/commit/push、创建/更新 PR、读取 GitHub checks、checkpoint decision、merge、状态落库。
- Agent 产出必须写入 proof 文件，且 Work Item 详情页能看到实际 runner、model、耗时、token/process 摘要。
- 本地控制动作可以作为 Agent 的输入或证据，但不能替代角色 Agent 的最终人类可读报告。
- Runtime policy、skills、MCP 会物化到隔离 workspace 的 `.omega/.codex/.claude` 等文件中；提交用户仓库时会排除这些运行时目录。

## 角色映射

| 阶段角色 | 当前执行方式 | Agent 输出 | 本地确定性输入/动作 |
| --- | --- | --- | --- |
| Requirement | 调用 Requirement Agent | `requirement-handoff.md` | `requirement-artifact.json`、Repository boundary、Acceptance Criteria |
| Master | 调用 Master Agent | `master-dispatch.md`、`rework-assessment.md`、`rework-checklist-*.md` | `task-classification.json`、review/CI/human feedback 聚合 |
| Architect | 调用 Architect Agent | `solution-plan.md` | workflow prompt、仓库路径、需求上下文；Omega 会补 TODO guardrail |
| Coding | 调用 Coding Agent | `coding-agent-note.md`、真实 repository diff | workspace 隔离、runner policy、commit/push 由 runtime 执行 |
| Testing | 本地验证 + Testing Agent 复核 | `test-report.md`、`test-report-rework-*.md` | `npm/go/项目命令`输出、GitHub Actions checks 作为证据 |
| Review | 调用 Review Agent | `review-*.md` | PR diff、test report、CI/check logs、review focus |
| Delivery | 调用 Delivery Agent | `delivery-handoff.md` 或 `delivery-handoff-fast-rework.md` | `handoff-bundle.json`、review packet、human gate、merge 状态 |
| Human | 人类 / 飞书 / checkpoint decision | checkpoint decision proof | runtime 统一处理 approve/request changes |

## 为什么 Testing 里仍然会看到 local-validation

项目测试命令必须真实在 repository workspace 中运行，不能让 Agent 自己声称“测试通过”。因此 Testing 阶段采用两层结构：

1. Omega runtime 运行本地验证命令，收集 stdout/stderr 和退出码。
2. Testing Agent 读取这份输出，产出最终 `test-report.md`，说明通过/失败、覆盖范围和剩余风险。

详情页中 `process.localValidation` 是 Testing Agent 的证据源，不是替代 Testing Agent。GitHub Actions 同理：`github-actions` 是远端 CI 证据源，自动 rework 或 human gate 会继续消费它。

## 为什么 Delivery 不直接合并

Delivery Agent 负责把 PR、diff、测试、review、TODO 复核和风险整理成可交接内容。真正的 merge、checkpoint approve/request changes、GitHub API 调用仍由 runtime 执行，因为这些动作需要幂等、权限检查和可恢复状态机。

## 当前 proof 文件

- Requirement: `requirement-artifact.json` + `requirement-handoff.md`
- Master: `task-classification.json` + `master-dispatch.md`
- Architect: `solution-plan.md`
- Coding: `coding-prompt.md` + `coding-agent-note.md` + `git-diff.patch` + `implementation-summary.md`
- Testing: `test-report.md` / `test-report-rework-*.md`
- Review: `review-*.md`
- Human Review: `human-review-request.md` + `review-packet.md`
- Delivery: `handoff-bundle.json` + `delivery-handoff.md`

## 验证建议

轻量回归：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestRunDevFlowPRCycleCreatesBranchPRAndMergeProof|TestPrepareDevFlowHumanRequestedReworkStartsAttemptWithFeedback|TestProjectAgentProfilePersistsAndFeedsRuntimeBundle|TestProfileSkillsAndMCPAreMaterializedForRunnerProcess' -count=1 -timeout=120s
```

UI/配置回归：

```bash
npm run test -- apps/web/src/components/__tests__/WorkspaceAgentStudio.test.tsx --testTimeout=30000
npm run lint
git diff --check
```
