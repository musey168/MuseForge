# 数据库换成 SQLite

个人版只有一份数据、一个人访问，MySQL 容器（常驻内存约 400MB+）是纯负担。
改用 SQLite 单文件，部署收敛到单个容器。

**上游现状**：`internal/repo/repo.go` 注释明确「MySQL is the only production
datasource; tests open an in-process sqlite database through the same gorm
code paths」—— 也就是说 gorm 代码路径本来就同时被 SQLite 覆盖过测试，
可移植性不是从零开始。

**可行性的依据**：MySQL 专属 SQL（`JSON_TABLE` / `JSON_UNQUOTE` /
`JSON_EXTRACT` / `JSON_SET`）全部集中在将要删除或重写的文件里
（analytics、assistant、runs、runners）；存活的 `issues` / `projects` /
`user_configs` / `insight_*` 的 JSON 查询走 `sqlJSONText` 这类跨库 helper。
剩余工作量集中在「重写统计聚合」一处。

**代价**：`migrations/*.sql` 是 MySQL 方言的补丁流，个人版不沿用；
新装走 `internal/repo/models.go` 的 `allModels()` 自动建表 + 一份种子 SQL。
