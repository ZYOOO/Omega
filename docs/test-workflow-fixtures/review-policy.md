# Review Policy 测试样例

## Verdict

Review Agent 只能输出以下三种 verdict：

- `approved`：需求满足、主要验证通过，可进入 Human Review。
- `changes_requested`：需要代码或文档修改，必须进入 Rework。
- `needs_human_info`：缺少产品判断、权限或外部信息，进入 Human Review。

## Review 输出要求

每次 Review 必须包含：

- Summary。
- Blocking findings。
- Validation gaps。
- Rework instructions。
- Residual risks。
- Human Review notes。

## Rework Checklist 来源

Rework checklist 可以来自：

- Review Agent 的 blocking findings。
- Human Review 的 request changes。
- PR comments / reviews。
- failed checks / flaky checks。
- merge conflict / branch behind。
- runtime blocker。

## 去重与分组

- 同一文件、同一行、同一原因的反馈合并为一条。
- 按来源保留 drilldown：human、review、PR、check、runtime。
- checklist 必须写成 Agent 可执行的动作，不只写“修一下”。

## 回流

- Rework 完成后必须重新 Testing。
- Testing 通过后回到 Review。
- Review 通过后再进入 Human Review。
- Human request changes 后，下一轮 Review 必须核对人工意见是否处理。
