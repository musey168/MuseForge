# 统计页改为 omp 用量台，走上游外部数据源机制

MuseForge 不再执行 Agent，统计的数据来源从平台内的 runs 变成 omp 的会话用量。
复用上游已有的「外部数据源」机制，不自造上报协议。

**机制**（上游现成）：
`POST /api/insights/v1/ingest` 收 `domain.InsightExecution`
（`type=workflow_execution`，含 `workflow`、`status`、起止时间、`usage[]`
逐模型的 input/output/cache token 与 `costUsd`）；
`internal/repo/analytics_insights.go` 把外部记录投影成内存里的 `domain.Run`
喂给同一个统计引擎，所以趋势图、Token 汇总、成本汇总照常工作。
数据源凭据存在 `insight_sources` 表，与 `api_tokens`（访问密钥页）是两张表 ——
访问密钥页被删除不影响本机制。

**推送端**：写一个 omp extension，在 `session_shutdown` 时把本 session 的
用量聚合成一条 `workflow_execution` POST 过去。omp 的用量数据现成：
`~/.omp/stats.db` 的 `messages` 表已有逐模型 token 与 cost。

**因此删除的统计卡片**：成功率、P95 执行、工具调用成功率、平均人工干预、
自动完成率、累计运行、平均 Token/Run —— 这些全部统计平台内的 runs，
在个人版恒为 0，删掉比显示空值诚实。

**保留的统计卡片**：Token 用量、成本、缓存命中、趋势变化、需求关联。
