# omp 用量上报扩展

omp 每次会话的逐模型用量已经记在 `~/.omp/stats.db` 里。这个扩展在 omp 的
`session_shutdown` 事件上把本次会话聚合成**一条执行汇总**，POST 到 MuseForge 的
`/api/insights/v1/ingest`，用量台（「用量分析 → 外部数据源」）随即出现这次会话，
不需要手工记账。

扩展是**可安装的独立文件**，不属于平台二进制：平台只提供上报接口，扩展放在 omp 的扩展
目录里。设计依据见 `docs/adr/0003-omp-usage-console.md`。

```
omp-extension/
├── src/museforge-usage.ts       扩展本体（单文件，无依赖）
├── scripts/install.sh           复制到 omp 扩展目录
├── tests/museforge-usage.test.mjs
└── README.md
```

## 1. 安装

先把扩展文件复制到 omp 的扩展目录。两个位置二选一：

| 级别 | 路径 | 安装命令 |
|---|---|---|
| 用户（所有项目） | `~/.omp/agent/extensions/museforge-usage.ts` | `bash omp-extension/scripts/install.sh --user` |
| 项目（仅当前项目） | `<项目>/.omp/extensions/museforge-usage.ts` | `bash omp-extension/scripts/install.sh --project /path/to/project` |

安装脚本把绝对路径打印到 stdout，重复执行会覆盖同名文件。`omp` 启动时加载扩展目录里的
`*.ts`／`*.js` 文件；`--user` 读取 `OMP_HOME`（默认 `~/.omp`），也可以用 `--dir PATH`
指定任意扩展目录。omp 用其它 profile（隔离 home）时，装到那个 profile 的扩展目录里。

## 2. 配置

扩展每次会话结束都重新读取配置，所以改配置、换 Token 都不用重装扩展。

| 键 | 含义 |
|---|---|
| `url` | MuseForge 基础地址，如 `http://127.0.0.1:8080`；上报路径固定追加 `/api/insights/v1/ingest` |
| `token` | 数据源凭据（`afi_…`），只放在 `Authorization: Bearer` 头里 |
| `statsDb` | omp 统计库路径，默认 `~/.omp/stats.db`；omp 用隔离 profile 时改这里 |
| `timeoutMs` | 单次上报的等待上限，默认 `1500`，最大 `10000` |
| `debug` | `true` 时把失败原因写到 stderr；默认不输出任何日志 |

配置来源按优先级从低到高合并（后者覆盖前者的同名键）：

1. 用户级文件 `~/.config/museforge/omp-usage.json`（固定路径，不读 `XDG_CONFIG_HOME`）
2. 项目级文件 `<cwd>/.omp/museforge-usage.json`，`cwd` 是 omp 会话的工作目录
3. 环境变量 `MUSEFORGE_URL`、`MUSEFORGE_INSIGHT_TOKEN`、`MUSEFORGE_STATS_DB`、
   `MUSEFORGE_TIMEOUT_MS`、`MUSEFORGE_DEBUG=1`

模板：

```sh
mkdir -p ~/.config/museforge
bash omp-extension/scripts/install.sh --print-config > ~/.config/museforge/omp-usage.json
```

```json
{
  "url": "http://127.0.0.1:8080",
  "token": "afi_XXXXXXXXXXXXXXXXXXXXXXXX",
  "statsDb": "~/.omp/stats.db",
  "timeoutMs": 1500
}
```

Token 在用量台的「用量分析 → 外部数据源」里创建：选项目、填名称和有效期，创建时只显示一次。
它只能上报，不能读取或管理平台资源。

**撤销**：在同一个对话框里撤销数据源即可。之后扩展继续尝试上报，平台返回 401，扩展静默丢弃；
不需要改动或删除扩展。换 Token 只要改配置，下一次会话就生效。

## 3. 行为保证

- **没有配置 Token 就不上报**：不联网、不写日志、不影响会话。
- **上报失败不上抛**：处理函数内的所有异常都被吞掉，`session_shutdown` 不会被推迟或打断；
  单次上报有 `timeoutMs` 上限（默认 1.5 秒），超时即放弃，不重试。
- **下一次会话照常上报**：扩展不保存失败状态。
- **不跟随重定向**：凭据只在请求头里，重定向直接按失败处理。
- **幂等**：记录 ID 由会话文件派生（`omp-session:<会话文件名>`），同一会话重复上报时，
  平台按「同 ID 同内容」返回 `200 duplicate`，不会重复计数；内容不同则返回 `409`，
  平台保留原事实，扩展静默放弃（不覆盖、不改 ID 重报）。

## 4. 上报口径

- 只取 `messages` 表里 `session_file` 等于本次会话文件的行，按 (`model`, `provider`) 分组。
- `inputTokens` = omp 的 `input_tokens` + `cache_read_tokens` + `cache_write_tokens`：
  上报契约要求输入**包含**缓存读写，而 omp 的 `input_tokens` 不含缓存。
- `outputTokens` = `output_tokens`；omp 不记录推理 token，`reasoningTokens` 为 0（可省略的细分）。
- `costUsd` 取 `cost_total` 汇总，`costStatus`：
  - 该组没有 `cost_unpriced` 行 → `reported`（包括明确为 0 的免费额度，不当作未知）；
  - 有未定价行且金额大于 0 → `estimated`（是下界，不是完整价）；
  - 有未定价行且金额为 0 → `unknown`，`costUsd` 为 `null`（缺失不等于免费）。
- `startedAt` / `finishedAt` = 该会话最早/最晚一条记录的 `timestamp`；
  `executionDurationMs` = 各条 `duration` 之和（模型调用耗时，不用墙钟代替）。
- `status`：任一行 `stop_reason=error` 或带 `error_message` → `failed`；
  否则有 `stop_reason=aborted` → `cancelled`；否则 `succeeded`。
- 会话没有任何用量行（例如还没调用模型）就不上报。

## 5. 自测

扩展按脚本测试：聚合逻辑跑在真实的 `messages` 表结构上，上报路径对着本地真实 HTTP 服务。

```sh
node --test "omp-extension/tests/*.test.mjs"
```

覆盖：多模型聚合、Token/成本/时长/状态的字段口径、ID 稳定（重试幂等）、
未配置凭据不联网、500／连接拒绝／重定向／挂起超时都不抛错且不阻塞、
配置来源优先级、处理函数注册与容错、安装脚本的目标路径。

## 6. 排查

`debug: true`（或 `MUSEFORGE_DEBUG=1`）会把失败原因写到 stderr，形如
`[museforge-usage] report for <会话 ID> was not accepted`。常见原因：

- 401：Token 已撤销、已过期，或复制不完整；
- 400：上报内容超出契约（扩展按契约生成，出现即说明会话数据异常）；
- 连不上：`url` 写错，或 MuseForge 没在运行。

平台侧可用「用量分析 → 外部数据源」的拒绝计数确认上报是否到达但被拒。
