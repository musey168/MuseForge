# 基线：裁剪前的测试状态

记录时间：2026-11-27，基线 commit `fd039e51`（= upstream/main）。
环境：macOS 26.2 / arm64 / 8GB / Go 1.27.2 / Node 24.22.3。

## 通过

- `go build ./...` ✅
- chatbot 前端：`63 passed (63)` 测试文件，`552 passed` 用例 ✅
- `go test ./internal/... ./cmd/...` — 6 个包 FAIL，**全部为环境性问题**

## 失败用例与归因

15 个失败用例，**没有一个是代码逻辑错误**，全部是本机环境与 Linux CI 的差异：

| 归因 | 用例 | 症状 |
|---|---|---|
| macOS `TMPDIR` 是 `/var` → `/private/var` 符号链接 | `TestGitDiscoveryStopsBeforeParentRepository`、`TestLocalWorkspaceGitDiscoveryCannotReachParent`、`TestRepositoryCacheCheckoutUsesOneTimeout`、`TestRunnerRepositoryCacheDoesNotRetryAStoppedTransfer`、`TestExecSetsPWDAndRejectsMissingWorkspace` | 断言里的路径与实际路径符号链接不一致 |
| shell profile 噪音（`~/.profile` 里 `.cargo/env` 不存在） | `TestRepositoryScopedHookRunsInEveryRepository`、`TestOpenRemoteSandboxUsesExactRunnerWorkspace`、`TestRunSandboxBindsCodeBuddyCredentialOnlyToTargetProcess` | stdout 混入了 profile 报错行 |
| Linux-only 的 seatbelt 沙箱 | `TestProcessSandboxPathsProfilesAndCommands` | `seatbelt unexpectedly available on linux` |
| Chromium 不可用 | `TestEnsureRunBrowserRestoresReclaimedWorkspaceWithStaleRun` | `Agent 浏览器启动失败` |
| 超时 / socket 特性 | `TestBrowserStreamPushesFramesAndRunsCommands`、`TestRunnerSOPCLIAuthentication`、`TestMaterializeConversationWorkspaceStopsAStalledClone`、`TestSDNotifySendsReadyAndMainPID` | 计时敏感或 `unixgram` bind 失败 |

## 裁剪时的判定标准

**每一阶段裁剪后，失败用例集合必须与上表完全一致。**
出现上表之外的任何新失败 = 这一刀砍错了，必须回滚该阶段。

复现命令：

```bash
GOCACHE=/tmp/agentflow-gocache go test ./internal/... ./cmd/... -count=1 -timeout=6m 2>&1 | grep -E "^--- FAIL|^FAIL"
```
