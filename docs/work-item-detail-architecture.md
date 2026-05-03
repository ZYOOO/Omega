# Work Item 详情页与 Run Workpad 架构

日期：2026-04-29

## 背景

Work Item 详情页原先直接写在 `App.tsx` 中：列表点击、详情路由、Requirement 展示、Attempt 状态、Agent trace、proof、Human Review 操作都集中在一个入口组件里。这个旧做法能快速验证功能一闭环，但随着 Attempt、checkpoint、PR、review/rework、Page Pilot proof 都接入后，页面信息密度失控，也让 `App.tsx` 继续膨胀。

新的做法把 Work Item 详情页作为独立产品面维护：

- `App.tsx` 只负责全局状态、数据刷新和动作回调。
- Work Item 详情页独立成组件，并支持单个 item 的可分享路径。
- 详情页优先展示 Run Workpad，再展示流程、操作、交付物和历史。
- Requirement 原文、Agent 操作和 artifact 都必须可读、可展开或可点击，不能只做静态装饰卡片。

## 信息架构

单个 Work Item 详情页按下面顺序组织：

```text
Header
  -> Delivery Flow
      -> compact stage grid
      -> running / waiting / done signal
  -> Run Workpad
      -> Rework Assessment
      -> Plan
      -> Acceptance Criteria
      -> Validation
      -> Notes
      -> Blockers
      -> PR
      -> Review Feedback
      -> Retry Reason
  -> Requirement Source
  -> Current Attempt
  -> Agent Operations
  -> Artifacts
  -> Attempt History
  -> Target
```

旧做法：Run Workpad 和长列表信息直接展开在详情页里，Delivery Flow、Agent Operations、Artifacts、Run Timeline 都占用较多垂直空间，用户需要滚动很远才能找到真正关心的执行状态。

新做法：Delivery Flow 前置为紧凑阶段网格，让用户先看到当前链路卡在哪；Run Workpad、Agent Operations、Artifacts、Run Timeline 都默认展示摘要，点击后展开详情。Run Workpad 仍是执行简报来源，前端优先消费 runtime 维护的 `runWorkpads` 结构化记录，缺失时再用真实执行记录兜底派生，避免再造假数据。

## Run Workpad Record

新增记录集合：

```text
runWorkpads
```

新增 API：

```text
GET /run-workpads
GET /run-workpads?attemptId={attemptId}
GET /run-workpads?pipelineId={pipelineId}
GET /run-workpads?workItemId={workItemId}
PATCH /run-workpads/{id}
```

每条记录绑定一个明确 Attempt：

```text
id
attemptId
pipelineId
workItemId
repositoryTargetId
status
workpad
createdAt
updatedAt
```

`fieldPatches` 是可选的字段级覆盖记录。旧做法：Workpad 每次刷新完全由 runtime 派生，Agent / supervisor 不能稳定写入单个字段。新做法：`PATCH /run-workpads/{id}` 写入 `fieldPatches`，runtime 后续刷新时先生成真实派生 Workpad，再叠加 field patches，避免人工 / supervisor 的 Blockers、Validation、Review Feedback 等被 heartbeat 或 attempt 更新冲掉。

2026-04-30 更新：字段级 patch 新增 `fieldPatchSources` 和 `fieldPatchHistory`。旧做法只保留最终 patch 值，无法说明某个 Blocker / Validation / Retry Reason 来自 CI、人工审核、Agent 还是 supervisor。新做法要求 PATCH 写入者使用明确 `updatedBy`，并可附带 `reason` 与 `source`；runtime 会按字段记录来源，并追加最多 100 条变更历史。当前基础权限边界按写入者限制字段范围：operator / human-review 只能写 review、blocker、validation、retry 等人工判断字段；job-supervisor 可写运行门禁字段；agent 类写入者可写完整交接字段。详情页新增默认折叠的 Patch history 卡片。旧说法：UI 编辑入口仍是后续项。

2026-05-01 更新：Work Item 详情页新增 Run Workpad 字段级编辑入口。旧做法：字段 patch 只能由 API、Agent 或 supervisor 写入，operator 只能看 Patch history，无法在 UI 中补充“真实 blocker / retry reason / review feedback”。新做法：Run Workpad header 提供 `Edit fields`，页内弹窗选择 operator 允许字段并提交到真实 `PATCH /run-workpads/{id}`；payload 固定写入 `updatedBy=operator`、`reason` 和 `source.kind=ui`，后端继续沿用字段权限、来源归因和历史审计。第一版 UI 开放 `Notes`、`Blockers`、`Review Feedback`、`Retry Reason`、`Validation`、`Rework Checklist`、`Rework Assessment`，不开放 PR / Plan / Review Packet 等应由运行时或 Agent 生成的字段，避免人工覆盖交付证据。

`workpad` 内部结构：

```text
plan
acceptanceCriteria
validation
notes
blockers
pr
reviewFeedback
retryReason
reworkAssessment
updatedBy
```

runtime 会在 Attempt 创建、Agent invocation 持久化、Attempt complete / fail / cancel、retry 创建、Human Review approve 后进入 merging 时刷新 Workpad。这样详情页、retry、rework 和后续 supervisor 都可以围绕同一份记录读写。

## 路由

旧做法：Workboard 列表和详情共用 `#workboard`，靠 `activeWorkItemDetailId` 在同一个页面里切换。

新做法：

```text
#workboard
#/work-items/{itemId}
```

列表页点击 Work Item 后进入独立 hash 路由。刷新页面时，App 会从路由恢复当前详情页，避免详情状态只存在于 React 内存中。

## 详情页交互原则

- Requirement 正文限制最大高度，超出后内部滚动，light / dark 都必须保持可读。
- Delivery Flow 使用紧凑网格展示，一行可展示多个阶段；running / waiting-human / merging 必须有清晰动画。
- Run Workpad 旧做法是每个区块默认折叠，展开后在卡片内部滚动。2026-04-30 更新：改为紧凑信号卡，卡片只展示 label、状态标题和一行真实来源摘要；点击卡片后打开页内弹窗浏览完整内容。
- Agent Operations 旧做法是点击后行内展开 prompt、stdout、stderr、runner metadata。2026-04-30 更新：改为结构化摘要卡 + 页内弹窗浏览详情，避免一个 operation 把详情页撑长。
- Artifacts 展示为可点击记录，点击后展开 source path / URL / stage，不再只显示不可操作卡片。
- Run Timeline 默认折叠，只展示最新事件摘要；展开后展示当前 API 返回的完整事件列表，避免详情页默认被日志淹没。
- Human Review approve 只批准 checkpoint，之后展示独立 `merging` 阶段，由后台 merge job 更新 checks、branch sync、conflict、merge 输出和失败原因。
- Human Review header 不展示重复产品名；PR / Changed / Validation / Artifacts 使用左右紧凑布局，优先减少行高。
- 已通过阶段不再整卡大面积绿色，只保留轻量通过信号，避免和当前执行 / 成功摘要混在一起。

## Review / Rework 输入

旧做法：失败时主要展示 Attempt failure reason 或 runner stderr，Review Agent 的意见、PR comments、Human request changes 分散在不同事件或 proof 中。

新做法：详情页把 review result、human change request、PR/check recommended action 和 failure detail 合并成 Workpad 的 Review Feedback / Retry Reason 区块，并持久化到 `runWorkpads`。Human Review 点 Request changes 后，后端会把旧 Attempt 标记为 `changes-requested`，创建新的 `human-request-changes` Attempt，并把人工反馈写入 `humanChangeRequest`、`retryReason` 和下一轮 Agent prompt。新 Attempt 会继承上一轮 delivery branch、PR URL 和 workspace path；执行器会优先 checkout 本地/远端同名 delivery branch，让修改基于第一次已完成版本继续推进。二次 review 会看到人工总意见和本轮增量 diff；PR 描述会在人工 rework 后按需更新，避免 GitHub PR 与 Omega 详情页信息脱节。

2026-04-30 更新：Human Review Request changes 先写入 `reworkAssessment`，再按评估结果选择入口。局部修改走 `fast_rework`，直接从 `rework` 阶段续跑并复用同一 PR；涉及需求、接口、权限、数据模型或跨模块流程的修改走 `replan_rework`，从 `todo` 重新规划；人工意见为空或不明确时走 `needs_human_info`，不启动 Agent，Attempt 保持等待人工补充。详见 `docs/human-review-rework-assessment.md`。
