# Omega 桌面应用发布打包

本文说明如何把 Omega 构建为桌面应用，并将产物发布到 GitHub Release。

English version: [release-packaging.md](release-packaging.md)

## 当前发布状态

macOS 打包链路已经可用，并已在本机 dry-run：

- `npm run release:prepare` 构建 React Web UI 和 Go binaries。
- `npm run desktop:pack` 构建 unpacked `.app`，用于本地 smoke test。
- `npm run desktop:dist` 在 `dist/desktop` 下生成发布产物。

本机 dry-run 产物：

```text
dist/desktop/Omega-0.1.0-arm64.dmg
dist/desktop/Omega-0.1.0-mac-arm64.zip
dist/desktop/latest-mac.yml
```

打包后的应用包含：

- Electron desktop shell；
- `Resources/web` 下的静态 Web UI；
- `Resources/bin` 下的 `omega-local-runtime` 和 `omega` Go binaries；
- `docs/openapi.yaml`；
- `Resources/workflows` 下的内置 workflow contracts，包括 `devflow-pr` 和 `saas-launch`。

## 环境要求

构建机器：

- 构建 macOS `.dmg` / `.zip` 推荐使用 macOS。
- 推荐 Node.js 22+。Node 20 可以运行当前本地构建，但 `electron-builder` 依赖会提示 Node 22.12+。
- npm。
- Go 1.21+。
- 如果要手动发布到 GitHub Release，需要 GitHub CLI `gh`。
- Git。

本次提交前验证版本：

```text
Node.js v20.19.4
npm 10.8.2
Go go1.21.6 darwin/arm64
Git 2.39.3 (Apple Git-145)
GitHub CLI 2.88.0
Electron 41.3.0
electron-builder 26.8.1
```

终端用户运行时工具：

- GitHub 交付功能需要 `git` 和已登录的 GitHub CLI `gh`。
- Codex / Claude Code / opencode / Trae Agent 需要本机安装对应 CLI 或在 Omega 中配置 runner credential。
- 飞书功能需要配置可用投递路由，或使用 `lark-cli` current-user fallback。

公开 macOS 发布的可选签名 / notarization 环境：

- Apple Developer ID certificate。
- 用于 `electron-builder` 签名的 `CSC_LINK` 和 `CSC_KEY_PASSWORD`，或本机已安装的签名 identity。
- 用于 notarization 的 `APPLE_ID`、`APPLE_APP_SPECIFIC_PASSWORD` 和 `APPLE_TEAM_ID`。

未签名本地构建可以使用：

```bash
CSC_IDENTITY_AUTO_DISCOVERY=false npm run desktop:dist
```

## 构建命令

安装依赖：

```bash
npm ci
```

运行验证：

```bash
npm run lint
npm run test -- --reporter=dot
npm run go:test:focused
```

准备 release 输入：

```bash
npm run release:prepare
```

构建 unpacked desktop app，用于本地 smoke test：

```bash
CSC_IDENTITY_AUTO_DISCOVERY=false npm run desktop:pack
```

构建可分发 macOS 产物：

```bash
CSC_IDENTITY_AUTO_DISCOVERY=false npm run desktop:dist
```

如果要构建签名和 notarized 版本，先导出签名 / notarization 环境变量，再运行：

```bash
npm run desktop:dist
```

## Smoke Test

执行 `desktop:pack` 后，确认 packaged app 包含预期资源：

```bash
APP="dist/desktop/mac-arm64/Omega.app"
test -f "$APP/Contents/Resources/web/index.html"
test -x "$APP/Contents/Resources/bin/omega-local-runtime"
test -x "$APP/Contents/Resources/bin/omega"
test -f "$APP/Contents/Resources/docs/openapi.yaml"
test -f "$APP/Contents/Resources/workflows/devflow-pr.md"
test -f "$APP/Contents/Resources/workflows/saas-launch.md"
```

验证 packaged runtime binary：

```bash
TMPROOT="$(mktemp -d /tmp/omega-release-smoke.XXXXXX)"
"$APP/Contents/Resources/bin/omega-local-runtime" \
  --host 127.0.0.1 \
  --port 3899 \
  --database "$TMPROOT/.omega/omega.db" \
  --workspace-root "$TMPROOT/workspaces" \
  --openapi "$APP/Contents/Resources/docs/openapi.yaml" \
  --job-supervisor=false
```

然后打开 `http://127.0.0.1:3899/health`，确认服务正常后停止进程。

验证 packaged CLI binary：

```bash
"$APP/Contents/Resources/bin/omega" --help
"$APP/Contents/Resources/bin/omega" --api-url http://127.0.0.1:3899 health
```

## 发布到 GitHub Release

推荐手动 draft release 流程：

```bash
VERSION="v0.1.0"
git tag "$VERSION"
git push origin "$VERSION"

gh release create "$VERSION" \
  "dist/desktop/Omega-0.1.0-arm64.dmg" \
  "dist/desktop/Omega-0.1.0-arm64.dmg.blockmap" \
  "dist/desktop/Omega-0.1.0-mac-arm64.zip" \
  "dist/desktop/Omega-0.1.0-mac-arm64.zip.blockmap" \
  "dist/desktop/latest-mac.yml" \
  --draft \
  --title "Omega 0.1.0" \
  --notes "Initial desktop release."
```

如果配置了 `GH_TOKEN`，也可以让 `electron-builder` 发布：

```bash
GH_TOKEN="..." npm run release:github
```

建议先创建 draft release，在干净机器上下载并安装验证，再公开发布。

## 安装方式

macOS：

1. 从 GitHub Release 下载 `.dmg`。
2. 打开 `.dmg`。
3. 将 `Omega.app` 拖到 `Applications`。
4. 启动应用。

如果是未签名内部测试构建，macOS Gatekeeper 可能阻止首次启动。仅内部测试时可执行：

```bash
xattr -dr com.apple.quarantine "/Applications/Omega.app"
```

公开发布不应要求用户移除 quarantine，而应该使用签名和 notarization。

## 首次运行

桌面应用启动后会启动：

- `127.0.0.1:3888` 上的 bundled Go local runtime；
- `Resources/web` 中的 bundled static Web UI。

运行时数据存储在 app user data 目录：

```text
~/Library/Application Support/Omega/.omega/omega.db
~/Library/Application Support/Omega/workspaces
```

## 基本使用

1. 打开 `Settings`。
2. 配置 GitHub access：本机 `gh` 登录态或 OAuth。
3. 配置 Agent access：
   - Codex 和 Claude Code 继承本机 CLI 账号；
   - opencode 和 Trae Agent 可配置 provider / model / base URL / API key。
4. 在 `Projects` 中绑定 Repository Workspace。
5. 创建或导入 Requirement。
6. 转成 Work Item。
7. 运行 DevFlow。
8. 查看 Work Item 详情：
   - Plan；
   - Acceptance Criteria；
   - Validation；
   - Review Packet；
   - stage Agent statistics；
   - PR、blockers、retry reason、notes、proof。
9. 在 Human Review 中 approve 或 request changes。

## 已知发布缺口

- 当前本机 dry-run 使用 ad-hoc macOS signing，未 notarize。
- 自定义 app icon 尚未配置，目前使用 Electron 默认 icon。
- Windows 和 Linux 打包配置已存在，但本轮未进行 smoke test。
- macOS auto-update metadata 已生成，但应用内 updater flow 尚未接入。
