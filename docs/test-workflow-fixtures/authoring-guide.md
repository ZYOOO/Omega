# Omega Workflow Markdown 准备指南

这份指南说明在实际测试功能一和功能二前，建议准备哪些 Markdown 文件，以及每个文件在 Omega 运行链路中的作用。

## 推荐文件结构

```text
.omega/
  WORKFLOW.md
  PROMPTS.md
  AGENTS.md
  RUNTIME_POLICY.md
  REVIEW_POLICY.md
  DELIVERY_POLICY.md
  REQUIREMENT_TEMPLATE.md
  page-pilot/
    PAGE_PILOT.md
    PREVIEW_RUNTIME.md
```

测试时也可以先使用 `docs/test-workflow-fixtures/` 下的样例内容复制到 Workspace Agent Studio。

## 文件职责

### WORKFLOW.md

定义 DevFlow 的阶段、动作、流转和 gate。它回答：

- 有哪些阶段。
- 每个阶段由哪些 action 组成。
- action 成功、失败、需要人工时流向哪里。
- Human Review、Rework、Merging、Done 的边界是什么。

建议包含：

- workflow id / name。
- states / actions。
- transitions。
- task class。
- runtime policy snapshot。
- required checks。

### PROMPTS.md

定义每个 Agent 的稳定工作契约。它回答：

- 这个 Agent 负责什么，不负责什么。
- 它会收到哪些输入。
- 它必须遵守哪些边界。
- 它必须输出哪些结构化字段。
- 遇到失败、阻塞和缺信息时怎么写。

Prompt 不应该只有一句职责说明。Omega 的 Rework、Retry、Review Packet 和 Workpad 都依赖结构化输出，越明确越稳。

### AGENTS.md

定义各阶段 Agent 的 runner / model / skills / MCP 默认值。它回答：

- Requirement、Architect、Coding、Testing、Review、Rework、Delivery 分别用哪个 runner。
- 每个 Agent 默认 model 是什么。
- 可以使用哪些 skills。
- 可以访问哪些 MCP / 本地能力。

密钥不写入 Markdown。opencode / Trae Agent 这类账号信息应通过设置页加密保存；Codex / Claude Code 这类本机登录型 runner 继续使用本地 CLI 登录态。

### RUNTIME_POLICY.md

定义本地 runtime 的执行边界。它回答：

- workspace 如何创建和清理。
- runner 超时、心跳、取消和 retry 怎么处理。
- stdout / stderr / runtime log 如何记录。
- proof、handoff bundle、run report 如何生成。

### REVIEW_POLICY.md

定义 Review / Rework 的判定和回流。它回答：

- Review verdict 有哪些。
- `changes_requested` 如何生成 checklist。
- Human request changes、PR comments、failed checks 如何进入 rework input。
- 哪些情况应该进入 Human Review，而不是自动继续。

### DELIVERY_POLICY.md

定义出站交付。它回答：

- PR 什么时候创建或更新。
- merge 前如何检查 required checks、branch sync 和 conflict。
- merge 失败时如何分类和推荐操作。
- merge 成功后如何生成 proof 和 handoff。

### REQUIREMENT_TEMPLATE.md

定义需求录入模板。它回答：

- 用户应该提供哪些背景。
- 验收标准怎么写。
- 哪些内容明确不做。
- 需要哪些人工验证。

### PAGE_PILOT.md / PREVIEW_RUNTIME.md

定义功能二的预览、圈选和确认交付规则。它回答：

- 目标 repo 如何进入隔离 preview workspace。
- preview dev server 如何由 runtime/agent 启动和健康检查。
- DOM selection 如何映射到 source context。
- Confirm / Discard 如何处理 workspace、branch、commit 和 PR。

## 编写原则

- 每个文件只负责一个层面，不把所有内容塞进一个巨大 Markdown。
- workflow 负责“怎么流转”，prompt 负责“Agent 怎么做”，runtime policy 负责“执行边界”。
- 所有写仓库动作都必须锁定明确 Repository target。
- Human Review 和 Rework 必须是可追溯状态，不要只在日志里留一行文字。
- 失败原因要写成用户能行动的语言：缺权限、check failed、冲突、网络临时失败、需求缺信息。
- 不要把密钥、token、个人账号信息写进 Markdown。

## 从模板方法中吸收的部分

Omega 采用的是自己的 Requirement / Work Item / Pipeline / Attempt / Workpad 模型，但可以吸收成熟模板的组织方式：

- 全局运行规则放前面。
- 简单任务和复杂任务分流。
- 每个阶段都有明确输入、过程、输出。
- Review 必须产生结构化 verdict。
- Rework 必须消费 checklist。
- Delivery 必须先检查 PR / checks / branch sync，再 merge。
- 外部阻塞必须转成 Human Review 或明确 blocker。

## 推荐测试顺序

1. 先用 `WORKFLOW.md` 跑默认 DevFlow。
2. 再用 `PROMPTS.md` 替换各 Agent prompt。
3. 配置 `AGENTS.md` 中的 runner/model/skills/MCP。
4. 跑一个简单 UI 或文案变更，检查 PR / Human Review / proof。
5. 人工 Request changes，确认进入 Rework 并复用原 workspace。
6. 再跑一个故意失败的 validation，确认 retry reason 和 checklist 可读。
