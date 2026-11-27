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
| 24 | 执行机器 | **保留**（降级为机器清单 + CLI 环境） | 截图⑩ |

### 最后一批截图带来的两个后果（已核实，均不阻塞）

**访问密钥删掉不影响 omp 推送**：访问密钥页管的是 `api_tokens` 表，
而 omp 推送用的是 `insight_sources` 表的数据源凭据 ——
两张不同的表。数据源凭据在统计页的「外部数据源」对话框里管理
（`web/app/observability.js:1626`），该入口保留。

**执行机器保留但要降级**：Runner 的执行能力（`runner_tasks` /
`runner_task_events` 表、`internal/runner` 的 task_lease / exec /
run_sandbox / workspace_* / artifact_transfer / repository_* 共约 1.5 万行）
全部服务于 LOOP 节点分发，随 LOOP 一并删除。保留的是：机器注册与清单、
CLI 探测与版本、登录态模型、连接与维护。`internal/runner` 从 15,840 行
降到约 3,000 行。

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
| internal/mcpserver | 876 | 保留（omp 推送不需要，但 MCP API 是公开能力） |

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

- `internal/httpapi/git_http_credentials.go:120`：
  「平台只能是 github、gitlab 或 other」—— GitLab 支持是现成的
- `internal/tracker/github.go` —— GitHub tracker
- `internal/httpapi/issues.go` 的 `source` 取值：
  `local` / `github` / `gongfeng` / `tapd`（GongFeng 和 TAPD 是企业内网，
  个人版不需要，删）
- `internal/repo/models.go:306 issueRow` 已有 `source` / `external_id` /
  `kind` / `assignee_user_id` 字段，GitHub 同步落在这套结构里

→ GitHub/GitLab 需求连接保留，去掉 GongFeng/TAPD。

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
├── 需求清单      标题/描述/状态/优先级/项目归属/标签/备注 + GitHub/GitLab 同步
├── 项目分组      项目列表、新建项目（无成员、无业务域）
├── 个人配置      模型 Key、GitHub 授权、SSH/HTTP 凭据
├── omp 用量台    Token/成本/缓存命中/趋势/需求关联（数据来自 omp 推送）
├── 访问密钥      API Token（给 omp 推送和外部调用）
└── omp 扩展      session_shutdown 时把用量 POST 到 MuseForge
```

数据库：SQLite 单文件。部署：单容器。