# Omega DevFlow Workflow

这份文档把外部参考项目模板改写成 Omega 当前产品形态下的工作流契约。

## 1. 核心变化

原模板服务于一套外部 issue tracker + 本地编排器的组合：

- 每个项目有自己的 Project slug。
- 每个项目有自己的目标仓库地址和默认分支。
- 每个项目有自己的 workspace 根目录。
- `WORKFLOW.md` 描述 Codex/Claude/PR/review/human review 的固定流程。

Omega 自己实现 Workboard，所以这些配置应尽量进入 App 和本地服务端，而不是让用户维护一堆 `.env.local`。

第一阶段的真实工程闭环默认依赖 GitHub。Omega 自己负责需求、Item、Pipeline、Agent、Checkpoint 和 Proof 的编排；GitHub 负责承载代码交付协议：

```text
GitHub issue / manual requirement
  -> Omega Requirement
  -> Omega Item
  -> isolated branch
  -> PR
  -> checks / review
  -> merge / delivery proof
```

因此，“GitHub issue 不是 Omega 核心对象”不代表 GitHub 可有可无；它表示 GitHub issue 不能替代 Requirement / Item 的内部边界。进入真实执行后，Item 必须绑定 GitHub repository target。

## 2. 配置映射

| 模板概念 | Omega 实现 |
| --- | --- |
| Project slug | Omega `Project` / `Repository workspace` |
| Requirement | Omega `Requirement`，保存用户原始需求或外部 issue/message 来源 |
| Work item | Omega `Item`，从 Requirement 派生出来的可执行单元 |
| Target repo URL | Omega `Repository target` |
| Default branch | Repository target 的 `defaultBranch` |
| Workspace root | App 里的 `Workspace location`，通过 `GET/PUT /local-workspace-root` 持久化 |
| Workflow file | 内置或未来可编辑的 `Pipeline template` |
| Human Review | Omega `Checkpoint` + Feishu/lark-cli 通知 |
| Proof of work | Omega `ProofRecord` + 本地 proof files |

## 3. 当前可执行模板：devflow-pr

`devflow-pr` 是当前最接近 Omega DevFlow 周期的本地模板。默认定义已经从纯 Go hardcode 抽到 Markdown workflow：

```text
/Users/zyong/Projects/Omega/services/local-runtime/workflows/devflow-pr.md
```

当前 Go runtime 会解析该文件的 front matter，并把 stage、agents、artifact、review round 编译成 Pipeline run。前期可以继续使用这个默认模板；后续 App 会允许按 repository workspace 编辑和覆盖该模板。

```text
Requirement
  -> Item
  -> requirement intake
  -> solution design
  -> coding
  -> testing
  -> code review round 1
  -> code review round 2
  -> rework if review requests changes
  -> human review
  -> merging
  -> done
```

当前执行会在隔离 workspace 下产出一组可交接 artifact：

| Artifact | 作用 |
| --- | --- |
| `requirement-artifact.json` | 固化 Requirement、Item、验收标准、repo target、默认分支等输入 |
| `solution-plan.md` | 固化方案、影响范围、阶段交接计划 |
| `implementation-summary.md` | 固化 branch、commit、PR、changed files 和 git summary |
| `test-report.md` | 固化本轮最小测试命令与结果 |
| `code-review-round-1.md` | 固化第一轮 PR diff review 证据 |
| `code-review-round-2.md` | 固化第二轮 checks / delivery readiness review 证据 |
| `rework-summary-N.md` | 固化第 N 次 review feedback 修复、commit 与 changed files |
| `human-review-request.md` | 固化等待人工审核的上下文、PR、Review Agent verdict 和待决策内容 |
| `human-review.md` | 用户 Approve 后生成，固化真实人工 gate 决策 |
| `merge.md` | 用户 Approve 后生成，固化 PR merge 结果 |
| `handoff-bundle.json` | 把上面所有阶段输出串成最终交接包；人工审核前标记为 pending，审核通过后更新为 approved / merged |

运行时必须满足：

1. Item 已绑定 `requirementId`。
2. Item 已绑定 `repositoryTargetId`。
3. Repository target 能解析出 GitHub owner/repo、clone target 和 default branch。
4. 本机 `git` 和 `gh` 可用。
5. GitHub 目标 repo 允许当前 `gh` 登录用户 push branch / create PR / merge PR。

执行语义：

1. `run-devflow-cycle` 默认异步启动 Attempt，并把后续阶段交给后台 job。
2. 后台 job 会执行 requirement / planning / implementation / review round 1 / review round 2。
3. Review Agent 必须输出明确 verdict：`APPROVED` 继续下一轮，`CHANGES_REQUESTED` 进入 `rework`，`NEEDS_HUMAN_INFO` 进入人工等待。
4. `CHANGES_REQUESTED` 是正常业务流转，不是 Attempt 失败。Omega 会把 review report 作为 Rework Agent 输入，在同一 workspace、同一 branch、同一 PR 上继续修改，验证后回到 Code Review。
5. 自动 rework 达到 workflow 的 `maxReviewCycles` 后，会停在 Human Review，让人类决定继续修改、接受风险或终止。
6. 进入 `human_review` 后，Pipeline 状态为 `waiting-human`，不会自动 merge。
7. 用户在 Checkpoint UI 或 API 中 Approve 后，Omega 才执行 merge / delivery；Reject 会携带原因回退，等待重做。

## 4. 安全边界

Repository 是高风险边界。Omega 的执行器不能从全局默认路径或自由文本里猜仓库。

正确顺序是：

```text
Active Repository workspace
  -> Item.requirementId
  -> Item.repositoryTargetId
  -> GitHub repository target + default branch
  -> Isolated local workspace
  -> Branch / commit / PR / checks / review / merge proof
```

如果 Item 没有 `requirementId`、没有 repository target，或者它不属于当前打开的 Repository workspace，App 和本地服务端都应该拒绝运行。

## 5. 后续演进

- 当前本地 orchestrator 已有 `POST /orchestrator/tick`：它会从绑定的 GitHub repo 读取 open issues，只 claim 带 `omega-ready` / `devflow-ready` / `agent-ready` / `omega-run` 标签的 issue，先创建 `source = github_issue` 的 Requirement，再创建 repository-scoped Item 和 `devflow-pr` Pipeline；显式传入 `autoRun` 时会创建 Attempt，并把 PR 周期交给后台 job 执行。
- 手动 `Run` 也走同一语义：先返回 `accepted` 和 Attempt，前端通过轮询展示后续 stage / Agent / proof 进度；流程会停在 Human Review，等待真实用户决策。
- 把 `devflow-pr` 从默认 Markdown 模板继续升级成 App 内可编辑模板，并支持 repository workspace 覆盖。
- 把上述 artifact 从 proof file 继续升级成数据库一等 artifact 表。
- `AgentRunner` 已抽出第一版，当前默认 Codex CLI；下一步继续接入 opencode / Claude Code / 长期 session runner。
- 抽出正式 `JobSupervisor`，补齐 heartbeat、stall retry、cancel、timeout、多 turn continuation 和 worker host 分配。
- 把 GitHub issue comment / label / PR status 回写补齐。
- 把 Feishu checkpoint card 接入人工确认，而不是只做文本通知。
