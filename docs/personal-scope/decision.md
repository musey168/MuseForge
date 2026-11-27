# MuseForge 功能取舍决策记录

按 grilling 流程逐项确定。每条记录：截图依据、结论、影响到的代码。

---

## 产品定位（Q4 结论）

**MuseForge = 个人项目管理台**。

Agent 执行交给 omp（你 Mac 本地的 `omp`），MuseForge 负责：
需求清单、项目分组、凭据配置、统计（对接 omp 的执行数据）。

**明确不做**：LoopForge 的执行引擎（LOOP 派发、沙箱、Runner、CLI 执行器）、
人工关口、审批收件箱、聊天工作台、定时任务、知识库、企业 IM、团队成员体系。

---

## 已定决策总表

| # | 功能 | 结论 | 依据 |
|---|---|---|---|
| 01 | AI 工作台（Main Bot 聊天） | **删** | 截图①；日常在 omp 里工作 |
| 02 | 统计 | **保留，改为 omp 用量台** | 截图② |
| 03 | 需求列表 | **保留，精简为待办清单** | 截图③ |
| 04 | 待我确认（审批收件箱） | **删** | 截图④ |
| 05 | 个人配置（凭据） | **保留** | 截图⑤ |
| 06 | AI 助手（助手设置） | **删** | 截图⑥ |
| 07 | 项目配置 | **保留，删成员/加入申请/业务域** | 截图⑦ + Q5=strip |
| 08 | 通道管理（企微/飞书/钉钉 Bot） | **删** | 截图⑧ |
| 09 | 流程模板（LOOP 画布） | **删** | 截图⑨ + Q1=C |
| 10 | 运行记录 | **删** | 截图⑨ + Q2=全删 |
| 11 | 定时任务 | **删**（含个人任务与共享市场） | 截图⑨ + Q4=all |
| 12 | 知识库 | **删** | Q4 回答原文 |
| 13 | 人工关口 | **删** | Q3=全删 |
| 14 | omp → MuseForge 数据桥 | **omp 侧 hook 主动推送** | Q1=B |
| 15 | 统计卡片 | **删平台内专属卡片，保留 Token/成本/缓存/趋势/需求关联** | Q2=B |
| 16 | 需求字段 | 标题、描述、状态、优先级、项目归属、标签、备注 + **GitHub/GitLab 连接** | Q3 自由作答 |
| 17 | 数据库 | **换 SQLite** | Q4=sqlite |
| 18 | 鉴权 | **界面免登录**（只绑 127.0.0.1），API Token 保留给 omp 推送用 | Q5=noauth |
| 19 | 角色定义 | **删** | 截图⑩ |
| 20 | 能力资产（MCP/Rules/Skills） | **删** | 截图⑩ |
| 21 | 环境资产（沙箱） | **删** | 截图⑩ |
| 22 | 访问密钥（`api_tokens`） | **删** | 截图⑩ |
| 23 | 平台管理 | **删** | 截图⑩ |
| 24 | 执行机器 | **保留** | 截图⑩ |

### 需要向你确认的两点（我在整理时查证代码发现的偏差）

**① 第 16 行的「标签」和「备注」在上游需求模型里不存在。**
`internal/domain/issue.go` 的 `Issue` 结构体实际字段是：
`id / projectId / number / title / description / status / closeSource / priority /
assigneeType / assigneeId / assigneeUserId / workflowId / playbookId /
sopBindings / sopMode / repositoryId / repositoryIds / baseBranch /
repositoryBranches / latestTaskId / automationState / deliveryState /
deliveryMrUrl / deliveryMrState / deliveryMrs / kind / clarification / source /
tapdId / externalRef / conversationSessionId / createdAt / updatedAt / completedAt / operations`。

- **优先级** ✅ 存在（`priority`）
- **标签** ❌ 没有 tag/label 字段 —— 需要**新建**
- **备注** ✅ 有，但形态是 `issue_comments` 表里的评论（`IssueComment`），
  不是需求上的一个文本字段 —— 保留评论功能即可

**② 上游没有「GitLab 需求同步」，只有 GitHub 的。**
- GitHub 需求导入是现成的：`internal/tracker/github.go:260 pullGitHubIssues`
  （入口 `internal/tracker/service.go:295 PullAll` 与 `:370` 的单源同步）
- GitLab 在上游只出现在**凭据层**（`internal/httpapi/git_http_credentials.go:120`
  「平台只能是 github、gitlab 或 other」）和 **MR 状态查询**
  （`internal/tracker/external_mr.go` 刷 GitLab MR 状态）
- `Issue.Source` 的取值是 `local / gongfeng / tapd / github` —— **没有 gitlab**

→ 「连接 GitHub」保留现成实现；「连接 GitLab」是**新增功能**，
上游没有需求同步的实现可抄，需要新建（GitLab Issues API 与 GitHub 形状不同：
用 `GET /projects/:id/issues` + `private_token`，状态与标签映射也不同）。

### 第 24 项「执行机器」保留，但页面内容需要拆

截图⑩ 你标的是「执行机器**我需要**」，其余（角色定义 / 能力资产 / 环境资产 /
访问密钥 / 平台管理）不需要。

执行机器页面（`web/app/runners.js`）的内容是**混合**的：

| 页面元素 | 依赖 | 个人版 |
|---|---|---|
| 机器清单（名称、地址、在线状态） | `runners` 表 | **保留** |
| CLI 探测（装了哪些 Agent CLI、版本号、二进制路径） | runner daemon 的 `refreshRuntimes` + `internal/runner/detect.go` | **保留** |
| CLI 登录态（用账号登录还是 API Key）、可用模型探测 | `internal/runner/login_models.go`、`/api/runners/:id/models/probe` | **保留** |
| 运行限额、按顶层 LOOP 计名额 | `runnerRunLimitHTML` | **删**（没有 LOOP 就没有名额） |
| 「共享项目」按钮、平台级名额 | `runner_sharing.go`、`platform_runner_settings` | **删**（团队功能） |
| 平台 runner 栏 / 平台默认名额 | `web/app/runners.js` 的 platform 视图 | **删** |
| 任务下发（claimLoop / executeTask / executeInvocation / executeTerminal） | `internal/runner/daemon.go` 的执行半边 | **删** |
| 工作区、沙箱、产物传输、仓库缓存 | `internal/runner/{workspace*,run_sandbox,artifact_transfer,repository_*}.go` | **删** |

`internal/runner` 的 15,840 行里，**执行那半边随 LOOP 一起删**，
保留注册、心跳、CLI 探测、CLI 登录态、CLI 升级维护。
`cmd/agentflow-runner` 二进制保留，但作为「机器注册客户端」而不是「任务执行器」。

### 最后一批截图带来的一个后果（已核实，不阻塞）

**访问密钥删掉不影响 omp 推送**：访问密钥页管的是 `api_tokens` 表，
而 omp 推送用的是 `insight_sources` 表的数据源凭据 ——
两张不同的表。数据源凭据在统计页的「外部数据源」对话框里管理
（`web/app/observability.js:1626`），该入口保留。

---

## 由决策推出的技术结论（均为代码实测，非推测）

### T1 — 删掉约 12.5 万行后剩下的东西

| 模块 | 非测试行数 | 去留 |
|---|---|---|
| internal/engine | 17,788 | **删**（LOOP 执行引擎） |
| internal/runtime | 24,690 | **删**（沙箱 + CLI 执行器） |
| internal/httpapi 聊天/运行/流程/定时/IM 相关 | ~69,932 | **删** |
| internal/{schedule,botinteraction,notification,tracker,evolution} | 12,569 | **删** |
| internal/repo | 30,308 | 保留但大幅瘦身 |
| internal/domain | 12,766 | 保留但大幅瘦身 |
| internal/knowledge | 11,620 | **删** |
| internal/tracker | 3,742 | **部分删**：GitHub 需求同步保留，企微/TAPD/工蜂删 |
| internal/mcpserver | 876 | **待定**（见下） |

**`internal/tracker` 不能整体删** —— GitHub 需求同步在里面：
- `internal/tracker/github.go:260 pullGitHubIssues`：
  `GET /issues?state=all&per_page=100`（跳过带 `pull_request` 的 PR），
  落成 `domain.Issue{ID:"gh-<repoID>-<number>", Source:"github",
  Kind:"issue", Status:"todo"/"closed", ExternalRef{...}}`
- 入口：`internal/tracker/service.go:295 PullAll`（全部仓库）、
  `:366 PullScheduleJob`（按来源）
- 需要的接口：`IssueStoreAdapter`（`service.go:62`）里的
  `Issue / PutIssue / PutSyncedIssue / Repositories / Repository / Projects / Project`
- 删掉的是：TAPD（`tapd_url.go`、`pullTAPDIssues`）、工蜂/GongFeng
  （`pullGongfengIssues`、`CreateGongfengIssue`）、MR 交付状态回写
  （`merge_mr.go` / `mr_delivery_sync.go` / `mr_state_lookup.go`，
  这些服务于 LOOP 的「发起 MR」节点）、`catalog.go` 里的 MCP 绑定
  （MCP 能力资产已删）、`delivery.go`、`mcp_client.go`

**`internal/mcpserver` 待定**：它是把平台自己的 HTTP API 以 OpenAPI 契约
暴露成 MCP 工具（`internal/mcpserver/schema.go` 读 `docs/api/openapi.yaml`，
`server.go` 用 `application.Operation` 调度）。它的使用者是
**聊天层的 agent 工具**（`assistant.go`、`agent_tool_mcp.go`）。
聊天层删掉后它没有消费者 —— 但如果保留 MCP API，
它就是 omp 或其他外部 agent 访问 MuseForge 的入口。
**这一项我需要你定，见决策记录末尾的提问。**

### T2 — 统计页接 omp 用的现成机制

上游有「外部数据源」机制，正好是这条桥：

- `POST /api/insights/v1/ingest` —— Bearer Token 鉴权，收 JSON
- `domain.InsightExecution`（`internal/domain/insights.go:37`）：
  `id / type=workflow_execution / externalExecutionId / workflow{id,name} /
  startedAt / finishedAt / status / usage[]`
- `domain.InsightUsage`（同文件 `:25`）：
  `provider / model / inputTokens / outputTokens / cacheReadTokens /
  cacheWriteTokens / reasoningTokens / costUsd / costStatus`
- `internal/repo/analytics_insights.go:16` 把外部记录**投影成内存里的
  `domain.Run`**（ID 形如 `external:<rowID>`），喂给同一个统计引擎
- 落库表：`insight_sources`（数据源凭据）、`insight_executions`（上报记录）

→ 统计引擎**一行不改**就能吃 omp 的数据。

### T3 — omp 侧的数据和推送点都现成

- **数据**：`~/.omp/stats.db` 的 `messages` 表有
  `model / provider / input_tokens / output_tokens / cache_read_tokens /
  cache_write_tokens / cost_total / duration / timestamp`；
  `tool_calls` 表有工具调用明细。
- **推送点**：omp 有 hook/extension 机制
  （`omp.sh/docs/hooks`，`packages/coding-agent/src/extensibility/`），
  事件包含 `session_start / agent_start / agent_end / turn_end /
  session_shutdown / tool_call / tool_result`。
  扩展可 `default export function(pi) { pi.on("session_shutdown", ...) }`。
  项目级发现路径：`<cwd>/.omp/hooks/pre/*.{ts,js}` 和 `<cwd>/.omp/hooks/post/*.{ts,js}`；
  用户级：`~/.omp/agent/hooks/{pre,post}/*.{ts,js}`。

→ 写一个 omp extension，在 `session_shutdown` 时把本 session 的用量聚合成一条
`workflow_execution` POST 到 MuseForge 的 ingest 接口即可。

### T4 — 换 SQLite 的可行性

- `internal/repo/repo.go:2` 注释明确：「MySQL is the only production datasource;
  tests open an in-process sqlite database through the same gorm code paths」
- 已有 `isMySQL()` 分支 38 处、`sqlJSONText`/`locking` 等可移植性 helper
- **MySQL 专属 SQL（`JSON_TABLE`/`JSON_UNQUOTE`/`JSON_EXTRACT`/`JSON_SET`）
  全部集中在要删或要重写的文件里**：
  `analytics.go` / `analytics_activity_usage.go` / `analytics_dashboard.go` /
  `analytics_dashboard_signals.go` / `assistant*.go`（删）、
  `runners.go` / `runs.go` / `runs_sop_detection.go`（删）
- 剩下的 `issues` / `projects` / `user_configs` / `insight_*` 表，
  JSON 查询用 `sqlJSONText` helper 即可跨库

→ 换 SQLite 的实际工作量集中在「重写统计聚合」这一处，不是全库重写。

### T5 — GitHub / GitLab 连接能力在代码里的位置

**GitHub 需求同步：上游现成，保留。**
- `internal/tracker/github.go:260 pullGitHubIssues` —— 导入实现
- `internal/tracker/service.go:295 PullAll`、`:366 PullScheduleJob` —— 入口
- `internal/httpapi/issues.go` 的 `source` 取值含 `github`
- `internal/repo/models.go:306 issueRow` 已有 `source` / `external_id` /
  `kind` / `assignee_user_id` 字段，GitHub 同步落在这套结构里

**GitLab 需求同步：上游没有，是新增功能。**
上游的 GitLab 只出现在两处，都不是需求同步：
- `internal/httpapi/git_http_credentials.go:120` ——
  「平台只能是 github、gitlab 或 other」，这是 **git 凭据**（clone/push 用）
- `internal/tracker/external_mr.go` / `internal/repo/../domain/mr.go:27`
  （`MRProjectGitLab = "gitlab:"`）—— **MR 状态查询**

`Issue.Source` 的完整取值是 `local / gongfeng / tapd / github` ——
**没有 gitlab**。

→ 若要「连接 GitLab」，需要新建：GitLab Issues API 用
`GET /projects/:id/issues` + `private_token`（或 OAuth），
返回形状与 GitHub 不同（无 `pull_request` 字段需要跳过），
状态映射与标签体系也不同。这是新工作量，不是裁剪。

**要删的外部源**：GongFeng（工蜂）和 TAPD —— 企业内网，个人版不需要。

### T6 — 鉴权免登录的安全边界

- `internal/httpapi/auth_tokens.go:25 authRequired()` 读 `AGENTFLOW_AUTH_REQUIRED`，
  默认 false 时**所有写守卫放行**（`requireProjectWriter` /
  `requirePlatformAdmin` 直接 return true）
- `internal/httpapi/auth_tokens.go:165 previewLocalAuthUser` 提供 loopback 免登录路径
- `AGENTFLOW_AUTH_REQUIRED=false` + `LOOPFORGE_BIND=127.0.0.1` =
  本机免登录，且不对外暴露

→ 免登录可行，但必须**强制绑 127.0.0.1**，否则等于裸奔。

---

## 裁剪后的最终形态

```
MuseForge
├── 需求清单      标题/描述/状态/优先级/项目归属/标签/备注 + GitHub 同步
├── 项目分组      项目列表、新建项目（无成员、无业务域）
├── 个人配置      模型 Key、GitHub 授权、SSH/HTTP 凭据
├── omp 用量台    Token/成本/缓存命中/趋势/需求关联（数据来自 omp 推送）
├── 执行机器      机器清单 + CLI 探测/版本/登录态（无任务下发、无共享、无名额）
└── omp 扩展      session_shutdown 时把用量 POST 到 MuseForge
```

数据库：SQLite 单文件。部署：单容器。

---

## 待你确认（我查证代码后发现的偏差，不擅自决定）

**Q-A. 「GitLab 需求同步」是新增功能，不是裁剪。**
上游只有 GitHub 的需求导入（`internal/tracker/github.go:260`）。
GitLab 在上游只用于 git 凭据和 MR 状态查询。
选项：① 只做 GitHub（省掉新开发）② 也新建 GitLab 需求同步（新工作量）。

**Q-B. 需求「标签」字段上游不存在**，需要新建。
「备注」上游有，但是评论表（`issue_comments`）里的评论，不是需求上的文本字段。

**Q-C. `internal/mcpserver`（平台自己的 MCP API）留不留？**
它把 MuseForge 的 HTTP API 以 OpenAPI 契约暴露成 MCP 工具，
供外部 agent 调用。上游的消费者是聊天层，聊天层删掉后就没有内部消费者了。
但如果保留，omp 就能通过 MCP 直接读写 MuseForge 的需求和项目。
选项：① 删（最轻量）② 保留，作为 omp 访问 MuseForge 的正式通道。

**Q-D. 执行机器页面里「共享项目 / 运行名额 / 平台默认名额」这三个团队元素删掉后，
页面只剩「机器清单 + CLI 环境」。这个形态你接受吗？**
如果接受，执行机器页面会明显变简单 —— 这正是它变轻量的方式。
