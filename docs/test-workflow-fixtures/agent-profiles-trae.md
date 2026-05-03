# Agent Profiles 测试样例

这个样例用于测试 Trae Agent 账号绑定后能否被工作流选择。

## 推荐配置

| Agent | Runner | Model | Skills | MCP |
| --- | --- | --- | --- | --- |
| Requirement | Codex | gpt-5.4-mini | GitHub, Browser | GitHub, Repo files |
| Architect | Codex | gpt-5.4-mini | GitHub | Repo files |
| Coding | Trae Agent | trae-default | Browser, Fix CI | Repo files, Preview browser |
| Testing | Codex | gpt-5.4-mini | Browser | Repo files, Runtime logs |
| Review | Codex | gpt-5.4-mini | GitHub, PR comments, Fix CI | GitHub, Repo files |
| Delivery | Codex | gpt-5.4-mini | Publish PR, GitHub | GitHub, Repo files |

## Trae Agent model 规则

- 建议在 `Runtime files -> Trae Agent profile` 中保存 EP ID。
- `Agents` 页里把需要使用 Trae 的 Agent runner 设为 `Trae Agent`。
- Model 选择 `trae-default` 时，runtime 会优先使用账号配置里的 EP ID。
- 如果某个 Agent 要覆盖账号默认模型，可以把 Model 写成 `doubao:你的-EP-ID`。

## Skills 建议

Coding Agent:

```text
browser-use
github:gh-fix-ci
```

Testing Agent:

```text
browser-use
```

Review Agent:

```text
github:github
github:gh-fix-ci
github:gh-address-comments
```

Delivery Agent:

```text
github:yeet
github:github
```

## MCP 建议

Coding Agent:

```text
filesystem:repository-workspace
browser:localhost-preview
```

Review / Delivery Agent:

```text
github
filesystem:repository-workspace
runtime-logs
```
