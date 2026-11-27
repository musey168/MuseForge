# MuseForge 功能取舍清单（截图驱动）

> 目的：把「你在旧版服务器上看到的界面」逐项映射到代码，最终产出一张
> **保留 / 删除 / 待定** 的功能表，作为裁剪团队功能的唯一依据。
>
> 用法：对着旧版服务器的界面截图，勾选每项。截图直接丢进
> `docs/personal-scope/screenshots/`，文件名写成本文档里的编号，例如
> `04-approvals.png`。

---

## 0. 怎么用这张表

| 你的操作 | 我做的事 |
|---|---|
| 截一张图，扔进 `screenshots/` | 我按截图定位对应的前端文件、HTTP 路由、DB 表 |
| 在「你的选择」列写 `留` / `删` / `不确定` | 我把它变成裁剪任务，写进 `docs/personal-scope/decision.md` |
| 截图里出现这张表没有的功能 | 我补一行进去，不猜 |

**默认规则**（除非你明确说删，否则按「留」处理）：
- 跟「一个人写代码 → 交付」这条主链路有关 → 留
- 只在「多个人一起用」时才成立 → 待定，等你的截图
- 纯企业/组织形态（部门、SSO、业务域报表） → 删

---

## 1. 主界面（左侧图标栏，5 项）

| # | 界面 | 上游路径 | 你的选择 | 截图 |
|---|---|---|---|---|
| 01 | **AI 工作台**（主聊天页） | `web/app/workbench.js`、`internal/httpapi/assistant_*.go` | 留 | `01-` |
| 02 | **统计** | `web/app/observability*.js`、`internal/repo/analytics_*.go`（14 个文件） | | `02-` |
| 03 | **需求列表** | `web/app/issues*.js`、`internal/httpapi/issues.go` | | `03-` |
| 04 | **待我确认** | `web/app/approvals.js`、`internal/httpapi/runs.go` | | `04-` |
| 05 | **设置**（左侧展开面板） | `web/app/access.js` | | `05-` |

> **02 统计**需要单独说明：上游把「效能指标 / 成员每日 / 业务域报表」和
> 「成本与 Token 用量」揉在同一套 API 里。成本统计是个人功能，必须留；
> 成员维度是团队功能，删掉后统计页会只剩你自己的数据。
> → 请截图告诉我：**你平时看统计页里的哪几块？**（我按截图保留对应部分）

---

## 2. 设置面板里的功能项（左侧 `settings` 下）

| # | 功能 | 上游路径 | 你的选择 | 截图 |
|---|---|---|---|---|
| 06 | **个人配置**（自己的模型 Key、GitHub 连接、默认模型） | `web/app/workbench.js` 的 `renderPersonalSettings`、`internal/repo/user_configs.go` | 留 | `06-` |
| 07 | **AI 助手**（助手模型、偏好、Skills） | `web/app/assistant-settings.js` | | `07-` |
| 08 | **项目配置**（项目、成员、加入申请、业务域） | `web/app/projects.js`、`internal/httpapi/project_join_requests.go` | | `08-` |
| 09 | **通道管理**（企微 / 飞书 / 钉钉机器人） | `web/app/bots.js`、`internal/botinteraction/` | | `09-` |
| 10 | **凭据管理**（项目级凭据） | `web/app/project-credentials.js` | | `10-` |
| 11 | **流程模板**（LOOP 画布） | `web/app/flows.js`、`internal/httpapi/catalog.go` | 留 | `11-` |
| 12 | **运行记录** | `web/app/runs.js` | | `12-` |
| 13 | **定时任务**（AI 定时任务） | `web/app/sync-schedule.js`、`internal/schedule/service.go` | | `13-` |
| 14 | **角色定义** | `web/app/agents.js` | 留 | `14-` |
| 15 | **能力资产**（MCP / Rules / Skills） | `web/app/mcp.js` | | `15-` |
| 16 | **我的知识库** | `web/app/knowledge.js` | | `16-` |
| 17 | **RSI 工作台**（资产进化） | `web/app/evolution-*.js`、`internal/evolution/` | | `17-` |
| 18 | **执行机器**（Runner） | `web/app/runners.js` | | `18-` |
| 19 | **沙箱管理** | `web/app/flows.js` 内的沙箱配置 | | `19-` |
| 20 | **访问密钥**（API Token / MCP 接入） | `web/app/access.js` | | `20-` |
| 21 | **平台管理**（成员账号 / 平台管理员 / 共享 Key / Git 主机信任 / 外部访问地址） | `web/app/workbench.js` 的 `renderPlatformAdminPanel` | | `21-` |

> **08 项目配置**和 **21 平台管理**是团队功能的大本营。一个项目里现在可以有：
> 多个成员（维护者/成员/只读）、加入申请审批、业务域归属。
> → 单人版我的初步方案是：**保留项目和项目内资产，砍掉成员与加入申请**。
> 但你截图里如果显示你确实用「成员」区分权限，请告诉我。

---

## 3. 需要单独确认的团队功能（截图里可能不明显）

| # | 功能 | 涉及代码 | 你的选择 | 截图 |
|---|---|---|---|---|
| 22 | **需求空间 / 协作房间**（多人在同一空间讨论、发消息、共享产物） | `internal/httpapi/spaces.go`、`rooms.go`、`internal/repo/spaces.go` | | `22-` |
| 23 | **共享任务市场**（把定时任务发布成模板给别人用） | `internal/httpapi/shared_schedules.go`、`internal/repo/shared_schedules.go` | | `23-` |
| 24 | **企业 IM 通知**（企微/飞书/钉钉推送 + 群里点卡片审批） | `internal/botinteraction/`、`internal/notification/` | | `24-` |
| 25 | **业务域与业务报表**（按部门/产品线汇总效能） | `internal/repo/business_domains.go`、`internal/httpapi/business_domains.go` | | `25-` |
| 26 | **SSO / 网关身份**（工蜂、统一登录、扫码） | `internal/auth/`、`internal/httpapi/auth.go` | | `26-` |
| 27 | **外部需求系统同步**（TAPD / 工蜂工单双向同步） | `internal/tracker/`、`internal/httpapi/tracker.go` | | `27-` |
| 28 | **执行机器共享**（一台机器授权给多个项目/成员的成员使用） | `internal/repo/runner_sharing.go` | | `28-` |
| 29 | **共享 CodeBuddy Key**（平台统一发 Key，成员共用额度） | `internal/repo/platform_admin.go`、`migrations/024_platform_codebuddy_key.sql` | | `29-` |

---

## 4. 旧版 vs 最新：可能影响你判断的差异

你服务器上是**上周的版本**，上游现在已经到 `v0.4.3`（2026-11-26）。
`v0.4.0 → v0.4.3` 之间上游改了 **1513 个文件、+14.6 万行**，界面也重做过：

| 上周（旧） | 现在（新） | 对你的影响 |
|---|---|---|
| `web/` 单文件时代 | 拆成 `web/app/*.js` 模块化 | 截图对不上新代码，我按**功能**而非**像素**定位 |
| 无「RSI 工作台」 | 新增资产进化 + RSI 工作台 | 截图里不会有，先按「待定」处理 |
| 沙箱不能限 CPU/内存 | 可配 Docker CPU/内存上限 | 个人版保留（你 8GB 内存的 Mac 需要） |
| 无特权沙箱模式 | Docker 沙箱支持 privileged | **建议删**（本地个人机不需要） |
| 无执行机器共享 | 支持共享执行机 | 团队功能，删 |
| 飞书/钉钉 Bot 不存在 | 新增 | 团队功能，删 |
| 定时任务不能带项目资产 | 支持引用项目资产 | 个人功能，保留 |
| 项目成员无加入申请 | 新增申请/审批流 | 团队功能，删 |

> **结论：不要拿旧版截图去比对新代码的界面细节。** 我只按「这个功能你用不用」来判断，
> 代码在新版里的位置由我来找。

---

## 5. 裁剪后的目标形态（先给你看，确认后我才动手）

**保留**（单人完整可用）：
对话工作台 · 需求列表 · LOOP 流程画布 · 角色定义 · 能力资产（MCP/Rules/Skills）·
待我确认（审批关口）· 运行记录与产物 · 沙箱与执行器 · 执行机器 · 凭据管理 ·
成本与 Token 统计（仅个人维度）· 定时任务（仅自己）· 我的知识库 · 访问密钥 / MCP API

**删除**（纯团队）：
成员账号管理 · 平台管理员角色 · 项目成员与加入申请 · 共享任务市场 ·
企业 IM Bot 与群审批 · 业务域与业务报表 · SSO / 网关身份 · 外部工单系统同步 ·
执行机器跨项目共享 · 共享 CodeBuddy Key · 协作房间 / 需求空间成员

**代码层面要动的地方**（不是删文件那么简单）：
1. `internal/repo/models.go` 的 `allModels()` —— 删表要改这里（GORM 自动建表）
2. `internal/httpapi/server.go` 的 `newWithNotifications()` —— 约 440 条路由注册在一处
3. `internal/httpapi/project_write_access.go` —— **最关键**，个人写入守卫和团队成员判定写在同一个文件，
   拆错会连带打挂所有运行控制（approve / reject / cancel / 终端 / 预览）
4. `internal/domain/user.go` —— 组织字段（部门）和个人字段（模型 Key）混在一个结构体
5. `internal/httpapi/assistant_shared.go` —— 共享会话与私有会话的 owner 解析混在一起
6. `internal/repo/analytics_*.go` —— 统计按 `ResponsibleUserID` 聚合，删成员维度要改聚合口径
7. `migrations/*.sql` —— **11 个迁移文件同时含团队表和个人表**，不能整份删

**做法**：按「叶子 → 核心」的顺序分阶段砍，每阶段跑 `go build` + `go test` +
前端 harness 测试，保证随时可停、可回滚。

---

## 6. 你现在要做的

1. 打开你服务器上的旧版，按上面 **#01–#21** 逐个界面截图
2. 截图存进 `docs/personal-scope/screenshots/`，按编号命名
3. 在上面表格的「你的选择」列填 `留` / `删`
4. 特别回答两个问题：
   - **统计页你平时看哪几块？**（决定成本统计保留到什么粒度）
   - **需求空间/协作房间你用过吗？**（决定要不要保留多会话协作）
