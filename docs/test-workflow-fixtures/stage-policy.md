# Stage Policy 测试样例

复制到 Workspace Agent Studio 的 `Workflow` 页，按左侧阶段逐个填写。

## Requirement

先确认需求是否有明确用户价值、验收标准和 repository target。若缺少目标仓库或验收条件，必须在 requirement artifact 中列出 open questions，不允许直接进入 coding。

## Architecture

先列出可能受影响的文件、状态流、接口边界和风险点。只做足够支撑 coding 的轻量设计，不要把 architecture 阶段变成大篇文档。

## Coding

只修改绑定的 repository workspace。优先小步提交可审查 diff；如果发现需求与现有代码冲突，把冲突写入 implementation notes。

## Testing

先跑与改动直接相关的最小测试，再根据影响范围补充 lint/build 或页面 smoke test。失败时记录命令、退出码和关键日志。

## Review

必须给出明确 verdict：approved、changes_requested 或 needs_human_info。changes_requested 必须附带可执行 checklist，不要把 review agent 的业务拒绝当成系统错误。

## Rework

复用当前 attempt 的 workspace 和 PR，不重新从空白仓库开始。先消费 review / human / check feedback，再更新 PR 描述和 diff summary。

## Human Review

等待人工明确 approve 或 request changes。request changes 的正文必须进入 rework input，并在下一轮 review 中核对是否已处理。

## Delivery

Approve 后进入单独 merging 阶段，显示 checks、branch sync、conflict、merge 输出。merge 成功后再收集 proof 和 handoff bundle。
