# MuseForge

**A personal project board. It records your work; it does not run it.**

MuseForge is a **single-user** trimmed build of
[Tencent/LoopForge](https://github.com/Tencent/LoopForge). The team collaboration
surface is gone — member accounts, project members and join requests, the shared task
market, enterprise IM bots, business-domain reports, SSO and gateway identity — and
so is the agent execution surface. What remains is what one person actually uses:

- a **requirement list** — your to-do items, with a project, a status, a priority,
  tags, comments, and a link to a GitHub or GitLab issue
- **project grouping** — a container for requirements, with no roster
- **personal settings** — model API keys, the GitHub device-code authorization, and
  your SSH and HTTP Git credentials
- a **machine inventory** — the agent CLIs installed on your machines, their
  versions, their login state and the models they can reach
- an **omp usage console** — the tokens and cost of the work your own agent reported
  from outside
- an **MCP / HTTP API** — the channel an external agent (omp, Claude Code, Codex,
  Cursor, OpenCode) reads and writes your requirements through

**It never executes anything.** There is no LOOP, no sandbox, no runner task queue,
no preview, no scheduled task, no knowledge base and no chat workbench. The work
happens in your own agent; MuseForge keeps the ledger.

This is a **standalone repository**. It no longer tracks upstream and does not sync
upstream updates. Branch convention: `dev` for development, `stable` for releases.
The upstream MIT license and Tencent's original copyright notice are retained in
[LICENSE](./LICENSE); MuseForge's own copyright covers the personal changes.

Fork point: upstream `Tencent/LoopForge` at `v0.4.3` (commit `fd039e51`), marked in
this repository by the tag `upstream-v0.4.3`.

---

## Quick start

One container, one SQLite file, no database server.

```bash
# Build and run from source
make build
./bin/agentflow -datasource file:./museforge.db -addr 127.0.0.1:8080
```

The platform creates its schema from the model manifest on first start — there is no
migration step. Open <http://127.0.0.1:8080> and you are in your requirement list;
there is no sign-in.

With Docker:

```bash
bash scripts/configure.sh          # writes .env with the datasource and the secrets
docker compose up -d --wait
```

The datasource is one setting:

| setting | meaning |
|---|---|
| `-datasource` / `AGENTFLOW_DATASOURCE_DSN` | `file:/path/to/museforge.db` for SQLite, or a MySQL DSN. A `file:` URI, a bare path, or a MySQL DSN — the platform tells them apart. |
| `-addr` / `AGENTFLOW_HTTP_PORT` | the listen address. **Loopback only.** A non-loopback bind is refused at startup. |
| `AGENTFLOW_SESSION_SECRET` | required. |
| `AGENTFLOW_AUTH_REQUIRED` | `false` for the personal build — there is no login. |

---

## The omp usage console

Your agent sessions already cost you money; MuseForge shows the bill. omp records each
session's per-model usage in its own statistics database, and a small **omp
extension** posts one summary per session to MuseForge when the session ends.

```bash
# 1. install the extension
bash omp-extension/scripts/install.sh --user     # or --project /path/to/project

# 2. point it at the platform and give it a reporting credential
bash omp-extension/scripts/install.sh --print-config > ~/.config/museforge/omp-usage.json
```

The reporting credential is created in the usage console's **数据源 / Data source**
dialog — it is independent of any login state, revocable, and expiring.

The extension reads its configuration on every session end, so rotating the token
needs no reinstall. It reports nothing when no credential is configured, and a failed
report never interrupts the session it reports on.

Full details — the installation paths, the configuration keys, and how a session's
usage is mapped onto a reported execution — are in
[`omp-extension/README.md`](./omp-extension/README.md). The design is recorded in
[`docs/adr/0003-omp-usage-console.md`](./docs/adr/0003-omp-usage-console.md).

---

## Documentation

| document | what it covers |
|---|---|
| [`docs/personal-scope/spec.md`](./docs/personal-scope/spec.md) | the specification this build implements |
| [`docs/personal-scope/decision.md`](./docs/personal-scope/decision.md) | what was kept, what was cut, and why — the authority on scope |
| [`docs/personal-scope/baseline-tests.md`](./docs/personal-scope/baseline-tests.md) | the measured pre-trim test baseline |
| [`docs/architecture.md`](./docs/architecture.md) | how the personal build is put together |
| [`docs/adr/`](./docs/adr/) | the decisions that shaped it: the trim's depth, the console's position, the usage console, the SQLite-only datasource, the no-login loopback-only exposure model |
| [`docs/api/`](./docs/api/) | the HTTP API and its OpenAPI contract |
| [`docs/deployment.md`](./docs/deployment.md) | running it |
| [`docs/testing/`](./docs/testing/) | the test suites and how to run them |
| [`GLOSSARY.md`](./GLOSSARY.md) | the vocabulary, and the upstream words this build no longer uses |

---

## Working on MuseForge

```bash
make build          # the platform binaries
make test           # the Go suites
npm --prefix webtest test    # the console's DOM suite
npm --prefix chatbot test    # (removed with the chat workbench)
```

`AGENTS.md` holds the repository's rules. `docs/personal-scope/decision.md` is the
authority on scope: when the code and the decision record disagree about what should
exist, the decision record wins.

---

## Relationship to upstream

MuseForge began as [Tencent/LoopForge](https://github.com/Tencent/LoopForge), a team
development workbench, and was trimmed to one person's use. The upstream MIT licence
and Tencent's original copyright notice are retained in [LICENSE](./LICENSE); the
personal changes are covered by MuseForge's own copyright. Upstream remains the
source of the requirement model, the credential vault, the git integration and the
machine probing that this build keeps.
