# Omega Agent Skills 与 MCP 配置说明

> 更新时间：2026-05-05

## 当前结论

Omega 现在不是只在 UI 上保存 Skills / MCP。每个 stage 的 Agent Profile 会在启动 runner 前物化到当前运行 workspace：

- `.omega/agent-runtime.json`：运行时元数据，包含 Agent Profile 摘要。
- `.omega/agent-capabilities.json`：机器可读能力清单，包含 stage skills、stage MCP、项目 allowlist、runner、model。
- `.omega/agent-capabilities.md`：人和 Agent 都容易阅读的能力说明。
- `.codex/OMEGA.md`：Codex runner 的阶段策略与能力边界。
- `.claude/CLAUDE.md`：Claude Code runner 的阶段策略与能力边界。
- 子进程环境变量：`OMEGA_AGENT_SKILLS`、`OMEGA_AGENT_MCP`、`OMEGA_AGENT_SKILL_ALLOWLIST`、`OMEGA_AGENT_MCP_ALLOWLIST` 等。

本轮新增测试会启动 fake `opencode` runner，并让子进程在自己的 cwd 中读取这些文件和环境变量；如果缺少 stage skill / MCP，测试会失败。这样能证明服务启动的 Agent 进程确实拿到了对应能力声明。

## 本机安装位置

### Skills

安装目录：`/Users/zyong/.codex/skills`

| Skill | 路径 | 主要 Stage | 用途 |
| --- | --- | --- | --- |
| `bb-browser` | `/Users/zyong/.codex/skills/bb-browser` | Requirement / 调试 / 手测 | 使用真实浏览器和登录态检查页面、私有页面或本地页面。 |
| `playwright` | `/Users/zyong/.codex/skills/playwright` | Coding / Testing | 进行浏览器自动化、UI 回归、截图验证和交互测试。 |
| `gh-address-comments` | `/Users/zyong/.codex/skills/gh-address-comments` | Review / Delivery | 读取和处理 PR review comments、unresolved threads。 |
| `gh-fix-ci` | `/Users/zyong/.codex/skills/gh-fix-ci` | Testing / Review / Rework | 定位 GitHub Actions / CI 失败日志并指导修复。 |
| `yeet` | `/Users/zyong/.codex/skills/yeet` | Delivery | 提交、push、创建 draft PR 或交付 PR。 |
| `security-best-practices` | `/Users/zyong/.codex/skills/security-best-practices` | Coding / Review | 在实现和 review 中检查常见安全风险。 |
| `security-threat-model` | `/Users/zyong/.codex/skills/security-threat-model` | Architect | 在方案阶段做边界、资产、威胁和缓解措施分析。 |
| `openai-docs` | `/Users/zyong/.codex/skills/openai-docs` | Requirement / Architect | 查询 OpenAI 官方文档相关实现约束。 |

### MCP

全局配置文件：`/Users/zyong/.codex/config.toml`

| MCP 名称 | 命令 | 主要 Stage | 用途 |
| --- | --- | --- | --- |
| `omega-filesystem` | `/Users/zyong/.nvm/versions/node/v20.19.4/bin/mcp-server-filesystem` | 全阶段 | 只暴露 Omega 项目和 Omega workspace 目录，供 Agent 读取/操作仓库文件。 |
| `omega-git` | `/Users/zyong/.nvm/versions/node/v20.19.4/bin/git-mcp-server` | Coding / Testing / Review / Delivery | Git 状态、diff、提交历史和交付相关操作。 |
| `omega-puppeteer` | `/Users/zyong/.nvm/versions/node/v20.19.4/bin/mcp-server-puppeteer` | Coding / Testing / Page Pilot | 浏览器预览、页面检查和交互验证。 |
| `omega-memory` | `/Users/zyong/.nvm/versions/node/v20.19.4/bin/mcp-server-memory` | Requirement | 保存需求澄清、上下文摘要和跨阶段记忆。 |
| `omega-sequential-thinking` | `/Users/zyong/.nvm/versions/node/v20.19.4/bin/mcp-server-sequential-thinking` | Requirement / Architect | 辅助需求拆解、方案推理和风险梳理。 |
| `x-mcp` | 既有远端配置 | 按需 | 继续保留已有远端 MCP 能力。 |

> 新安装的 Skills / MCP 需要重启 Codex 或相关 runner 才会被宿主进程重新发现。Omega runtime 写入的 `.omega/agent-capabilities.*` 不依赖重启，但 MCP server 的宿主发现通常需要重启。

## 默认 Stage 映射

| Stage | 默认 Skills | 默认 MCP |
| --- | --- | --- |
| Requirement | `bb-browser`, `openai-docs` | `omega-filesystem`, `omega-memory`, `omega-sequential-thinking` |
| Architect | `security-threat-model`, `openai-docs` | `omega-filesystem`, `omega-sequential-thinking` |
| Coding | `playwright`, `security-best-practices` | `omega-filesystem`, `omega-git`, `omega-puppeteer` |
| Testing | `playwright`, `gh-fix-ci` | `omega-filesystem`, `omega-puppeteer`, `omega-git` |
| Review | `gh-address-comments`, `gh-fix-ci`, `security-best-practices` | `omega-git`, `omega-filesystem` |
| Delivery | `yeet`, `gh-address-comments` | `omega-git`, `omega-filesystem` |

## 数据链路

1. Web 的 Workspace Agent Studio 保存 `ProjectAgentProfile`。
2. Go local runtime 通过 `/agent-profile` 持久化到 SQLite。
3. Pipeline / Operation 解析当前 Work Item 的 Project 和 Repository Target，选择项目级或仓库级 Agent Profile。
4. 启动 runner 前写入 `.omega/agent-runtime.json`、`.omega/agent-capabilities.json`、`.omega/agent-capabilities.md`、`.codex/OMEGA.md`、`.claude/CLAUDE.md`。
5. 启动 runner 子进程时注入 `OMEGA_AGENT_SKILLS`、`OMEGA_AGENT_MCP` 等环境变量。
6. 对有明确 repository target 的 DevFlow，能力文件写入隔离 clone 的 repo workspace；无 target 的 operation 写入该 operation 的临时 workspace。
7. Agent prompt 仍附带 `Omega Agent Profile` 摘要，作为文本层冗余提示；真实可验证边界以 runtime files 和 runner env 为准。

## 验证方式

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestProjectAgentProfilePersistsAndFeedsRuntimeBundle|TestProfileRunnerRegistrySelectsConfiguredAgentRunner|TestProfileSkillsAndMCPAreMaterializedForRunnerProcess' -count=1
```

重点测试：

- `TestProjectAgentProfilePersistsAndFeedsRuntimeBundle`：确认 profile 持久化、runtime bundle、policy files 和 capabilities files 都包含 Skills/MCP。
- `TestProfileSkillsAndMCPAreMaterializedForRunnerProcess`：启动 fake `opencode`，由 runner 子进程读取 `.omega/agent-capabilities.*`、`.codex/OMEGA.md`、`.claude/CLAUDE.md` 和 `OMEGA_AGENT_*` 环境变量，证明不是 UI-only。
