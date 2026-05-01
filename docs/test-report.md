# Omega 测试报告

## 2026-04-30 功能一 / 功能二 P0 闭环

### 覆盖范围

- 功能一 GitHub delivery contract preflight：运行前验证 `gh` 登录、仓库可读、PR 权限和 checks 元数据读取能力。
- 功能一 Reject -> Rework：人工拒绝后进入真实 rework 记录，Workpad 展示 rework assessment / checklist / feedback。
- 功能一 Go runtime 长链路：JobSupervisor、Human Review、Attempt retry、Page Pilot linkage 与新增 preflight 合约在同一测试套件中回归。
- 功能二 Go Preview Runtime：`resolve` / `start` / `restart` API 生成 profile、启动服务、记录 pid/stdout/stderr/health check。
- 功能二 Page Pilot result panel：Recent runs 详情弹窗展示 PR preview、diff summary、visual proof、conversation 和 Work Item 回跳。
- 功能二 Page Pilot multi-round：同一个 run 可继续追加 apply，并递增 round。
- 功能二 visual proof：run / pipeline artifacts / PR preview 都能拿到 DOM snapshot 证据。

### 固定命令

```bash
npm run test:feature-p0
```

等价分解：

```bash
npm run lint
npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000
node --check apps/desktop/src/process-supervisor.cjs
node --check apps/desktop/src/pilot-preload.cjs
go test ./services/local-runtime/internal/omegalocal
```

### 本轮结果

- `npm run lint`：通过。
- `npm run test -- apps/web/src/components/__tests__/PagePilotPreview.test.tsx apps/web/src/components/__tests__/WorkItemDetailPage.test.tsx --testTimeout=15000`：通过，9 个测试。
- `node --check apps/desktop/src/process-supervisor.cjs`：通过。
- `node --check apps/desktop/src/pilot-preload.cjs`：通过。
- `go test ./services/local-runtime/internal/omegalocal`：通过。

### 仍需手测

Electron BrowserView、真实目标项目 HMR、目标页面内三元素圈选和 Confirm / Discard 终态需要手动验证。详见 `docs/manual-testing-needed.md`。
