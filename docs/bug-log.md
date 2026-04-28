# Omega Bug Log

本文记录开发过程中遇到并修复的实现问题。产品功能记录继续写入 `docs/feature-implementation-log.md`；这里专门保留 bug、原因、修复和验证。

## 2026-04-28: Page Pilot pure SPA preview could not satisfy target-project selection

### 现象

功能二最初在 React SPA 中用 iframe 承载目标项目 preview，并把 Overlay 绑定到 iframe document。这个方案只能在 same-origin iframe 中工作；真实用户项目通常跑在另一个 localhost origin，浏览器同源策略会阻止 Omega 读取 DOM、注入圈选脚本和采集 selector/context。

更重要的是，Page Pilot 的产品语义不是圈选 Omega 自己的管理 UI，而是圈选用户正在构建的软件页面。纯 SPA iframe 只能验证管线，不能稳定满足赛题要求。

### 原因

- 浏览器同源策略限制跨 origin iframe DOM 访问。
- 用户项目 preview、Omega SPA、Go runtime 通常会运行在不同端口。
- 赛题需要“内部浏览器”式能力：打开目标项目页面、注入选择逻辑、刷新预览。

### 修复 / 架构选择

启用 Electron 作为桌面壳，但不替代现有 React SPA 和 Go runtime：

```text
Electron
  -> BrowserWindow loads React SPA
  -> BrowserView loads target project preview
  -> preview preload injects Page Pilot selection bridge
  -> Go runtime keeps SQLite / runner / git / PR execution
```

开发模式不需要打包：

```bash
npm run local-runtime:dev
npm run web:dev -- --host 127.0.0.1 --port 5174
npm run desktop:dev
```

### 验证

本次新增 Electron dev shell 和 preload bridge，并继续通过：

```bash
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx
go test ./services/local-runtime/...
npm run build
```

## 2026-04-28: Electron install exposed dependency and Node stdio typing issues

### 现象

安装 Electron dev shell 依赖后：

- `npm audit` 报 Electron 38 存在 high severity advisory。
- `npm run lint` 报旧本地 Node 脚本中的 `child.stdout` / `child.stderr` / `child.stdin` 可能为 `null`。

### 原因

- 初始 Electron 版本落在 npm audit 标记的 vulnerable range。
- Electron 带来的 Node 类型解析让 `ChildProcess` stdio nullable 类型更严格地进入当前 TypeScript build。

### 修复

- 升级 Electron 到 `^41.3.0`，`npm audit` 归零。
- 在本地 runner / sqlite / gh CLI helper 和相关测试中显式检查 stdio streams，不再假设 spawn 总是返回 pipe。

### 验证

```bash
npm audit --json
npm run lint
```

## 2026-04-28: Page Pilot overlay blocked element selection

### 现象

浏览器模式下打开 Page Pilot 后，完整浮窗会遮挡目标 preview，进入 Select 后仍然挡住用户要圈选的区域。hover 解析也不像检查器一样随鼠标持续更新。Apply / Confirm / Discard 的禁用条件虽然正确，但 UI 没解释启用条件，容易被误解为按钮不可用。

### 原因

- Select 模式复用了完整对话面板，没有切换成轻量 inspector control。
- hover 只监听 `pointerover`，对 iframe 内部连续移动反馈不够及时。
- 可选元素匹配范围偏窄，主要覆盖 Omega 自身组件类名，没有把目标项目常见 `.card` / `.hero` / `data-omega-source` 作为优先候选。
- 按钮禁用状态缺少 inline hint。

### 修复

- Select 模式下把浮窗移动到左下并压缩成 compact inspector，只显示 Cancel、Clear、当前 hover 元素类型、文本和 source mapping。
- 增加 `pointermove` 监听，实时更新当前鼠标指向元素和高亮框。
- 选择候选优先匹配 `[data-omega-source]`，并覆盖按钮、标题、正文、`.card`、`.hero`、article 等目标项目常见结构。
- Apply / Confirm / Discard 增加 title 和 inline 状态提示：Apply 需要 selection + instruction，Confirm / Discard 需要先 Apply 成功。

### 验证

```bash
npm run lint
```

## 2026-04-28: Page Pilot selected Omega chrome instead of target product

### 现象

用户在 Page Pilot 中打开目标产品后，页面显示 `Needs preview bridge`，但 Select 仍然允许启动。结果 hover 高亮看起来落在目标产品区域，解析出来的文本却是 Omega Page Pilot 自己的说明文案，体验不像之前的页面检查器。

### 原因

- Overlay 在 `targetDocument` 不可用时回退到了父页面 `document`，导致圈选对象变成 Omega chrome。
- 本地调试时用户可能用 `localhost:5174` 打开 Omega，却在 preview URL 中填 `127.0.0.1:5174/page-pilot-target/`。浏览器把这两个 host 当成不同 origin，iframe 不可 inspect。
- Page Pilot 仍嵌在 Workboard shell 中，左侧导航、顶栏和右侧 rail 抢占空间，没有形成“在 Omega 内直接打开用户产品”的工作区。

### 修复

- Select 模式只允许绑定到目标 preview document；如果 target iframe 不可 inspect，立即阻止选择，不再回退检查 Omega 页面。
- 对 `/page-pilot-target` dev proxy URL 做本地 host 归一：同端口的 `localhost` / `127.0.0.1` 会转成相对路径，保持 iframe 与 Omega 同源。
- Page Pilot nav 进入 immersive preview mode：隐藏 Workboard sidebar、topbar、inspector rail，让目标产品 iframe 占满 Omega 主工作区；只保留浮层 URL 控制和 Page Pilot overlay。

### 验证

```bash
npm run lint
```

## 2026-04-28: Chrome fallback could not match the Page Pilot interaction model

### 现象

用户期望的功能二体验是：Omega 像浏览器一样直接打开用户产品页面，例如 `http://127.0.0.1:5173/`，页面右下角有一个悬浮手指按钮；点击后进入选择模式，hover 时高亮当前元素，点击元素后在页面上弹出修改输入框。此前 Chrome / iframe fallback 虽能验证 API，但体验不像真正的页面内 Agent。

### 原因

- Chrome 普通网页不能让 Omega 稳定向任意目标 origin 注入控制层。
- iframe fallback 需要同源代理，容易把用户注意力放在 Workboard chrome 和 proxy URL 上。
- 赛题核心更接近“内置浏览器 + preload 注入”，Electron 天然能提供这个边界。

### 修复 / 架构选择

- 新增 `npm run desktop:pilot`。
- Electron 主窗口直接加载目标产品 URL，默认 `http://127.0.0.1:5173/`。
- `pilot-preload.cjs` 注入悬浮手指、hover 高亮、元素 tooltip、修改输入框，并直接调用 Go runtime 的 Page Pilot API。
- Chrome / iframe 路径保留为开发 fallback，不再作为主体验判断标准。

### 验证

```bash
node --check apps/desktop/src/pilot-main.cjs
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot exposed source mapping internals to users

### 现象

Electron Page Pilot tooltip 显示 `No data-omega-source`，让用户误以为没有 source mapping 的元素不能选择。输入 comment 后也会直接触发 apply，不适合同时选择多个元素并批量描述修改。

### 原因

- 第一版把 `data-omega-source` 当成用户可见状态，而不是内部强源码映射。
- Composer 的发送按钮直接调用 `/page-pilot/apply`，没有批注队列。
- 选择候选偏向按钮、标题、卡片，普通 DOM 元素虽然能被浏览器看到，但没有进入 Page Pilot 批注体验。

### 修复

- Tooltip 改为显示源码映射或 `DOM context captured`，不再向用户暴露 `No data-omega-source`。
- Electron direct pilot 增加批注队列：发送 comment 只添加批注和编号 pin，不立刻提交。
- 增加底部悬浮输入框，显示批注 chip 和整体补充说明输入区；用户点击发送后才统一调用 runtime。
- 选择候选扩展到 label、input、textarea、select、link 以及普通布局 DOM，未识别类型标为 `other`。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot batch annotations lacked a persistent prompt box

### 现象

Electron direct pilot 可以在元素上添加批注 pin，但批注完成后页面只显示“批注中 / 批注数量”状态，没有保留一个可继续输入整体需求的悬浮输入框。用户想一次圈选多个元素，再像聊天输入框一样补充整体修改意图，最后统一提交给 Agent。

### 原因

- 前一版把“单个元素 comment composer”和“批注队列状态”拆成两个阶段，但队列阶段只展示状态，没有继续输入框。
- 这会让用户误以为添加 comment 后已经进入提交流程，也无法对多个选区写统一需求。

### 修复

- 批注发送后只加入本地 annotation queue，并在选中元素附近插入编号 pin。
- 页面底部保留悬浮输入框，展示最近批注 chip、批注数量和整体补充说明 textarea。
- 用户可以继续点击右下角手指选择更多元素；只有点击底部输入框的发送按钮时，才把批注集合和整体说明提交给 `/page-pilot/apply`。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Full Go runtime test suite has watcher TempDir cleanup flake

### 现象

在验证 Page Pilot 单一 Agent 模式时，局部测试通过：

```bash
go test ./services/local-runtime/internal/omegalocal -run TestPagePilot
```

但两次运行全量 runtime 测试时，失败点出现在 watcher/orchestrator 相关测试的临时目录清理：

```text
TempDir RemoveAll cleanup: unlinkat ... directory not empty
```

两次失败分别落在：

- `TestOrchestratorTickCanClaimAndRunDevFlowCycle`
- `TestOrchestratorWatcherPersistsAndScansReadyIssues`

### 判断

这不是 Page Pilot 断言失败，而是 watcher/background process 或 goroutine 在测试结束时仍可能持有或写入临时目录，导致 Go test 的 `t.TempDir()` cleanup 失败。

### 后续修复方向

- 给 orchestrator watcher 测试增加显式 stop/cleanup。
- 确保 watcher goroutine 和外部 fake command 进程在测试返回前完全退出。
- 将 watcher 测试与长跑 DevFlow cycle 的临时 workspace 生命周期隔离。

## 2026-04-28: Page Pilot selector helper TypeScript inference failure

### 现象

新增 `PagePilotOverlay` 后，`npm run lint` 报错：

```text
PagePilotOverlay.tsx: parent implicitly has type any
PagePilotOverlay.tsx: child is of type unknown
```

### 原因

`selectorFor` 中遍历 DOM ancestor 时，`cursor` 会在循环内变化；TypeScript 对 `cursor.parentElement` 和 `Array.from(parent.children)` 的类型收窄不稳定，导致 `parent` 被推断成隐式 `any`，children 被推断为 `unknown`。

### 修复

- 显式声明 `parentElement: Element | null`。
- 保存 `currentTagName`，避免 filter callback 捕获变化中的 `cursor`。
- 给 `filter` callback 参数标注 `Element`。

### 验证

```bash
npm run lint
npm run test -- apps/web/src/__tests__/App.operatorView.test.tsx
```

## 2026-04-28: Page Pilot discard route parsed the wrong id

### 现象

新增 `POST /page-pilot/runs/{id}/discard` 后，Go 测试返回：

```text
Page Pilot run runs not found
```

### 原因

已有 `pathID` helper 只适用于 `/resource/{id}` 形态，会固定返回第三段路径。`/page-pilot/runs/{id}/discard` 的第三段是 `runs`，导致 runtime 用错误 id 查询记录。

### 修复

为 Page Pilot discard route 使用专用解析：

```text
strings.TrimSuffix(strings.TrimPrefix(path, "/page-pilot/runs/"), "/discard")
```

### 验证

```bash
go test ./services/local-runtime/internal/omegalocal -run TestPagePilot
```

## 2026-04-28: Page Pilot nav was missing from persisted Workboard session type

### 现象

把 Page Pilot 做成 Workboard 内独立 nav 后，`npm run lint` 报错：

```text
Type '"Page Pilot"' is not assignable to type 'PrimaryNavPersistence'
```

### 原因

`App.tsx` 的 `PrimaryNav` 已经加入 `Page Pilot`，但 `workspacePersistence.ts` 中用于 local/session 持久化的 `PrimaryNavPersistence` 仍只允许 `Projects | Views | Issues`。

### 修复

将 `PrimaryNavPersistence` 扩展为：

```text
Projects | Views | Issues | Page Pilot
```

### 验证

```bash
npm run lint
```

## 2026-04-28: Page Pilot multi-annotation apply used the wrong primary target

### 现象

Electron direct pilot 中同时选择多条批注后，用户把第三条 `login-submit` 登录按钮标注为“改成红蓝绿渐变”，但实际改到了页面顶部的两个 `.brand-mark` 图案。

### 原因

提交批注时，preload 代码使用第一条带 `sourceMapping.file` 的批注作为 `/page-pilot/apply` 的 `selection`：

```text
annotations.find(...)
```

多选场景下，用户最新选中的元素通常才是当前主目标。旧逻辑把第一条 `login-title` 当作主目标传给 Agent，后端 prompt 又强调优先使用 source mapping，导致 Agent 在错误源码区域附近落点。

### 修复

- Electron direct pilot 改为选择“最新一条带源码映射的批注”作为 primary target；如果没有源码映射，则回退到最新批注，让 Agent 走 DOM context / selector。
- 提交给 Agent 的 instruction 明确写入“主目标是第 N 条批注”。
- Go runtime 的 Page Pilot prompt 增加约束：`Selected element context` 是 primary target，多批注只作为辅助上下文。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
go test ./services/local-runtime/internal/omegalocal -run TestPagePilot
```

## 2026-04-28: Page Pilot submit kept stale annotation controls visible

### 现象

用户点击提交后，底部仍显示上一轮批注 chip 和输入框，看不到 Agent 正在做什么，也容易误以为旧批注还处于可编辑待提交状态。

### 原因

Electron direct pilot 只有 toast 状态提示，`/page-pilot/apply` 请求期间仍保持批注编辑 tray；apply 成功后虽然会 reload，但网络/runner 执行期间没有持久的过程信息面板。

### 修复

- 提交后立即把编辑 tray 切换为 Page Pilot process panel。
- process panel 展示本次提交的批注、primary target、Agent 提交流程、已修改文件、功能一 Work Item / Pipeline linkage 和预览刷新步骤。
- 批注历史默认折叠，只显示上一条；点击 `^` 展开全部，避免挡住目标页面。
- 成功、失败、Confirm、Discard 后都保留过程事件，用户能看到上一轮发生了什么。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot could miss small link/button targets

### 现象

Electron direct pilot 中，用户想圈选登录页里的 `忘记密码？`、`立即注册` 或某个具体按钮时，hover/点击有时命中父级行容器、卡片或其它带源码映射的祖先元素，而不是鼠标下的小型链接/按钮。

### 原因

旧逻辑基于 `event.target.closest(...)` 从当前事件目标向祖先查找候选元素。这个方式只看 DOM ancestor，不看鼠标坐标下的完整堆叠元素；当用户点在链接周边空白、文字行高区域、内部 span，或父级容器先命中时，小链接/按钮会被更大的祖先元素吞掉。`kindFor` 也没有把 `a`、`input`、`label` 独立分类，导致 tooltip 更像泛化的 `other` / `card-copy`。

### 修复

- 圈选候选从 `closest(...)` 改为 `document.elementsFromPoint(x, y)`。
- 按元素类型打分：`button/a/input/textarea/select/[role=button]` 优先，其次 `label`、`data-omega-source`、标题、文本、卡片、普通容器。
- `kindFor` 增加 `link`、`field`、`label` 类型。
- 分类不再作为可选门槛；所有可见 DOM 元素都可以被选中，未知类型保留为 `other` 并继续采集上下文。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot could miss dynamic status messages

### 现象

用户点击登录后，页面动态写入 `这是静态登录页，暂未接入 API。` 状态提示。该提示可见，但进入 Page Pilot 选择模式后不容易被高亮选中。

### 原因

这类动态提示通常是空 `div` 后续通过 JS 写入 `textContent`，没有 `data-omega-source`，也不是按钮、链接、标题或卡片。旧候选排序会把它当成普通 `div`，优先级低，容易被周围 form/card/container 抢走。

这不是因为 Page Pilot 缓存了旧 DOM。Electron preload 在每次 hover/click 时读取当前 live DOM；日夜模式切换、表单校验提示、hash 路由切换后的元素都应该按最新页面状态读取。问题在于动态状态元素没有被识别成高价值候选。

### 修复

- `role="status"`、`aria-live`、`.message`、`.alert`、`toast/notice/error/success` 类元素提升为高优先级候选。
- `elementKind` 增加 `status` 类型。
- 每次进入选择模式先清除旧 highlight，避免用户看到上一次 selection 的残影。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```

## 2026-04-28: Page Pilot composer badge always showed 1

### 现象

添加第二条批注时，底部 composer 左侧 badge 仍显示 `1`，导致用户无法确认当前输入的是第几条批注。

### 原因

`showComposer` 中 badge 文案写死为 `1`，没有读取当前批注队列长度。

### 修复

- 新批注 badge 使用 `annotations.length + 1`。
- 编辑已有批注时 badge 使用原批注 id。
- 页面 pin 和底部 chip 支持点击编辑 comment，不改变原顺序。
- 执行过程面板增加 loading spinner，批注折叠按钮改为明确文字。

### 验证

```bash
node --check apps/desktop/src/pilot-preload.cjs
```
