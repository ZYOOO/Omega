# Omega Demo Playbook

本文是一套面向项目演示的完整流程。目标不是把所有功能都点一遍，而是用一个从 0 到可交付 demo 的 SaaS 场景，把 Omega 的两条主能力讲清楚：

- 功能一：Requirement -> Work Item -> DevFlow -> Agent -> PR -> Review -> Human Gate -> Merge -> Proof。
- 功能二：Page Pilot 在真实前端页面上圈选 DOM，提交修改，刷新预览，Confirm 后物化成 Work Item / Pipeline / PR / Proof。

## 1. 演示定位

### 一句话

Omega 是一个 local-first AI DevFlow 引擎：它把一个自然语言需求变成可审计、可回滚、可人工把关的工程交付流程。

### 演示产品设定

建议准备一个轻量 SaaS 前端 demo repo，名字可以叫 `AcmeBoard` 或 `Northstar CRM`。它不需要复杂后端，但必须有真实前端页面，方便 Page Pilot 展示功能二。

推荐页面：

- Dashboard：首页 KPI、趋势图、最近客户列表。
- Customers：客户表、健康分、风险标签。
- Settings：团队设置、通知偏好、Billing 占位。
- Login / Onboarding：适合 Page Pilot 圈选文案、按钮、表单和卡片。

推荐技术栈：

- Vite + React + TypeScript。
- `npm run dev` 可本地启动。
- `npm test` 或 `npm run build` 至少一个验证命令。
- GitHub repo 开启一个简单 Actions workflow，例如 `npm ci && npm run build`。

### 演示主张

传统 AI coding demo 通常只展示“我让模型改了代码”。Omega 要展示的是“一个需求如何被系统完整交付”：

```text
需求输入
  -> 任务归档到明确 Repository Workspace
  -> Agent 分工和计划
  -> 编码 / 测试 / PR / CI / Review
  -> 人工审批或退回
  -> merge 和 proof
```

## 2. 演示前准备

### 本机工具

演示机提前确认：

```bash
git --version
gh --version
node --version
npm --version
go version
codex --version
```

可选：

```bash
claude --version
opencode --version
trae-cli --version
lark-cli --version
```

### GitHub

准备事项：

- `gh auth status` 已登录。
- demo repo 已 push 到 GitHub。
- repo 默认分支干净。
- Actions workflow 可运行。
- 演示用 GitHub token / OAuth 不在屏幕上展示。

### Cloudflare

Cloudflare 只作为演示目标环境，不要求 Omega 产品本身先内置 Cloudflare provider。推荐先用 Cloudflare Pages 的 GitHub integration，让 PR / merge 自动产生 preview 或 production deployment；Omega 负责把 CI、PR、Human Review 和 deployment proof 串起来。

准备事项：

- Cloudflare 账号已登录。
- demo repo 已连接到 Cloudflare Pages。
- Production branch 指向 `main`。
- Build command 使用 demo repo 的真实命令，例如 `npm run build`。
- Build output directory 使用真实产物目录，例如 `dist`。
- 如果不使用 Pages GitHub integration，而是通过 GitHub Actions 部署，则提前准备：
  - `CLOUDFLARE_API_TOKEN`
  - `CLOUDFLARE_ACCOUNT_ID`
  - 可选 `CLOUDFLARE_PROJECT_NAME`
- 演示时不要在屏幕上展示 token；只展示 GitHub Actions secret 名称、Cloudflare deployment URL 和 Omega proof。

推荐 GitHub Actions 形态：

```yaml
name: Web CI and Cloudflare Preview

on:
  pull_request:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 22
          cache: npm
      - run: npm ci
      - run: npm test
      - run: npm run build

  deploy:
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 22
          cache: npm
      - run: npm ci
      - run: npm run build
      - run: npx wrangler pages deploy dist --project-name "$CLOUDFLARE_PROJECT_NAME"
        env:
          CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
          CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
          CLOUDFLARE_PROJECT_NAME: ${{ secrets.CLOUDFLARE_PROJECT_NAME }}
```

演示时可以二选一：

- `Pages GitHub integration`：最省事，适合现场演示。GitHub PR 和 main branch 自动部署，Omega 只需要展示部署链接进入 proof。
- `Wrangler from GitHub Actions`：更工程化，适合展示 CI/CD 可审计链路。Actions 日志能说明 test、build、deploy 的每一步。

### Omega 启动

开发演示：

```bash
npm run local-runtime:dev
npm run web:dev
npm run desktop:dev
```

打包演示：

1. 从 GitHub Release 下载 `Omega-<version>-arm64.dmg`。
2. 拖入 Applications。
3. 首次打开 Omega。
4. 如为内部未签名包，演示前在本机处理 Gatekeeper，不要现场处理。

### Omega 配置

进入 `Settings`，演示或提前完成：

- Workspace root：使用演示专用目录，例如 `~/Omega/demo-workspaces`。
- GitHub：确认 `gh` 登录态或 OAuth 可用。
- Agent Access：Codex 作为默认 runner；opencode / Trae Agent 作为可配置 runner 展示。
- Model：选择当前真实可用模型，不要展示静态假列表。
- Feishu：如果要展示审批通知，确认至少一条可用投递路由。
- Agent Studio：导入 sample template 或 repo `.omega` 模板。

## 3. 两套编排 Workflow Template

演示时建议在 Pipeline Templates 区域展示两套真实 workflow-backed 编排模板，而不是只展示静态卡片：

### Template 1：DevFlow PR cycle

文件：`services/local-runtime/workflows/devflow-pr.md`

定位：默认完整工程交付闭环。

适合展示：

- 通用 Requirement -> PR 交付。
- 两轮 Code Review。
- Rework 回流。
- Human Review approve / request changes。
- Merge / Done / Proof。

### Template 2：SaaS Launch Flow

文件：`services/local-runtime/workflows/saas-launch.md`

定位：演示专用 SaaS 功能发布编排。它复用同一套 DevFlow executor，但阶段标题、说明和演示语义更贴合 SaaS 产品发布：

- Requirement shaping。
- Product implementation and CI。
- Product Review Round 1。
- Launch Readiness Review。
- Human Launch Review。
- Merge and Deploy。
- Launch Done。

适合展示：

- SaaS 产品需求的业务拆解。
- 前端功能、测试、CI、部署 readiness。
- 人类 launch gate 的说服力。
- 和 Page Pilot 视觉迭代进入同一条 Work Item / Pipeline / Proof 模型。

## 4. 两个演示 Requirement

### Requirement A：SaaS Revenue Command Center

用途：功能一主线，从复杂需求到 PR / CI / Review / Human Gate / Proof。这个 Requirement 有业务规则、前端状态、测试和部署要求，更能体现 Architect / Coding / Testing / Review 的分工。

建议标题：

```text
Build a revenue command center for customer risk and expansion
```

需求正文：

```text
We are building a B2B SaaS customer-success dashboard for account managers.

Add a "Revenue Command Center" to the dashboard that helps the team decide which accounts need action today.

Business context:
- Account managers need to see customer health, renewal risk, expansion potential, and owner accountability in one view.
- The demo should be frontend-only and deployable as a static SaaS app.
- Use local mock data, but structure it as if it could later come from an API.

Functional requirements:
- Add a summary section with these computed metrics:
  - Active customers.
  - At-risk customers.
  - Expansion-ready customers.
  - Average health score.
  - Revenue at risk.
- Add a risk queue that lists the highest-priority accounts first.
- Add filters for segment or status, such as Enterprise / Scale / Startup and Healthy / Watch / Risk.
- Add an account detail drawer or side panel that shows:
  - Health score breakdown.
  - Renewal date.
  - Owner.
  - Risk reason.
  - Recommended next action.
- Add an "Action playbook" area with at least three playbooks:
  - Renewal rescue.
  - Expansion motion.
  - Onboarding recovery.
- Keep the design restrained and SaaS-like: dense enough for operators, clear enough for demo.

Engineering requirements:
- Do not add a backend service.
- Keep the app deployable as a static site.
- Keep data and metric calculation in a separate module from UI rendering.
- Add or update tests for metric calculation and risk sorting.
- Add or preserve GitHub Actions CI that runs tests and production build on pull requests.
- Ensure the production build can be deployed to Cloudflare Pages or another static host.
- Add deployment notes to the PR body: Cloudflare project, build command, output directory, and expected preview / production URL.

Acceptance criteria:
- The dashboard shows the Revenue Command Center above the account table.
- Summary metrics are computed from mock data, not hard-coded directly in the markup.
- Risk queue ordering is deterministic and covered by tests.
- Filters update the visible account list without reloading the page.
- Account detail drawer/panel works from the account list.
- Layout is responsive at desktop and mobile widths.
- `npm test` passes.
- `npm run build` passes.
- GitHub Actions CI reports success on the PR.
- PR description summarizes changed files, validation commands, Cloudflare deployment readiness, and the expected deployment URL.
- After merge, the Cloudflare deployment URL is reachable and captured in Omega proof or the Human Review notes.
```

演示价值：

- Requirement Agent 把自然语言需求结构化。
- Architect Agent 生成 technical plan、functional todo、project todo。
- Coding Agent 修改真实 repo。
- Testing Agent 运行验证。
- Review Agent 生成风险判断和 review packet。
- Human Review 展示 todo completion、validation、PR、proof。
- Feishu approve 或 UI approve 进入 merge / done。
- Cloudflare deployment URL 作为发布 proof，证明交付不是只停在代码层。

### Requirement B：Page Pilot 试用转化页迭代

用途：功能二主线，在真实页面上圈选并修改前端。它不只是改一句文案，而是围绕 SaaS 试用转化页做多点视觉和交互调整。

建议标题：

```text
Improve trial onboarding conversion from the live preview
```

Page Pilot 批注建议：

```text
#1 hero title
Change the headline to "Turn customer risk into revenue action".

#2 primary CTA
Rename the button to "Start risk review" and make it the clearest action on the screen.

#3 proof / trust strip
Add three compact trust signals near the hero:
- CI-validated changes.
- Human approval gate.
- Deploy-ready proof.

#4 metric cards
Add a "Revenue at risk" metric and keep all cards aligned.

#5 risk queue card
Make the highest-risk account visually stand out without using a destructive red-heavy layout.

Overall instruction:
Keep the SaaS dashboard style restrained and product-focused. Do not introduce decorative blobs or fake marketing sections. Update real source files, preserve responsive layout, and keep the app deployable through the existing CI/build flow.
```

演示价值：

- Page Pilot 不是截图标注；它圈选真实 DOM。
- 没有强 source mapping 时也能带 selector / DOM / text / style context 给 Agent。
- apply 后写真实源码，并刷新 preview。
- Confirm 后生成 Page Pilot Work Item / Pipeline / PR / Proof。
- Discard 是取消，不是 blocked。

## 5. 推荐演示顺序

总时长建议 18 到 25 分钟。

### 0. 开场，30 秒

讲解稿：

```text
今天我不演示一个单点 AI coding 工具，而是演示一个 AI DevFlow。
Omega 关注的是从需求到交付的完整链路：需求怎么变成任务，Agent 怎么分工，代码怎么进 PR，review 和人工审批怎么留下证据。
```

### 1. 安装与首次启动，2 分钟

动作：

1. 展示 GitHub Release 下载的 `Omega.dmg`。
2. 打开 Omega。
3. 说明本地 runtime 和 UI 都在本机启动。

讲解稿：

```text
Omega 是 local-first。代码修改、runner 执行、SQLite 状态和 proof 默认都在本机，远端 GitHub 和飞书是协作与审批入口。
这意味着它不会把一个未知需求直接丢到云端 runner，也不会误写没有绑定的仓库。
```

可略过细节：

- Apple 签名 / notarization。
- 发布流程命令。

### 2. 基础配置，3 分钟

动作：

1. 打开 Settings。
2. 展示 GitHub、Agent Access、Workspace root。
3. 展示 Agent Studio 的 workflow / prompt / skills / MCP。
4. 说明 Codex / Claude 继承本机 CLI，opencode / Trae 可配置 provider/model/base URL/API key。

讲解稿：

```text
这里不是配置一个聊天模型，而是在配置交付能力。
每个阶段可以有自己的 Agent、runner、model、skills 和 MCP。
运行时 Omega 会把这些配置物化到隔离 workspace，而不是只在 UI 上显示。
```

重点展示：

- Repository Workspace 是执行边界。
- runner model 候选来自真实 discovery 或用户输入。
- Stage 级 Skills / MCP 会写入 runner workspace。

### 3. 创建 Project 和 Repository Workspace，2 分钟

动作：

1. 进入 Projects。
2. 创建或打开 demo Project。
3. 绑定 GitHub repo 或本地 repo。
4. 进入该 Repository Workspace。

讲解稿：

```text
Project 是产品目标，Repository Workspace 才是代码执行边界。
后面所有 Work Item、Agent、Page Pilot 都必须锁定这个 repo，避免 AI 写到 Omega 自己或其他目录。
```

### 4. 功能一：从 Requirement 到 Work Item，3 分钟

动作：

1. 进入 Workboard。
2. 新建 Requirement，粘贴 Requirement A。
3. 展示 Work Item 进入 Not Started。
4. 启动 DevFlow 或打开 Auto run。

讲解稿：

```text
现在我们输入的是一个产品需求，不是一个代码补丁。
Omega 会把它变成 Work Item，并用 workflow contract 决定后续阶段。
```

要点：

- Not Started / Running / Human Review / Blocked / Done。
- Auto run 可覆盖 GitHub ready issue 和 Omega 内部 Not Started Work Item。
- 不要只展示 loading，及时进入详情页看 pipeline。

### 5. 功能一：Run Workpad 和阶段详情，4 分钟

动作：

1. 打开 Work Item 详情页。
2. 展示 Plan、Acceptance Criteria、Validation、Review Packet、Blockers、Retry Reason、Notes、PR。
3. 展开每个阶段的 Agent 信息。
4. 展示 Architect 产出的 functional todo / project todo。
5. 展示 Review / Human Review 中 todo completion 的打勾结果。

讲解稿：

```text
这里是 Omega 和普通任务看板最大的区别。
详情页不是只显示一个状态，而是显示这次交付的过程证据：谁计划、谁编码、谁测试、谁 review、耗时多久、产物是什么。
Plan 里的 todo 会在测试和 review 阶段被复核，Human Review 时可以看到最终完成度。
```

重点：

- Agent 统计不只是数量，要解释阶段角色。
- Risk 必须有依据，high risk 需要真实 blocking signal。
- Review Packet 是给人看的，不是给机器看的。

### 6. 功能一：PR、CI、Review、Human Gate，4 分钟

动作：

1. 打开 PR 链接。
2. 展示 changed files / checks。
3. 回到 Omega 展示 Review Packet。
4. 展示 Feishu 通知或 UI Human Review。
5. Approve。
6. 展示进入 Merging / Done 和 proof。

讲解稿：

```text
Omega 不绕过 GitHub，也不绕过人工。
Agent 可以完成编码、测试和 review，但 merge 前必须经过明确的 checkpoint decision path。
飞书 approve、UI approve、CLI approve 走的是同一条后端决策链路。
```

如果现场不方便 merge：

- 停在 Human Review，说明 merge 可以由 approve 后触发。
- 展示已有 Done Work Item 的 proof。

### 7. 功能二：Page Pilot 圈选真实页面，5 分钟

动作：

1. 进入 Page Pilot。
2. 选择同一个 Repository Workspace。
3. 使用 `Dev server by Agent` 或 `HTML file` 打开 demo SaaS 页面。
4. 点击右下角 Page Pilot 手指按钮。
5. 圈选 hero title、CTA、metric card。
6. 提交 Requirement B 批注。
7. 展示执行状态、changed files、预览刷新。
8. Confirm 生成 PR；或者先展示 Discard 不会变成 Blocked。

讲解稿：

```text
功能二解决的是另一个很真实的问题：很多产品修改不是从 issue 开始，而是从“这个页面这里不对”开始。
Page Pilot 让用户直接在正在构建的软件上圈选真实 DOM，Omega 把这个视觉意图转成源码修改，再进入同一套 Work Item、Pipeline、PR 和 proof。
```

强调：

- 它不是修改 Omega 的 UI，而是修改目标 repo 的 preview。
- package.json 项目会通过 Preview Runtime Agent 启动，不伪造默认 URL。
- Confirm / Discard 都会物化成可追溯记录。

### 8. CLI 和 Observability 彩蛋，1 到 2 分钟

动作：

```bash
omega status
omega work-items list
omega operations list --work-item <id>
omega workpads show <id>
```

打开 Views：

- Delivery overview。
- Runtime health。
- Delivery movement。
- Recent failures。

讲解稿：

```text
UI、CLI 和后台 runtime 读的是同一套本地 API。
所以它既能做产品演示，也能被本地 operator 用命令行排查。
```

## 6. 两个 Requirement 的现场复制版

### 快速复制版：Requirement A

```text
Build a revenue command center for customer risk and expansion.

We are building a B2B SaaS customer-success dashboard for account managers.

Add a Revenue Command Center that helps the team decide which accounts need action today.

Business context:
- Account managers need customer health, renewal risk, expansion potential, and owner accountability in one view.
- The demo should be frontend-only and deployable as a static SaaS app.
- Use local mock data, but structure it as if it could later come from an API.

Functional requirements:
- Add computed metrics: active customers, at-risk customers, expansion-ready customers, average health score, and revenue at risk.
- Add a risk queue sorted by highest-priority accounts first.
- Add filters for segment/status, such as Enterprise / Scale / Startup and Healthy / Watch / Risk.
- Add an account detail drawer or side panel with health breakdown, renewal date, owner, risk reason, and recommended next action.
- Add three action playbooks: Renewal rescue, Expansion motion, and Onboarding recovery.
- Keep the design restrained and SaaS-like.

Engineering requirements:
- Do not add a backend service.
- Keep the app deployable as a static site.
- Keep data and metric calculation in a separate module from UI rendering.
- Add or update tests for metric calculation and risk sorting.
- Add or preserve GitHub Actions CI that runs tests and production build on pull requests.

Acceptance criteria:
- The dashboard shows the Revenue Command Center above the account table.
- Summary metrics are computed from mock data, not hard-coded directly in the markup.
- Risk queue ordering is deterministic and covered by tests.
- Filters update the visible account list without reloading the page.
- Account detail drawer/panel works from the account list.
- Layout is responsive at desktop and mobile widths.
- npm test passes.
- npm run build passes.
- GitHub Actions CI reports success on the PR.
- PR description summarizes changed files, validation commands, and deployment readiness.
```

### 快速复制版：Requirement B

```text
Improve trial onboarding conversion from the live preview.

Selected changes:
- Hero headline: "Turn customer risk into revenue action".
- Primary CTA: "Start risk review".
- Add a compact trust strip: CI-validated changes, Human approval gate, Deploy-ready proof.
- Add a Revenue at risk metric and keep all metric cards aligned.
- Make the highest-risk account stand out without using a destructive red-heavy layout.

Overall:
Keep the SaaS style restrained and product-focused. Do not add decorative blobs or fake marketing sections. Update real source files, preserve responsive layout, and keep the app deployable through the existing CI/build flow.
```

## 7. 现场风险和兜底

### Runner 太慢

兜底：

- 准备一个已经跑到 Human Review 的 Work Item。
- 准备一个已经 Done 的 Work Item。
- 现场只跑 Page Pilot 的小改动，DevFlow 主链路用已有记录讲解。

讲法：

```text
真实 Agent 执行会受模型、网络、CI 速度影响。Omega 的重点是把慢过程沉淀成可追踪状态，而不是假装瞬间完成。
```

### GitHub Actions 慢或失败

兜底：

- 展示 checks proof 和 failed check log。
- 说明失败会进入 Rework 或 Human Review。
- 不把失败解释成 bug，而是解释成 delivery gate 生效。

### Feishu 不稳定

兜底：

- 使用 UI approve。
- 使用 CLI checkpoint approve。
- 说明三者进入同一后端 checkpoint decision path。

### Page Pilot 没有产生 diff

兜底：

- 选择文本更明确的目标元素，例如 hero title / button / label。
- 使用带明确文件路径的整体说明。
- 准备一条已成功 Page Pilot run 展示 details / proof。

### 预览服务启动失败

兜底：

- 使用纯静态 HTML file 模式。
- 或提前启动 `npm run dev`，Page Pilot 选择已有 preview URL。

## 8. 结尾讲解

```text
Omega 不是替代 GitHub、飞书或本地 IDE。
它把这些碎片串成一条可审计的 AI DevFlow：需求有来源，执行有边界，Agent 有分工，代码有 PR，review 有依据，人工审批有记录，最终交付有 proof。

功能一展示从需求到交付的完整工程闭环。
功能二展示从页面视觉反馈到真实源码修改的入口。
这两条线最后汇入同一个 Work Item / Pipeline / Proof 模型。
```

## 9. 演示检查清单

演示前 30 分钟：

- [ ] `gh auth status` 正常。
- [ ] demo repo 默认分支干净。
- [ ] `npm run build` 或测试命令在 demo repo 可通过。
- [ ] Omega 能打开目标 Repository Workspace。
- [ ] Agent runner 至少一个可用。
- [ ] Page Pilot 可打开 demo 页面。
- [ ] 准备一个 Not Started Work Item。
- [ ] 准备一个 Human Review Work Item。
- [ ] 准备一个 Done Work Item。
- [ ] 确认 Pipeline Templates 中能看到 DevFlow PR cycle 和 SaaS Launch Flow。
- [ ] 准备 Requirement A / B 复制文本。
- [ ] 如果展示 Feishu，提前发一条测试消息确认投递路线。
- [ ] 关闭无关通知和敏感窗口。

## 10. 推荐镜头顺序

1. Release / desktop app 启动。
2. Settings：GitHub / Agent / Workspace root。
3. Projects：绑定 repo。
4. Workboard：创建 Requirement A。
5. Work Item detail：Plan / TODO / Agent stats / Review Packet。
6. GitHub PR：diff / checks。
7. Human Review：approve / request changes。
8. Done：proof。
9. Page Pilot：Requirement B 圈选并 apply。
10. Confirm：PR / Work Item / Proof。
11. Views / CLI：observability 和 operator 能力。
