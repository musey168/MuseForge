# 产品定位：个人项目管理台，Agent 执行交给 omp

MuseForge 保留需求清单、项目分组、凭据配置、统计和机器清单；
**不再执行任何 Agent**。

**为什么**：上游 LoopForge 的核心价值是「让编程 Agent 按可审批的 SOP 交付代码」。
本项目的使用者日常在 Mac 本地用 `omp` 写代码，LoopForge 自带的执行引擎
（LOOP 派发、Docker 沙箱、Runner 任务分发、10 种 Agent CLI 执行器）
在这条链路上是重复建设 —— 约 12.5 万行代码，换来的是一个人根本用不上的
多角色编排、人工关口和审批流。

**代价**：MuseForge 不再能「开箱即用」地跑通一次交付。要执行 Agent 就用 omp，
MuseForge 只负责记录与配置。

**结果**：上游的 LOOP、run、approval gate、schedule、bot、knowledge、
evolution、tracker（企业工单）全部删除。执行机器保留，但只作为
「机器清单 + CLI 环境管理」，不再有任务分发能力。
