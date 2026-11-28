# 参与贡献

MuseForge 是个人项目工作台：平台代码位于仓库根目录，另一套本地工作流版本位于 `local/`。开始修改前请阅读对应的 `AGENTS.md`。

平台改动应保留 `AGENTS.md` 规定的 repo 与 handler 分层；身份、外部提供商和部署差异应通过明确适配实现。修复行为问题时附带回归测试，说明 schema 与 runner 协议的变化。

平台的控制台内嵌在 `web/`，不单独构建。检查命令：

```bash
go build ./...
go vet ./...
go test ./internal/... ./cmd/... ./web/ -count=1 -timeout=900s
npm --prefix webtest ci --no-audit --no-fund && npm --prefix webtest test
python3 scripts/check-public.py
bash scripts/scan-secrets.sh
```

`e2e/` 下的实机套件对着你自己启动的平台运行：`cd e2e && ./local.sh api` 会编译当前代码，用临时 SQLite 文件、免登录启动，再跑 API 用例。请区分通过、失败、跳过与未运行。

平台的 pull request 会跑 `core` 作业，也就是上面的检查。仓库管理员需要在 GitHub 分支保护里把它手动勾成必须通过的检查；这次代码改动不会修改 GitHub 上的分支保护设置。

LoopForge Local 请在 `local/` 内修改，遵循[本地开发约定](local/AGENTS.md)。先明确改动属于 Classic、Portable 或共享行为合同；三者与平台运行资产保持清晰边界。Portable 修改 `local/skills/`；Classic 的 Cursor/Claude 包在 `local/` 内运行 `python3 scripts/build-classic-hosts.py --write` 生成。工作流模板保持通用，可选工具放在 `local/tools/`，单独分发。

本地检查在 `local/` 内运行：

```bash
python3 scripts/build-classic-hosts.py --check
bash scripts/validate.sh
bash scripts/smoke-install.sh
bash scripts/e2e.sh
```

[本地 E2E 套件](local/tests/e2e/README.md)使用假宿主与录制的输出，不需要模型凭据。请说明改动对各个版本的影响，保留既有的权限行为与用户文件保护。提交信息遵循 Conventional Commits。

公共 pull request 不得包含凭据、私有基础设施、员工访问默认值或生产数据集。保留用户已有文件。保留来源署名与第三方许可。请在 PR 中说明问题、修改后的行为以及实际做过的验证。