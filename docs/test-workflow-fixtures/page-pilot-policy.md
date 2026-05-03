# Page Pilot Policy 测试样例

## Preview Runtime

- Page Pilot 必须绑定明确 Repository target。
- 对需要 dev server 的项目，由 Preview Runtime Agent 读取项目结构、生成启动 profile、启动服务并健康检查。
- 对静态 HTML 项目，可以使用 file preview，但仍需要绑定 repository workspace。
- 用户手动输入 URL 时，也要记录 URL 与 repository target 的绑定关系。

## Selection Context

圈选元素至少记录：

- selector。
- DOM text / attributes。
- style snapshot。
- source context。
- screenshot / visual proof。
- 用户修改指令。

## Source Mapping

- 优先用真实 source context 定位。
- DOM-only selection 必须标记覆盖率不足，不要假装已定位源码。
- 多批选区应合并成一个 run，并保留 round。

## Apply

- 默认在 isolated preview workspace 内修改。
- Agent 修改后触发热更新或重启 preview runtime。
- 修改结果必须生成 diff summary 和 visual proof。

## Confirm

- Confirm 后创建 branch / commit / PR。
- 生成语义化摘要和行级 diff 摘要。
- Confirm 后按钮进入只读状态，避免重复确认。

## Discard

- Discard 必须清理或 reset isolated workspace 的未确认改动。
- Discard 后 Work Item 不应进入 Done，应展示 discarded/canceled 状态。
