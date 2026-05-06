# Omega AI Delivery Engine

Omega 是一个 **local-first AI DevFlow 引擎**，用于把一个需求转化为绑定明确代码仓库的可交付工作。它把 Workboard、Workflow Template、多 Agent 编排、隔离 Repository Workspace、GitHub PR / CI、飞书 Human Review、Page Pilot 可视化修改和 proof 证据链串成一条可运行、可审计的交付闭环。

English version: [README.en.md](README.en.md)

当前版本为 `v0Beta`，重点面向本地演示、本地开发和单人/小团队试用：代码、凭据、workspace、SQLite runtime database 和 Agent 配置都保留在你的机器上。

## 核心能力

- **功能一：AI DevFlow Workboard**
  - 从 Requirement 创建 Work Item。
  - 绑定明确 Repository Workspace，避免 Agent 写错仓库。
  - 按 Workflow Template 执行 Requirement、Plan / Architect、Coding、Testing、GitHub PR、GitHub Actions CI、Review、Human Review、Rework、Merging、Done。
  - 在 Work Item 详情页展示 Plan、TODO、Acceptance Criteria、Validation、Review Packet、Blockers、Retry Reason、Notes、PR、Proof 和阶段级 Agent 统计。

- **功能二：Page Pilot**
  - 打开真实预览页面。
  - 圈选真实 DOM 元素并提交修改要求。
  - Agent 根据 selector、文本快照、页面上下文和仓库边界修改源码。
  - 支持 Confirm / Discard，并把确认后的修改物化为 Work Item、Pipeline、diff、proof 和 PR 记录。

- **GitHub 交付**
  - 创建 branch / commit / pull request。
  - 读取 PR diff。
  - 采集 GitHub Actions checks 和失败日志。
  - 将 PR、CI、review、merge 结果写入 proof。

- **飞书 Human Review**
  - Human Review checkpoint 可投递到飞书。
  - 支持飞书 Task 审核：完成任务表示 approve，评论修改意见可同步为 request changes。
  - Omega UI、飞书 callback、飞书 Task bridge 共用同一条 checkpoint decision path。

- **可配置 Agent / Workflow**
  - 支持 Codex、Claude Code、opencode、Trae Agent。
  - 支持 OpenAI-compatible provider，如 Kimi / Moonshot 等。
  - `devflow-pr` 和 `saas-launch` 是内置 Workflow Template；模板可定义 stage、agent、action、artifact、transition、review round 和 human gate。

## 架构概览

```text
React Web UI / Electron Desktop
  -> Go Local Runtime API
      -> SQLite session / execution read models
      -> Workflow Contract State Runner
      -> Action Executor
      -> Agent Runner Registry
          -> Codex / Claude Code / opencode / Trae Agent
      -> Isolated Repository Workspace
          -> git / gh / GitHub PR / GitHub Actions / merge
      -> Proof / Runtime Logs / Run Workpads / Checkpoints
```

Go Local Runtime 是事实来源。Web UI、Electron、CLI、Page Pilot、飞书回调和 JobSupervisor 都通过同一套本地 API 工作。

详细架构见：[docs/architecture.md](docs/architecture.md)。

## 仓库结构

```text
apps/web                  React + TypeScript Web UI
apps/desktop              Electron desktop shell 和 Page Pilot preload
services/local-runtime    Go local runtime、CLI、workflow executor、SQLite store
services/local-runtime/workflows
                          内置 workflow contracts
docs                      架构、发布、CLI、GitHub、飞书、Page Pilot 文档
scripts                   构建和发布辅助脚本
packages/shared           共享包预留目录
```

## 环境准备

### 必需依赖

推荐开发环境：

- macOS。当前桌面打包和 Page Pilot direct pilot 主要在 macOS 上验证。
- Node.js 22+。Node 20 可以运行大部分本地开发命令，但 Electron 相关依赖建议 Node 22.12+。
- npm。
- Go 1.21+。
- Git。
- GitHub CLI `gh`。

本次提交前验证环境：

```text
macOS arm64
Node.js v20.19.4
npm 10.8.2
Go go1.21.6 darwin/arm64
Git 2.39.3 (Apple Git-145)
GitHub CLI 2.88.0
Electron 41.3.0
electron-builder 26.8.1
Vite 7.1.12
TypeScript 5.9.3
Vitest 3.2.4
```

检查命令：

```bash
node -v
npm -v
go version
git --version
gh --version
```

### 可选 Agent Runner

按需要安装：

- Codex CLI：用于 Codex Agent。
- Claude Code CLI：用于 Claude Code Agent。
- opencode：用于 opencode Agent。
- Trae Agent / trae-cli：用于 Trae Agent。
- lark-cli：用于飞书消息、飞书 Task 和当前用户 fallback。

Codex / Claude Code 通常继承本机 CLI 登录态；opencode / Trae Agent 可以在 Omega 的 Settings / Agent Studio 中配置 provider、model、base URL 和 API key。

## GitHub 配置

Omega 的 GitHub 交付链路依赖本机 `git` 和 `gh`。

登录 GitHub：

```bash
gh auth login
gh auth status
```

推荐 scope：

```text
repo
workflow
read:org（可选，用于组织仓库读取）
```

如果已经登录但 scope 不足，可以刷新：

```bash
gh auth refresh -s repo -s workflow -s read:org
```

演示或首次测试时，建议目标仓库先有一个 `README.md` 初始提交，并确认默认分支是 `main`。空仓库如果第一条推送是 Omega 的功能分支，GitHub 可能会把功能分支设为默认分支，导致后续 PR 创建时 base branch 不存在。

最小演示仓库初始化方式：

```bash
mkdir DemoRepo
cd DemoRepo
git init -b main
printf '%s\n' '# DemoRepo' '' 'Demo repository for Omega DevFlow.' > README.md
git add README.md
git commit -m "Initial README"
git remote add origin https://github.com/<owner>/<repo>.git
git push -u origin main
```

## 飞书配置

Omega 支持三类飞书投递路径：

1. Chat message：机器人向群聊发送审核消息或卡片。
2. Task review：创建飞书任务，完成任务表示 approve，评论修改意见可同步为 request changes。
3. Bot webhook：通过群机器人 webhook 发送消息。

推荐优先使用 **自建应用 + lark-cli + Task review**，不需要公网入口，适合本地 demo。

### 1. 在飞书开放平台创建自建应用

进入飞书开放平台：

```text
https://open.feishu.cn/
```

操作步骤：

1. 创建企业自建应用。
2. 在「凭证与基础信息」中复制：
   - App ID
   - App Secret
3. 开启「机器人」能力。
4. 在「权限管理」中添加所需权限。
5. 发布应用版本。
6. 将应用安装到目标租户。
7. 如果使用群聊消息，把机器人加入目标群。

### 2. 推荐权限

最小消息能力：

```text
im:message
im:message:send_as_bot
im:chat
```

Task review 推荐权限：

```text
task:task:read
task:task:write
```

如果需要把长 Review Packet 写成飞书文档，可额外开启：

```text
drive:drive
space:folder:create
space:document:retrieve
docs:document:copy
```

如果需要按邮箱或手机号解析用户，可能还需要通讯录 / 获取用户 ID 相关权限。不同租户权限名称可能略有差异，以飞书开放平台当前页面为准。

更完整说明见：[docs/feishu-bot-permissions.md](docs/feishu-bot-permissions.md)。

### 3. 安装并配置 lark-cli

安装 `lark-cli` 后初始化 App ID / App Secret：

```bash
lark-cli config init
```

按提示填入：

```text
App ID
App Secret
```

检查：

```bash
lark-cli doctor
lark-cli auth scopes
```

如果要使用当前用户 fallback 或 Reviewer lookup，继续登录用户态：

```bash
lark-cli auth login
lark-cli contact +get-user --as user --format json
```

### 4. 在 Omega 中配置飞书

打开 Omega：

```text
Settings -> Provider access -> Feishu
```

建议流程：

1. 点击 `Test connection`，确认本机 `lark-cli` 可用。
2. 选择 Review channel：
   - `Task review`：推荐本地演示使用。
   - `Chat message`：发送到群聊。
   - `Bot webhook`：使用群机器人 webhook。
3. 如果使用 Task review：
   - 在 Reviewer 中搜索审核人，或使用 `Use current user`。
   - 可选填写 Tasklist ID。
   - 可开启 local task bridge sync。
4. 如果使用 Chat message：
   - 填写 Chat ID，例如 `oc_xxx`。
5. 如果使用 Bot webhook：
   - 填写 webhook URL。
   - 可选填写 webhook secret。
6. 保存配置。

无公网时建议使用 Task review：Omega 本机 runtime 主动通过 `lark-cli` 创建任务，再通过 Task bridge / sync 查询任务完成状态，不需要飞书云端回调你的本机。

## 安装依赖

克隆仓库：

```bash
git clone <repo-url> omega
cd omega
```

安装 Node 依赖：

```bash
npm install
```

如果你希望使用 CI 风格的可重复安装：

```bash
npm ci
```

## 从源码启动

建议开三个终端。

### 终端 1：启动 Go Local Runtime

```bash
npm run local-runtime:dev
```

默认地址：

```text
http://127.0.0.1:3888
```

默认数据库：

```text
<repo>/.omega/omega.db
```

默认 workspace root：

```text
~/Omega/workspaces
```

也可以手动指定：

```bash
go run ./services/local-runtime/cmd/omega-local-runtime \
  --host 127.0.0.1 \
  --port 3888 \
  --database .omega/omega.db \
  --workspace-root "$HOME/Omega/workspaces"
```

### 终端 2：启动 Web UI

```bash
npm run web:dev
```

打开：

```text
http://127.0.0.1:5173
```

### 终端 3：启动 Electron Desktop

```bash
npm run desktop:dev
```

Desktop 会打开 Omega 桌面壳，并支持 Electron direct Page Pilot。

## 本地使用流程

1. 打开 Omega。
2. 在 `Settings -> Provider access` 检查 GitHub / Feishu / Agent Access。
3. 在 `Projects` 或 Workboard 中绑定 GitHub repository。
4. 在 Agent Studio 中选择 workflow template：
   - `devflow-pr`：通用 requirement-to-PR 流程。
   - `saas-launch`：SaaS demo / 发布演示流程。
5. 创建 Requirement。
6. 转成 Work Item。
7. 运行 DevFlow。
8. 在 Work Item 详情页查看：
   - Plan / TODO
   - Acceptance Criteria
   - Validation
   - Review Packet
   - Agent statistics
   - PR / CI / Proof
9. Human Review 阶段在 Omega 或飞书中 approve / request changes。
10. 通过后进入 Merging / Done，并生成最终 proof。

## Page Pilot 使用流程

1. 进入 Page Pilot。
2. 选择 Repository Workspace。
3. 对 package 项目，使用 Preview Runtime Agent 启动 dev server；纯静态项目可打开 `index.html`。
4. 在页面中圈选 DOM 元素。
5. 输入修改要求。
6. 选择 Agent runner。
7. 等待 Agent 修改源码。
8. 点击 Confirm 将修改物化为 Work Item / Pipeline / proof；或点击 Discard 丢弃本轮修改。

Page Pilot 不会为 package 项目伪造默认 URL。它会尽量启动真实预览运行时，确保圈选、源码修改、diff、PR 和 proof 落到真实数据上。

## CLI

开发期使用：

```bash
npm run omega -- health
npm run omega -- work-items list
npm run omega -- attempts list --limit 20
npm run omega -- operations list --limit 20
npm run omega -- workpads list --limit 20
npm run omega -- proof list --limit 20
```

构建后的 binary：

```bash
dist/release/bin/omega --api-url http://127.0.0.1:3888 health
```

完整命令见：[docs/omega-cli.md](docs/omega-cli.md)。

## 构建桌面应用

构建 Web assets 和 Go binaries：

```bash
npm run release:prepare
```

构建 unpacked desktop app：

```bash
CSC_IDENTITY_AUTO_DISCOVERY=false npm run desktop:pack
```

构建 macOS `.dmg` 和 `.zip`：

```bash
CSC_IDENTITY_AUTO_DISCOVERY=false npm run desktop:dist
```

预期产物：

```text
dist/desktop/Omega-0.1.0-arm64.dmg
dist/desktop/Omega-0.1.0-mac-arm64.zip
dist/release/bin/omega
dist/release/bin/omega-local-runtime
```

未签名构建适合内部测试。公开发布 macOS 应用建议配置 Developer ID 签名和 notarization。

更多说明见：[docs/release-packaging.zh-CN.md](docs/release-packaging.zh-CN.md)。

## 未来 TODO

- Workflow Template 可视化编辑：支持 stage 拖拽、连线、版本历史、导入导出和模板市场化。
- Agent 观测增强：补齐 token、成本、失败率、重试次数、阶段耗时和 Agent 产物质量统计。
- Page Pilot 源码定位增强：在复杂前端项目里更稳定地从 DOM selector 追踪到组件、样式和数据源。
- 团队协作：增加多人本地协同、远端同步和共享审核记录。
- 执行日志长期留存：将 stdout / stderr / runner details 从 SQLite 热库中分层归档，降低本地数据库膨胀风险。
- 更多交付模板：补充 SaaS Launch、Bug Bash、UI Iteration、Docs Release 等可复用 DevFlow。

## 公开文档

- [当前架构](docs/architecture.md)
- [产品总结](docs/product.md)
- [DevFlow Agent 执行边界](docs/devflow-agent-execution-policy.md)
- [数据模型](docs/data-model.md)
- [Page Pilot](docs/page-pilot.md)
- [GitHub Actions CI 链路](docs/github-actions-ci-chain.md)
- [飞书 Review 链路](docs/feishu-review-chain.md)
- [飞书应用权限](docs/feishu-bot-permissions.md)
- [Agent Skills / MCP](docs/agent-skills-and-mcp.md)
- [CLI 参考](docs/omega-cli.md)
- [Demo 手册](docs/demo-playbook.md)
- [发布打包](docs/release-packaging.zh-CN.md)
- [Release packaging](docs/release-packaging.md)
- [OpenAPI](docs/openapi.yaml)
- [TODO](docs/todo.md)
- [Bug Log](docs/bug-log.md)

开发日志、交接 prompt、临时方案和测试 fixture 笔记已归档到 `docs/internal-dev-notes/`，该目录已加入 `.gitignore`，不作为 GitHub 公开提交内容。`docs/todo.md` 和 `docs/bug-log.md` 保留为公开项目跟踪文档。
