# Omega 赛题对照矩阵

## 1. 文档目的

这份文档用于把比赛要求和 Omega 当前方案逐条对齐，避免理解偏差和遗漏。

说明：

- 本文以你手动提供的完整赛题文本为准。
- 本文默认 **第一阶段只做功能一为主**。
- 功能二不作为当前主线实现，但架构需要预留兼容空间。
- 文中保留少量结构名词，如 `Workboard`、`Pipeline`、`Mission Control`、`DevFlow`，因为它们已经是 Omega 当前架构中的正式术语。

## 2. 赛题核心理解

赛题不是在考“会不会用 AI 写代码”，而是在考：

```text
能不能让 AI 驱动整条研发流水线
```

也就是把下面这条链路真正串起来：

```text
需求 -> 方案 -> 编码 -> 测试 -> 评审 -> 交付
```

核心思想：

- `Pipeline` 是骨架
- `Agent` 是肌肉
- 人类负责监督与 Review

因此 Omega 的产品目标不是做一个“AI 编码器”，而是做一个：

```text
AI 驱动的研发全流程引擎
```

## 3. Omega 当前阶段产品定位

Omega 当前阶段应明确定位为：

```text
一个 Omega Workboard
  +
一个 local-first Mission Control / DevFlow Engine
```

关键说明：

1. Omega 不依赖外部项目管理工具作为内部工作模型。
2. Omega 要自己实现 `Requirement / Item / Pipeline / Proof` 这套 Workboard 状态。
3. 真正的外部系统优先级应该是：
   - GitHub
   - CI
   - Feishu（后续）

## 4. 第一阶段与第二阶段的边界

### 第一阶段

以功能一为主，目标是做出一个可信的本地可运行版本：

- 自己的 Workboard
- 多阶段 Pipeline
- Agent 编排与执行
- 本地代码仓库上下文
- Human-in-the-Loop
- API-first
- 可跑通的端到端演示

### 第二阶段兼容方向

先不作为当前主线实现，但架构必须兼容：

- Feishu 审批与工具调用链
- 多人协作
- GitHub 网页 OAuth / GitHub App
- 更强的后台编排
- 功能二的页面注入 / 圈选 / 热更新链路
- 共享控制面 + 本地执行 App 的最终产品形态

## 5. 研发流程参考模型对照

赛题给出的流程参考模型如下：

| 阶段 | 输入 | Agent 职责 | 输出 |
| --- | --- | --- | --- |
| 需求分析 | 自然语言需求描述 | 理解意图，澄清歧义，输出结构化需求 | 结构化需求文档（含验收标准） |
| 方案设计 | 结构化需求 + 代码库上下文 | 分析现有架构，设计技术方案，确定影响范围 | 技术方案（含文件变更清单、API 设计） |
| 代码生成 | 技术方案 + 代码库 | 按方案逐文件生成/修改代码 | 代码变更集（Diff） |
| 测试生成 | 代码变更集 + 需求 | 生成单元测试和集成测试 | 测试代码 + 执行结果 |
| 代码评审 | 代码变更集 + 方案 + 测试结果 | 多维度审查（正确性、安全性、规范性） | 评审报告（含问题列表和修复建议） |
| 交付集成 | 评审通过的变更集 | 整合变更，生成最终交付物 | 可合并的代码变更 + 变更摘要 |

Omega 当前必须覆盖这条完整链路，哪怕第一阶段里某些阶段先用简化实现。

## 6. Must-have 对照

### 6.1 功能一：Pipeline 引擎

比赛要求：

1. 阶段（Stage）的定义、排序与依赖管理
2. 每个阶段绑定一个或多个 AI Agent 执行
3. 阶段间的数据流转
4. `Pipeline` 的启动、暂停、恢复、终止等生命周期管理

Omega 当前状态：

- 已有：
  - `Stage` 概念
  - 阶段顺序模型
  - 每个 Stage 有显式 `dependsOn`
  - Stage 带 `inputArtifacts / outputArtifacts`
  - Pipeline run 带 `dataFlow`，描述上游产物如何进入下游阶段
  - `WorkItem -> Mission -> Operation`
  - 阶段产物通过 proof / artifact / event / dataFlow 传递
  - `Pipeline` 生命周期 API：start / pause / resume / terminate
  - `run-current-stage` 与 `run-devflow-cycle` 两条执行入口
  - `run-devflow-cycle` 默认异步：先返回 `accepted` 和 Attempt，后台 job 继续执行，前端通过轮询观察进度
- 未完成：
  - 正式 JobSupervisor、heartbeat、stall retry、cancel、timeout、worker host 分配和多 turn continuation 仍需增强

结论：

```text
v0Beta 满足功能一的基础要求；Run 已具备异步 Attempt 主路径，但 JobSupervisor 和 Pipeline 模板编辑仍是后续增强项
```

### 6.2 功能一：Agent 编排与执行

比赛要求：

1. 每个阶段有对应 Agent
2. Agent 有明确角色定义（System Prompt）
3. Agent 有输入输出契约
4. Agent 能感知代码库上下文
5. LLM Provider 可配置，且至少支持 2 个不同模型提供商，运行时可切换

Omega 当前状态：

- 已有：
  - `master` 主 Agent：理解需求、选择流程、生成 dispatch plan、分发阶段 Agent
  - 角色化阶段与 Agent：requirement / architect / coding / testing / review / delivery
  - Pipeline run 内物化 `agents`，包含 System Prompt、输入契约、输出契约、默认工具和默认模型
  - 任务 prompt 生成
  - 本地隔离 workspace 执行
  - 通过 workspace / prompt 文件注入代码库上下文
  - LLM Provider registry：OpenAI 与 OpenAI-compatible
  - 运行时 Provider selection API
- 未完成：
  - Provider selection 目前主要进入配置与 Agent contract，尚未完整映射到每一种 runner 的真实模型调用
  - 多 Agent 并行协商仍是后续 Good-to-have，不作为 v0Beta 主路径

结论：

```text
v0Beta 满足 Agent 契约与编排建模要求；真实多 Provider 执行映射仍需继续增强
```

### 6.3 功能一：Human-in-the-Loop 检查点

比赛要求：

1. 至少 2 个人工检查点
2. 支持 `Approve` / `Reject`
3. `Reject` 要能回退上一阶段并携带理由
4. 有清晰的检查点产出展示 UI 或 API

Omega 当前状态：

- 已有：
  - 持久化 checkpoint 实体
  - `Approve / Reject / 重做` API
  - Reject 理由回传
  - 回退重跑闭环
  - 默认 feature 流程包含多个 human gate：intake / solution / testing / review / delivery
  - API 可以展示当前 checkpoint 产出，并决定继续或回退
- 未完成：
  - UI 的 checkpoint timeline 仍需更清晰，尤其是 Reject 后的重做提示

结论：

```text
v0Beta 满足 API 与数据模型要求；UI 展示仍是提交前体验风险
```

### 6.4 功能一：API-First 架构

比赛要求：

1. 核心操作必须通过 RESTful API 暴露
2. API 设计规范、文档完整
3. 至少提供 Swagger / OpenAPI 或等效文档

Omega 当前状态：

- 已有：
  - 本地 REST API
  - `GET /health`
  - `GET /workspace`
  - `PUT /workspace`
  - `GET /events`
  - `POST /work-items`
  - `PATCH /work-items/:id`
  - `GET /requirements`
  - `POST /requirements/decompose`
  - `GET /pipelines`
  - `POST /pipelines/from-template`
  - `POST /pipelines/:id/start`
  - `POST /pipelines/:id/pause`
  - `POST /pipelines/:id/resume`
  - `POST /pipelines/:id/terminate`
  - `POST /pipelines/:id/run-current-stage`
  - `POST /pipelines/:id/run-devflow-cycle`
  - `GET /checkpoints`
  - `POST /checkpoints/:id/approve`
  - `POST /checkpoints/:id/request-changes`
  - `POST /missions/from-work-item`
  - `POST /operations/run`
  - `POST /run-operation`
  - `openapi.yaml`
- 未完成：
  - 完整 REST CRUD 中的 delete / update pipeline template 仍是后续增强
  - OpenAPI 需要随 v0Beta 继续补充字段级 schema

结论：

```text
v0Beta 满足 API-first 的核心操作暴露；schema 精细度仍需增强
```

### 6.5 功能一：可运行的端到端演示

比赛要求：

1. 至少一次完整 Pipeline 运行
2. 从需求描述输入开始
3. 经过全部阶段
4. 最终产出可运行的代码变更
5. 目标代码库可以是平台自身

Omega 当前状态：

- 已有：
  - App 内手动输入 Requirement，不依赖 GitHub issue 起点
  - Requirement 落库时由 `master` 主 Agent 生成 structured requirement / acceptance criteria / risks / dispatch plan
  - Item 继承当前 Repository workspace，执行前必须解析 `repositoryTargetId`
  - `devflow-pr` 全链路：clone repo -> isolated workspace -> branch -> commit -> PR -> Review Agent verdict -> Human Review checkpoint -> approve 后 merge proof
  - proof 落库并生成 handoff bundle
  - 已在 `ZYOOO/TestRepo` 完成真实闭环：`OMG-52` -> PR #8 -> merge -> `omega-99-multiplication-table.md`
- 未完成 / 风险：
  - 平台自身作为目标代码库的自举演示尚未固定成标准脚本
  - 当前真实代码生成仍偏 demo 模板化；更复杂需求需要接入 Codex/opencode runner 与模板化 prompt

结论：

```text
v0Beta 可演示功能一端到端闭环；复杂需求自动开发能力仍是下一阶段重点
```

## 7. 功能二对照（当前不主做，但要兼容）

比赛要求的功能二包括：

1. 自建前端 UI 和官网
2. 页面内注入悬浮控件
3. 圈选至少 3 个元素并修改
4. 热更新预览
5. 自动创建 MR 并生成摘要

Omega 当前状态：

- 前端 UI 已有基础
- 页面注入 / 圈选 / 热更新 / MR 自动创建未开始

结论：

```text
当前未实现，但架构上应为第二阶段保留空间
```

## 8. Good-to-have 对照

比赛要求至少覆盖 4 个以上。Omega 当前应该把下面这些作为正式目标：

### 8.1 多 Agent 协作

当前状态：
- 有角色化 Agent 概念
- 还没有同阶段并行 / 协商机制

### 8.2 自动回归

当前状态：
- 还没有真正的 review 失败后自动修复与重试

### 8.3 可观测性面板

当前状态：
- 有 activity / event 基础
- 没有实时可视化面板、耗时、Token、成功率

### 8.4 代码库索引

当前状态：
- 还没有语义索引

### 8.5 Pipeline 模板

当前状态：
- 还没有 bugfix / feature / refactor 模板

### 8.6 Git 集成

当前状态：
- 已有 `git` / `gh` 路线规划
- 正在推进真实 GitHub 接入

结论：

```text
当前尚未满足 4 个以上 Good-to-have 的可验证完成状态
```

## 9. 演示与验收要求对照

### 9.1 环节一：平台能力展示

比赛要求：

- 展示需求输入
- 展示各阶段自动流转
- 展示至少一个人工检查点
- 展示最终代码变更质量

Omega 当前状态：

- 需求输入：有
- 自动流转：没有完整跑通
- 人工检查点交互：只有概念，没有闭环
- 最终代码变更质量：可局部演示，不足以称完整

### 9.2 环节二：现场命题挑战

比赛要求：

- 评委现场出题
- 平台驱动研发流程跑通

Omega 当前状态：

- 有 Go Local Service 和 SQLite 持久化基础
- 但完整 Pipeline 尚未闭环

### 9.3 环节三：技术答辩

比赛要求：

- 架构设计
- 工程质量
- AI Native 思维
- 实际价值

Omega 当前状态：

- 架构设计基础已经较清晰
- 工程质量基础可答
- AI Native 思维可答
- 但“实际价值”要建立在完整流程闭环之上，当前仍需补强

## 10. 通用交付物清单对照

比赛要求：

1. 源代码
2. 可运行程序
3. Docker Compose 或等效一键部署
4. 技术方案设计文档
5. 安装与运行指南
6. 测试报告
7. 演示材料

Omega 当前状态：

### 已有

- 源代码
- 本地可运行程序
- 技术方案设计文档（已有多份）
- 安装与运行说明
- 测试体系与测试结果

### 未完全具备

- Docker Compose 或等效一键部署方案（当前更偏本地脚本启动）
- 面向评委的统一测试报告
- 演示材料打包

## 11. 当前最重要的缺口

按比赛要求排序，当前最关键的缺口是：

1. **完整 Pipeline 生命周期**
2. **Agent 编排与 2 个真实 Provider**
3. **至少 2 个完整人工检查点**
4. **全阶段端到端演示**
5. **4 个以上可验收的 Good-to-have**

## 12. 当前建议的第一阶段范围

为了第一阶段聚焦，建议这样收敛：

### 主线

- 自己的 `Workboard`
- 多阶段 `Pipeline`
- Go Local Service
- SQLite
- 本地隔离 workspace
- Codex / 本地 runner
- GitHub / CI 真实接入优先

### 第二阶段兼容但不强做

- Feishu 实时审批 / 工具调用链
- 功能二的页面注入 / 圈选
- 云端多人协作

## 13. 当前文档结论

v0Beta 复查后的结论如下：

```text
Omega v0Beta 已经可以支撑功能一的本地端到端演示：
Requirement -> master dispatch -> Pipeline stages/agents/dataFlow -> workspace -> branch/commit/PR -> review/human/merge proof。

但它仍不是生产完成态：
复杂需求的任意代码生成、runner/model 映射、Pipeline Template 可编辑化、PR/Checkpoint 展示体验仍需继续打磨。
```

因此接下来所有实现都应该围绕赛题矩阵收口，而不是继续只做零散能力。
