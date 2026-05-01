# 飞书通知 / 审核链路

## 目标

功能一的 Human Review 不只停留在 Omega 页面内，也能同步推送到飞书，让审核人从飞书看到需求、PR、风险和操作入口，并且飞书侧的审核动作和 Omega 本地审核动作走同一条 checkpoint 决策链路。

## 旧做法

- Go local runtime 只有 `POST /feishu/notify` 文本通知。
- 文本通知需要本机安装 `lark-cli`。
- Human Review checkpoint 只能在 Omega Web 内 Approve / Request changes。
- 飞书消息没有结构化 card、没有 callback、没有写回 checkpoint 的同步记录。

## 新做法

新增五条 API：

- `POST /feishu/review-request`
  - 输入 `checkpointId`，可选 `chatId`，也可用 `mode=task` 进入任务审核模式。
  - 读取 Work Item、Requirement、Attempt、Run Workpad、Review Packet。
  - Webhook / chatId 模式生成飞书 interactive card。
  - Task 模式通过 `lark-cli task +create` 创建一条绑定 checkpoint 的审核任务。
  - Task 模式可选创建飞书文档，把长需求、PR、Review Packet 和风险信息放入文档正文。
  - 没有 webhook 但有 `lark-cli + chatId` 时发送 interactive card；如果本地 CLI 不支持卡片参数，再退回文本 fallback。
  - 两者都没有时返回 `needs-configuration`，并把 card/doc preview 写入 checkpoint 的 `feishuReview`。
- `POST /feishu/review-callback`
  - 接收飞书侧 approve / request changes 动作。
  - 通过同一个 checkpoint decision helper 执行本地 approve / reject。
  - approve 后继续原本的 merge delivery。
  - request changes 后继续原本的 rework assessment / rework attempt。
- `POST /feishu/review-task/sync`
  - 读取 checkpoint 上保存的 `feishuReview.taskGuid`。
  - 通过 `lark-cli task tasks get` 查询任务状态。
  - 任务已完成时写回 checkpoint approved，并继续原本的 merging / delivery 链路。
- `POST /feishu/review-task/comment`
  - 接收本地事件桥转发的任务评论。
  - 有明确修改意见时写回 checkpoint rejected，并进入原本的 rework 链路。
  - 只有问题 / 缺少信息时记录为 `need-info`，不改变 checkpoint 决策。
- `POST /feishu/review-task/bridge/tick`
  - 本地常驻事件桥的 tick 入口。
  - `dryRun=true` 时只列出 pending checkpoint 和 taskGuid，方便检查多任务绑定是否正确。
  - 正式 tick 会复用 `/feishu/review-task/sync` 的同步语义，把已完成任务同步为 approved。
  - JobSupervisor 可通过 `OMEGA_FEISHU_TASK_BRIDGE_ENABLED=true` 自动调用，减少人工手动 sync。

DevFlow 在进入 `human_review.waiting` 后会自动检查配置：

- 有 `OMEGA_FEISHU_REVIEW_MODE=task`、`OMEGA_FEISHU_REVIEW_ASSIGNEE_ID` 或 `OMEGA_FEISHU_REVIEW_TASKLIST_ID` 时自动创建飞书审核任务。
- 有 `OMEGA_FEISHU_WEBHOOK_URL` 或 `FEISHU_BOT_WEBHOOK` 时自动推送卡片。
- 或有 `OMEGA_FEISHU_REVIEW_CHAT_ID` 且本机存在 `lark-cli` 时发送本地 CLI interactive card。
- 没有配置时只记录 debug log，不阻塞主链路。

## 消息结构

飞书卡片包含：

- Work item key / title
- 当前状态
- 风险等级
- 需求摘要
- PR 链接
- Review Packet 摘要
- `Open review`
- `Approve`
- `Request changes`

长内容不会全部塞进卡片。runtime 会生成一份 Markdown 形式的 review doc preview，后续如果接入飞书文档 API，可以把这份内容直接作为文档正文发布。

Task 审核模式包含：

- task title：Work Item key、Human Review、Work Item 标题。
- task description：review token、Work Item、PR、branch、操作规则、需求摘要、文档链接。
- task comment：首次创建后补一条说明，告诉审核人“完成任务=通过，评论具体修改=请求变更”。
- checkpoint `feishuReview`：保存 `format=task-review`、`taskGuid`、`taskUrl`、`nonce`、doc 信息和 raw CLI 输出。

## 配置

推荐的无公网审核配置：

```bash
lark-cli config init
export OMEGA_FEISHU_REVIEW_MODE="task"
export OMEGA_FEISHU_REVIEW_ASSIGNEE_ID="ou_xxx"
export OMEGA_FEISHU_REVIEW_TASKLIST_ID="可选：任务清单 id"
export OMEGA_FEISHU_REVIEW_FOLLOWER_ID="可选：关注人 open id"
export OMEGA_FEISHU_REVIEW_CREATE_DOC="true"
export OMEGA_FEISHU_REVIEW_DOC_FOLDER_TOKEN="可选：飞书文档目录 token"
export OMEGA_FEISHU_TASK_BRIDGE_ENABLED="true"
```

这条路径不要求本机有公网入口。Omega 本机 runtime 主动通过 `lark-cli` 创建任务 / 文档 / 评论，再通过同步接口查询任务完成状态。

也可以继续使用机器人 webhook：

```bash
export OMEGA_FEISHU_WEBHOOK_URL="https://open.feishu.cn/open-apis/bot/v2/hook/..."
export OMEGA_FEISHU_WEBHOOK_SECRET="可选：机器人签名密钥"
export OMEGA_PUBLIC_APP_URL="https://你的 Omega Web 地址"
export OMEGA_PUBLIC_API_URL="https://你的 Omega Runtime 地址"
export OMEGA_FEISHU_REVIEW_TOKEN="可选：飞书回调校验 token"
```

如果只使用 `lark-cli` 本地发送：

```bash
export OMEGA_FEISHU_REVIEW_CHAT_ID="oc_xxx"
```

当前本机测试结果：已安装 `lark-cli version 1.0.23`，并确认 `im +messages-send` 支持 `--msg-type interactive --content`。真实发送仍需要完成 `lark-cli` 登录 / profile 配置，并提供目标群或会话的 chat id。

## 公网与本地回复边界

出站消息不需要公网：

- 机器人 webhook：本地 runtime 主动向飞书 webhook 发 HTTP 请求。
- `lark-cli`：本机 CLI 主动向飞书发送消息或卡片。
- 这些路径只要求本机能访问外网、机器人或 CLI 权限正确。

飞书按钮直连回调需要额外入口：

- 如果卡片里的 `Approve` / `Request changes` 要由飞书云端直接调用 `POST /feishu/review-callback`，就需要配置飞书可访问的 `OMEGA_PUBLIC_API_URL`。
- 如果不暴露公网，可以保留卡片通知和 `Open review`，审核人回到 Omega Web 操作；这条路径和飞书消息同步同级，但按钮不直接改 checkpoint。
- Task 审核模式不依赖按钮回调。完成飞书任务后，本机调用 `/feishu/review-task/sync` 即可把审核通过同步回 Omega。
- 任务评论可以由本地事件桥或手动脚本转发到 `/feishu/review-task/comment`。这一步同样只调用本机 `127.0.0.1` runtime，不要求公网。
- 如果启用 `OMEGA_FEISHU_TASK_BRIDGE_ENABLED=true`，JobSupervisor 会在 tick 中顺带运行 Task bridge；也可以手动调用 `/feishu/review-task/bridge/tick` 做 dry-run 检查。

## 同步语义

- Omega Web approve 和飞书 callback approve 同级，都会进入 `applyCheckpointDecision(..., "approved", ...)`。
- Omega Web request changes 和飞书 callback request changes 同级，都会进入 `applyCheckpointDecision(..., "rejected", ...)`。
- 飞书任务完成和 Omega Web approve 同级，通过 `/feishu/review-task/sync` 写回 approved。
- 飞书任务评论里的明确修改意见和 Omega Web request changes 同级，通过 `/feishu/review-task/comment` 写回 rejected。
- 飞书任务评论里的问题 / 缺少信息不会直接拒绝，只会写入 checkpoint `feishuReview.lastComment`，供操作者补充上下文。
- 飞书 callback 建议配置 `OMEGA_FEISHU_REVIEW_TOKEN`，避免未经授权的外部请求直接修改 checkpoint。
- 每次发送结果会写入 checkpoint 的 `feishuReview` 字段，包含 `status`、`provider`、`tool`、`format`、message/card preview 等信息。

## Task 审核使用方法

1. 登录并配置 `lark-cli`，确认 bot 有任务和文档权限。
2. 配置 `OMEGA_FEISHU_REVIEW_MODE=task` 和审核人 `OMEGA_FEISHU_REVIEW_ASSIGNEE_ID`。
3. 运行 DevFlow 到 Human Review，或手动调用：

```bash
curl -X POST http://127.0.0.1:3888/feishu/review-request \
  -H 'content-type: application/json' \
  -d '{"checkpointId":"pipeline_xxx:human_review","mode":"task","assigneeId":"ou_xxx"}'
```

4. 审核人在飞书里完成任务表示通过。
5. 本机同步任务状态：

```bash
curl -X POST http://127.0.0.1:3888/feishu/review-task/sync \
  -H 'content-type: application/json' \
  -d '{"checkpointId":"pipeline_xxx:human_review"}'
```

6. 审核人在任务评论里写“请改成……”等具体修改意见时，本地事件桥转发：

```bash
curl -X POST http://127.0.0.1:3888/feishu/review-task/comment \
  -H 'content-type: application/json' \
  -d '{"taskGuid":"task_xxx","comment":"请改成章四","reviewer":"ou_xxx","eventId":"evt_xxx"}'
```

7. Omega 会把这条评论作为 Human Review request changes，生成 rework 输入和 checklist。

## 验证

自动化测试：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuReviewRequestCreatesTaskReviewWithStrongBinding|TestFeishuReviewTaskSyncApprovesCompletedTask|TestFeishuReviewTaskCommentRequestsChanges|TestFeishuReviewTaskCommentNeedInfoRecordsOnly|TestFeishuReviewRequestSendsInteractiveWebhookCard|TestFeishuReviewRequestUsesLarkCLIInteractiveCard|TestFeishuReviewCallbackApprovesCheckpointThroughSharedDecisionPath|TestFeishuNotifyUsesLocalLarkCLI'
```

需要人工验证：

- 配置真实飞书机器人 webhook 后，触发一个 DevFlow Human Review。
- 飞书群收到卡片，内容包含 Work Item、需求摘要、PR 和风险。
- 点击卡片中的本地 review 链接能打开 Omega 对应 Work Item。
- 配置公网 callback 或后续本地事件桥后，飞书侧 Approve 能让 Omega checkpoint 进入 approved，并继续 merging。
- 配置公网 callback 或后续本地事件桥后，飞书侧 Request changes 能让 Omega checkpoint 进入 rejected，并生成 rework attempt / checklist。
- Task 模式下完成任务后调用 `/feishu/review-task/sync`，Omega checkpoint 应进入 approved。
- Task 模式下写明确修改评论后调用 `/feishu/review-task/comment`，Omega checkpoint 应进入 rejected，并进入 rework。
