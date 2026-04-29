# Omega 文档入口

这组文档以当前 v0Beta 代码为准，用于整理架构、开发路线、开发日志、赛题对照和测试流程。

## 当前主文档

- `architecture.md`：当前真实架构，覆盖 React Workboard、Go Local Service、SQLite、Pipeline、Agent、GitHub 交付闭环。
- `development-plan.md`：开发思路、阶段目标、测试策略和近期优先级。
- `development-log.md`：已完成的关键工程节点和验证命令。
- `competition-requirements-matrix.md`：比赛要求逐条对照。
- `current-product-design.md`：产品形态、对象关系和功能缺口。
- `work-model-reference.md`：Requirement / Item / Pipeline / Mission / Operation / Proof 的关系。
- `persistence-schema.md`：当前 SQLite / Product Layer 持久化模型。
- `manual-testing-guide.md`：本地手动验证流程。
- `omega-devflow-workflow.md`：当前 repository-backed delivery workflow。
- `devflow-production-core.md`：功能一生产化内核，记录 workflow runtime、runner telemetry、PR checks、workspace lifecycle 和 execution lock 的当前做法。
- `omega-cli.md`：本地 operator CLI 的命令、架构约束和使用说明。
- `openapi.yaml`：本地 REST API 文档。
- `todo.md`：开发任务清单。

## 当前仓库结构

```text
apps/web                  # TS + React SPA，日常 UI 开发主路径
apps/desktop              # Electron shell 预留，后续负责本地 App 打包和预览 webview
services/local-runtime    # Go local runtime，负责 API、SQLite、编排、本地 runner、GitHub 交付
packages/shared           # 共享类型/API schema 预留
docs                       # 当前文档
scripts                    # 兼容脚本和 smoke 工具
```

## 当前对象主线

```text
Requirement
  -> master dispatch
  -> Item
  -> Pipeline stages / Agent contracts / dataFlow
  -> isolated workspace under configured workspace root
  -> GitHub branch / commit / pull request / checks / review / merge proof
```

## 维护规则

- 当前主文档必须对齐代码里真实存在的 API、数据结构和 UI 行为。
- 外部产品只能作为集成目标或测试目标，不作为 Omega 的内部模型命名。
- 新功能落地时，要同步更新 `development-log.md` 和 `todo.md`。
- 比赛要求变化时，要同步更新 `competition-requirements-matrix.md`。

## 历史资料

旧的探索记录可以保留作背景，但不作为当前 v0Beta 的验收依据。检查产品方向时，以本文件列出的主文档为准。
