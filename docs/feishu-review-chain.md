# 飞书通知 / 审核链路

## 目标

功能一的 Human Review 不只停留在 Omega 页面内，也能同步推送到飞书，让审核人从飞书看到需求、PR、风险和操作入口，并且飞书侧的审核动作和 Omega 本地审核动作走同一条 checkpoint 决策链路。

## 旧做法

- Go local runtime 只有 `POST /feishu/notify` 文本通知。
- 文本通知需要本机安装 `lark-cli`。
- Human Review checkpoint 只能在 Omega Web 内 Approve / Request changes。
- 飞书消息没有结构化 card、没有 callback、没有写回 checkpoint 的同步记录。

## 新做法

新增两条 API：

- `POST /feishu/review-request`
  - 输入 `checkpointId` 和可选 `chatId`。
  - 读取 Work Item、Requirement、Attempt、Run Workpad、Review Packet。
  - 生成飞书 interactive card。
  - 优先通过机器人 webhook 发送卡片。
- 没有 webhook 但有 `lark-cli + chatId` 时发送 interactive card；如果本地 CLI 不支持卡片参数，再退回文本 fallback。
  - 两者都没有时返回 `needs-configuration`，并把 card/doc preview 写入 checkpoint 的 `feishuReview`。
- `POST /feishu/review-callback`
  - 接收飞书侧 approve / request changes 动作。
  - 通过同一个 checkpoint decision helper 执行本地 approve / reject。
  - approve 后继续原本的 merge delivery。
  - request changes 后继续原本的 rework assessment / rework attempt。

DevFlow 在进入 `human_review.waiting` 后会自动检查配置：

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

## 配置

推荐配置机器人 webhook：

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
- 后续可加本地事件桥：由本机 `lark-cli event consume` 拉取卡片交互事件，再调用本地 `127.0.0.1` runtime。这个方案可以避免把本机 API 暴露到公网，但需要配置飞书应用事件订阅和一个本地常驻桥进程。

## 同步语义

- Omega Web approve 和飞书 callback approve 同级，都会进入 `applyCheckpointDecision(..., "approved", ...)`。
- Omega Web request changes 和飞书 callback request changes 同级，都会进入 `applyCheckpointDecision(..., "rejected", ...)`。
- 飞书 callback 建议配置 `OMEGA_FEISHU_REVIEW_TOKEN`，避免未经授权的外部请求直接修改 checkpoint。
- 每次发送结果会写入 checkpoint 的 `feishuReview` 字段，包含 `status`、`provider`、`tool`、`format`、message/card preview 等信息。

## 验证

自动化测试：

```bash
go test ./services/local-runtime/internal/omegalocal -run 'TestFeishuReviewRequestSendsInteractiveWebhookCard|TestFeishuReviewRequestUsesLarkCLIInteractiveCard|TestFeishuReviewCallbackApprovesCheckpointThroughSharedDecisionPath|TestFeishuNotifyUsesLocalLarkCLI'
```

需要人工验证：

- 配置真实飞书机器人 webhook 后，触发一个 DevFlow Human Review。
- 飞书群收到卡片，内容包含 Work Item、需求摘要、PR 和风险。
- 点击卡片中的本地 review 链接能打开 Omega 对应 Work Item。
- 配置公网 callback 或后续本地事件桥后，飞书侧 Approve 能让 Omega checkpoint 进入 approved，并继续 merging。
- 配置公网 callback 或后续本地事件桥后，飞书侧 Request changes 能让 Omega checkpoint 进入 rejected，并生成 rework attempt / checklist。
