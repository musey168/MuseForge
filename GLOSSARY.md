# MuseForge

MuseForge 是 LoopForge 的个人版：一个人在自己的机器上管理项目、需求和用量，
并把外部 Agent（omp）的工作记录下来。

它**不执行 Agent**。执行交给 omp，MuseForge 负责记录与配置。

## Language

**需求（Requirement）**:
要交付的一件事。MuseForge 里它是待办条目：标题、描述、状态、优先级、
所属项目、备注，可以从 GitHub 导入，可以连到 GitHub / GitLab 仓库。
_Avoid_: Issue, 任务, 工单

**备注（Note）**:
需求下的评论。上游存放在 `issue_comments` 表，是一条条追加的评论，
不是需求上的一个文本字段。
_Avoid_: 备注字段, Description

**项目（Project）**:
一组需求的归属容器。个人版里项目只是分组，没有成员和权限。
_Avoid_: 工作区, 空间, 团队

**用量台（Usage Console）**:
统计页。展示 omp 上报过来的 Token、成本、缓存命中和趋势。
_Avoid_: 统计, 报表, 效能看板

**数据源（Data Source）**:
一个允许向 MuseForge 上报用量的凭据。omp 推送时使用。
存在 `insight_sources` 表，与访问密钥（`api_tokens`）是两回事。
_Avoid_: 访问密钥, Token, 集成

**执行机器（Runner）**:
登记在 MuseForge 里的开发机器。个人版只用来查看机器清单和它们安装的
Agent CLI 环境，不接受任务分发，也没有共享与名额概念。
_Avoid_: Worker, 节点, 执行器

**个人配置（Personal Settings）**:
属于使用者本人的凭据与偏好：模型 Key、GitHub 授权、SSH/HTTP 凭据。
_Avoid_: 账号设置, 用户管理

**上游 LoopForge**:
MuseForge 的来源项目。它是团队工作台，覆盖多人协作。
MuseForge 是它的个人精简版，不含任何多人协作能力。
_Avoid_: 原版, 官方版

---

**上游术语（MuseForge 已不再使用，仅为对照）**：
LOOP、Run、角色、执行器、沙箱、人工关口、产物、定时任务、知识库。
