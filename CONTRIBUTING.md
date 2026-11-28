# Contributing

MuseForge is a personal project board: a Go platform at the repository root and a separate local workflow edition in `local/`. Read the applicable `AGENTS.md` before changing either.

For platform changes, keep the repository and handler boundaries `AGENTS.md` describes. Keep identity, provider and deployment adaptations explicit. Add a regression test for a behavioural fix and document any schema or runner-protocol change.

The platform serves a console embedded from `web/`; it is not built separately. The checks:

```bash
go build ./...
go vet ./...
go test ./internal/... ./cmd/... ./web/ -count=1 -timeout=900s
npm --prefix webtest ci --no-audit --no-fund && npm --prefix webtest test
python3 scripts/check-public.py
bash scripts/scan-secrets.sh
```

The live end-to-end suite under `e2e/` starts the platform itself: `cd e2e && ./verify-personal.sh` builds the current checkout, starts it on a temporary SQLite file with no sign-in, and runs the API and console scenarios against it. State passed, failed, skipped and not-run checks accurately.

Platform pull requests run the `core` job, which is the gate above. A repository administrator marks it as a required status check in GitHub branch protection; this repository does not change that setting.

For the local workflow edition, work inside `local/` and follow [local agent instructions](local/AGENTS.md). Identify whether a change affects Classic, Portable or shared behaviour contracts. Keep these workflow assets independent from the platform and each other. Change Portable assets in `local/skills/`; rebuild generated Classic Cursor/Claude bundles with `python3 scripts/build-classic-hosts.py --write` from `local/`. Keep workflow templates generic; optional tools under `local/tools/` are distributed separately.

Run the local checks from `local/`:

```bash
python3 scripts/build-classic-hosts.py --check
bash scripts/validate.sh
bash scripts/smoke-install.sh
bash scripts/e2e.sh
```

The [local E2E suite](local/tests/e2e/README.md) uses a fake host and recorded outputs, without model credentials. Describe the impact on each affected edition. Preserve existing permission behaviour and user-file protection. Commit messages follow Conventional Commits.

Public pull requests must not contain credentials, private infrastructure, employee access defaults or production datasets. Preserve existing user files. Keep source attribution and third-party licences. Describe the problem, the resulting behaviour and the validation actually performed in the PR.