# Delivery Policy 测试样例

## PR

- Coding 或 Rework 产生可审查 diff 后，应创建或更新 PR。
- PR 描述必须包含需求摘要、变更摘要、验证结果、风险和下一步。
- Human request changes 后，如果实现或风险说明变化，需要更新 PR 描述。

## Checks

- Merge 前必须读取 PR checks。
- required checks 缺失、失败或 pending 时，不允许直接 merge。
- failed check 必须生成可读原因和推荐动作。

## Branch Sync

- Merge 前检查分支是否落后主分支。
- 能安全同步时执行 sync/rebase。
- 有冲突时进入 Human Review 或 Rework，并附带冲突文件。

## Merge

- Human Review approve 后只进入 Merging，不直接把 approve 当作完成。
- Merging 阶段负责 checks、branch sync、conflict、merge 和 proof。
- Merge 成功后再进入 Done。

## Failure 分类

- `permission_failed`：权限不足，需要人工配置。
- `branch_missing`：分支不存在或已删除，需要恢复。
- `checks_failed`：CI 或 required checks 未通过。
- `merge_conflict`：分支冲突，需要 Rework。
- `temporary_network`：可重试。
- `github_api_temporary`：可重试。

## Proof

交付 proof 至少包含：

- PR URL。
- branch / commit。
- checks summary。
- merge output。
- handoff bundle。
- rollback hint。
