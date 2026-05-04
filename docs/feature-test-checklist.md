# Omega 功能测试清单

更新时间：2026-05-01

这份文档用于手动功能验收。建议每次测试前先跑自动化回归，再按下面的功能一、功能二清单逐项确认。测试结果可以直接写在每个用例的“记录”下方。

## 0. 测试前准备

### 环境准备

- [ ] 本机已安装并可运行 `git`。
- [ ] 本机已安装并登录 `gh`，执行 `gh auth status` 正常。
- [ ] 至少有一个可写的测试仓库，且当前 GitHub 用户对该仓库有 branch / PR / checks 读取权限。
- [ ] 测试仓库可以安全创建分支、commit、PR。
- [ ] Codex / opencode / Claude Code 至少有一个 runner 可用；如果只验证 deterministic 流程，可先使用本地兜底 runner。
- [ ] 目标仓库已绑定为 Omega Repository Workspace。

### 自动化回归

```bash
npm run test:feature-p0
```

通过标准：

- [ ] `npm run lint` 通过。
- [ ] Page Pilot / Work Item 关键前端测试通过。
- [ ] desktop preload 语法检查通过。
- [ ] `go test ./services/local-runtime/internal/omegalocal` 通过。

记录：

```text
测试时间：
执行人：
命令结果：
异常：
```

## 1. 启动 Omega

### 方式 A：Web + Go runtime

终端 1：

```bash
npm run local-runtime:dev
```

终端 2：

```bash
npm run dev
```

浏览器打开：

```text
http://127.0.0.1:5173
```

通过标准：

- [ ] 首页可打开。
- [ ] Work items 页面可打开。
- [ ] 当前 Repository Workspace 可见。
- [ ] 左侧 Page Pilot 入口可见。

### 方式 B：Electron 桌面壳

```bash
npm run desktop
```

通过标准：

- [ ] Electron 窗口打开 Omega 首页或 Workboard。
- [ ] Page Pilot 页面可进入。
- [ ] Electron 中可刷新 Omega App。
- [ ] 没有空白页或无法返回的状态。

记录：

```text
启动方式：
Omega URL：
runtime URL：
异常：
```

## 2. 功能一：Requirement -> Work Item -> DevFlow

### 2.1 新建需求并绑定仓库

步骤：

1. 进入 Work items。
2. 点击 `New requirement`。
3. 输入一个小需求，例如“把首页按钮文案改为测试文案”。
4. 确认 Repository Workspace 为测试仓库。
5. 创建 Work Item。

通过标准：

- [ ] Work Item 出现在 Not started / Ready 区域。
- [ ] Work Item 行内显示正确的 repository target。
- [ ] 详情页中 Requirement source 可读，长文本不影响页面主布局。
- [ ] Delivery flow 和 Run Workpad 能正常显示初始状态。

记录：

```text
Work Item key：
Repository target：
问题：
```

### 2.2 运行 DevFlow

步骤：

1. 在 Work Item 行点击 `Run`。
2. 进入详情页观察状态。
3. 等待 pipeline 进入执行状态。

通过标准：

- [ ] 运行前会做 preflight；缺少 repo / runner / GitHub 权限时应明确失败。
- [ ] Running 状态出现，行内进度条可见。
- [ ] Attempt 创建成功。
- [ ] Run Timeline 默认折叠，展开后有真实事件。
- [ ] Agent operations 默认紧凑，不把详情页撑得很长。
- [ ] Run Workpad 展示 Plan / Acceptance Criteria / Validation / PR / Review Feedback / Retry Reason 等摘要。

记录：

```text
Attempt id：
当前 stage：
preflight 结果：
异常：
```

### 2.3 Human Review approve

步骤：

1. 等待 Work Item 进入 Human Review。
2. 点击 Human Review 状态入口。
3. 查看 PR、Changed、Validation、Artifacts。
4. 点击 `Approve delivery`。

通过标准：

- [ ] Human Review 入口明显且可点击。
- [ ] 详情页不是只跳到普通 item 详情，而是能看到 Review 操作区。
- [ ] 点击 approve 后 UI 立即反馈，不能长时间像无响应。
- [ ] Pipeline 进入 Merging。
- [ ] merge job 单独记录输出。
- [ ] 成功后进入 Done，并生成 proof。
- [ ] 如果 PR 合并失败，错误显示在 Merging / Retry context，而不是只弹 API failed。

记录：

```text
PR URL：
Approve 点击时间：
UI 首次反馈耗时：
最终状态：
异常：
```

### 2.4 Human Review request changes

步骤：

1. 在 Human Review 评论框输入明确修改意见。
2. 点击 `Request changes`。
3. 观察详情页和 Workboard 列表。

通过标准：

- [ ] 提交后能看到 rejected / rework 相关状态。
- [ ] Run Workpad 出现 Rework Assessment / Review Feedback / Retry Reason。
- [ ] Rework Checklist 有明确来源和动作。
- [ ] 新的 rework Attempt 或回流状态可见。
- [ ] Rework 基于上一轮 workspace / branch / PR 继续，而不是重新从空白开始。
- [ ] PR description 在需要时更新人工意见和本轮增量 diff 摘要。

记录：

```text
人工意见：
新 Attempt id：
Rework 策略：
是否复用 PR：
异常：
```

### 2.5 Retry / stalled / failed

步骤：

1. 找一个 failed / stalled Attempt。
2. 点击 Retry attempt。
3. 观察 Retry context。

通过标准：

- [ ] Retry 前能看到真正原因，不只看到底层 stderr。
- [ ] Retry Reason 来自 rework checklist / review feedback / failed check / human feedback。
- [ ] 新 Attempt 保留 retryOfAttemptId / retryRootAttemptId / retryIndex。
- [ ] 旧 Attempt 不被覆盖。
- [ ] Workboard 状态进入 Running 或 Blocked，和详情页一致。

记录：

```text
旧 Attempt：
新 Attempt：
Retry reason：
异常：
```

## 3. 功能二：Page Pilot 页面编辑模式

### 3.1 进入 Page Pilot 并选择仓库

步骤：

1. 从左侧导航进入 Page Pilot。
2. 或从 Page Pilot 物化出来的 Work Item 详情页点击 `Open in Page Pilot`。
3. 选择明确 Repository Workspace。

通过标准：

- [ ] Page Pilot 不使用隐式默认仓库。
- [ ] 从 Work Item 进入时默认选中该 Work Item 的 repository target。
- [ ] 页面显示当前 target repo。
- [ ] 没有空白页。

记录：

```text
进入方式：
Repository target：
异常：
```

### 3.2 打开目标预览

测试三种预览来源，至少选择一种完整跑通。

#### Dev server by Agent

步骤：

1. 选择 Dev server by Agent。
2. 点击打开预览。
3. 等待 Preview Runtime Agent 启动目标服务。

通过标准：

- [ ] UI 显示启动中 / 成功 / 失败状态。
- [ ] 目标项目在完整页面中打开，不是嵌在 Omega 小预览里。
- [ ] Preview Runtime Profile 记录 preview URL、dev command、working directory、health check、reload strategy。
- [ ] 失败时显示原因，例如缺少 script、端口不可用、health check 失败。

#### Manual URL

步骤：

1. 手动启动目标项目 dev server。
2. 输入 `http://127.0.0.1:<port>/`。
3. 点击打开预览。

通过标准：

- [ ] 无协议 localhost / 127.0.0.1 会被规范为 `http://...`。
- [ ] 目标页面打开成功。
- [ ] Electron 返回 / 刷新不影响目标页面主布局。

#### HTML file / Repository source

步骤：

1. 选择目标 HTML 或 repository source。
2. 打开预览。

通过标准：

- [ ] 静态 HTML 可以打开。
- [ ] 如果项目必须依赖 dev server，UI 能提示需要 dev server。

记录：

```text
预览来源：
preview URL：
dev command：
异常：
```

### 3.3 圈选 3 个元素并提交修改

步骤：

1. 在目标页面点击 Page Pilot 浮层入口。
2. 圈选至少 3 个真实 DOM 元素，例如标题、按钮、卡片文案。
3. 为每个元素添加批注。
4. 在整体说明中写清楚要改什么。
5. 点击 Apply。

通过标准：

- [ ] 高亮命中真实目标元素，不选中 Omega chrome。
- [ ] 支持多个批注。
- [ ] primary target 符合最后一个或最明确的目标。
- [ ] 提交后显示过程状态，而不是卡住无反馈。
- [ ] Agent 只修改绑定 repository workspace 中的文件。
- [ ] Apply 完成后目标页面刷新，并能看到修改结果。
- [ ] run 记录中有 changed files、diff summary、source mapping report、source locator 或明确 source mapping。

记录：

```text
选择元素数量：
整体说明：
changed files：
source mapping 状态：
异常：
```

### 3.4 查看 Page Pilot 结果面板

步骤：

1. 回到 Page Pilot 启动器。
2. 查看 Recent runs。
3. 点击 Details。

通过标准：

- [ ] Recent runs 只显示当前 repository target 相关记录。
- [ ] Details 弹窗打开，不直接撑开主页面。
- [ ] 能看到 PR preview。
- [ ] 能看到 diff summary。
- [ ] 能看到 visual proof / DOM snapshot。
- [ ] 能看到 preview runtime profile。
- [ ] 能看到 conversation / 批注轮次。
- [ ] Work Item 链接能跳到对应详情页。

记录：

```text
Run id：
roundNumber：
Work Item：
visual proof：
异常：
```

### 3.5 同一 run 多轮修改

步骤：

1. 在第一次 Apply 成功后，不 Confirm / Discard。
2. 继续在目标页面追加一轮批注。
3. 再次 Apply。
4. 回到 Recent runs Details。

通过标准：

- [ ] 复用同一个 runId。
- [ ] roundNumber 递增。
- [ ] 仍关联同一个 Work Item / Pipeline。
- [ ] 第二轮 diff 是基于第一轮结果继续修改。
- [ ] 目标页面刷新后显示第二轮结果。

记录：

```text
第一轮 runId：
第二轮 runId：
roundNumber：
异常：
```

### 3.6 Confirm delivery

步骤：

1. 在目标页面结果面板点击 Confirm。
2. 等待 delivery 完成。
3. 回到 Page Pilot / Work Item 查看记录。

通过标准：

- [ ] Confirm 后不允许重复 Confirm / Discard。
- [ ] 创建 branch / commit。
- [ ] GitHub target 会创建 PR。
- [ ] PR body 有语义化摘要、changed files、DOM/source context、visual proof 基础信息。
- [ ] Work Item / Pipeline 进入 delivered / done 或等待后续 review 的真实状态。
- [ ] Recent runs 显示 delivered。

记录：

```text
Branch：
Commit：
PR URL：
最终状态：
异常：
```

### 3.7 Discard

步骤：

1. 新开一轮 Page Pilot Apply。
2. 点击 Discard。
3. 刷新目标页面并检查源码。

通过标准：

- [ ] Discard 撤销本轮 changed files。
- [ ] Confirm / Discard 终态后不可重复点击。
- [ ] live-preview lock 释放。
- [ ] Recent runs 显示 discarded。
- [ ] Work Item 状态不应误显示 Done；应显示 blocked / discarded 或对应真实状态。

记录：

```text
Run id：
撤销文件：
最终状态：
异常：
```

## 4. Page Pilot 并发与锁

步骤：

1. 在同一个 repository target 上启动一个 Page Pilot run，并停在 applied 未 Confirm / Discard。
2. 再开启第二个 Page Pilot run，尝试 Apply。

通过标准：

- [ ] 第二个 run 被拒绝或提示已有 live-preview lock。
- [ ] 提示信息能说明哪个 run / workspace 正在持有锁。
- [ ] 第一个 run Confirm / Discard 后，锁释放。
- [ ] 锁释放后可以重新 Apply。

记录：

```text
第一个 run：
第二个 run：
锁提示：
异常：
```

## 5. 日志与可观测性

步骤：

1. 在功能一或功能二执行过程中打开 Work Item 详情。
2. 展开 Run Timeline。
3. 查看 runtime logs / attempt events / proof。

通过标准：

- [ ] 时间显示符合当前本地时区。
- [ ] INFO / DEBUG / ERROR 能区分。
- [ ] API 失败、runner 失败、GitHub 失败、preview runtime 失败都能在 timeline 中找到原因。
- [ ] 长日志默认不淹没页面，详情可展开查看。

记录：

```text
Work Item：
Attempt：
关键错误：
是否能定位原因：
```

## 6. 测试结论模板

```text
测试日期：
测试人：
Git branch：
Commit：
测试仓库：
Omega 启动方式：
目标项目启动方式：

自动化回归：
- npm run test:feature-p0：

功能一结论：
- 新建需求：
- DevFlow run：
- Human Review approve：
- Human Review request changes：
- Retry：

功能二结论：
- 进入 Page Pilot：
- 打开预览：
- 圈选 3 个元素：
- Apply：
- 多轮 apply：
- Details：
- Confirm：
- Discard：
- lock：

阻塞问题：
1.
2.
3.

非阻塞体验问题：
1.
2.
3.

是否通过本轮验收：
```

## 7. 常见失败判断

- 如果 Open preview 没反应：先看 Page Pilot 页面是否显示 IPC / runtime 错误，再确认目标服务端口是否启动。
- 如果 Apply 报 workspace 错误：检查 repository target 是否有明确 local path，或 GitHub target 是否已经准备隔离 preview workspace。
- 如果修改落不到源码：查看 source mapping report；DOM-only 元素应有 source locator 候选。
- 如果 Confirm 后仍能点击 Confirm / Discard：这是终态按钮状态 bug，需要记录 runId 和截图。
- 如果 Request changes 后没有新 Attempt 或 rework 状态：这是功能一回流链路 bug，需要记录 checkpoint id、attempt id 和输入的人工意见。
- 如果 approve 很慢：记录点击时间、首次 UI 反馈时间、最终 merge 时间，并查看 Merging stage 日志。
