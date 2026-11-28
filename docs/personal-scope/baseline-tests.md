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

## 复核：本机实测基线（2026-11-28）

裁剪开始前在**本机**（macOS 26.2 / arm64 / Go 1.27.2）实测的失败集合是 **19 个**，
比上表记录的 15 个多 4 个。多出的 4 个都是环境/时序相关的，不在原表里：

| 多出的用例 | 症状 |
|---|---|
| `TestLoopStartupCheckUsesFixtureAndIgnoresLegacyConfig` | 启动检查依赖固定的时序窗口 |
| `TestLoopStartupCrossingWindowDoesNotSkipToday` | 同上 |
| `TestLoopTimerStartsNextCheck` | 同上 |
| `TestRunLeaseMutualExclusionAndExpiryTakeover` | 租约到期接管依赖墙钟精度 |

复现命令同上。这 4 个分别属于 LOOP 引擎与运行租约 —— 两者都随裁剪被删除
（见 #9 删执行引擎、#11 执行机器降级为清单），因此终态基线会比 19 更小。

**判定标准不变**：每一阶段裁剪后，失败集合必须是上表的子集，且不得出现上表之外的
任何新失败。逐阶段比较时用集合包含关系，而不是个数相等。

## 终态（2026-11-28，`feature/personal-build`）

裁剪完成后，本机 `go test ./internal/... ./cmd/... ./web/ -count=1 -timeout=900s`
**0 失败**。上表的失败分两类：

- **随着裁剪一起消失**：LOOP 引擎、运行租约、浏览器/流、仓库缓存、seatbelt 沙箱和
  profile 噪音相关的用例，所在的包已被删除，或包内已不含这些用例。
- **留下来、按契约修正的 2 个**（修正断言的前提，不放宽断言本身）：
  - `TestGitDiscoveryStopsBeforeParentRepository`：git 返回真实路径，而 macOS 的
    `t.TempDir()` 在 `/var` → `/private/var` 符号链接下。断言改为比较
    `filepath.EvalSymlinks` 后的路径；「父仓库必须无法被发现」那条断言不变。
  - `TestSDNotifySendsReadyAndMainPID`：`unixgram` 的 `sun_path` 上限 104 字节，
    macOS 的 `t.TempDir()` 会超出（`bind: invalid argument`）。socket 改在短目录里
    创建；payload 断言不变。

