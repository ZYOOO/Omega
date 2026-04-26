# Omega Work Model：产品对象关系

## 1. 结论

Omega 应该有自己的 Workboard 信息架构，并且不要照搬 GitHub 或任何外部 tracker 的对象边界。

推荐关系是：

```text
Workspace
  -> Team
      -> Project
          -> Repository target
          -> Requirement
              -> Item
                  -> Pipeline run
                      -> Stage
                          -> Mission / Operation / Proof / Checkpoint
```

其中最重要的边界是：

```text
Project 不等于 GitHub repo
Issue 不等于 Omega 核心对象
Requirement 是需求源
Item 是内部可执行工作单元
Pipeline run 是 Item 被 AI 执行时生成的一次运行记录
```

## 2. Omega 的核心对象

| Omega | 说明 |
| --- | --- |
| Workspace | 一个组织或一个本地/共享控制面的最高空间 |
| Team | 一组人或一个产品/工程小组，拥有自己的 work item workflow |
| Project | 一个有目标、进度、负责人、target 的研发目标 |
| Repository target | Project 绑定的 GitHub repo 或本地 repo path |
| Requirement | 用户需求或外部来源的产品语义，可以来自手动、GitHub issue、飞书或 API |
| Item | 最小可排期、可执行、可审核的任务单元，由 Requirement 生成或链接 |
| Views | 对 work items / projects / pipelines 的过滤和排序 |
| Project phase / milestone | 可选，用于大项目的阶段拆分 |
| Cycle / Sprint | 可选，第一阶段不优先实现 |

## 3. Project 和 GitHub repo 的关系

`Project` 不应该简单等于 GitHub repo。

更合理的是：

```text
Project = 一个产品/工程目标
Repository target = Project 的代码落点之一
```

原因：

1. 一个 repo 可以承载多个 Project。
   - 例如同一个 `omega-app` repo 里可以有 “Workboard UI 重构”、“Pipeline engine v1”、“Feishu checkpoint integration” 三个 Project。

2. 一个 Project 也可能涉及多个 repo。
   - 例如 “Feishu 审批闭环” 可能同时影响 desktop app、local runtime、shared control plane、docs site。

3. 有些 Project 主要是产品/流程/文档工作，不一定马上绑定 repo。

所以 Omega 中 Project 应该有：

- `id`
- `key`
- `name`
- `description`
- `teamId`
- `status`
- `health`
- `lead`
- `targetDate`
- `repositoryTargets[]`
- `defaultRepositoryTargetId`
- `labels`
- `createdAt`
- `updatedAt`

`repositoryTargets[]` 是代码落点，不是 Project 本体。

第一阶段产品虽然不把 GitHub repo 当成 Project 本体，但工程交付必须依赖 GitHub。也就是说：

```text
Project 可以没有 repo
但只要 Item 要进入真实 DevFlow 执行，就必须解析到一个 Repository target
第一阶段默认 Repository target 是 GitHub repo
```

GitHub 在 Omega 里的定位是“交付协议层”，负责承载：

- `Issue`：可作为 Requirement 来源，也可作为执行状态同步目标。
- `Branch`：每次 Pipeline run / Operation 的隔离代码变更载体。
- `Pull Request`：代码评审、CI、交付审核的主对象。
- `Review / Checks`：测试、质量门禁和人工确认的证据来源。
- `Merge`：完成交付的最终工程动作。

所以正确边界不是“是否依赖 GitHub”，而是：

```text
产品对象不照搬 GitHub
工程闭环强绑定 GitHub delivery contract
```

## 4. Requirement / Issue / Item 的关系

`Requirement` 是 Omega 的需求源对象。

它可以来自：

- 用户手动创建
- GitHub issue 导入
- Feishu 群聊 requirement
- 评委现场命题
- API 或未来共享控制面同步

`Issue` 不是 Omega 内部的核心对象边界。GitHub issue 只是一种外部来源，会被记录成：

```text
Requirement.source = github_issue
Requirement.sourceExternalRef = owner/repo#123
```

`Item` 是 Omega 内部真正进入编排和执行的工作单元。一个 Requirement 可以生成一个或多个 Item：

```text
Requirement: “支持在页面圈选元素并修改文案”
  -> Item: “实现圈选层”
  -> Item: “实现代码定位”
  -> Item: “实现热更新预览”
  -> Item: “创建 PR 并生成摘要”
```

Item 应该表达的是：

```text
谁要做什么，为什么做，验收标准是什么，当前状态是什么，最终会被哪个 Pipeline 执行。
```

推荐字段：

- `id`
- `key`
- `requirementId`
- `projectId`
- `teamId`
- `title`
- `description`
- `status`
- `priority`
- `assignee`
- `creator`
- `labels`
- `source`
- `sourceExternalRef`
- `repositoryTargetId`
- `branchName`
- `acceptanceCriteria[]`
- `estimate`
- `parentItemId`
- `blockedByItemIds[]`
- `createdAt`
- `updatedAt`

Requirement 和外部系统的关系：

| 来源 | Omega 表达 |
| --- | --- |
| 手动创建 | `Requirement.source = manual` |
| GitHub issue | `Requirement.source = github_issue`, `Requirement.sourceExternalRef = owner/repo#123` |
| Feishu 消息 | `Requirement.source = feishu_message`, `Requirement.sourceExternalRef = chat/message id` |
| AI 拆解 | `Requirement.source = ai_generated`，生成多个 linked Item |

## 5. Pipeline / Mission / Operation 的关系

Item 是产品工作项；Pipeline 是它被执行时的流程实例。

推荐关系：

```text
Requirement 1
  -> Item 1
  -> Pipeline run 1
      -> Stage: intake
          -> Mission
          -> Operation attempt
          -> Proof records
          -> Checkpoint
      -> Stage: solution
      -> Stage: coding
      -> Stage: testing
      -> Stage: review
      -> Stage: delivery
```

这意味着：

- 一个 Work item 可以有多次 Pipeline run。
  - 比如第一次失败后重跑。
  - 或者 reject 后保留旧 run，再开新 run。

- Stage 不是 Work item。
  - Stage 是 Pipeline 内部步骤。
  - 当前代码里曾经把每个 stage 展开成 item，这适合早期 demo，但长期会让模型混乱。

- Mission 是某个 stage 的执行任务包。
  - 包含 agent 角色、输入上下文、prompt、工具、输出契约。

- Operation 是 Mission 的一次具体执行 attempt。
  - 可以失败、重试、记录耗时、token、stdout/stderr、proof。

## 6. 推荐第一阶段产品层级

第一阶段为了比赛演示，不需要一次做满所有对象，但需要把边界定稳。

优先实现：

```text
Workspace
Team
Project
Repository target
Requirement
Item
Pipeline run
Checkpoint
Operation
Proof record
View
```

可以暂缓：

```text
Initiative
Cycle
Milestone
Release
Roadmap
```

## 7. Omega 和 GitHub 的推荐映射

| Omega | GitHub |
| --- | --- |
| Project | 不直接等于 repo，可绑定一个或多个 repo |
| Repository target | GitHub repo 或本地 repo path |
| Requirement | 可由 GitHub issue 导入，也可同步成 GitHub issue |
| Item | Omega 内部执行单元，必须绑定明确 requirementId / repositoryTargetId 后才能运行 |
| Pipeline run | 可对应一个 branch / PR / checks 组合 |
| Operation | 本地 codex/opencode/git/gh/test 的一次执行 |
| Proof record | test logs、diff summary、PR URL、CI URL、artifact path |
| Checkpoint | PR review、Feishu approval、人工 approve/reject |

第一阶段默认执行规则：

1. App 内手动创建 Requirement 时，如果当前处于 Repository workspace，自动继承该 GitHub repository target。
2. GitHub issue 导入时，先创建 `Requirement.source = github_issue`，再创建对应 Item。
3. Item 进入 Pipeline 前必须有 `repositoryTargetId`。
4. Runner 必须在 workspace root 下 clone / checkout 该 target，不能从自由文本或当前目录猜仓库。
5. Coding stage 必须产出 branch / commit / diff proof。
6. Review / delivery stage 必须围绕 PR、checks、review、merge 形成 proof。

## 8. 当前代码需要调整的地方

当前实现里已经有：

- `projects`
- `requirements`
- `work_items`
- `pipelines`
- `checkpoints`
- `missions`
- `operations`
- `proof_records`

当前已对齐：

1. `Project` 已显式支持 `repositoryTargets[]`，不再使用 `repositoryOwner / repositoryName / repositoryUrl` 表达 Project 本体。
2. `Requirement` 已成为一等持久化对象，手动需求、GitHub issue import、orchestrator claim 都会创建或链接 Requirement。
3. `WorkItem` 已补 `requirementId / source / sourceExternalRef / repositoryTargetId / acceptanceCriteria / blockedByItemIds`。
4. GitHub issue import 会先落成 `Requirement.source = github_issue`，再生成对应 Item，并记录 external ref 与 repository target id。
5. Pipeline 创建从 Item 触发，Pipeline stage 不再被表现成独立 Work item。
6. Project 页面已支持通过本机 `gh` 仓库列表选择 repo，并在当前 Project 下创建 / 打开对应的 Repository workspace；issues、PR、pipeline runs 等 repo 细节入口放在 workspace 内部。

还需要调整：

1. UI 里的 `Project` 页面还需要继续展示 project health、active work items、linked pipelines。
2. SQLite 结构仍需要从 snapshot-first 继续推进到 repository-first，让 repository targets 和 work item metadata 都可查询。
3. Views 应该是过滤器，例如：
   - Active work items
   - My work
   - Waiting for human
   - GitHub imported
   - Failed pipelines

## 9. 命名建议

为了避免“item”太泛，UI 可以逐步改成：

```text
Work item
```

代码里可以保留 `WorkItem`，但产品文案不要只写 `item`。

推荐中文语义：

- Project：项目 / 研发目标
- Requirement：需求源 / 需求记录
- Item：工作项 / 执行项
- Pipeline run：流水线运行
- Mission：阶段任务包
- Operation：执行尝试
- Proof：证据
- Checkpoint：人工检查点
