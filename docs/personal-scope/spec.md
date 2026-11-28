# MuseForge：个人项目工作台（单人版裁剪）

## Problem Statement

LoopForge 是腾讯开源的**团队**工作台：它把「需求 → LOOP 流程 → 多角色 Agent 执行 → 人工关口 → 交付」这条链路产品化，一个人用的时候，其中约 12.5 万行代码、数十张数据库表和整套多用户权限体系都是纯负担 —— 成员账号、项目成员与加入申请、共享任务市场、企业 IM 机器人、业务域报表、SSO/网关身份，全部只有一个使用者却要维护。

同时，使用者日常已经在 Mac 本地的 `omp` 里写代码，LoopForge 自带的执行引擎（LOOP 派发、Docker 沙箱、Runner 任务分发、10 种 Agent CLI 执行器）是重复建设；而 LoopForge 里唯一真正有价值的部分 —— 需求清单、项目分组、凭据配置、机器清单与用量统计 —— 又被团队功能和执行引擎夹在中间，难以单独使用。

结果是：想要一个**能长期自己维护**的个人项目工作台，只能从零写；而从零写又丢掉了上游已经打磨好的需求管理、凭据托管、Git 集成、机器探测这些能力。

MuseForge 的目标是把上游 LoopForge 裁成一个人用得上的样子：**删掉一切团队协作面与 Agent 执行面，保留并加强一个人真正需要的能力**，并且让整个项目收敛到「一个容器、一个数据库文件、一个使用者」的形态。

## Solution

MuseForge 是 LoopForge 的单人精简版，定位为**个人项目工作台**。

它提供：

- **需求清单** —— 个人待办。支持从 GitHub / GitLab 导入工单，带标题、描述、状态、优先级、项目归属、标签、评论。
- **项目分组** —— 需求与资产的分组容器。没有成员、没有角色、没有加入申请。
- **个人配置** —— 模型 Key、GitHub 授权、SSH / HTTP 凭据、Git 主机信任。
- **执行机器** —— 登记的开发机器清单：装了哪些 Agent CLI、版本、二进制路径、登录态、可探测到的模型。不下发任何任务。
- **omp 用量台** —— 统计 omp 汇报上来的 Token、成本、缓存命中、趋势与需求关联。
- **MCP / HTTP API** —— 外部 agent（含 omp）读写 MuseForge 的正式通道。

它**不再执行任何 Agent**：LOOP 派发、沙箱、Runner 任务分发、人工关口、聊天工作台、定时任务、知识库、企业 IM 全部删除。

它**只服务一个人**：无账号体系、无登录页、无成员授权，监听回环地址；数据落在一个 SQLite 文件里。

## User Stories

1. As a solo developer, I want a single place to see what I owe myself, so that I know what to work on next.
2. As a solo developer, I want to write a requirement with a title and description, so that the idea is captured before I forget it.
3. As a solo developer, I want to give a requirement a priority, so that I can order my work.
4. As a solo developer, I want to give a requirement a status, so that I can see what is pending, in progress and done.
5. As a solo developer, I want to group requirements into projects, so that unrelated work does not mix in one list.
6. As a solo developer, I want to add a tag to a requirement, so that I can find related work later.
7. As a solo developer, I want to comment on a requirement, so that I can record notes and decisions against it.
8. As a solo developer, I want to close and reopen a requirement, so that the list reflects reality.
9. As a solo developer, I want to delete a requirement I no longer need, so that my list stays meaningful.
10. As a solo developer, I want to import my open issues from GitHub, so that I do not maintain the same task twice.
11. As a solo developer, I want the GitHub import to skip pull requests, so that only real work items land in my list.
12. As a solo developer, I want the GitHub import to preserve my local edits to an imported requirement, so that a re-import does not wipe my notes.
13. As a solo developer, I want to import my issues from GitLab, so that I keep one list regardless of where the work is tracked.
14. As a solo developer, I want the GitLab import to map GitLab's issue types and states onto my statuses, so that the list reads consistently.
15. As a solo developer, I want to see when the last GitHub or GitLab import ran and whether it failed, so that I know my list is current.
16. As a solo developer, I want to see which repositories a project is connected to, so that I know what the requirements refer to.
17. As a solo developer, I want to connect a GitHub or GitLab repository over HTTPS or SSH, so that git operations use my own credentials.
18. As a solo developer, I want to store my model API keys once, so that every tool that needs a model uses the same key.
19. As a solo developer, I want to authorize GitHub with a device code, so that I do not paste a personal access token.
20. As a solo developer, I want to see which of my credentials are configured without seeing their secrets, so that I can audit my setup safely.
21. As a solo developer, I want to register my development machines, so that I have one list of where I work.
22. As a solo developer, I want to see which agent CLIs a machine has installed and at what version, so that I know what my machines can do.
23. As a solo developer, I want to see whether a machine's CLI is logged in with an account or needs an API key, so that I can fix the ones that will fail.
24. As a solo developer, I want to see which models a machine's CLI can reach, so that I pick a model that actually works there.
25. As a solo developer, I want to know when a machine was last seen, so that I can tell a sleeping machine from a broken one.
26. As a solo developer, I want to see my total token usage over a period, so that I know what my work costs me.
27. As a solo developer, I want to see my cost in my own currency, so that the number means something to me.
28. As a solo developer, I want to see input, output and cache tokens separately, so that I understand where my tokens go.
29. As a solo developer, I want to see my usage broken down by model, so that I notice an expensive model creeping in.
30. As a solo developer, I want to see my usage broken down by provider, so that I can attribute spend to a subscription.
31. As a solo developer, I want to see my usage trend by day, week or month, so that I can tell a busy week from a normal one.
32. As a solo developer, I want to see which projects my reported work belongs to, so that I can see effort per project.
33. As a solo developer, I want the usage console to say when it has no data instead of showing zeros, so that I am not misled.
34. As a solo developer, I want to have my usage reported automatically when an omp session ends, so that I never have to record it by hand.
35. As a solo developer, I want to see a single day's usage and drill into the individual sessions, so that I can find the session that cost the most.
36. As a solo developer, I want my reported usage to survive a restart, so that history is not lost.
37. As a solo developer, I want to grant a reporting credential and revoke it, so that a leaked token can be cut off.
38. As a solo developer, I want a reporting credential to expire on its own, so that old tokens do not stay valid forever.
39. As a solo developer, I want my usage console to work while I am offline from the internet, so that my data is always accessible.
40. As a solo developer, I want the whole thing to be one container and one database file, so that backing it up is copying one file.
41. As a solo developer, I want to start the platform without configuring a database server, so that there is nothing to install.
42. As a solo developer, I want to open the platform and land directly in my requirements, so that there is no login between me and my work.
43. As a solo developer, I want the platform to refuse to listen on a public interface, so that my requirements and keys cannot be reached from the network.
44. As a solo developer, I want to use omp to file a requirement into MuseForge, so that my agent and my tracker stay in step.
45. As a solo developer, I want omp to read my project list and my requirements, so that my agent knows the context of my work.
46. As a solo developer, I want the platform to work with the tools I already use (Claude Code, Codex, Cursor, OpenCode), so that nothing is lost by leaving the chat workbench.
47. As a solo developer, I want the platform to be usable from a phone on my own network, so that I can check my list away from my desk.
48. As a solo developer, I want the interface in my own language, so that it reads naturally to me.
49. As a solo developer, I want the interface to follow my system's light or dark setting, so that it does not glare at night.
50. As a solo developer, I want the trimmed platform to be much smaller than the upstream image, so that it starts fast and costs little.
51. As a solo developer, I want the trimmed platform to keep the upstream MIT license and its attribution, so that the project stays honestly licensed.
52. As a solo developer, I want my own copyright on the parts I changed, so that the work is mine.
53. As a solo developer, I want the upstream's team features to be gone from the code, not merely hidden, so that they cannot rot or be re-enabled by accident.
54. As a solo developer, I want the remaining code to be understandable without reading the upstream team's history, so that I can maintain it.
55. As a solo developer, I want a test that proves the requirements, projects, usage console and machine inventory still work after each change, so that I can change the code without fear.
56. As a solo developer, I want a test that proves the team surfaces are really gone, so that they do not creep back.
57. As a solo developer, I want to start from a known-good commit and see the trim as one reviewable change, so that I can audit what was removed.
58. As a solo developer, I want a running record of which upstream feature corresponded to which removal, so that I can answer "where did that feature go?" later.

## Implementation Decisions

### Positioning

- MuseForge is a **standalone repository**. It does not track upstream, holds no upstream remote, and opens no upstream sync PRs. The upstream MIT license and Tencent's original copyright notice are retained in `LICENSE`; MuseForge's own copyright covers the personal changes.
- The repository has two branches: `dev` (development, the default branch) and `stable` (releases). There is no upstream-mirror branch.

### Removed surfaces

The following are deleted from the code, not disabled by configuration: the LOOP execution engine and its sandbox and CLI-executor runtime; the conversation workspace and chat workbench (Main Bot); the workflow/SOP designer and role, rule, skill, executor and sandbox asset catalogues; run records, artifacts, previews and the human-gate approval flow; the approvals inbox; scheduled AI tasks and the shared task market; the knowledge base and its connectors; enterprise IM bots (WeCom, Feishu, DingTalk); business domains and their aggregate reporting; platform administration (member accounts, platform admins, shared CodeBuddy key, public URL setting, API tokens for external callers); project members, member roles and join requests; SSO and gateway identity headers; the external tracker integrations for TAPD and GongFeng.

Deleting rather than disabling keeps the trimmed binary, schema and startup path small, and prevents a disabled surface from silently rotting.

### Kept surfaces and their contracts

- **Requirements** become a plain to-do item: title, description, status, priority, owning project, tags, comments, and an optional binding to a repository. Requirement-to-LOOP bindings, task records, delivery state and clarification assessment are removed. A **tag** field is new — upstream has no equivalent, so it is added to the requirement model and schema. **Notes** are the existing comment collection, not a field on the requirement.
- **Projects** stay as an asset grouping. Owner, members, member roles, members-version concurrency token and business-domain assignment are removed; the project row keeps only identity, key, name, description, colour and default repository.
- **Requirement import** keeps GitHub, which upstream already implements, and adds **GitLab** as new work. GitLab's issue API differs in shape from GitHub's (different listing path, different token style, no pull-request field to skip, different state and label vocabulary), so it needs its own adapter and its own mapping onto the local status and tag vocabulary. Both imports must preserve local edits to an already-imported requirement.
- **Repositories** keep the existing provider-agnostic model. GitHub and GitLab are already supported for Git transport and credentials, so no new work is needed there.
- **Personal settings** keep model API keys, the GitHub device-code authorization, SSH and HTTP Git credentials, and Git host trust. The Main Bot preference block (assistant defaults) is removed with the chat layer.
- **Execution machines** become an inventory. The registry, heartbeat, CLI detection (binary path, version, capabilities), CLI login state and model probe survive. Task dispatch, task leases, task events, workspace preparation, sandbox execution, artifact transfer, repository checkout and caching, work branches, conversation execution, project sharing and run quotas are deleted. The runner daemon becomes a registration client: it connects, registers, heartbeats and reports its detected CLIs; it never receives work. The control socket answers three operations — CLI maintenance, CLI model probe and the Git SSH host-key probe (the trust step personal settings keep on line above); everything else it used to answer is deleted.
- **Usage console** replaces the statistics page. It aggregates exclusively from externally reported executions. Tokens (input, output, cache read, cache write), cost, cache-hit rate, execution counts, per-model and per-provider breakdowns, and a day/week/month trend are kept. Everything that described internal LOOP runs — success rate, P95, tool-call success, human-intervention counts, automation rate, reworks, run quality — is removed, because those inputs no longer exist.
- **Reporting contract.** The upstream external-data-source mechanism is the bridge: a bearer-authenticated ingest endpoint accepting one reported execution per request, carrying an id, an external execution id, a workflow id and name, start and finish timestamps, a terminal status, and a list of per-model usage entries with token counts and an optional cost. Reported executions are projected into the statistics engine in memory, so the same aggregation serves both. Storage is a data-source credential table and a reported-execution table — separate from the access-token table, which is deleted.
- **Cost precedence.** An explicitly reported cost wins. Absent one, the published price table is used. A cost marked as estimated counts toward the total but is not counted as reported. Malformed reports are skipped and counted, not fatal.
- **Cache-hit convention.** Reported input tokens include cache tokens, so cache-hit rate is cache-read divided by input, and the price-table fallback subtracts cache tokens from input before applying the input rate.
- **omp bridge.** An omp extension aggregates a finished session's usage from the local statistics database and posts one reported execution to the ingest endpoint. The extension registers on the session-shutdown event. Project-level and user-level extension discovery paths are used.
- **MCP API** is kept. It exposes the platform's own HTTP API as MCP tools using the existing OpenAPI contract, and is the channel through which omp and other external agents read and write requirements and projects.

### Data layer

- The only datasource becomes **SQLite**. Upstream supports MySQL in production and SQLite only in tests, but the same gorm code paths already serve both, and the MySQL-specific SQL lives in files that the trim removes. What remains — requirements, projects, credentials, data sources, reported executions, machines — uses portable JSON access through the existing cross-database helpers.
- The schema becomes a fresh-install manifest rather than a patch stream: the trim no longer needs upstream's historical migration scripts, so the canonical schema is the model manifest plus one seed.
- The platform deploys as a **single container** with one SQLite file. The MySQL service and its configuration disappear.

### Authentication and exposure

- There is no login page and no account directory. The interface is reachable directly.
- The platform **binds to the loopback interface only**. This is a hard default, not a setting to remember: an unauthenticated service on a public interface would expose requirements, projects, credentials and cost data.
- The reporting credential used by the omp bridge is independent of login state and remains revocable and expiring.
- The upstream access-token mechanism for external callers is removed; the reporting credential replaces it.

### Repository shape

- The trim lands as one reviewable change on `dev`, starting from the last upstream commit, with a running record mapping each removed module to the feature decision that removed it.
- The trimmed code must read as the personal project it is, not as a fork with parts missing: the retained domain and repository layers are re-expressed in the personal vocabulary rather than keeping upstream names for concepts that no longer exist.

## Testing Decisions

### What makes a good test here

Tests assert **observable behaviour through a stable interface**, never internal structure. A test that names a deleted table, a removed route or a renamed struct is measuring implementation, not behaviour, and will be rewritten by any refactor. The one exception is a small set of explicit guard tests whose entire purpose is to assert that a removed surface stays removed — those name what is absent on purpose.

### Primary seam: the HTTP API

The main verification seam is the HTTP API, exercised against a real running platform. Upstream already provides this: a local harness that starts its own database container and platform process, seeds an account, and tears everything down afterwards, plus a small HTTP helper library used by the existing API test files.

Against that seam, the personal build is verified by:

- **Requirements** — create, read, update, tag, comment, close, reopen, delete; list filtered by project and status; import from GitHub and GitLab; the import skipping pull requests; a re-import preserving local edits.
- **Projects** — list, create, update, archive, restore; the absence of members and of join requests in the response.
- **Personal settings** — saving and reading credentials; secrets never appearing in a response; a configured flag appearing.
- **Machines** — registration, heartbeat with a CLI list, the CLI list being readable, model probe, deregistration.
- **Reporting** — ingesting a reported execution, then reading back its tokens, cost, cache-hit rate, model breakdown, provider breakdown, trend and execution list; the scoping by project, user and workflow; a rejected report with a bad token; a malformed report being skipped rather than fatal; an expired or revoked credential being refused.
- **Exposure** — the platform refusing to bind a non-loopback interface.

### Secondary seam: in-process Go

Used where the behaviour is a pure function of stored data and does not deserve a running server: the cost aggregation (a reported cost beating a priced one, a price-table fallback, the cache-hit formula), the trend bucketing across a timezone, and the per-model and per-provider breakdowns. This seam uses the in-memory store that already exists in the repository package, so it runs on the same gorm code paths as production.

### Frontend seam

The frontend is tested the way upstream tests it: a DOM-level harness that loads the frontend modules and asserts what a user sees. For this build it covers the requirement list, the project list, the personal settings panel, the machine inventory and the usage console — in particular that the usage console renders the token, cost, cache and trend panels and does **not** render the removed run-quality panels.

### Removal guards

A small number of tests assert absence, at the route level and the schema level: that no removed route answers, that no removed table is created, and that the platform's route surface contains only the personal routes. These are the tests that make "the team features are gone" a checkable claim rather than an assertion in a document.

### Prior art

Upstream already has this shape and it is reused rather than reinvented: the local harness and HTTP helper library for API tests, the in-memory store for repository tests, the DOM-level frontend harness, and the route-sweep test that enumerates the API surface — which becomes the natural home for the removal guards.

## Out of Scope

- Any team collaboration capability: accounts, roles, members, join requests, sharing, an approval inbox, a shared task market.
- Any agent execution capability: LOOP definition or dispatch, sandboxes, the runner task queue, human gates, previews, run artifacts, the conversation workspace and chat workbench.
- Scheduled tasks, in any form — including AI scheduled tasks and the shared market.
- The knowledge base, its connectors and its review queue.
- Enterprise IM bots and outbound notifications.
- Business domains, business-domain assignment and aggregate business reporting.
- SSO, gateway identity headers, TAPD and GongFeng integration.
- Following upstream: no upstream remote, no mirror branch, no automatic sync.
- A rewritten README: the upstream text remains below the MuseForge banner until the personal features are actually delivered.
- Multi-user support of any kind, including a future "add a second account".

## Further Notes

- The decisions behind this spec are recorded in `docs/personal-scope/decision.md`, including the screenshot-by-screenshot keep/cut table and the code-level consequences of each choice. Where that record and this spec disagree, the record is the authority on scope and this spec is the authority on how it is built.
- `GLOSSARY.md` defines the personal build's vocabulary and explicitly lists the upstream terms it no longer uses, so the two vocabularies do not blur.
- The recorded baseline is fifteen environment-only test failures on macOS (path symlinks in the temporary directory, a missing browser, a Linux-only sandbox primitive, a noisy shell profile). Every phase of the trim is judged against exactly that set: a new failure means that phase cut something it should not have.
- The five triage labels (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`) exist on the issue tracker and are used by the engineering skills to route incoming work.
