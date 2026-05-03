# 飞书应用权限配置说明

## 目标

这份文档只说明 Omega 的飞书 Human Review 需要在飞书开放平台配置哪些能力和权限。当前建议先用自建应用的 App ID / App Secret 配置 `lark-cli`，再由 Omega 本地 runtime 调用 `lark-cli` 发送审核消息、创建审核任务或创建长审核文档。

## 推荐最小配置

如果只是先验证 Human Review 能推送到飞书，推荐使用两档配置。

### A. 只发审核卡片到群

适合先确认消息链路是否通。

需要能力：

- 机器人能力：启用应用机器人，并把机器人加入目标群。
- 消息能力：允许机器人发送消息。

建议权限：

```text
im:message
im:message:send_as_bot
im:chat
```

Omega 使用方式：

- `lark-cli im +messages-send --chat-id oc_xxx --msg-type interactive --content ...`
- 或失败后退回文本消息：`lark-cli im +messages-send --chat-id oc_xxx --text ...`

页面里只需要：

- App ID / App Secret：用于初始化 `lark-cli`
- Review channel：Chat message
- Chat ID：`oc_xxx`

这条路径不需要公网入口。卡片里的按钮如果要直接回调 Omega，则另行需要公网或内网穿透。

### B. 飞书任务审核

适合不暴露公网时做审核闭环。审核人完成任务表示 approve；任务评论里的明确修改意见会被同步为 request changes。

需要能力：

- 机器人能力：启用应用机器人。
- 任务能力：允许创建、读取、更新任务。

必需权限：

```text
task:task:read
task:task:write
```

可选权限：

```text
task:task:writeonly
task:tasklist:read
task:tasklist:write
```

其中：

- `task:task:writeonly`：只写任务时可用，但 Omega 需要同步任务完成状态，所以仍建议保留 `task:task:read`。
- `task:tasklist:read` / `task:tasklist:write`：只有当你要把审核任务放进指定任务清单，或让 Omega 搜索/管理任务清单时才需要。

Omega 使用方式：

- 创建任务：`lark-cli task +create --as bot ...`
- 查询任务：`lark-cli task tasks get --as bot ...`
- 写任务评论：`lark-cli task +comment --as bot ...`

页面里只需要：

- App ID / App Secret：用于初始化 `lark-cli`
- Review channel：Task review
- Reviewer：在 Omega Settings 里按姓名、企业邮箱或手机号搜索并选择，系统会保存审核人的内部 id。
- 如果审核人就是当前登录用户，直接点 `Use current user`，系统会调用 `lark-cli contact +get-user` 读取自己，不需要拉群。
- Tasklist ID：可选

审核人搜索说明：

- 按姓名搜索依赖本机 `lark-cli auth login` 的用户登录态，适合日常使用。
- 联系人搜索不一定返回当前登录用户；这是飞书搜索接口的正常边界，使用 `Use current user` 即可。
- 按企业邮箱 / 手机号解析可以走 App 机器人凭据，通常需要额外开通“获取用户 ID”相关通讯录权限。
- 页面不要求最终用户手动复制 `open_id`；高级配置里仍保留原始 id 输入，主要用于调试或迁移旧配置。

## 长审核文档配置

当需求、Review Packet 或 diff 摘要较长时，Omega 可以把审核包写成飞书文档，任务或卡片里只放摘要和文档链接。

需要能力：

- 云文档 / 云空间能力。
- 机器人需要对目标文件夹有写入权限。

建议权限：

```text
drive:drive
space:folder:create
space:document:retrieve
docs:document:copy
```

如后续切到新版文档 API，可能还需要补充对应 `docx` 文档创建 / 读取 / 写入权限。当前 Omega 代码路径使用的是：

```text
lark-cli docs +create --as bot --title ... --markdown ...
```

页面里对应：

- Create review doc for long packets
- Doc folder token

如果不创建飞书文档，这一组权限可以先不配。

## Bot webhook 模式

如果你只想用群机器人的 webhook URL 发消息，不使用 App ID / App Secret，那么不需要 `lark-cli`，也不需要给 Omega 配 App 权限。

需要在飞书群里添加自定义机器人，并拿到：

```text
Bot webhook URL
Webhook secret，可选
```

Omega 使用方式：

- 本机 runtime 主动向 webhook URL 发送 interactive card。
- 不需要公网。
- 但 webhook 卡片按钮不能直接改 Omega checkpoint，除非另行配置公网 callback。

## App ID / App Secret 初始化

本机当前链路依赖 `lark-cli` 读取飞书 App ID / App Secret。初始化命令：

```bash
lark-cli config init
```

按提示输入：

```text
App ID
App Secret
```

配置完成后检查：

```bash
lark-cli doctor
lark-cli auth scopes
```

Omega 页面里的 `Test connection` 会调用本机 runtime，runtime 再检查 `lark-cli doctor`。

## 开放平台配置步骤

1. 创建自建应用。
2. 在「凭证与基础信息」里复制 App ID / App Secret。
3. 开启机器人能力。
4. 在「权限管理」中添加上面对应权限。
5. 发布版本，并安装到目标租户。
6. 把应用机器人加入审核群，或确认任务审核人属于应用可用范围。
7. 本机执行 `lark-cli config init`。
8. 在 Omega Settings 里配置 Feishu Review channel。
9. 点击 `Test connection`。

## 推荐首测组合

为了最快跑通，建议先配：

```text
im:message
im:message:send_as_bot
im:chat
task:task:read
task:task:write
```

然后先测两件事：

1. Chat message：能否往目标群发一条审核卡片。
2. Task review：能否创建一条审核任务，并通过任务完成同步回 Omega。

长文档、任务清单、webhook secret、callback token 都可以后置。

## 常见问题

### 为什么页面现在有很多字段？

因为当前实现同时支持三条链路：

- `Chat message`：通过 `lark-cli` 往群发消息。
- `Task review`：通过 `lark-cli` 创建任务并同步状态。
- `Bot webhook`：通过群机器人 webhook 发卡片。

产品层应该把 App ID / App Secret、Review channel、目标群或审核人放在主路径，把 webhook、文档目录、tasklist、callback token 放进 Advanced。

### 是否需要公网？

不需要公网：

- `lark-cli` 发消息。
- `lark-cli` 创建任务。
- 本机轮询任务完成状态。
- webhook 出站发消息。

需要公网或内网穿透：

- 飞书卡片按钮直接调用 `POST /feishu/review-callback`。

### Chat ID 和用户 ID 怎么拿？

常见格式：

- 群 ID：`oc_xxx`
- 用户 open_id：`ou_xxx`

可以用飞书开放平台调试台、`lark-cli contact`、`lark-cli im +chat-search` 或已有群消息上下文获取。
