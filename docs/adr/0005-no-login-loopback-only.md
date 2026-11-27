# 界面免登录，且强制只监听回环地址

MuseForge 只有一个人使用，登录界面是纯粹的摩擦。删掉登录页，
本地直接进。

**机制**：`internal/httpapi/auth_tokens.go` 的 `authRequired()` 读
`AGENTFLOW_AUTH_REQUIRED`；为 false 时所有写守卫（`requireProjectWriter`、
`requirePlatformAdmin`）直接放行。`previewLocalAuthUser` 另有一条 loopback
免登录路径。`users` / `user_configs` 表保留 —— 个人配置页的模型 Key、
GitHub 授权、SSH 凭据都存在这里，鉴权与凭据存储是两张皮。

**强制约束**：免登录的前提是**只监听 127.0.0.1**（`LOOPFORGE_BIND=127.0.0.1`）。
一旦对外暴露，等于把需求、项目、凭据、Token 成本全部裸奔。
这条约束写进部署默认值，不依赖使用者自觉。

**例外**：omp 推送统计走 `insight_sources` 的 Bearer Token，不依赖登录态。
