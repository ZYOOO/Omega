# Omega 开发思路

本文记录当前开发路线，目标是让 v0Beta 可以稳定演示比赛功能一：从需求输入到可交付代码变更的完整闭环。

## 1. 原则

### 1.1 先保证真实闭环

Omega 现在不再接受“只有 UI 没有真实行为”的流程。凡是用户点击创建、绑定、运行、删除，都应当：

- 走 Go local service。
- 写入 SQLite。
- 对真实 repository target 生效。
- 有错误反馈。
- 有测试覆盖。

### 1.2 本地优先

当前版本先做本地 App 可独立工作：

```text
apps/web React SPA
  -> services/local-runtime Go Local Runtime
  -> SQLite
  -> local git / gh / runner
```

未来可以加 shared sync control plane，但它不是当前本地闭环的前置条件。

Electron shell 位于 `apps/desktop`。它是打包与桌面能力层，不改变日常 Web 开发主路径。

### 1.3 Repository target 是安全边界

每个 Item 必须知道自己属于哪个 repository target。执行前要校验：

- Item 是否绑定 `repositoryTargetId`。
- 当前 active workspace 是否匹配。
- workspace path 是否在配置的 workspace root 内。
- 是否已有 execution lock。
- 需要的 CLI 能力是否存在。

### 1.4 Requirement 与 Item 分层

Requirement 是需求语义，Item 是可执行单元。

```text
Requirement
  -> master dispatch
  -> one or more Items
```

因此 GitHub issue、手动输入、飞书消息都只是 Requirement 的来源；Omega 内部运行的是 Item。

## 2. 开发主线

### 阶段 A：产品模型稳定

目标：

- Project、Repository target、Requirement、Item、Pipeline 的边界清晰。
- UI 列表、详情页、左侧 workspace、右侧 inspector 能表达对象从属关系。
- `Ready` 不再被误解为完成，创建后准备编排有 `Planning` 状态。

已完成：

- Repository workspace 进入 Work items 后默认聚焦当前 repo。
- App 内新建需求继承当前 repo target。
- Item 列表展示 repository target、source、requirement、agent、proof 数。
- Done 项禁用 Run。
- 行内展示 Pipeline stages 和 Agent 分配。

### 阶段 B：Go local service 成为唯一主写入路径

目标：

- 业务写入不依赖浏览器 local state。
- SQLite 保存核心对象。
- 前端只通过 API 创建、更新和运行。

已完成：

- `/work-items`
- `/requirements`
- `/pipelines`
- `/operations`
- `/proof-records`
- `/github/*`
- `/local-workspace-root`

继续：

- 把更多 snapshot 字段推进为一等表。
- attempts 已经落地，并成为 Run / AutoRun 的主记录。
- 继续增加 retry policy / template records。

### 阶段 C：真实 GitHub 交付闭环

目标：

```text
Item
  -> isolated workspace
  -> branch
  -> commit
  -> PR
  -> checks / review
  -> merge proof
```

已完成：

- 本机 `gh` 登录态读取 repo 列表。
- App 内 GitHub OAuth 配置和 callback。
- repository target 绑定。
- issue import。
- PR 创建。
- PR / checks 状态读取。
- `ZYOOO/TestRepo` 已跑通过一次 App Requirement -> PR -> merge。

继续：

- issue comment / label 回写。
- PR lifecycle UI。
- checks failure -> auto fix retry。

### 阶段 D：Agent 编排能力增强

目标：

- master 负责需求理解和 dispatch。
- stage Agent 有明确输入输出契约。
- runner 能消费 stage artifact 和上下文。
- 每个 runner 独立 process，失败不拖垮 orchestrator。
- 默认 workflow 可以强约束流程，但不能成为 Go 代码里的唯一流程。

已完成：

- master / requirement / architect / coding / testing / review / delivery definitions。
- Pipeline stages 包含 `agentIds`、`dependsOn`、`inputArtifacts`、`outputArtifacts`。
- `.omega/agent-runtime.json` 记录运行上下文。
- Codex runner process supervisor 基础版。
- `run-devflow-cycle` 默认异步化：Run 立即返回 Attempt，后台 job 继续执行，前端通过轮询观察进度。
- 默认 `devflow-pr` 已抽成 Markdown workflow：`services/local-runtime/workflows/devflow-pr.md`。Go runtime 会从该模板读取 stages、agents、artifact 和 review rounds，而不是只依赖代码 hardcode。

继续：

- Workflow Template 持久化和 App 内编辑器。
- Prompt sections / stage instructions 从 Markdown body 渲染到每个 Agent。
- Agent runner registry。
- Codex / opencode / Claude Code prompt 文件模板。
- JobSupervisor：heartbeat、timeout、cancel、retry、stall detection、worker host 分配和多 turn continuation。
- stage artifact 一等查询。

### 阶段 E：演示体验

目标：

用户能清楚看到：

- 需求从哪里来。
- 哪个 master dispatch 创建了哪些 Items。
- 现在处于哪个 Pipeline stage。
- 哪些 Agent 参与了当前阶段。
- 当前 runner 是否在跑。
- 产出了哪些 proof。
- GitHub PR / checks / review / merge 状态如何。

已完成：

- React SPA 已形成双页面结构：门户首页作为默认入口，Workboard 作为真实功能页。
- 门户首页已拆到 `apps/web/src/components/PortalHome.tsx`，`App.tsx` 后续继续拆 Workboard 子模块。
- Workboard 已统一到浅色工作台视觉体系，减少旧暗色界面的信息噪声。
- Work items 列表行内 stage strip。
- Detail 页面优先展示真实 pipeline stages。
- Operator 面板展示 stage timeline、execution locks、runner telemetry。

继续：

- 更完整的 delivery activity timeline。
- checkpoint reject / redo 可视化。
- PR lifecycle 卡片。

## 3. 测试策略

### 3.1 前端

主要测试：

```bash
npm run lint
npm test -- --run src/__tests__/App.operatorView.test.tsx
npm test -- --run src/core/__tests__/workboard.test.ts
```

重点覆盖：

- Requirement 创建。
- repository workspace 作用域。
- Run 按钮状态。
- Planning / Not started / Running / Done 展示。
- Pipeline stages / Agent 展示。
- GitHub OAuth / repo list / import。

### 3.2 后端

主要测试：

```bash
go test ./services/local-runtime/internal/omegalocal
go test ./...
```

重点覆盖：

- SQLite migration / backfill。
- GitHub repo target bind/delete。
- Requirement decomposition。
- Pipeline template / run-current-stage / run-devflow-cycle。
- workspace root safety。
- execution locks。
- PR / checks API。

### 3.3 手动验证

标准本地验证使用 `ZYOOO/TestRepo`：

1. 启动 Go local service。
2. 启动 Vite。
3. GitHub 登录或使用本机 `gh`。
4. 选择 `ZYOOO/TestRepo`。
5. 创建 repository workspace。
6. 在 workspace 内新建 Requirement。
7. 点击 Run。
8. 检查 Not started / Running / Human Review / Done 等状态。
9. 在 Human Review 处检查 PR、Review artifacts、changed files 和 proof。
10. Approve 后再检查 merge / delivery 结果。

## 4. 近期优先级

1. Pipeline Template 从内置 Go 模板变成 App 内可编辑配置。
2. Runner registry 深化：在已接入 Codex / opencode / Claude Code 统一分发骨架后，继续补 runner-specific 模板、timeout/cancel/retry 和 provider 映射。
3. Attempts 表：每次 Operation 独立记录状态、耗时、错误、输出、重试关系。
4. GitHub delivery UI：PR、checks、review、merge gate 一屏可见。
5. Checkpoint timeline：Approve / Reject / redo 更清楚。
6. Feishu/lark-cli 审核卡片。
7. 继续拆分 Workboard 前端组件，降低 `App.tsx` 的维护成本。
