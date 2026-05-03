# Runtime Policy 测试样例

用于 Workspace Agent Studio / Runtime files 页的策略参考。

## Workspace

- 每个 Work Item 必须绑定明确 Repository target。
- 每次 Attempt 使用隔离 workspace 执行。
- Rework 优先复用当前 Attempt 的 branch / workspace / PR 上下文，不从空白仓库重做。
- Page Pilot 默认使用 isolated preview workspace，Confirm 后再进入 branch / commit / PR。

## Heartbeat

- runner 进程启动、stdout/stderr、阶段完成、checkpoint 决策、PR/checks 轮询都要刷新 `lastSeenAt`。
- running Attempt 超过 timeout 未更新 heartbeat 时进入 stalled。

## Timeout

- Requirement / Architect：短超时，适合快速生成 artifact。
- Coding / Rework：中长超时，允许真实修改仓库。
- Review / Testing：中等超时，失败要输出原因。
- Delivery / Merging：受 GitHub API 和 checks 状态影响，失败必须分类。

## Retry

- 临时网络失败、GitHub API 临时失败、CI flaky 可以自动重试。
- 权限失败、缺 repository target、需求缺信息应进入 Human Review。
- Review changes requested 应进入 Rework，不当作系统错误 retry。

## Logs

- 关键事件写 runtime log：request、attempt、operation、agent、checkpoint、GitHub、Feishu、Page Pilot。
- stdout/stderr 保留摘要，详细内容进入 operation / artifact。
- 日志时间按本地时区展示，存储仍保留 UTC 时间戳。

## Proof

- 每次交付必须生成 proof。
- proof 至少包含 changed files、diff summary、test report、PR/check status、human decision 和 merge result。
