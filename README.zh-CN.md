# MuseForge

**一个人的项目工作台。它记录你的工作，不替你执行。**

MuseForge 是 [Tencent/LoopForge](https://github.com/Tencent/LoopForge) 的
**单人精简版**。团队协作面已经删除 —— 成员账号、项目成员与加入申请、共享任务市场、
企业 IM 机器人、业务域报表、SSO 与网关身份；Agent 执行面同样删除。留下来的是一个人
真正会用的东西：

- **需求清单** —— 你的待办条目，带项目、状态、优先级、标签、备注，以及指向
  GitHub / GitLab 工单的链接
- **项目分组** —— 需求的归属容器，没有成员名册
- **个人配置** —— 模型 API Key、GitHub 设备码授权、SSH 与 HTTP Git 凭据
- **执行机器** —— 你的机器上装了哪些 Agent CLI、版本号、登录态、能触达哪些模型
- **统计台** —— 一页看清两件事：你自己的数据（项目、需求、状态与来源分布、创建/关闭
  趋势、标签、仓库、机器），以及接入的 Agent 花掉的 Token 与费用
- **MCP / HTTP API** —— 外部 Agent（omp、Claude Code、Codex、Cursor、OpenCode）
  读写你需求的正式通道

**它不执行任何东西。** 没有 LOOP、没有沙箱、没有执行机器任务队列、没有产物预览、
没有定时任务、没有知识库、没有聊天工作台。工作发生在你自己的 Agent 里；MuseForge
只负责记账。

这是一个**独立仓库**。不再跟随上游，也不同步上游更新。分支约定：`dev` 开发，
`stable` 发布。上游 MIT 许可证与腾讯原始版权声明保留在 [LICENSE](./LICENSE)；
MuseForge 自己的版权覆盖个人改动部分。

Fork 点：上游 `Tencent/LoopForge` 的 `v0.4.3`（commit `fd039e51`），在本仓库以
标签 `upstream-v0.4.3` 标记。

---

## 快速开始

一个容器、一个 SQLite 文件，不需要数据库服务。

```bash
# 从源码构建并运行
make build
./bin/agentflow -datasource file:./museforge.db -addr 127.0.0.1:8080
```

首次启动时平台按模型清单建表 —— 没有迁移步骤。打开
<http://127.0.0.1:8080> 直接进入需求清单，没有登录页。

用 Docker：

```bash
bash scripts/configure.sh          # 写入 .env：数据源与各项密钥
docker compose up -d --wait
```

数据源只有一个设置项：

| 设置 | 含义 |
|---|---|
| `-datasource` / `AGENTFLOW_DATASOURCE_DSN` | SQLite 用 `file:/path/to/museforge.db`，或填 MySQL DSN。`file:` URI、裸路径、MySQL DSN 三种形态平台能自行区分。 |
| `-addr` / `AGENTFLOW_HTTP_PORT` | 监听地址。**只绑回环地址**，绑定其它地址会被拒绝启动。 |
| `AGENTFLOW_SESSION_SECRET` | 必填。 |
| `AGENTFLOW_AUTH_REQUIRED` | 个人版填 `false` —— 没有登录。 |

---

## 统计台与 Agent 用量

你的 Agent 会话本来就在花钱；MuseForge 负责把账摆出来。omp 把每次会话的逐模型
用量记在自己的统计库里，一个小 **omp 扩展**在会话结束时把这次会话汇总成
**一条执行记录**上报到 MuseForge。

```bash
# 1. 安装扩展
bash omp-extension/scripts/install.sh --user     # 或 --project /path/to/project

# 2. 指向平台，并给它一个上报凭据
bash omp-extension/scripts/install.sh --print-config > ~/.config/museforge/omp-usage.json
```

上报凭据在统计台的**数据源**对话框里创建 —— 它与登录状态无关，可撤销，也会过期。
同一张页面还给出 Codex / pi / Claude Code 的回填命令（`museforge-usage`），以及四个
CLI 的 MCP 配置片段 —— 接上之后 Agent 可以直接读写需求与项目。

扩展每次会话结束都重新读取配置，所以换 Token 不需要重装扩展。没有配置凭据时它
什么也不上报；上报失败也绝不会打断它正在汇报的那次会话。

安装路径、配置项，以及一次会话的用量如何映射成一条上报记录，详见
[`omp-extension/README.md`](./omp-extension/README.md)。设计依据见
[`docs/adr/0003-omp-usage-console.md`](./docs/adr/0003-omp-usage-console.md)。

---

## 文档

| 文档 | 内容 |
|---|---|
| [`docs/personal-scope/spec.md`](./docs/personal-scope/spec.md) | 本次裁剪实现的规格 |
| [`docs/personal-scope/decision.md`](./docs/personal-scope/decision.md) | 保留了什么、删掉了什么、为什么 —— 范围的权威 |
| [`docs/personal-scope/baseline-tests.md`](./docs/personal-scope/baseline-tests.md) | 裁剪前实测的测试基线 |
| [`docs/architecture.md`](./docs/architecture.md) | 个人版是怎么搭起来的 |
| [`docs/adr/`](./docs/adr/) | 决定这次形态的几个决策：裁剪深度、控制台定位、统计台、只用 SQLite、免登录且只绑回环 |
| [`docs/api/`](./docs/api/) | HTTP API 及其 OpenAPI 契约 |
| [`docs/deployment.md`](./docs/deployment.md) | 怎么部署运行 |
| [`docs/testing/`](./docs/testing/) | 测试套件与运行方式 |
| [`GLOSSARY.md`](./GLOSSARY.md) | 词汇表，以及本版本不再使用的上游词汇 |

---

## 开发 MuseForge

```bash
make build          # 平台二进制
make test           # Go 测试
npm --prefix webtest test    # 控制台的 DOM 测试
```

`AGENTS.md` 是仓库的规矩。`docs/personal-scope/decision.md` 是范围的权威：代码和
决策记录对「应该存在什么」有分歧时，以决策记录为准。

---

## 与上游的关系

MuseForge 起始于 [Tencent/LoopForge](https://github.com/Tencent/LoopForge) —— 一个
团队研发工作台 —— 后来被裁剪成一个人的用法。上游 MIT 许可证与腾讯原始版权声明
保留在 [LICENSE](./LICENSE)，个人改动部分由 MuseForge 自己的版权覆盖。上游仍然是
本版本保留下来的需求模型、凭据保管、git 集成与机器探测能力的来源。
