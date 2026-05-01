# Omega 待办清单

这份待办清单用于跟踪 Omega 朝比赛目标推进的主线工作：做出自己的 Workboard、自己的 DevFlow 执行系统，以及后续可兼容的飞书人工校验与工具调用链。

当前产品主轴：

```text
Requirement 需求源
  -> Item 内部可执行工作项
  -> Pipeline 流程编排
  -> Agent 协作与安排
  -> GitHub 工程管理与交付证据
```

因此，GitHub 是第一阶段最核心的外部工程管理依赖；Feishu 仍然是重要的人机协作入口，但优先级要服从这个主闭环。

当前对象从属关系：

```text
Project
  -> Repository target
  -> Requirement
      -> Item
          -> Pipeline run
              -> Mission
                  -> Operation
                      -> Proof
```

这里 `Requirement` 是用户需求或外部来源的产品语义；GitHub issue、飞书消息、手动输入都只是 Requirement 的 source。`Item` 是 Omega 内部真正可排期、可运行、可审计的执行单元；一个 Requirement 后续可以拆成一个或多个 Item。

第一阶段的真实工程闭环默认强依赖 GitHub：

```text
Item
  -> GitHub repository target
  -> branch / commit
  -> pull request
  -> checks / review
  -> merge / delivery proof
```

所以“GitHub issue 只是 Requirement 的 source”不等于 GitHub 不重要。Omega 不照搬 GitHub 的对象边界，但会把 GitHub repo / issue / PR / checks / review / merge 作为默认交付协议。

## 2026-04-30 八项测试闭环计划

本节记录本轮拆出的 8 个高优先级验收项。下面均已落地；更细的后续增强继续留在原有功能一 / Page Pilot 产品化清单中跟踪。

- [x] 功能一：GitHub delivery contract preflight。运行前校验 repository target、`gh` 登录、branch / PR 权限和 PR/checks 元数据读取能力，避免 Human Review 才暴露交付权限问题。
- [x] 功能一：Reject -> Rework 可视化。Work Item 详情页展示人工拒绝、rework assessment、rework checklist 和回流状态，让用户能看懂为什么回去改、会怎么改。
- [x] 功能一：标准测试脚本与测试报告。新增 `npm run test:feature-p0`，并新增 `docs/test-report.md` 记录本轮测试命令、结果和剩余手测项。
- [x] 功能一：Go runtime 全量测试可靠性。补齐 fake `gh` 合约，`go test ./services/local-runtime/internal/omegalocal` 可覆盖新增 preflight / Page Pilot runtime / multi-round / visual proof。
- [x] 功能二：Go Preview Runtime supervisor/API。新增 `/page-pilot/preview-runtime/resolve|start|restart`，锁定明确 Repository Workspace，持久化 profile / pid / stdout / stderr / health check 基础信息。
- [x] 功能二：Page Pilot 结果面板。Recent runs 详情弹窗展示 diff summary、PR body preview、Work Item 回跳、source mapping、visual proof 和 conversation。
- [x] 功能二：同一 Page Pilot run 多轮追加。`/page-pilot/apply` 支持 `runId`，同一 run 内追加批注 / 追加说明并递增 round。
- [x] 功能二：视觉 proof。Page Pilot run 生成 DOM snapshot visual proof，并同步到 Work Item / pipeline artifacts / PR preview。

## 功能一核心 Todo（下一阶段补强）

这些是功能一从“可演示闭环”升级到“可托管运行”的核心缺口，优先级高于继续扩展功能二体验。

当前执行优先级：workspace cleanup / repo-owned workflow contract / prompt sections / JobSupervisor worker-host 与恢复基础版已完成；CI 自动 rework、远端 signal polling 和 Workflow Template 一等化已在 2026-05-02 补齐。第四项按当前决策暂缓，Run Timeline 深挖、统一 E2E 报告不从清单删除。

- [x] 建立正式 JobSupervisor v1：常驻扫描 Ready Work Item、running attempt、failed/stalled attempt、waiting-human gate 和 workflow contract；危险写仓库动作由显式策略开关控制。
- [x] JobSupervisor daemon 基础版：Go Local Runtime 启动时默认开启后台维护 tick，周期性执行 checkpoint integrity、stalled detection 和 Ready item preflight scan；默认不自动启动 Ready item，必须显式开启 `job-supervisor-auto-run-ready`。
- [x] JobSupervisor runnable scan 基础版：`POST /job-supervisor/tick` 会扫描 Ready + repository target 的 Work Item，执行 DevFlow preflight，并在 `autoRunReady=true` 时创建 Attempt 交给后台 job。
- [x] JobSupervisor integrity tick 基础版：`POST /job-supervisor/tick` 扫描 pending human gates，修复 checkpoint -> attempt 断链，并在必要时 backfill 可审计 attempt。
- [x] JobSupervisor heartbeat 基础版：Attempt 创建、Agent invocation、完成/失败会写入 `lastSeenAt`，作为运行健康判断的主字段。
- [x] JobSupervisor stalled detection 基础版：`POST /job-supervisor/tick` 会扫描 running attempts，超过阈值未更新 heartbeat 时标记为 `stalled`，Pipeline 进入 `stalled`，Work Item 进入 `Blocked`，并写 runtime ERROR log。
- [x] 扩展 heartbeat 基础版：runner process stdout/stderr 流和长时间 Codex/opencode/Claude Code 子进程会写入 attempt event、runtime DEBUG log，并周期性刷新 `lastSeenAt`；旧做法只在 Attempt 创建、Agent invocation 和完成/失败时刷新。
- [x] 扩展 heartbeat：JobSupervisor 会对 running / waiting-human Attempt 轮询 PR checks / required checks，并消费远端 worker host `lastSeenAt` 刷新运行心跳；旧做法只依赖本机 runner 进程和 attempt event。
- [x] Attempt retry API 基础版：`POST /attempts/{id}/retry` 支持 failed / stalled / canceled attempt 创建新 Attempt，保留旧 Attempt，并写入 retry metadata。
- [x] 扩展 stalled recovery 基础版：JobSupervisor 可扫描 stalled / failed Attempt，按 retry 上限和 backoff 生成 recoverable summary，并在 `autoRetryFailed=true` 时创建真实 retry Attempt。
- [x] 增加自动恢复策略基础版：`autoRetryFailed` + `maxRetryAttempts` + `retryBackoffSeconds` 支持有限次数自动 retry 和 backoff；更细的失败分类仍保留为后续项。
- [x] 扩展自动恢复策略：继续区分 runner crash、临时网络失败、GitHub API 临时失败、CI flaky failure、权限失败，并给出不同动作。
- [x] 增加 runner process context timeout/cancel 基础：Codex / opencode / Claude Code 子进程使用 context-aware supervisor，deadline/cancel 会真实终止子进程并返回 `timed-out` / `canceled` process status。
- [x] 增加 Attempt cancel API 基础：`POST /attempts/{id}/cancel` 会向本机注册的 background job 发送 cancel signal，并把 Attempt / Pipeline / Work Item 状态落库。
- [x] 扩展 timeout / retry policy 基础版：`devflow-pr` workflow runtime 开始驱动 runner heartbeat、Attempt timeout、retry 上限和 backoff；旧做法主要依赖 Go 常量和 API 参数。
- [x] 扩展 cancel / timeout 策略基础版：workspace cleanup、worker host lease 和 workflow runtime timeout/retry 已纳入 JobSupervisor；GitHub/git operation timeout 继续保留为后续项。
- [x] 建立 append-only runtime log 基础版：API request、DevFlow job、agent invocation、checkpoint decision、Page Pilot apply/deliver/discard、PR merge 等事件写入结构化日志。
- [x] 新增 runtime log API 基础版：`GET /runtime-logs` 支持按 project / repository target / work item / pipeline / attempt、level、event type 等条件查询。
- [x] 扩展 runtime log 查询：补齐 Requirement 维度、cursor pagination、全文搜索和导出。
- [x] Operator UI 增加 Run Timeline 基础版：`GET /attempts/{id}/timeline` 聚合 runtime log、attempt events、stage status、operation、proof、checkpoint decision，并在 Work Item 详情页展示。
- [ ] 暂缓：扩展 Run Timeline：补齐 cursor pagination、runner stdout/stderr 摘要展开、GitHub checks/rebase/conflict 事件和按 stage/agent 过滤。
- [x] 增加数据分析指标基础版：`/observability.dashboard` 返回 Attempt 成功率、失败原因分布、慢阶段、待人工队列、活跃运行和推荐动作。
- [x] 扩展数据分析指标：继续补 stage 平均耗时、runner 使用次数、checkpoint 等待时长、PR 创建/合并数量和趋势统计。
- [x] 扩展 `/observability` dashboard 基础版：保留旧 summary 字段，并新增 dashboard data，供 UI/CLI 后续消费。
- [ ] 扩展 `/observability` dashboard：补齐时间窗口、分组统计、趋势、最近失败详情和慢阶段 drilldown。
- [ ] 暂缓：增加接口测试套件：按 OpenAPI 覆盖 requirement、pipeline、attempt、checkpoint、proof、GitHub delivery、Page Pilot linkage 的 smoke / e2e。
- [ ] 暂缓：生成统一测试报告：把 API 测试、Go 测试、前端测试、一次端到端演示结果汇总到 `docs/test-report.md`。
- [x] 强化 workspace lifecycle 基础版：DevFlow run 统一生成 workspace lifecycle spec，写入 `.omega/workspace-lifecycle.json`，并在 manual run / retry / JobSupervisor auto run 中声明 execution lock；旧做法主要依赖分散的 workspace path 计算和 active attempt 判断。
- [x] 增加 workspace cleanup 策略：已完成 Attempt 可按 retention 清理 repo checkout 并保留 `.omega` proof/lifecycle，失败/取消/stalled 默认保留；`POST /workspaces/cleanup` 和 JobSupervisor 显式开关会写回 attempt cleanup metadata。
- [x] 强化并发控制基础版：DevFlow manual run / retry / JobSupervisor auto run 对同一 repository workspace scope 使用 execution lock，并在 preflight 中提示冲突；旧做法只有部分 GitHub issue auto-run 有 lock。
- [x] 增加 CI/checks 处理基础版：`/github/pr-status` 汇总 passed / pending / failed / missing required checks，并输出 delivery gate 和推荐动作；旧做法只透传 `gh pr checks` 原始列表。
- [x] 增加 rebase / branch sync 检测基础版：`/github/pr-status` 在提供 repository/workspace path 时用真实 git fetch / merge-base / merge-tree 判断 current / behind / conflict；旧做法不判断 PR 分支是否落后。
- [x] 增加 merge conflict 检测基础版：发现冲突时 delivery recommended action 输出 merge-conflict，供 Human Review / Rework 决策；自动生成 rework instruction 仍是后续项。
- [x] 增加 Rework Checklist 运行时基础版：旧做法把 review、人工反馈、失败原因和 PR/check 推荐动作分散展示；新做法生成 Attempt / Workpad 级 `reworkChecklist`，Retry API 和 Rework prompt 会直接消费。
- [x] 扩展自动回归 / 自动修复重试：Review Agent 或 CI / required checks 发现阻塞问题时，在 `maxReviewCycles` 内自动进入 Rework，再回到测试与评审；PR comments/reviews、failed check log、required checks 和 checklist 会汇入 rework input。
- [x] 让 JobSupervisor 消费 workflow contract 基础版：tick 校验/回填 devflow pipeline 的 workflow source、runtime、review rounds、transitions，并在 summary/log 中暴露 contract 状态。
- [x] 把 workflow contract 升级为 repo-owned 运行协议：目标仓库 `.omega/WORKFLOW.md` 优先于默认模板；Agent Profile 中的 front matter workflow markdown 可作为 Project / Repository override。
- [x] 扩展 workflow contract 消费基础版：timeout、retry、required checks 和 runner heartbeat 已从 `devflow-pr` runtime 读取；旧做法保留在 Go 常量、CLI flag 或 API payload 中。
- [x] 扩展 workflow contract 消费：requirement / architect / coding / testing / rework / review / delivery prompt sections、Project / Repository override、cleanup retention、continuation turns 已从契约读取；runner policy 和 stage-specific timeout 继续后续增强。旧做法只完整覆盖 coding / rework / review。
- [x] 增加 workflow action graph 基础版：`devflow-pr` workflow contract 新增 `states.actions`、action verdict / transition、taskClasses 和 hooks snapshot；Pipeline run 会保存 states、扁平 actions、taskClasses、hooks 与 executionMode，`GET /workflow-templates` 可返回这些元数据。
- [x] 增加 workflow action graph 校验基础版：加载 workflow 时校验 action id、action type、state/action transition 指向的 stage，避免配置错误进入运行链路。
- [x] 扩展 workflow transition 消费：Agent invocation 后的 stage 推进优先读取 Pipeline snapshot transitions，缺失时才回退旧 DevFlow 顺序；旧做法完全依赖固定 Go switch。
- [x] 迁移通用 action executor 阶段 2 基础版：新增 `GET /attempts/{attemptId}/action-plan`，从 Pipeline workflow snapshot 生成当前 state、current action、state actions、可达 transitions、retry action 和恢复策略，不执行真实写仓库动作。
- [x] 迁移通用 action executor 阶段 2 增强：JobSupervisor recovery summary / accepted retry job 会附带 action plan 摘要，自动恢复决策与 workflow snapshot 对齐。
- [x] 迁移通用 action executor 阶段 2 UI：Work Item 详情页的 Delivery flow、Attempt stage、Retry / Rework signal 已直接消费 `/attempts/{attemptId}/action-plan`，旧的 stage / attempt 状态推断只作为兼容 fallback。
- [ ] 迁移通用 action executor 阶段 3：先把 review / rework / merging 从 DevFlow 固定函数迁到 action handler，保留现有真实 PR/check/proof 行为。
- [x] 增强 Review / Rework 交接契约：Review Prompt 必须输出 Summary、Blocking findings、Validation gaps、Rework instructions、Residual risks；旧做法主要依赖 verdict line，容易让 retry/rework 缺少可执行原因。
- [x] 补齐全 Agent 交接契约：Requirement、Architect、Testing、Delivery 的 prompt section 和 Agent output contract 已统一为结构化 handoff，避免只在 Review 阶段有明确原因和下一步。
- [x] 增加 workflow contract 校验基础版：加载 repo/profile workflow 时检查 stage id、transition 引用、review round 引用、agent 和 runtime 非负值，失败时阻止运行。
- [x] 增加 run report / review packet 基础版：DevFlow 进入 Human Review 前生成 `attempt-run-report.md`，聚合需求、PR、changed files、测试、checks、review 和 artifact。
- [x] 扩展 run report / review packet：补结构化 diff/test/check preview、风险分级、下一步推荐动作和前端一页预览。
- [x] 增加 Run Workpad UI 基础版：旧做法把 Requirement、Attempt、Agent trace、Proof 分散展示；新做法先在 Work Item 详情页聚合 Plan、Acceptance Criteria、Validation、Notes、Blockers、PR、Review Feedback、Retry Reason，并全部来自真实 Requirement / Pipeline / Attempt / Operation / Proof / Checkpoint / PR status 记录。
- [x] 扩展一等 Run Workpad record 基础版：runtime 新增 `runWorkpads` 记录和 `GET /run-workpads`，Attempt 创建、Agent invocation、完成/失败/取消、retry、Human Review approve 后进入 merging 时都会刷新 Plan、Acceptance Criteria、Validation、Notes、Blockers、PR、Review Feedback、Retry Reason。
- [x] 扩展一等 Run Workpad record：新增 `PATCH /run-workpads/{id}` 字段级 patch 基础版，Agent / supervisor 可写入 Plan、Validation、Blockers、Review Feedback、Retry Reason 等字段；runtime 刷新后会重新叠加 `fieldPatches`。
- [x] 扩展一等 Run Workpad record：字段级 patch 增加 `updatedBy` 权限边界、`fieldPatchSources` 来源归因和 `fieldPatchHistory` 变更历史；旧做法只保存最终覆盖值。
- [x] 扩展一等 Run Workpad record：Work Item 详情页新增默认折叠的 Patch history 卡片，展示字段级 patch 的写入者、字段、来源和原因。
- [x] 扩展一等 Run Workpad record：补字段级 patch 的 UI 编辑入口，Work Item 详情页可通过真实 `PATCH /run-workpads/{id}` 写入 operator 允许字段，并保留来源和原因审计。
- [x] 拆分 Work Item 详情页：旧做法由 `App.tsx` 内嵌详情大面板；新做法使用独立 item 路由和独立详情组件，减少入口文件耦合。
- [x] 增强 Review/Rework feedback sweep UI 基础版：旧做法失败原因、review 意见、PR/check 推荐动作分散；新做法在 Workpad 汇总 Review Feedback / Retry Reason，让用户先看到为什么要 rework / retry。
- [x] 扩展 Review/Rework feedback sweep 运行时基础版：review agent 结果、human request changes、失败原因、operation/event 和 checks/rebase/conflict 推荐动作会持久化成 `reworkChecklist`，并让 Rework Agent 和 Retry API 直接消费。
- [x] 扩展 Review/Rework feedback sweep：Workpad Rework Checklist 增加 source drilldown 基础版，展开后可看到 human / review / PR comment / check log / gate 等来源摘要。
- [x] 扩展 Review/Rework feedback sweep：PR comments/reviews 和 failed check log 基础采集已完成；继续接入 PR thread resolved 状态、行级上下文、check source drilldown 深链和自动 checklist 去重分组。
- [ ] 减少人工盯守成本：失败、等待、重试、PR/checks 状态都通过 Workboard/Operator 明确给出推荐操作，而不是只暴露原始日志。
- [x] 打通两个真实 runner/provider 的端到端执行映射：Project Agent Profile 中的 runner/model/policy 会实际影响 Codex / opencode / Claude Code / Trae Agent 执行；Trae Agent 账号凭据可加密落库并在运行时注入 provider 环境变量。
- [ ] 固定功能一标准演示脚本：从输入 Requirement 到 PR/checks/human gate/proof/report 的流程可重复跑，并纳入测试报告。

## 已完成

- [x] 明确两层架构（Product Layer + Execution Layer）
- [x] 增加 SQLite 持久化的本地服务端
- [x] 把 Workboard 主持久化从浏览器状态迁到本地服务端主路径
- [x] 增加 work item 创建 / 修改 API
- [x] 增加从 work item 生成 mission 的 API
- [x] 增加 operation 执行并持久化 Mission 事件的 API
- [x] 增加 GitHub 仓库信息读取与 issue 导入的服务端基础接口
- [x] 暴露 OpenAPI 文档
- [x] 增加 pipeline 生命周期基础 API（start / pause / resume / terminate）
- [x] 增加 checkpoint 基础 API（approve / request-changes）
- [x] 新增 Go Local Service v1，作为默认 `mission-control:api` 实现
- [x] Go Local Service 支持 workspace / work item / pipeline / checkpoint / mission / operation 基础闭环
- [x] 增加 `GET /missions`、`GET /operations`、`GET /proof-records`
- [x] 增加 `GET /attempts`，把一次 Run 的状态、workspace、branch、PR、stage 证据串成主记录
- [x] 增加 Go 侧 migration metadata 表和 `GET /migrations`
- [x] 增加内置 Pipeline 模板 API（feature / bugfix / refactor）
- [x] 增加 LLM Provider registry（OpenAI + OpenAI-compatible）
- [x] 增加运行时 LLM Provider 选择 API
- [x] 增加 Agent definitions API，包含 System Prompt、输入契约、输出契约
- [x] 增加可观测性 summary API（pipeline / checkpoint / operation / proof / attention）
- [x] 把 observability summary 接到 Operator 面板
- [x] Operator 面板展示最近 runtime logs 和最近失败，便于本地联调排障
- [x] 增加 Omega operator CLI 基础版：`omega` 命令通过 Go Local Runtime API 查看 status/logs/work items/attempts/checkpoints，并显式触发 run/retry/cancel/approve/supervisor tick。
- [x] 把 LLM Provider selection 接到前端运行时设置 UI
- [x] 把 Pipeline / Checkpoint API 接到前端 Operator UI
- [x] Operator 面板支持从模板创建并启动 Pipeline
- [x] Operator 面板支持 Human checkpoint 的 Approve / Request changes 操作入口
- [x] Checkpoint Reject 会把对应 Pipeline stage 回退成 ready，并携带拒绝理由供重做
- [x] 明确最终形态：local-first，本地 App 可直接创建/安排/执行任务，远端共享控制面负责可选同步与协作可见性
- [x] 明确 `lark-cli` 作为本地 App 的 Feishu Tool Adapter
- [x] 固定 sample workspace id，保证刷新后能恢复同一工作空间
- [x] 修复 `localRunnerBridge` 的旧测试失败
- [x] 增加本地 CLI capability detection：codex / opencode / git / gh / lark-cli
- [x] 在 Go Local Runtime 中增加 `lark-cli` adapter，支持发送基础文本通知
- [x] 增加 Go Pipeline `run-current-stage` 本地闭环入口：执行 stage、持久化 mission / operation / proof，并推进到 checkpoint 或下一阶段
- [x] 增加 `demo-code` 本地代码写入 runner：隔离 clone 目标 repo，创建分支，写入真实 TypeScript 文件，提交 commit，并生成 diff / summary proof
- [x] 升级 `codex` runner：有本地 repo target 时在隔离 clone 中执行 Codex，检测真实 git 变更，提交 commit，并生成 diff / summary proof
- [x] 创建 Work item 时支持填写本地仓库路径，并将该 target 传递到 Mission，保证本地代码写入 runner 能直接消费
- [x] Operator 面板支持直接运行当前 Pipeline stage
- [x] Operator 面板支持本地 runner 选择：检测到 Codex 时可切换到真实 Codex runner，否则保留 `local-proof` 稳定兜底
- [x] 修复前端启动时旧 workspace snapshot 覆盖 Go SQLite 中 Pipeline / Checkpoint / Proof 表的问题
- [x] 增加 Vite `/api` 本地代理与 dev fallback，降低本地 APP 联调环境变量成本
- [x] 明确 Omega Workboard 的核心对象关系：Project 不是 repo，Requirement 承接需求源，Item 是内部可执行单元，Pipeline run 才是执行实例
- [x] 前端 Workboard 模型补齐 Project repository targets 与 Work item source / external ref / acceptance criteria / dependencies
- [x] Go Local Service 创建/导入 Work item 时补齐默认来源、验收标准和依赖字段
- [x] GitHub issue import 映射为 `source = github_issue`，并记录 external ref 与 repository target
- [x] 增加一等 Requirement 表与 API：手动需求、GitHub issue import、orchestrator claim 都会创建/链接 Requirement，并通过 Item 的 `requirementId` 进入执行链路
- [x] 前端 Work items 列表、详情页和 inspector 展示 Requirement -> Item -> Repository target 的从属关系
- [x] Go Local Service 支持 GitHub OAuth start/callback/token storage，并把 GitHub connection 写回 Workboard workspace
- [x] 明确产品核心内核：需求拆分、Pipeline 编排、Agent 协作安排，以及 GitHub-backed 工程管理闭环
- [x] 明确第一阶段交付协议：内部模型使用 Requirement / Item / Pipeline，真实工程闭环必须绑定 GitHub repository target，并围绕 issue / branch / PR / checks / review / merge 生成 proof
- [x] 增加需求拆分 API：`POST /requirements/decompose`，输出 structured requirement / acceptance criteria / risks / suggested stage work items
- [x] 增加 GitHub PR 创建 API：`POST /github/create-pr`，基于 runner workspace proof summary、branch、changed files 调用 `gh pr create`
- [x] 增加 GitHub PR/checks 读取 API：`POST /github/pr-status`，通过 `gh pr view` / `gh pr checks` 返回 review state、checks、deliveryGate、proofRecords
- [x] Project 页面支持从本机 `gh` 仓库列表选择 repo，并绑定为当前 Project 的 repository target
- [x] Repository workspace 下支持从 App 内部直接新建需求，自动继承当前 repo target，并进入本地 runner 执行链路
- [x] GitHub issue URL / repo URL 可作为 runner 的代码目标解析来源，支持真实 `gh repo clone` 后在隔离 workspace 生成 branch / commit / proof
- [x] 增加 App 可配置的本地 workspace root：默认 `~/Omega/workspaces`，并通过 Go API 持久化
- [x] 参照外部项目模板完成 Omega 映射：project slug -> Omega Project/Repository workspace，repo/default branch/workspace root -> App 配置与 repository target
- [x] 增加 `devflow-pr` Pipeline 模板与执行入口：clone repo、创建分支、提交代码变更、创建 PR、记录 Review Agent verdict / Human Review checkpoint / approve 后 merge proof
- [x] `devflow-pr` 执行链路产出阶段 artifact：requirement artifact、solution plan、implementation summary、test report、两轮 review、human/merge proof、handoff bundle
- [x] Operator 面板支持触发 `Run DevFlow cycle`
- [x] 增加 operation workspace root 安全校验，确保每个 workspace 都在配置的 workspace root 下
- [x] 每个 operation / DevFlow cycle 写入 `.omega/agent-runtime.json`，记录 runner、agent、repo target、workspace root 和 sandbox policy
- [x] 增加本地 orchestrator tick：从绑定 GitHub repo 拉取 open issues，跳过已导入 issue，自动创建 repository-scoped Work item 和 `devflow-pr` Pipeline
- [x] `orchestrator/tick` 支持显式 `autoRun`：claim 后立即跑 DevFlow cycle、生成 proof/PR、落库 item/pipeline 状态；默认不开启，避免后台轮询隐式写仓库
- [x] 增加 GitHub issue eligibility gate：只有带 `omega-ready` / `devflow-ready` / `agent-ready` / `omega-run` 标签的 open issue 才能被 orchestrator claim
- [x] 增加本地 execution lock：同一 GitHub issue 被 claim 后会持久化 lock，重复 tick 返回 `locked` 而不是静默跳过
- [x] 增加 execution lock API：`GET /execution-locks` 与 `POST /execution-locks/{id}/release`，支持 App 展示和手动释放
- [x] 增加本地 repository watcher：App 内可对单个 repository workspace 开启/暂停自动处理，Go runtime 会定时扫描带 ready 标签的 GitHub issue，并按 execution lock 防重
- [x] 增加 Codex runner process supervisor：独立子进程执行，结构化捕获 pid、exit code、duration、stdout、stderr，并回写 operation
- [x] Operator 视图展示 Pipeline stage timeline、execution locks、runner process telemetry，避免执行过程黑箱化
- [x] 自动运行完成后释放 execution lock，并标记 runner process state，避免已完成任务继续占用 claim
- [x] Work items 列表补齐执行前/执行中/完成后的状态语义：创建后准备编排进入 `Planning`，`Ready` 在 UI 显示为 Not started，完成项禁用 Run，行内展示 Pipeline stages 与 Agent 分配
- [x] 在真实 `ZYOOO/TestRepo` 上完成一次 GitHub issue -> Omega claim -> workspace -> branch -> PR -> merge -> proof 的闭环验证：issue #6，PR #7
- [x] 补齐 v0Beta 的主 Agent 结构：Requirement 创建时由 `master` 生成 structured requirement / dispatch plan / suggested work items
- [x] Pipeline run 物化 Agent contracts：每个 Agent 包含 System Prompt、输入契约、输出契约、默认工具和模型配置
- [x] Pipeline stage 补齐显式 `dependsOn`、`inputArtifacts`、`outputArtifacts` 与 `dataFlow`
- [x] v0Beta 数据审计修复：旧 Requirement / Pipeline 记录加载时自动补齐 master dispatch、Agent contracts、stage dependencies、artifact handoff、dataFlow
- [x] DevFlow PR 模板支持一阶段多 Agent：implementation 阶段明确绑定 architect / coding / testing，human review 阶段绑定 review / delivery
- [x] 在真实 `ZYOOO/TestRepo` 上完成 App 内 Requirement -> Item -> DevFlow PR -> merge 的闭环验证：`OMG-52`，PR #8
- [x] 清理本地 v0Beta 测试数据：SQLite 中 work item / requirement / pipeline / proof / checkpoint 归零，保留 GitHub 登录与 `ZYOOO/TestRepo` repository target
- [x] 建立 `docs/README.md` 作为当前文档入口，区分 canonical v0Beta 文档与历史工作笔记
- [x] 移除 `devflow-pr` 的表面自动通过逻辑：Review Agent 必须输出明确 verdict，Human Review checkpoint 会真实阻塞，Approve 后才执行 merge / delivery
- [x] 把默认 `devflow-pr` 从 Go hardcode 抽成 Markdown workflow：`services/local-runtime/workflows/devflow-pr.md`，并让 Go runtime 从模板读取 stages、agents、artifact、review rounds
- [x] 按赛题功能二的第一项补齐 TS + React SPA 双页面结构：门户首页 + Workboard 功能页，默认首页，`#workboard` 进入真实功能区
- [x] 把门户首页从 `App.tsx` 拆到 `apps/web/src/components/PortalHome.tsx`，降低单文件复杂度
- [x] 拆出 Requirement 创建链路：`RequirementComposer`、`manualWorkItem`、Go `work_items.go`，避免创建 UI / Work Item lifecycle 继续堆在巨型入口文件里
- [x] 拆出 Projects / Repository Workspace 总览：`ProjectSurface`，让 `App.tsx` 少承担一个完整页面
- [x] 拆出 Workboard shell：`WorkspaceChrome` 承担左侧导航、workspace 切换、顶部搜索、详情工具栏，`App.tsx` 继续保留状态编排
- [x] 拆出 DevFlow PR 长流程执行器：`devflow_cycle.go`，为后续 JobSupervisor / heartbeat / retry 留出清晰边界
- [x] 拆出 Pipeline record / template materialization：`pipeline_records.go`，减少 `server.go` 的状态构造负担
- [x] 增加未开始 Work Item 删除能力：只允许无执行历史的 not-started item，删除时同步清理未共享 Requirement 和 mission state 投影
- [x] 将 Workboard 视觉更新为与门户一致的浅色工作台风格，同时保留原有本地执行、GitHub workspace、Agent trace、Human gate、Proof 功能
- [x] 新增 Workspace Config 入口：Project 页面 `Project config`、Workspace 齿轮和 Workspace subnav 都可进入配置页
- [x] Workspace Config 支持本地 workspace root 配置 UI：路径输入、默认路径、目录选择器兜底和保存入口
- [x] Project Agent Profile 配置入口基础版：可编辑 workflow、runner、Skill allowlist、MCP allowlist、stage policy、`.codex` policy，并生成 runtime spec preview
- [x] Project Agent Profile UI 拆成 Workflow / Agents / Runtime files 三层：支持 workflow markdown 草稿、按 Agent 配置 runner/model/Skills/MCP，以及 `.omega` / `.codex` / `.claude` 运行时文件预览
  - 旧做法保留说明：该版本把 workflow markdown、Agent runner、prompt/policy 和 runtime preview 放在同一块编辑器内，能用但信息密度和入口层次不够清晰。
- [x] Workspace Agent Studio 基础版：在 Workspace Config 内提供工作区级共享的 Workflow 图形编排、Prompt 设置、Agent runner/model/Skills/MCP 选择和 Runtime files 预览；继续复用 Agent Profile / workflow markdown / runtime policy 保存链路。
- [x] Workspace Agent Studio runner / 凭据边界修正：runner 编排保留 Codex 优先、opencode、Claude Code、Trae Agent；页面内账号 / Key 配置只面向 opencode / Trae Agent，Codex / Claude Code 走本机 CLI 登录态。
- [x] Project Agent Profile 接入 Go API / SQLite：支持 `/agent-profile` 读写、Project / Repository scope 解析、Pipeline run metadata 绑定和 runner runtime bundle 写入
- [x] Agent Profile runner 配置接入本机 capability 预检：前端阻止保存不可用 runner，Go runtime 在创建 attempt / operation workspace 前拒绝未安装 runner
- [x] 建立 Agent / Workflow 配置方案文档：`docs/agent-workflow-configuration-plan.md`
- [x] 建立功能实现记录文档：`docs/feature-implementation-log.md`，后续功能按同一格式记录目标、做法、验证和后续工作
- [x] Operator Runner processes UI 收敛为摘要列表，stdout/stderr 默认折叠，避免执行记录页面被日志块淹没
- [x] 使用正式 Omega logo 资产：`apps/web/public/omega-logo.png`
- [x] 建立功能二 Page Pilot 架构文档：`docs/page-pilot-architecture.md`
- [x] 在 React SPA 中注入 Page Pilot Overlay：悬浮对话框、元素圈选模式、selector / DOM / 文本 / 样式快照采集
- [x] 增加 Page Pilot preview surface：在 Omega 内打开目标项目页面，并把 Overlay 绑定到目标 preview document，而不是 Omega 自身管理 UI
- [x] 增加 Page Pilot dev proxy：`/page-pilot-target` 同源代理本地目标项目，方便浏览器模式验证圈选和真实 patch
- [x] Page Pilot 进入 immersive preview mode：隐藏 Workboard chrome，让用户产品在 Omega 主工作区中直接打开
- [x] 增加 Electron dev shell 基础版：开发模式加载 React SPA，并预留目标项目 BrowserView / preload selection bridge
- [x] 增加 Electron direct pilot mode：直接打开目标产品 URL，并在页面内注入悬浮手指、hover 高亮和修改输入框
- [x] Electron direct pilot 支持多元素批注队列：单元素 comment 只加 pin，底部悬浮输入框统一收集整体需求后再提交 Agent
- [x] 为 Portal Home 关键可编辑元素加入最小 `data-omega-source` 源码映射
- [x] 增加 Page Pilot Go runtime API：`/page-pilot/apply` 接收 selection context + 用户指令，在明确 repository target 中执行真实源码 patch
- [x] Page Pilot apply 成功后物化为功能一记录：`source=page_pilot` Requirement / Work Item / Page Pilot pipeline run
- [x] Electron direct pilot apply 成功后自动 reload 目标页面，并恢复 Page Pilot run 结果面板
- [x] Electron direct pilot 结果面板基础版：展示 changed files、diff summary、Requirement/Work Item/Pipeline linkage
- [x] Electron direct pilot Confirm / Discard 基础版：Confirm 调用 deliver，Discard 调用 runtime discard 并刷新预览
- [x] Electron direct pilot 提交态基础版：提交后切换为 Agent 过程面板，展示批注历史、primary target、修改文件和 Work Item/Pipeline linkage
- [x] Electron direct pilot 多批注主目标修正：默认使用最新一条带源码映射的批注作为 `/page-pilot/apply` selection
- [x] 增加 Page Pilot 确认交付 API：`/page-pilot/deliver` 创建 branch / commit，并在 GitHub target 上继续创建 PR
- [x] 增加 Page Pilot run 记录与撤销 API：`/page-pilot/runs`、`/page-pilot/runs/{id}/discard`
- [x] 将 Page Pilot run 从 `omega_settings` 升级为 SQLite 一等表 `page_pilot_runs`

## 进行中

- [x] DevFlow 编排重构：把 Requirement / Architect / Coding / Review / Rework / Delivery 都作为明确 Agent role 记录到每次运行
- [x] DevFlow 编排重构：由 workflow markdown 定义状态、review outcome、rework 回流和 human gate，Go runtime 读取该契约执行
- [x] DevFlow 编排重构：Review `changes_requested` 不再把 Attempt 标为失败，而是进入 Rework，同一 workspace / branch / PR 上继续修改后回到 Code Review
- [x] DevFlow 编排重构：一个 Item 的执行复用稳定 workspace、稳定 branch 和同一个 PR；Attempt 只记录一次执行轮次和事件
- [x] DevFlow 编排重构：抽象 AgentRunner；默认和 profile / auto fallback 保持 Codex 优先，并支持 opencode / Claude Code / Trae Agent / local runner。
- [x] DevFlow 编排重构：前端详情页展示真实 Agent turn、输入/输出 artifact、状态流转和 runner telemetry，弱化单纯 proof 数字
- [x] DevFlow 编排重构：JobSupervisor 已接入 heartbeat、stall retry、cancel、contract-driven timeout/retry 和 workspace lock 基础版
- [x] DevFlow 编排重构：补 worker host 分配、本地 worker lease、continuation policy metadata 和 orphan running Attempt crash recovery 基础版
- [ ] DevFlow 编排重构：继续补远端 runner 崩溃恢复和 GitHub polling supervisor
- [ ] 前端模块化：继续拆分 `App.tsx`，优先拆 Workboard list/detail、Inspector、Operator 面板和 GitHub workspace 组件
- [ ] Go runtime 模块化：继续拆 `server.go`，优先拆 HTTP handler 注册、delivery/PR API、workspace cleanup API 和 runtime settings API
- [x] 把 Requirement Decomposition 建成一等服务端能力：raw requirement / GitHub issue -> structured requirement / acceptance criteria / risks / sub-items / target repo context
- [x] 把 Agent handoff bundle 建成基础 artifact：每个 stage 有输入/输出 artifact contract，`devflow-pr` 生成 handoff bundle
- [x] 把 Agent handoff bundle 从文件 proof 继续升级为可查询的一等表记录
  - 2026-05-02：新增 SQLite `handoff_bundles` 表和 `GET /handoff-bundles`，从真实 proof / Attempt / Operation 中抽取 bundle、summary、PR 和 changed files；旧文件 proof 仍保留为兼容来源。
- [x] 增加 Agent runtime spec 基础版：每个 operation / DevFlow cycle 写入 `.omega/agent-runtime.json`
- [x] 增加 runner-specific runtime 模板基础版：Codex / Claude Code 继续生成本机 CLI policy 文件；opencode / Trae Agent 已补账号配置入口、runner/model/prompt/Skills/MCP 配置和 Project Agent Profile 消费链路，后续继续增强更细的 tool policy 文件。
- [x] 扩展 runner process supervisor：opencode / Trae Agent / demo-code 已接入 runner registry、capability preflight、exit status、stdout/stderr 持久化；timeout / cancel 继续沿用 context-aware supervisor。
- [x] `run-devflow-cycle` 产品主路径异步化：点击 Run 先创建 Attempt 并立即返回，后端 background job 继续执行，前端通过轮询更新状态
- [x] `orchestrator/tick autoRun` 异步化：认领后创建 Attempt，后台 job 执行并在完成/失败后释放 execution lock
- [x] Human Review checkpoint 产品主路径真实化：默认不 auto approve / auto merge，Review Agent 通过后停在 `waiting-human`
- [x] 把 Workflow Template 从文件默认值继续升级为 SQLite 一等记录，并支持 Project / Repository Workspace 覆盖
- [x] 增加 Workflow Template 编辑 API 基础版：读取、保存、校验、恢复默认，并由 Agent Profile 运行时消费覆盖
- [ ] 后续：Workflow Template Markdown 文件导入/导出、版本对比和恢复历史
- [x] 把 coding / rework / review prompt sections 从 Workflow Markdown body 渲染到 Agent prompt，进一步减少 Go 里的 prompt hardcode
- [x] 增加正式 JobSupervisor v1：扫描 ready work items / failed attempts / stalled attempts / human gates / workflow contract，并按显式策略调度下一步
- [x] JobSupervisor 增加 heartbeat、stall detection、retry、cancel、timeout 基础版
- [x] JobSupervisor 增加 worker host 分配、多 turn continuation policy metadata 和本地 orphan crash recovery 基础版
- [x] JobSupervisor 增加远端 runner heartbeat 和 GitHub / CI polling 基础版
- [x] 增加 issue/work item preflight checks：repo target、workspace root、branch 权限、dirty state、重复执行锁、必要 CLI 能力检查
  - 2026-05-02 核对：DevFlow preflight 已被 manual run / retry / JobSupervisor scan 复用；GitHub delivery contract preflight 已覆盖 PR 权限和 checks 读取能力。
- [x] 增加 GitHub delivery contract preflight：issue source 可选，但 repository target、branch 权限、PR 创建权限、checks 读取权限必须在运行前验证
- [x] DevFlow preflight 基础版：运行前集中检查 repository target、workspace root、git/gh、runner availability、local dirty state，并被 manual run / retry / JobSupervisor scan 复用。
- [x] 增加 orchestrator execution lock：同一个 GitHub issue / Work item / repository target 不能被多个本地 App 重复认领或重复执行
- [x] 增加 Agent runner registry：按 stage/agent/runner 选择 Codex、opencode、Claude Code 或 local runner，并注入不同 issue、workspace、prompt、artifact 上下文
- [x] 增加 Agent runner availability preflight：按 Agent Profile 配置检查 Codex / opencode / Claude Code / demo-code 所需 CLI，缺失时阻止启动而不是生成假失败流程
- [x] 清理遗留外部参考命名：内部文件、类型、测试、历史文档统一使用 Omega Workboard / DevFlow 语言，外部产品名只出现在“参考来源”说明中
  - 2026-05-02 核对：当前 `services` / `apps` / `docs` 中未发现需避开的历史参考命名；后续新增文档继续按 Omega Workboard / DevFlow 语言维护。
- [x] 把 Product Layer 持久化从 workspace 快照推进到 missions / operations / checkpoints / proof_records 等一等表基础版
- [x] 按新的 Work Model 继续把 Project / Repository target / Work item / Pipeline run 的 SQLite 结构从 JSON snapshot 推进为可查询表基础版
  - 2026-05-02：新增 `repository_targets` 可查询表和 `GET /repository-targets`。旧 Project JSON 中的 `repositoryTargets` 仍作为兼容镜像保留；Project / Pipeline run 全量拆表和旧镜像移除继续单独跟踪。
- [x] 先把 Requirement 从 snapshot 中抽成服务端一等表，并在本地 SQLite / 前端本地 fallback SQLite 中持久化
- [x] 把 Go Local Service 的 SQLite 写入从 snapshot-first 继续推进为 repository-first 基础版
  - 2026-05-02：保存 workspace 时同步物化 Repository target / handoff bundle / operation queue 审计表，读路径可按 repository target 查询；旧 snapshot-first 保存仍保留为兼容层，后续逐步迁移 Project / Pipeline run 写入。
- [ ] 把 Go 侧 migration metadata 演进成可执行增量迁移
- [x] 把旧 Node / 前端本地服务路径降级为兼容回退；Go Local Runtime 已是默认主路径
- [ ] 清理旧 Node / 前端本地服务端兼容代码，减少双实现维护成本
  - 2026-05-02 注释：本轮只新增 repository-first 审计表，不移除旧兼容代码，避免影响当前功能一/功能二测试。
- [ ] 继续减少前端直接持有的业务逻辑，让服务端成为 Mission Control 的唯一写入者
- [x] 把 GitHub repo / issue import API 接到前端 UI
- [x] 把 App 内部需求入口接到 repository workspace：不依赖 GitHub issue 也能创建 Work item 并运行
- [x] Work item 详情页展示 Attempt stages / Agent turns / artifacts / Human Review 基础版，可看到真实状态流转
- [x] 把 Checkpoint Reject 的“回退重做”做成更清晰的 stage timeline 可视化，突出 rejected -> rework -> review 回流；后续继续增强动画和筛选，但主链路已能展示 rejected -> rework -> review 的原因与回流。
- [x] Operator 视图增加基础 stage timeline，可看到当前 Pipeline 每个阶段的状态
- [x] 按赛题要求持续维护 Must-have / Good-to-have 对照与完成度
  - 2026-05-02 核对：`docs/competition-requirements-matrix.md` 与本清单的 Must-have / Good-to-have 已同步到当前完成度；后续作为常规维护项继续更新。
- [x] Page Pilot：将 proof 从 run JSON / proof 文件升级为通用 Mission / Operation / Proof records，并在 Work Item 详情的 Agent trace / Proof 面板展示
- [x] Page Pilot：Electron Desktop Shell 增加本地服务自动启动基础版，可启动 Go local runtime、Omega Web Vite dev server，并在显式 `OMEGA_PREVIEW_REPO_PATH` 下启动目标 repo preview dev server
- [ ] Page Pilot：扩展 `data-omega-source` 到 Workboard 关键可编辑区域，不只覆盖 Portal Home
- [ ] Page Pilot：增加 patch preview、selection history、discard/revert 的更完整 UI 状态（基础 selection history / process panel 已落地，仍缺完整 diff preview）
- [x] Page Pilot：把 Electron direct pilot 的 target URL、projectId、repositoryTargetId 从 env/default 升级为用户可选配置；Page Pilot 启动器负责选择 repo / 预览来源，目标页内继续使用已验证的 direct pilot preload。
- [x] ~~Page Pilot：把 Electron preview webview、repository target、local worktree 绑定成一等配置~~（旧大项已拆分并完成主路径：Page Pilot 启动器选择 Repository Workspace，Electron direct pilot 接收明确 target，Go Preview Runtime 保存 profile）
- [x] Page Pilot：定义单一 Page Pilot Agent 的 stage contracts：preview-runtime / page-editing / delivery
- [x] ~~Page Pilot：增加 Preview Runtime stage，读取 repo 文件并生成 install/start/preview URL/health/reload 的可审计运行档案~~（旧大项已拆分并完成基础版：Preview Runtime Agent/Profile 生成启动档案并驱动打开/刷新）
- [x] ~~Page Pilot：增加 Preview Runtime Profile API：resolve/start/restart，所有命令锁定到明确 repository workspace~~（旧大项已拆分并完成基础版：Go runtime 已暴露 `/page-pilot/preview-runtime/resolve|start|restart`）
- [x] Page Pilot：把当前 Electron preview process supervisor 下沉为 Go runtime 一等能力，记录目标项目 dev server 的 pid、stdout/stderr、端口、健康检查和失败诊断
  - 2026-05-02 核对：Go Preview Runtime supervisor 基础版已完成；跨进程恢复、持久 process table、日志分页和更完整失败诊断继续作为后续增强。
- [x] Page Pilot：在 UI 中展示 session 对应的 Work Item / Page Pilot pipeline run 基础版；Page Pilot 页面展示 Recent runs 并支持回跳 Work Item，Work Item 可带 repository target 打开 Page Pilot
- [x] Page Pilot：增加 live-preview repository write lock，`apply` 持锁等待 Confirm/Discard，`deliver` / `discard` 终态释放，避免同一预览工作区被并发写入
- [x] Page Pilot：实现 isolated-devflow mode 基础版。GitHub Repository target 会解析或自动准备 Omega-managed isolated preview workspace，apply 写隔离 workspace，Confirm 后从隔离 workspace 创建 branch / commit / PR，Discard 对隔离 workspace 做安全 reset/clean。
- [x] Page Pilot：Electron 基础版 Preview Runtime stage 落地后，apply / Reload 会按 profile 做 health check、HMR 等待或必要的目标项目 dev server restart
- [x] Page Pilot：Electron direct pilot 结果面板升级为完整 diff preview / PR body preview / Work Item 回跳基础版；Recent runs 详情弹窗展示真实 run artifact，后续继续补行级 diff drilldown。
- [x] Page Pilot：支持同一 Page Pilot run 的多轮追加批注 / 追加说明 / 重新 apply
- [x] Page Pilot：把 Electron direct pilot 的批注历史、primary target、process events 从 localStorage 升级为服务端 run conversation 记录基础版；`apply` 持久化 conversation batch，`deliver` / `discard` 同步终态。
- [x] Page Pilot：增加 source mapping 覆盖率报告基础版，Page Pilot run 会记录强源码映射、DOM-only 选区、缺失文件映射和覆盖率；按页面/组件聚合继续后续扩展。
- [x] Page Pilot：增加修改前/修改后截图或 DOM snapshot 证据，并把视觉 proof 关联到 Work Item、PR body 和 run report；当前基础版为 DOM snapshot，截图证据继续保留为后续增强。
- [x] Page Pilot：为 DOM-only 批注增加源码候选定位能力基础版，缺少 `data-omega-source` 时按文本、selector、DOM tag 和批注 token 搜索候选源码，并写入 Agent prompt / run artifacts。
- [x] ~~Page Pilot：把 `/page-pilot-target` 调试代理替换为 Electron bridge 或显式开发配置~~（产品主路径已切到 Electron bridge；该代理仅作为 Vite 开发 fallback 保留，不再作为独立产品任务）
- [x] Page Pilot：实现 Electron direct pilot preload bridge，支持跨 origin / file URL 目标页内真实元素圈选、多批注、Apply、Confirm、Discard 和返回。
- [x] Page Pilot：把 React Page Pilot 页面接入 Electron `omegaDesktop.openPreview`，但页面只作为 direct pilot 启动器，不再承载目标页内圈选交互。
- [x] Page Pilot：增加 Go target dev server start/restart API，支持修改后重启用户项目 dev server；基础版为 `/page-pilot/preview-runtime/resolve|start|restart`。
- [x] Page Pilot：PR body 增加 DOM context、样式快照和视觉证据基础版；截图文件继续保留为后续增强。

## 已完成且不再作为独立任务

- [x] `devflow-pr` 从内置模板推进为默认 Markdown 模板：已并入 Workflow markdown 主路径，后续只跟踪模板持久化 / 编辑 API。
- [x] GitHub PR / checks 读取 API：已并入 GitHub PR lifecycle 能力，后续只跟踪 PR lifecycle UI 和 issue 状态回写。
- [x] GitHub issue import：Go API 和前端入口均已完成，后续只跟踪 issue 状态回写 / label / comment 同步。
- [x] Repository target 建模与绑定入口：已并入 Project / Repository Workspace 主模型，后续只跟踪项目创建和 repository target 配置增强。
- [x] Work item source / external reference / acceptance criteria / repository target 字段：已并入 Workboard 基础模型，后续只跟踪更多一等表和查询能力。
- [x] App 内部新建需求继承 Repository Workspace：已并入 Work item 创建主路径。
- [x] Execution lock API 设计：已实现并接入 UI，后续只跟踪 shared sync 下的跨设备协调。

## 下一步

- [ ] 增加 App 内可编辑 Workflow Template：在 UI/API 中编辑阶段、Agent、Gate、review rounds、默认 repo/workspace 配置，替代散落的 md/env 文件
  - 2026-05-02 更新：Workflow Template 已有 SQLite 一等记录和读取/保存/校验/恢复默认 API；Workspace Agent Studio 已完成图形化查看和基础编辑入口。阶段增删、连线编辑、导入/导出和版本恢复仍未完成。
- [x] Project Agent Profile 从 `omega_settings` 继续升级为一等 SQLite 表
- [ ] Project Agent Profile 继续补充版本历史展示、恢复默认、导入/导出，以及更完整的 workflow/schema 校验
- [ ] 持久化 connector sync report，而不只是 sync intent
  - 2026-05-01 更新：GitHub Issue 出站同步已先落地 sync report；其他 connector 仍待补齐。
- [ ] 增加新项目的 workspace bootstrap API
- [x] 把 LLM Provider / runner account selection 真正映射到 opencode / Trae Agent / compatible provider runner；Codex / Claude Code 继续走本机 CLI 登录态。
- [x] 把 `run-current-stage` 的 runner 扩展为 Codex / opencode / Claude Code / Trae Agent，并保留 `local-proof` / `demo-code` 兜底。
- [ ] 设计 shared sync API：本地/远端/GitHub/飞书创建 requirement 后双向同步状态
  - 2026-05-02 计划：先保留为设计项，不在本轮落地。需要定义本地事件版本、远端权限、repository lock 所有权和离线冲突解决。
- [ ] 在 Go Local Runtime 中扩展 `lark-cli` adapter，支持发送 checkpoint 卡片通知和 pipeline 结果通知
  - 2026-05-01：Human Review 已新增飞书 review-request / callback 链路；机器人 webhook 和 `lark-cli` chatId 都支持 interactive card。Pipeline 结果通知仍可继续扩展为 delivery summary 卡片。

## Workboard / 飞书 / GitHub 缺口

- [x] 把当前轻量 issue list 扩展成更完整的 Omega Workboard 基础版：Project、Views、Repository Workspace、Work item 分组
- [x] 增加项目级和视图级 UI 基础版，不再只是单条 work item CRUD
- [ ] 继续完善 Project 页面：增加 health、active work items、linked PR / checks / pipeline 状态摘要
- [x] 增加真实 GitHub 仓库信息读取 API
- [x] 增加 GitHub issue 导入到 Omega Workboard 的 Go API
- [x] 增加 GitHub repo / issue import 的前端入口
- [x] 完成 GitHub PR 创建：branch / commit / diff proof -> PR title/body
- [x] 完成 Omega DevFlow PR 周期基础版：branch / commit / PR / Review Agent verdict / Human Review checkpoint / approve 后 merge proof
- [x] 完成 GitHub issue 状态回写：Pipeline 状态 -> issue comment / label / status
  - 2026-05-01：DevFlow 会在 attempt started、human review waiting、merge failed、attempt failed、delivery completed 时通过 `gh issue comment` 和 `gh issue edit` 回写 GitHub Issue，并写入 sync report。
- [x] 完成 GitHub PR 生命周期 UI 基础版：Work Item detail 在真实 PR URL 存在时读取 `/github/pr-status`，展示 branch、checks、review、merge gate；旧做法只展示 PR URL / proof。
- [x] 完成 GitHub checks/CI 读取：check run / workflow run -> proof record / delivery gate
- [x] 完成 GitHub / CI 的真实出站同步
  - 2026-05-01：GitHub Issue comment 会带上 PR URL、changed files、checks 输出、failed check log / PR feedback 摘要、失败原因和 review packet 风险/推荐动作。PR comment 会同步结构化 review packet；CI 可按配置触发失败 run rerun 或 workflow dispatch。实现说明见 `docs/github-outbound-sync.md`。
- [x] 增加真实 Feishu/lark-cli 文本通知发送
- [x] 增加 App 内 Feishu/lark-cli checkpoint/pipeline 文本通知入口
- [x] 增加真实 Feishu/lark-cli checkpoint 审核卡片发送
  - 2026-05-01：`POST /feishu/review-request` 会从 checkpoint 读取 Requirement / Work Item / Attempt / Review Packet，生成飞书 interactive card；无 webhook 时可走 `lark-cli im +messages-send --msg-type interactive`。
- [x] 增加 Feishu webhook / 回调到 checkpoint 状态迁移
  - 2026-05-01：`POST /feishu/review-callback` 与本地 Approve / Request changes 使用同一 checkpoint decision helper；真实公网 callback 需要配置 `OMEGA_PUBLIC_API_URL` 和可选 `OMEGA_FEISHU_REVIEW_TOKEN`。
- [x] 增加无公网 Feishu Task 审核桥接
  - 2026-05-01：`POST /feishu/review-request` 支持 `mode=task`，通过 `lark-cli task +create` 创建审核任务；任务完成后 `/feishu/review-task/sync` 同步为 approve，任务评论通过 `/feishu/review-task/comment` 同步为 request changes 或 need-info。
- [ ] 增加 Feishu Task 本地常驻事件桥
  - 当前已完成本机 API 和同步语义；后续把任务评论 / 完成事件自动轮询或订阅并转发到本机 runtime，减少手动 sync。
- [ ] 把 Feishu 群聊 requirement 入口建模成 Product Layer 的一等事件源
- [x] 持久化 Feishu message/card/doc external reference，方便从 Workboard 回跳
  - 2026-05-01：发送结果写入 checkpoint.`feishuReview`，包含 provider、tool、format、message id、task id、task url、nonce 或 card/doc preview；后续可继续提升为一等 Connector Sync Report 表。

## 执行层缺口

- [x] 增加 requirement decomposition stage artifact 基础版，并通过 handoff bundle / stage input contract 串联 solution/coding/testing/review
- [x] 增加 Requirement -> Item 的持久化归属关系，避免 Agent 只拿到孤立 issue/item 后误判目标仓库或需求上下文
- [x] 增加 Agent 协作协议基础版：stage input contract、output contract、handoff bundle、reject reason、retry instruction
- [x] 增加 repository target 安全约束：执行前必须解析 Work item 的 `repositoryTargetId`，避免 Agent 在错误仓库运行
- [x] 增加 DevFlow 长运行的后台 Attempt job 基础版，避免 HTTP 请求阻塞整条执行流程
- [x] 增加长运行 operation 的正式 queue / retry 模型基础版
  - 2026-05-02：新增 SQLite `operation_queue` 和 `GET /operation-queue`，从真实 Operation 物化 queued/running/done/failed/canceled、priority、lock、attemptCount 和 queue payload；真正的 worker dequeue / retry mutation 继续后续实现。
- [ ] 增加本地 App sync loop：push local changes / pull remote changes / acquire execution lock / report events
  - 2026-05-02 计划：该项属于 shared sync / 多端协作授权，不阻塞当前单机测试；需要先确定远端控制面、授权模型和冲突策略。
- [x] 增加显式的 run attempts 表
- [x] AutoRun 会形成一条正式 Attempt，并把 workspace、branch、PR、proof、错误和耗时写入持久化记录
- [x] Done Item 在列表禁用 Run；详情页提供显式 Rerun；失败状态显示 Retry 入口
- [x] 增加 attempts 的 retry / cancel / timeout 策略，并接入 JobSupervisor（旧做法：列表 Retry 重新触发 item；新做法：按 concrete Attempt 建立 retry 链路；自动回归 / 自动修复重试仍由上方独立项跟踪）
- [x] 增加 timeout / retry policy 持久化
  - 2026-05-02 核对：workflow runtime 已持久化 Attempt timeout、retry 上限、backoff、heartbeat、cleanup retention，并被 DevFlow / JobSupervisor 消费。
- [x] 增加与数据库状态联动的 workspace cleanup 策略
- [x] Work item 详情页增加 Proof 一等展示，按 Requirement / Solution / Diff / Test / Review / PR / Merge 等类型展示证据
- [x] 增加 proof 内容解析和预览，不只停留在文件路径层基础版
  - 2026-05-02：新增 `GET /proof-records/{id}/preview`，按 proof sourcePath 读取本地文本、JSON、Markdown、diff 内容并限制预览大小；二进制截图和大文件分页继续后续增强。
- [x] 增加本地 CLI capability detection：codex / opencode / git / gh / lark-cli
- [x] Human Review Request changes 创建真实 rework Attempt，并把人工反馈写入 Workpad / retry reason / 下一轮 Agent prompt
- [x] Human Review Request changes 复用同一 workspace、branch 和 PR，并在本地 workspace 丢失时优先恢复远端 delivery branch
- [x] Human Review rework 后按需更新 PR description，并让二次 review 核对人工意见和本轮增量 diff
- [x] Human Review Request changes 增加 Rework Assessment：局部修改走 fast rework，需求 / 架构 / 接口变化走 replan rework，信息不足时等待人工补充
- [x] Human Review Request changes 对 fast rework 跳过无需重复的 requirement / architect 阶段，直接复用上一轮 branch / PR 从 Rework 阶段续跑
- [x] PR comments / reviews 基础采集进入 Rework Checklist，下一轮 Rework Agent 可同时消费人工意见、Review Agent 结果、PR 评论和交付门禁建议
- [x] Failed CI/check log 基础采集进入 Rework Checklist：从 check link 抽取 Actions run id，优先读取 failed log，并进入 Workpad Review Feedback / Attempt run report
- [x] Rework Checklist source drilldown 基础版：Workpad 展开后显示每条 checklist 来源的 kind、label 和摘要，避免只看到 action 看不到依据
- [ ] Human Review Rework Assessment 继续增强为可配置策略：按项目/仓库配置关键词、风险阈值和是否强制重新规划
- [x] PR review thread resolved/unresolved 状态、行级上下文和 CI/check source drilldown 进入 Rework Checklist；check 日志分页继续作为后续增强。

## 产品 / 体验缺口

- [x] 增加项目创建与 repository target 配置基础版：新增 `POST /projects` 和 Projects 页面创建入口，`github/bind-repository-target` 支持按 `projectId` 绑定；前端绑定仓库时携带当前 Project，后续继续补多项目切换和更完整 onboarding UI。
- [ ] 增加由服务端数据驱动的 activity / event timeline 页面
- [x] 增加 Operator 视图基础版（队列 / waiting-human / proof / runtime model / checkpoint）
- [ ] 增强 Operator 视图（失败队列 / 重试 / token 耗时 / runner cost）
- [x] 增强 Operator 视图第一版：展示阶段流转、执行锁、Runner 命令/exit code/耗时/stdout-stderr 摘要
- [x] 在 UI 中更清晰地展示 Mission / Operation / Checkpoint 基础版：Operator 列表、Work item 详情页、Human Review gate
- [ ] 优化空状态，引导用户导入 GitHub / Feishu / CI 或手动创建
- [ ] 明确未来多人协作模型
  - 2026-05-02 计划：与 shared sync / 多端授权合并设计，优先保证本地 repository-first 执行链路稳定。
- [x] 接入本地 GitHub 网页 OAuth 主路径
- [x] GitHub OAuth App 配置改为 App 内填写并持久化到 SQLite，`.env` 只作为开发 fallback
- [ ] 继续明确未来 GitHub App / 多人协作授权策略
  - 2026-05-02 计划：继续保留为远期产品化，不与本机 `gh` / OAuth 主链路混在一起。
- [x] 增加本地 App 执行器能力 / Runner process / 当前 execution lock 展示基础版
- [ ] 增加本地 App sync loop 在线状态和远端同步状态展示
  - 2026-05-02 计划：依赖 shared sync API 和远端授权模型，暂不进入本轮实现。
- [x] 增加飞书通知状态展示（runner message 基础版）

## 赛题追踪

### Must-have

- [x] 流水线引擎生命周期基础完整（启动 / 暂停 / 恢复 / 终止）
- [x] 需求拆分作为真实能力，而不是只靠手动创建 Work item
- [x] 支持从 APP 内部 requirement/work item 开始执行本地链路，GitHub issue 不再是唯一入口
- [x] Agent 协作与 handoff artifact 基础版贯穿各阶段：master dispatch、stage contracts、dataFlow、proof bundle
- [x] Agent 编排满足 2 个真实可切换模型提供商：Codex 走本机 CLI 登录态；opencode / Trae Agent 支持页面账号配置、model/provider 选择和运行时环境变量注入。
- [x] 至少 2 个人工检查点，支持 Approve / Reject / 重做
- [x] API-first 架构与 OpenAPI 文档
- [x] 覆盖全部阶段的端到端 Pipeline 演示基础版：GitHub issue claim 后完成 intake / implementation / review / human / merge / done，并产生 proof artifacts
- [x] 端到端演示加入飞书通知/审核链路
  - 2026-05-01：DevFlow 进入 Human Review 后会按配置自动发送飞书审核卡片，或创建飞书 Task 审核；飞书 callback / task sync approve / task comment request changes 与 Omega Web 本地审核同级。真实飞书体验见 `docs/manual-testing-needed.md`，实现说明见 `docs/feishu-review-chain.md`。
  - 2026-05-02：新增飞书 Task 本地常驻事件桥基础版，`POST /feishu/review-task/bridge/tick` 可 dry-run 或同步待审 Task，JobSupervisor 可通过 `OMEGA_FEISHU_TASK_BRIDGE_ENABLED=true` 自动 tick，减少手动 sync。
- [x] 本地端到端演示基础链路：Work item -> Pipeline -> local stage execution -> proof -> checkpoint

### Good-to-have

- [x] 多 Agent 协作基础版：Pipeline stage 支持 `agentIds`，DevFlow PR 模板在 implementation / human review 阶段绑定多个 Agent
- [x] 自动回归 / 自动修复重试基础版
- [x] 可观测性面板基础版（summary API + Operator UI）
- [x] 可观测性面板增强：`/observability` 支持时间窗口、分组统计、最近失败、慢阶段 drilldown 和趋势数据；Views 页面可切换窗口与分组，定位失败和慢阶段更直接。
- [ ] 代码库语义索引
  - 2026-05-02 计划：先记录为下一轮能力建设。建议拆成 repository scan、symbol/document index、selection/source mapping 消费、Agent prompt retrieval 四步，等功能一/功能二主链路测试稳定后实施。
- [x] Pipeline 模板
- [x] Git 集成基础版：隔离 clone、创建本地 branch、commit、生成 diff/summary proof
- [x] GitHub PR 集成基础版：推送 branch、创建 PR、读取 checks、合并 PR

## Page Pilot 产品化

- [x] Page Pilot 入口接入 Workboard / Work Item，并支持选择明确 Repository Workspace。
- [x] Electron Open preview 增加真实 IPC 结果反馈和失败日志，避免端口未启动时表现为无响应。
- [x] Page Pilot GitHub target 使用 Omega 管理的隔离 preview workspace；不再扫描用户本机默认目录猜测 worktree。
- [x] Electron 基础版 Preview Runtime Agent/Profile：Dev server 模式会读取所选 repo、生成 profile、启动 dev server、health check 通过后打开 direct pilot。
- [x] Electron 基础版 Preview Runtime reload supervisor：目标页 Reload / apply 后刷新会按 profile 做 health check、HMR 等待或 dev server restart。
- [x] Preview Runtime Agent/Profile Go 一等化基础版：Page Pilot run 会保存 profile，Go runtime 暴露 resolve/start/restart API，并持久化 profile 与 session summary。
- [x] Go preview runtime process supervisor 基础版：按 Repository Workspace 启动目标项目 dev server，记录 pid、端口、stdout/stderr tail 和健康检查；跨进程恢复和持久 process table 继续后续增强。
- [x] Page Pilot apply / deliver 使用同一隔离 preview workspace 的锁模型基础版，防止多个 Page Pilot run 同时写入同一目录；跨进程/Go supervisor 一等化继续由上方 Go 任务跟踪。
