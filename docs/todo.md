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
- [x] 将 Workboard 视觉更新为与门户一致的浅色工作台风格，同时保留原有本地执行、GitHub workspace、Agent trace、Human gate、Proof 功能
- [x] 新增 Workspace Config 入口：Project 页面 `Project config`、Workspace 齿轮和 Workspace subnav 都可进入配置页
- [x] Workspace Config 支持本地 workspace root 配置 UI：路径输入、默认路径、目录选择器兜底和保存入口
- [x] Project Agent Profile 配置入口基础版：可编辑 workflow、runner、Skill allowlist、MCP allowlist、stage policy、`.codex` policy，并生成 runtime spec preview
- [x] Project Agent Profile UI 拆成 Workflow / Agents / Runtime files 三层：支持 workflow markdown 草稿、按 Agent 配置 runner/model/Skills/MCP，以及 `.omega` / `.codex` / `.claude` 运行时文件预览
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
- [x] DevFlow 编排重构：抽象 AgentRunner，当前默认 Codex，后续可切换 opencode / Claude Code / local runner
- [x] DevFlow 编排重构：前端详情页展示真实 Agent turn、输入/输出 artifact、状态流转和 runner telemetry，弱化单纯 proof 数字
- [ ] DevFlow 编排重构：继续把当前 background goroutine 升级为 JobSupervisor，补齐 heartbeat、stall retry、cancel、timeout、多 turn continuation、worker host 分配
- [ ] 前端模块化：继续拆分 `App.tsx`，优先拆 Workboard list/detail、Inspector、Operator 面板和 GitHub workspace 组件
- [x] 把 Requirement Decomposition 建成一等服务端能力：raw requirement / GitHub issue -> structured requirement / acceptance criteria / risks / sub-items / target repo context
- [x] 把 Agent handoff bundle 建成基础 artifact：每个 stage 有输入/输出 artifact contract，`devflow-pr` 生成 handoff bundle
- [ ] 把 Agent handoff bundle 从文件 proof 继续升级为可查询的一等表记录
- [x] 增加 Agent runtime spec 基础版：每个 operation / DevFlow cycle 写入 `.omega/agent-runtime.json`
- [ ] 增加 runner-specific runtime 模板：为 Codex / opencode / Claude Code 生成对应 `.codex` / prompt / tool policy 文件，并消费 Project Agent Profile
- [ ] 扩展 runner process supervisor：为 opencode / Claude Code / demo-code 统一接入 timeout、cancel、retry、exit status、stdout/stderr 持久化
- [x] `run-devflow-cycle` 产品主路径异步化：点击 Run 先创建 Attempt 并立即返回，后端 background job 继续执行，前端通过轮询更新状态
- [x] `orchestrator/tick autoRun` 异步化：认领后创建 Attempt，后台 job 执行并在完成/失败后释放 execution lock
- [x] Human Review checkpoint 产品主路径真实化：默认不 auto approve / auto merge，Review Agent 通过后停在 `waiting-human`
- [ ] 把 Workflow Template 从文件默认值继续升级为 SQLite 一等记录，并支持 Project / Repository Workspace 覆盖
- [ ] 增加 Workflow Template 编辑 API：读取、保存、校验、恢复默认、导入/导出 Markdown
- [ ] 把 coding / review / delivery prompt sections 从 Markdown body 渲染到每个 Agent，进一步减少 Go 里的 prompt hardcode
- [ ] 增加正式 JobSupervisor：扫描 repository open issues / ready work items / runnable stages / failed attempts / human gates，并按模板调度下一步
- [ ] JobSupervisor 增加 heartbeat、stall detection、retry、cancel、timeout、worker host 分配、多 turn continuation 和崩溃恢复
- [ ] 增加 issue/work item preflight checks：repo target、workspace root、branch 权限、dirty state、重复执行锁、必要 CLI 能力检查
- [ ] 增加 GitHub delivery contract preflight：issue source 可选，但 repository target、branch 权限、PR 创建权限、checks 读取权限必须在运行前验证
- [x] 增加 orchestrator execution lock：同一个 GitHub issue / Work item / repository target 不能被多个本地 App 重复认领或重复执行
- [x] 增加 Agent runner registry：按 stage/agent/runner 选择 Codex、opencode、Claude Code 或 local runner，并注入不同 issue、workspace、prompt、artifact 上下文
- [x] 增加 Agent runner availability preflight：按 Agent Profile 配置检查 Codex / opencode / Claude Code / demo-code 所需 CLI，缺失时阻止启动而不是生成假失败流程
- [ ] 清理遗留外部参考命名：内部文件、类型、测试、历史文档统一使用 Omega Workboard / DevFlow 语言，外部产品名只出现在“参考来源”说明中
- [x] 把 Product Layer 持久化从 workspace 快照推进到 missions / operations / checkpoints / proof_records 等一等表基础版
- [ ] 按新的 Work Model 继续把 Project / Repository target / Work item / Pipeline run 的 SQLite 结构从 JSON snapshot 推进为可查询表
- [x] 先把 Requirement 从 snapshot 中抽成服务端一等表，并在本地 SQLite / 前端本地 fallback SQLite 中持久化
- [ ] 把 Go Local Service 的 SQLite 写入从 snapshot-first 继续推进为 repository-first
- [ ] 把 Go 侧 migration metadata 演进成可执行增量迁移
- [x] 把旧 Node / 前端本地服务路径降级为兼容回退；Go Local Runtime 已是默认主路径
- [ ] 清理旧 Node / 前端本地服务端兼容代码，减少双实现维护成本
- [ ] 继续减少前端直接持有的业务逻辑，让服务端成为 Mission Control 的唯一写入者
- [x] 把 GitHub repo / issue import API 接到前端 UI
- [x] 把 App 内部需求入口接到 repository workspace：不依赖 GitHub issue 也能创建 Work item 并运行
- [x] Work item 详情页展示 Attempt stages / Agent turns / artifacts / Human Review 基础版，可看到真实状态流转
- [ ] 把 Checkpoint Reject 的“回退重做”做成更清晰的 stage timeline 可视化，突出 rejected -> rework -> review 回流
- [x] Operator 视图增加基础 stage timeline，可看到当前 Pipeline 每个阶段的状态
- [ ] 按赛题要求持续维护 Must-have / Good-to-have 对照与完成度
- [ ] Page Pilot：将 proof 从 run JSON / proof 文件升级为 SQLite 一等表，并在 Operator / Proof UI 展示
- [ ] Page Pilot：扩展 `data-omega-source` 到 Workboard 关键可编辑区域，不只覆盖 Portal Home
- [ ] Page Pilot：增加 patch preview、selection history、discard/revert 的更完整 UI 状态（基础 selection history / process panel 已落地，仍缺完整 diff preview）
- [ ] Page Pilot：把 Electron preview webview、repository target、local worktree 绑定成一等配置
- [ ] Page Pilot：把 Electron direct pilot 的 target URL、projectId、repositoryTargetId 从 env/default 升级为用户可选配置
- [ ] Page Pilot：定义单一 Page Pilot Agent 的 stage contracts：preview-runtime / page-editing / delivery
- [ ] Page Pilot：增加 Preview Runtime stage，读取 repo 文件并生成 install/start/preview URL/health/reload 的可审计运行档案
- [ ] Page Pilot：增加 Preview Runtime Profile API：resolve/start/restart，所有命令锁定到明确 repository workspace
- [ ] Page Pilot：增加 preview process supervisor，记录目标项目 dev server 的 pid、stdout/stderr、端口、健康检查和失败诊断
- [ ] Page Pilot：在 UI 中展示 session 对应的 Requirement / Work Item / Page Pilot pipeline run，并支持从 Work Item 回到 Page Pilot run
- [ ] Page Pilot：增加 live-preview repository write lock，避免和 DevFlow/operation 同时写同一个 worktree
- [ ] Page Pilot：设计 isolated-devflow mode，让预览可以指向隔离 operation workspace 或在确认后回写原 workspace
- [ ] Page Pilot：Preview Runtime stage 落地后，apply 成功时按 profile 必要重启目标项目 dev server
- [ ] Page Pilot：Electron direct pilot 结果面板升级为完整 diff preview / PR body preview / Work Item 回跳
- [ ] Page Pilot：支持同一 Page Pilot run 的多轮追加批注 / 追加说明 / 重新 apply
- [ ] Page Pilot：为 DOM-only 批注增加源码候选定位能力，缺少 `data-omega-source` 时也能给 Agent 更强定位线索
- [ ] Page Pilot：把 `/page-pilot-target` 调试代理替换为 Electron bridge 或显式开发配置
- [ ] Page Pilot：实现 Electron webview preload bridge，支持跨 origin 本地预览的真实元素圈选
- [ ] Page Pilot：把 React Page Pilot 页面接入 Electron `omegaDesktop.openPreview` / `reloadPreview` / selection event
- [ ] Page Pilot：增加 Go target dev server start/restart API，支持修改后重启用户项目 dev server
- [ ] Page Pilot：PR body 增加 DOM context、样式快照和截图证据

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
- [x] Project Agent Profile 从 `omega_settings` 继续升级为一等 SQLite 表
- [ ] Project Agent Profile 继续补充版本历史展示、恢复默认、导入/导出，以及更完整的 workflow/schema 校验
- [ ] 持久化 connector sync report，而不只是 sync intent
- [ ] 增加新项目的 workspace bootstrap API
- [ ] 把 LLM Provider selection 真正映射到 Codex / opencode / compatible provider runner
- [ ] 把 `run-current-stage` 的 runner 继续扩展为 opencode 和更完整的 provider selection 映射
- [ ] 设计 shared sync API：本地/远端/GitHub/飞书创建 requirement 后双向同步状态
- [ ] 在 Go Local Runtime 中扩展 `lark-cli` adapter，支持发送 checkpoint 卡片通知和 pipeline 结果通知

## Workboard / 飞书 / GitHub 缺口

- [x] 把当前轻量 issue list 扩展成更完整的 Omega Workboard 基础版：Project、Views、Repository Workspace、Work item 分组
- [x] 增加项目级和视图级 UI 基础版，不再只是单条 work item CRUD
- [ ] 继续完善 Project 页面：增加 health、active work items、linked PR / checks / pipeline 状态摘要
- [x] 增加真实 GitHub 仓库信息读取 API
- [x] 增加 GitHub issue 导入到 Omega Workboard 的 Go API
- [x] 增加 GitHub repo / issue import 的前端入口
- [x] 完成 GitHub PR 创建：branch / commit / diff proof -> PR title/body
- [x] 完成 Omega DevFlow PR 周期基础版：branch / commit / PR / Review Agent verdict / Human Review checkpoint / approve 后 merge proof
- [ ] 完成 GitHub issue 状态回写：Pipeline 状态 -> issue comment / label / status
- [ ] 完成 GitHub PR 生命周期 UI：展示 branch、PR、checks、review、merge gate，而不是只把 PR URL 放进 proof
- [x] 完成 GitHub checks/CI 读取：check run / workflow run -> proof record / delivery gate
- [ ] 完成 GitHub / CI 的真实出站同步
- [x] 增加真实 Feishu/lark-cli 文本通知发送
- [x] 增加 App 内 Feishu/lark-cli checkpoint/pipeline 文本通知入口
- [ ] 增加真实 Feishu/lark-cli checkpoint 审核卡片发送
- [ ] 增加 Feishu webhook / 回调到 checkpoint 状态迁移
- [ ] 把 Feishu 群聊 requirement 入口建模成 Product Layer 的一等事件源
- [ ] 持久化 Feishu message/card/doc external reference，方便从 Workboard 回跳

## 执行层缺口

- [x] 增加 requirement decomposition stage artifact 基础版，并通过 handoff bundle / stage input contract 串联 solution/coding/testing/review
- [x] 增加 Requirement -> Item 的持久化归属关系，避免 Agent 只拿到孤立 issue/item 后误判目标仓库或需求上下文
- [x] 增加 Agent 协作协议基础版：stage input contract、output contract、handoff bundle、reject reason、retry instruction
- [x] 增加 repository target 安全约束：执行前必须解析 Work item 的 `repositoryTargetId`，避免 Agent 在错误仓库运行
- [x] 增加 DevFlow 长运行的后台 Attempt job 基础版，避免 HTTP 请求阻塞整条执行流程
- [ ] 增加长运行 operation 的正式 queue / retry 模型
- [ ] 增加本地 App sync loop：push local changes / pull remote changes / acquire execution lock / report events
- [x] 增加显式的 run attempts 表
- [x] AutoRun 会形成一条正式 Attempt，并把 workspace、branch、PR、proof、错误和耗时写入持久化记录
- [x] Done Item 在列表禁用 Run；详情页提供显式 Rerun；失败状态显示 Retry 入口
- [ ] 增加 attempts 的 retry / cancel / timeout 策略，并接入 JobSupervisor
- [ ] 增加 timeout / retry policy 持久化
- [ ] 增加与数据库状态联动的 workspace cleanup 策略
- [x] Work item 详情页增加 Proof 一等展示，按 Requirement / Solution / Diff / Test / Review / PR / Merge 等类型展示证据
- [ ] 增加 proof 内容解析和预览，不只停留在文件路径层
- [x] 增加本地 CLI capability detection：codex / opencode / git / gh / lark-cli

## 产品 / 体验缺口

- [ ] 增加项目创建与 repository target 配置
- [ ] 增加由服务端数据驱动的 activity / event timeline 页面
- [x] 增加 Operator 视图基础版（队列 / waiting-human / proof / runtime model / checkpoint）
- [ ] 增强 Operator 视图（失败队列 / 重试 / token 耗时 / runner cost）
- [x] 增强 Operator 视图第一版：展示阶段流转、执行锁、Runner 命令/exit code/耗时/stdout-stderr 摘要
- [x] 在 UI 中更清晰地展示 Mission / Operation / Checkpoint 基础版：Operator 列表、Work item 详情页、Human Review gate
- [ ] 优化空状态，引导用户导入 GitHub / Feishu / CI 或手动创建
- [ ] 明确未来多人协作模型
- [x] 接入本地 GitHub 网页 OAuth 主路径
- [x] GitHub OAuth App 配置改为 App 内填写并持久化到 SQLite，`.env` 只作为开发 fallback
- [ ] 继续明确未来 GitHub App / 多人协作授权策略
- [x] 增加本地 App 执行器能力 / Runner process / 当前 execution lock 展示基础版
- [ ] 增加本地 App sync loop 在线状态和远端同步状态展示
- [x] 增加飞书通知状态展示（runner message 基础版）

## 赛题追踪

### Must-have

- [x] 流水线引擎生命周期基础完整（启动 / 暂停 / 恢复 / 终止）
- [x] 需求拆分作为真实能力，而不是只靠手动创建 Work item
- [x] 支持从 APP 内部 requirement/work item 开始执行本地链路，GitHub issue 不再是唯一入口
- [x] Agent 协作与 handoff artifact 基础版贯穿各阶段：master dispatch、stage contracts、dataFlow、proof bundle
- [ ] Agent 编排满足 2 个真实可切换模型提供商（provider registry 与运行时选择已完成，runner 映射未完成）
- [x] 至少 2 个人工检查点，支持 Approve / Reject / 重做
- [x] API-first 架构与 OpenAPI 文档
- [x] 覆盖全部阶段的端到端 Pipeline 演示基础版：GitHub issue claim 后完成 intake / implementation / review / human / merge / done，并产生 proof artifacts
- [ ] 端到端演示加入飞书通知/审核链路
- [x] 本地端到端演示基础链路：Work item -> Pipeline -> local stage execution -> proof -> checkpoint

### Good-to-have

- [x] 多 Agent 协作基础版：Pipeline stage 支持 `agentIds`，DevFlow PR 模板在 implementation / human review 阶段绑定多个 Agent
- [ ] 自动回归 / 自动修复重试
- [x] 可观测性面板基础版（summary API + Operator UI）
- [ ] 代码库语义索引
- [x] Pipeline 模板
- [x] Git 集成基础版：隔离 clone、创建本地 branch、commit、生成 diff/summary proof
- [x] GitHub PR 集成基础版：推送 branch、创建 PR、读取 checks、合并 PR
