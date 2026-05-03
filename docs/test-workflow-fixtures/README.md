# Workflow 测试样例

这些文件用于手动测试 Workspace Agent Studio 的 workflow / prompt / agent / runtime policy 配置能力。样例按 Omega 自己的 Requirement / Work Item / Pipeline / Attempt / Workpad 模型组织，吸收了成熟模板的“全局规则、阶段契约、结构化输出、阻塞处理”写法，但不绑定外部项目对象。

## 使用方式

推荐方式：

1. 打开 Omega Web 或 Desktop。
2. 进入左侧 workspace 卡片的设置页。
3. 展开 `Workspace Agent Studio`。
4. 点击 `Import sample template` 可直接导入本目录的内置样例。
5. 如果目标仓库已经准备了 `.omega/WORKFLOW.md`、`.omega/PROMPTS.md`、`.omega/STAGE_POLICY.md`，点击 `Import from repository .omega` 可从当前绑定仓库导入。
6. 导入后点击 `Save draft`，后续新建 Pipeline 会使用保存后的 Agent Profile。

手动复制方式：

1. 打开 Omega Web 或 Desktop。
2. 进入左侧 workspace 卡片的设置页。
3. 展开 `Workspace Agent Studio`。
4. 在 `Workflow` 页选择 `Markdown contract`，把 `workflow.md` 或其他 `workflow-*.md` 内容复制进去。
5. 在 `Workflow` 页逐个选择阶段，把 `stage-policy.md` 中对应阶段策略复制到规则输入框。
6. 在 `Prompts` 页，把 `prompts.md` 中对应 Agent 的内容复制到 prompt 输入框。
7. 在 `Agents` 页参考 `agent-profiles-trae.md` 配置 runner / model / skills / MCP。
8. 在 `Runtime files` 或相关策略区参考 runtime / review / delivery / Page Pilot policy。
9. 点击 `Save draft`。

## 路径读取规则

- 运行时执行优先读取当前 Repository target 的本地路径：`{repository.path}/.omega/WORKFLOW.md`。
- 如果仓库内没有 workflow 文件，则读取 Repository scoped Agent Profile，再读取 Project scoped Agent Profile。
- 如果页面保存了 workflow template override，会优先应用该 override。
- 最后回退到 Omega 内置模板：`services/local-runtime/workflows/devflow-pr.md`。
- UI 导入的内置样例来自 Omega 项目目录：`docs/test-workflow-fixtures/`。
- UI 导入的仓库样例来自当前绑定仓库目录：`{repository.path}/.omega/`。

## 文件说明

- `workflow.md`：当前默认 DevFlow 的精简可测版。
- `workflow-fast-rework.md`：用于测试 review -> rework -> review 的快速回路。
- `workflow-human-review-heavy.md`：用于测试人工审核、飞书通知、合并前确认链路。
- `stage-policy.md`：各阶段规则样例。
- `prompts.md`：各 Agent prompt 契约样例，包含角色、输入、边界、输出和失败处理。
- `agent-profiles-trae.md`：Trae Agent / Codex 混合配置样例。
- `authoring-guide.md`：说明实际项目应该准备哪些 Markdown 文件，以及每个文件负责什么。
- `requirement-template.md`：新建需求时可用的描述模板。
- `runtime-policy.md`：workspace、heartbeat、timeout、retry、logs、proof 策略样例。
- `review-policy.md`：Review verdict、Rework checklist、反馈聚合和回流策略样例。
- `delivery-policy.md`：PR、checks、branch sync、merge 和 proof 策略样例。
- `page-pilot-policy.md`：Page Pilot preview runtime、圈选、source mapping、Confirm/Discard 策略样例。

这些样例只包含可复制配置，不包含真实 API Key。
