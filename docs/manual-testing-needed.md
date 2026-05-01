# 需要手动验证的场景

这个文档只记录自动化测试覆盖不到、需要在 Electron / 真实目标页面里确认的内容。完成后可以在对应条目后补充结果和问题。

## 2026-05-02 Repository-first 审计 API

本轮新增的 `repository_targets`、`handoff_bundles`、`operation_queue` 和 proof preview 基础 API 已用 Go / 前端 API client 自动化测试覆盖，当前不需要额外人工手测。

后续 shared sync、多端协作授权、App sync loop 和代码库语义索引需要先完成产品方案与远端环境，再补单独手测清单。

## 2026-04-30 Page Pilot 桌面端链路

### 需要验证

- Electron 启动后进入 Page Pilot，选择 Repository Workspace。
- 使用 `Repository source` 打开目标页面，确认不是空白页，也不是嵌在 Omega 内的小预览。
- 在目标页面内圈选至少 3 个真实 DOM 元素，提交一次包含多批注的修改。
- Apply 后目标页面刷新，能看到代码改动结果。
- 回到 Page Pilot 启动器后，Recent runs 能看到刚才的 run。
- Recent runs 的 Work Item 链接能打开对应 Work Item 详情页。
- Confirm 后按钮状态不再允许重复 Confirm / Discard。
- Discard 后代码恢复，Recent runs 和 Work Item 状态能看到 discarded / blocked 结果。
- 刷新目标页或回到 Page Pilot 启动器后，Recent runs / Work Item 中仍能回溯本次批注轮次、主目标和过程事件。
- Dev server by Agent 模式完成一次 Apply 后，run 记录中应能回溯本次 Preview Runtime Profile：preview URL、启动命令、reload strategy 和 health check。
- 混合选择带 `data-omega-source` 和 DOM-only 的元素后，run 记录中的 source mapping 覆盖率是否符合实际批注情况。
- DOM-only 元素提交后，run 记录中的 `sourceLocator` 是否给出合理候选文件；如果页面文案在源码中存在，Agent 是否优先修改候选文件。
- 同一 Page Pilot run 在 `applied` 后继续追加一轮批注并 Apply，Recent runs 详情中应显示 `roundNumber` 增长，且仍回跳同一个 Work Item。
- Recent runs 详情弹窗应能看到 PR preview、diff summary、source mapping、visual proof、preview runtime 和 conversation。
- 使用 Go Preview Runtime API 启动目标项目后，profile / pid / stdout / stderr / health check 信息应与实际目标项目一致。
- 功能一 Human Review 点 Request changes 后，Work Item 详情应展示 rework assessment / checklist，并看到新的 rework Attempt 或回流状态。

### 观察重点

- 目标页面上的 Page Pilot 控件是否遮挡业务页面关键内容。
- 返回 Omega 的入口是否容易找到，但不影响观察目标页面。
- Dev server by Agent 启动慢时是否有明确状态反馈。
- Page Pilot live-preview 写锁冲突时，错误信息是否能让用户理解“已有 run 正在持有预览工作区”。
- 服务端 run conversation 是否和目标页浮层显示一致，尤其是多批注提交后的主目标、批注数量和 Confirm / Discard 终态。
- Preview Runtime Profile 是否对应本次实际打开的目标项目，而不是上一次选择的仓库或旧 URL。
- GitHub delivery preflight 如果权限不足，应在运行前失败，而不是等到 PR 创建、Human Review approve 或 merge 时才失败。

### 自动化已覆盖

```bash
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx --testTimeout=15000
npm run lint
go test ./services/local-runtime/internal/omegalocal
```

## 2026-05-01 飞书 Human Review 审核链路

### 需要人工配置

- [ ] 配置 `OMEGA_FEISHU_WEBHOOK_URL`，指向真实飞书机器人 webhook。
- [ ] 如机器人启用了签名，配置 `OMEGA_FEISHU_WEBHOOK_SECRET`。
- [ ] 如需从飞书按钮直接回调 Omega，配置飞书可访问的 `OMEGA_PUBLIC_API_URL`；如果只发通知或通过 `Open review` 回到 Omega Web，则不需要公网。
- [ ] 如需卡片打开 Omega Web，配置 `OMEGA_PUBLIC_APP_URL`。
- [ ] 建议配置 `OMEGA_FEISHU_REVIEW_TOKEN`，并在飞书回调中带上同一个 token。
- [ ] 如果不使用 webhook，登录 `lark-cli`，然后配置 `OMEGA_FEISHU_REVIEW_CHAT_ID`。
- [ ] 如使用无公网 Task 审核，登录 `lark-cli`，配置 `OMEGA_FEISHU_REVIEW_MODE=task` 和 `OMEGA_FEISHU_REVIEW_ASSIGNEE_ID`。
- [ ] 如需长 review 详情进入飞书文档，配置 `OMEGA_FEISHU_REVIEW_CREATE_DOC=true`；如需写入指定目录，再配置 `OMEGA_FEISHU_REVIEW_DOC_FOLDER_TOKEN`。

当前本机检查：已安装 `lark-cli version 1.0.23`，并确认支持 interactive card、task create/comment/get 和 docs create。真实飞书发送还需要用户登录 / profile、bot 权限和真实 assignee / chat id。

### 需要验证

- [ ] 触发一个 DevFlow PR cycle，等待进入 Human Review。
- [ ] 飞书群收到 Omega Human Review 卡片；webhook 和 `lark-cli` chatId 两种路径至少验证一种。
- [ ] 卡片包含 Work Item、需求摘要、PR、风险等级和 Review Packet 摘要。
- [ ] 长需求不会把卡片撑爆；长内容以 review doc preview / 后续文档入口承载。
- [ ] 点击 `Open review` 能打开 Omega 对应 Work Item 页面。
- [ ] 如果配置了公网 callback，飞书侧 `Approve` 走 `/feishu/review-callback` 后，Omega checkpoint 变为 approved，并继续 merging。
- [ ] 如果配置了公网 callback，飞书侧 `Request changes` 走 `/feishu/review-callback` 后，Omega checkpoint 变为 rejected，并生成 rework attempt / checklist。
- [ ] Task 模式下，Human Review 后飞书里出现一条审核任务，任务描述包含 Work Item、PR、branch、需求摘要和 review token。
- [ ] Task 模式下，完成任务后调用 `/feishu/review-task/sync`，Omega checkpoint 变为 approved，并继续 merging。
- [ ] Task 模式下，调用 `/feishu/review-task/bridge/tick` 的 `dryRun=true` 能看到待同步 taskGuid；启用 `OMEGA_FEISHU_TASK_BRIDGE_ENABLED=true` 后，JobSupervisor tick 能自动同步已完成任务。
- [ ] Task 模式下，在任务评论里写明确修改意见并转发到 `/feishu/review-task/comment`，Omega checkpoint 变为 rejected，并生成 rework attempt / checklist。
- [ ] Task 模式下，任务评论只是问题 / 缺少信息时，Omega checkpoint 保持 pending，并在 checkpoint `feishuReview.lastComment` 记录 need-info。
- [ ] Omega Web 本地 Approve / Request changes 和飞书侧动作结果一致，不出现两套不同状态。

### 自动化已覆盖

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuReviewRequestCreatesTaskReviewWithStrongBinding|TestFeishuReviewTaskSyncApprovesCompletedTask|TestFeishuReviewTaskBridgeDryRunListsPendingTasks|TestFeishuReviewTaskCommentRequestsChanges|TestFeishuReviewTaskCommentNeedInfoRecordsOnly|TestFeishuReviewRequestSendsInteractiveWebhookCard|TestFeishuReviewRequestUsesLarkCLIInteractiveCard|TestFeishuReviewCallbackApprovesCheckpointThroughSharedDecisionPath|TestFeishuNotifyUsesLocalLarkCLI'
```
