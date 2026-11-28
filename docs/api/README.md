# MuseForge HTTP API

本文档以 `internal/httpapi/server.go` 注册的路由为准，OpenAPI 契约见 [openapi.yaml](openapi.yaml)（`GET /api/openapi.yaml` 原样返回）。接口按领域划分：

- **需求**：需求列表、单条需求、评论、关闭、重新打开、删除
- **项目**：项目、项目凭据、默认代码仓库
- **代码仓库**：仓库绑定（GitHub 与 GitLab）
- **个人**：账号、模型目录、个人凭据、Git 凭据
- **机器**：机器注册、心跳、CLI 清单、模型探测、CLI 维护
- **统计台与用量**：平台自身的统计（`GET /api/stats/overview`）、外部工作流的上报数据源与执行汇总（[接入说明](../external-workflow-usage.md)）
- **跟踪器导入**：从已绑定的 GitHub 或 GitLab 仓库导入需求
- **服务**：健康检查、能力清单、bootstrap、可观测性契约

平台**没有登录**：它只监听回环地址，唯一的账号由控制台在首次打开时记录。因此所有请求都不需要会话或 Token；唯一的例外是外部工作流上报用量，它使用数据源自己的 Bearer Token。

## 基础约定

- 基础 URL 示例：`http://127.0.0.1:8090`。
- 请求与响应均为 UTF-8 JSON。写请求使用 `Content-Type: application/json`。
- JSON 请求体上限为 2 MiB。
- 时间使用 RFC 3339，例如 `2026-09-02T12:00:00Z`。
- 服务可能对普通响应启用 gzip；客户端可发送 `Accept-Encoding: gzip`。OTLP 与显式请求 `text/event-stream` 的响应不压缩。
- 列表接口为完整数组，没有通用分页；需求列表在带 `limit` 时返回分页信封。
- 浏览器跨域仅允许与 API 相同的 host；异源请求返回 `403`。预检 `OPTIONS` 返回 `204`。
- 绑定非回环地址时平台拒绝启动；这是硬规则，不是配置项。