# Omega 公开文档

本目录只保留适合 GitHub 提交和作品展示的公开文档：产品介绍、架构说明、集成配置、CLI、Page Pilot、发布打包和 demo 手册。

开发日志、交接 prompt、临时方案和测试 fixture 笔记已归档到 `docs/internal-dev-notes/`。该目录已加入 `.gitignore`，不作为公开提交内容。`todo.md` 和 `bug-log.md` 保留为公开项目跟踪文档。

## 推荐阅读

- [中文 README](../README.md)：项目介绍、依赖安装、飞书 / GitHub 配置、本地源码启动和打包说明。
- [English README](../README.en.md)：英文版项目说明。
- [architecture.md](architecture.md)：当前架构，包括 React / Electron、Go Local Runtime、SQLite read model、Workflow Contract、Agent Runner、GitHub 交付、Page Pilot 和 CLI。
- [product.md](product.md)：产品对象、核心页面、DevFlow、Page Pilot 和操作者体验。
- [devflow-agent-execution-policy.md](devflow-agent-execution-policy.md)：AI Agent 与确定性 runtime action 的职责边界。
- [data-model.md](data-model.md)：本地 SQLite 数据归属、规范化 read model 和 legacy snapshot 边界。
- [page-pilot.md](page-pilot.md)：Direct Pilot、Web fallback、Preview Runtime、源码物化和 proof 链路。
- [github-actions-ci-chain.md](github-actions-ci-chain.md)：GitHub Actions checks 如何进入 DevFlow proof、Review 和 Rework。
- [feishu-review-chain.md](feishu-review-chain.md)：飞书 Human Review 投递和 checkpoint decision path。
- [feishu-bot-permissions.md](feishu-bot-permissions.md)：飞书开发者平台应用配置和权限说明。
- [agent-skills-and-mcp.md](agent-skills-and-mcp.md)：Stage 级 Skills / MCP 如何物化到 runner workspace。
- [omega-cli.md](omega-cli.md)：本地 CLI 使用说明。
- [demo-playbook.md](demo-playbook.md)：推荐演示流程和 SaaS Launch 需求文本。
- [release-packaging.zh-CN.md](release-packaging.zh-CN.md)：中文发布打包说明。
- [release-packaging.md](release-packaging.md)：英文发布打包说明。
- [openapi.yaml](openapi.yaml)：Go Local Runtime API 文档。
- [todo.md](todo.md)：公开后续计划。
- [bug-log.md](bug-log.md)：公开 bug 修复记录。

## 文档取舍

公开文档应描述当前产品能力、安装配置、架构边界和可复用演示流程。开发过程中的临时记录、排障流水、未确认方案和内部 handoff 内容应继续放在 `docs/internal-dev-notes/`，避免 GitHub 提交被过程材料淹没。
