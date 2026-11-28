# Live verification of the personal build

Two suites, both runnable on a laptop, neither needing a model key, a database
server or a sign-in.

## `verify-personal.sh` — the platform end to end

```bash
cd e2e && ./verify-personal.sh
```

It builds `cmd/agentflow` from this checkout, then:

- starts the platform on a temporary SQLite file bound to `127.0.0.1`, and first
  proves it **refuses** a non-loopback bind;
- records the single account and proves asking again returns the same one;
- creates a project and a requirement **with tags**, comments on it, closes it,
  reopens it, filters by tag, deletes it and proves the list is empty;
- saves a personal model key and proves it reads back configured and **never
  comes back in clear text**;
- registers a machine, heartbeats, and proves the console lists it with its
  last-seen time — and that the sharing and the run quota are gone;
- creates a reporting credential, posts a finished session's usage, and proves
  the console shows the execution, the token split and the reported cost;
- proves the GitLab import fails **with a reason** when a repository has no
  credential, then binds a GitLab repository and proves the import finds it;
- proves every removed surface answers 404, and every console asset loads;
- restarts the platform on the same file and proves the projects, the
  credentials and the reported usage survived.

It prints one line per check and exits non-zero if any check fails. Everything
it creates lives in a temporary directory that it wipes on the next run.

## `unit/` — the pure functions

```bash
cd e2e && node --test 'unit/*.test.mjs'
```

No platform, no browser. Today it covers the runner version comparison that the
machine inventory relies on.

## What is not here

The platform's own logic is covered by the Go suite (`go test ./internal/...
./cmd/... ./web/`), and the console's DOM by `webtest/` (`npm --prefix webtest
test`). Those are the gates CI runs; this directory is the live check a person
runs against a real process.