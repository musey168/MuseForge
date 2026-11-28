-- 删除项目级凭据表。
--
-- 这张表只服务一个消费者：LOOP 流程选用凭据、把变量注入沙箱。LOOP 已随裁剪删除，
-- 表里没有任何东西被读取，页面与接口也一并移除。新装不需要执行本脚本（代码不再
-- 创建这张表）；已经跑过 043 的部署执行一次即可。
--
--   sqlite3 .data/museforge.db < migrations/044_drop_project_credentials.sql
--
DROP TABLE IF EXISTS project_credentials;