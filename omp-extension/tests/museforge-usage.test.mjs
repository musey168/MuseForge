// Tests for the MuseForge omp usage-reporting extension.
//
// Run with: node --test omp-extension/tests/
//
// The extension is a script, so it is tested as one: the aggregation runs over
// a fixture copy of omp's statistics database (same table and column names) and
// the reported payload is asserted against the ingest contract; the reporting
// path is exercised against real local HTTP servers, never mocks of our own
// code.

import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createServer } from "node:http";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { DatabaseSync } from "node:sqlite";
import test from "node:test";

import registerUsageReporting, {
  aggregateSessionUsage,
  loadUsageConfig,
  reportSessionUsage,
} from "../src/museforge-usage.ts";

const ROOT = resolve(import.meta.dirname, "..", "..");
const BASE = Date.UTC(2026, 10, 27, 10, 0, 0);

function tempDir() {
  return mkdtempSync(join(tmpdir(), "museforge-omp-"));
}

// createStatsDb writes a statistics database with omp's `messages` schema
// subset this extension reads.
function createStatsDb(dir, rows) {
  const file = join(dir, "stats.db");
  const db = new DatabaseSync(file);
  db.exec(`
    CREATE TABLE messages (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      session_file TEXT NOT NULL,
      entry_id TEXT NOT NULL,
      folder TEXT NOT NULL,
      model TEXT NOT NULL,
      provider TEXT NOT NULL,
      api TEXT NOT NULL,
      timestamp INTEGER NOT NULL,
      duration INTEGER,
      ttft INTEGER,
      stop_reason TEXT NOT NULL,
      error_message TEXT,
      input_tokens INTEGER NOT NULL,
      output_tokens INTEGER NOT NULL,
      cache_read_tokens INTEGER NOT NULL,
      cache_write_tokens INTEGER NOT NULL,
      total_tokens INTEGER NOT NULL,
      premium_requests REAL NOT NULL,
      cost_input REAL NOT NULL,
      cost_output REAL NOT NULL,
      cost_cache_read REAL NOT NULL,
      cost_cache_write REAL NOT NULL,
      cost_total REAL NOT NULL,
      cost_no_cache_input REAL,
      cost_unpriced INTEGER NOT NULL DEFAULT 0,
      agent_type TEXT NOT NULL DEFAULT 'main',
      service_tier TEXT
    );
  `);
  const insert = db.prepare(`
    INSERT INTO messages (
      session_file, entry_id, folder, model, provider, api, timestamp, duration,
      stop_reason, error_message, input_tokens, output_tokens, cache_read_tokens,
      cache_write_tokens, total_tokens, premium_requests, cost_input, cost_output,
      cost_cache_read, cost_cache_write, cost_total, cost_no_cache_input, cost_unpriced
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  `);
  for (const [index, row] of rows.entries()) {
    const input = row.input ?? 0;
    const output = row.output ?? 0;
    const cacheRead = row.cacheRead ?? 0;
    const cacheWrite = row.cacheWrite ?? 0;
    insert.run(
      row.sessionFile,
      `entry-${index}`,
      "/tmp/project",
      row.model,
      row.provider,
      "api",
      row.timestamp,
      row.duration ?? 0,
      row.stopReason ?? "stop",
      row.errorMessage ?? null,
      input,
      output,
      cacheRead,
      cacheWrite,
      input + output + cacheRead + cacheWrite,
      0,
      0,
      0,
      0,
      0,
      row.cost ?? 0,
      0,
      row.unpriced ?? 0,
    );
  }
  db.close();
  return file;
}

// serve starts a local server whose handler observes the request and replies.
async function serve(handler) {
  const requests = [];
  const server = createServer((req, res) => {
    let body = "";
    req.on("data", (chunk) => {
      body += chunk;
    });
    req.on("end", () => {
      requests.push({
        method: req.method,
        url: req.url,
        headers: req.headers,
        body: body ? JSON.parse(body) : null,
      });
      handler(req, res, body);
    });
  });
  await new Promise((done) => server.listen(0, "127.0.0.1", done));
  const { port } = server.address();
  return {
    url: `http://127.0.0.1:${port}`,
    requests,
    close: () => new Promise((done) => server.close(done)),
  };
}

function config(overrides) {
  return {
    url: "http://127.0.0.1:1",
    token: "afi_test",
    statsDb: "",
    timeoutMs: 500,
    ...overrides,
  };
}

test("aggregates one session's usage per model into one reported execution", async () => {
  const dir = tempDir();
  try {
    const sessionFile = join(dir, "fixture-session.jsonl");
    const statsDb = createStatsDb(dir, [
      {
        sessionFile,
        model: "model-a",
        provider: "provider-1",
        timestamp: BASE + 1000,
        duration: 1000,
        input: 100,
        output: 10,
        cacheRead: 900,
        cost: 0.25,
        stopReason: "toolUse",
      },
      {
        sessionFile,
        model: "model-a",
        provider: "provider-1",
        timestamp: BASE + 4000,
        duration: 500,
        input: 50,
        output: 5,
        cacheRead: 950,
        cost: 0.125,
        stopReason: "stop",
      },
      {
        sessionFile,
        model: "model-b",
        provider: "provider-2",
        timestamp: BASE + 7000,
        duration: 2000,
        input: 200,
        output: 20,
        cacheWrite: 300,
        cost: 0.5,
        stopReason: "stop",
      },
      // Another session in the same database must not leak into the report.
      {
        sessionFile: join(dir, "other-session.jsonl"),
        model: "model-c",
        provider: "provider-3",
        timestamp: BASE + 8000,
        duration: 9000,
        input: 1000,
        output: 1000,
        cost: 99,
        stopReason: "stop",
      },
    ]);

    const execution = await aggregateSessionUsage({ sessionFile, statsDb });

    assert.deepEqual(execution, {
      id: "omp:fixture-session",
      type: "workflow_execution",
      externalExecutionId: "omp:fixture-session",
      workflow: { id: "omp", name: "omp" },
      startedAt: "2026-11-27T10:00:01.000Z",
      finishedAt: "2026-11-27T10:00:07.000Z",
      executionDurationMs: 3500,
      status: "succeeded",
      usage: [
        {
          provider: "provider-1",
          model: "model-a",
          inputTokens: 2000,
          outputTokens: 15,
          cacheReadTokens: 1850,
          cacheWriteTokens: 0,
          reasoningTokens: 0,
          costUsd: 0.375,
          costStatus: "reported",
        },
        {
          provider: "provider-2",
          model: "model-b",
          inputTokens: 500,
          outputTokens: 20,
          cacheReadTokens: 0,
          cacheWriteTokens: 300,
          reasoningTokens: 0,
          costUsd: 0.5,
          costStatus: "reported",
        },
      ],
    });
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("keeps the report id stable so a retry of the same session is a no-op", async () => {
  const dir = tempDir();
  try {
    const sessionFile = join(dir, "retry-session.jsonl");
    const statsDb = createStatsDb(dir, [
      {
        sessionFile,
        model: "model-a",
        provider: "provider-1",
        timestamp: BASE,
        duration: 10,
        input: 1,
        output: 1,
        cost: 0.001,
      },
    ]);
    const first = await aggregateSessionUsage({ sessionFile, statsDb });
    const second = await aggregateSessionUsage({ sessionFile, statsDb });
    assert.equal(first.id, second.id);
    assert.equal(first.externalExecutionId, second.externalExecutionId);
    assert.deepEqual(first, second);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("marks a partly unpriced cost as estimated and a fully unpriced zero cost as unknown", async () => {
  const dir = tempDir();
  try {
    const sessionFile = join(dir, "cost-session.jsonl");
    const statsDb = createStatsDb(dir, [
      {
        sessionFile,
        model: "model-priced",
        provider: "provider",
        timestamp: BASE,
        duration: 100,
        input: 10,
        output: 1,
        cost: 0.4,
        unpriced: 3,
      },
      {
        sessionFile,
        model: "model-unpriced",
        provider: "provider",
        timestamp: BASE + 100,
        duration: 100,
        input: 10,
        output: 1,
        cost: 0,
        unpriced: 2,
      },
    ]);

    const execution = await aggregateSessionUsage({ sessionFile, statsDb });
    assert.deepEqual(
      execution.usage.map((item) => [item.model, item.costUsd, item.costStatus]),
      [
        ["model-priced", 0.4, "estimated"],
        ["model-unpriced", null, "unknown"],
      ],
    );
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("reports a failed or cancelled session from omp's stop reasons", async () => {
  const dir = tempDir();
  try {
    const failedFile = join(dir, "failed-session.jsonl");
    const cancelledFile = join(dir, "cancelled-session.jsonl");
    const statsDb = createStatsDb(dir, [
      {
        sessionFile: failedFile,
        model: "model-a",
        provider: "provider",
        timestamp: BASE,
        duration: 100,
        input: 10,
        output: 1,
        stopReason: "error",
        errorMessage: "upstream failed",
      },
      {
        sessionFile: cancelledFile,
        model: "model-a",
        provider: "provider",
        timestamp: BASE,
        duration: 100,
        input: 10,
        output: 1,
        stopReason: "aborted",
      },
    ]);

    assert.equal((await aggregateSessionUsage({ sessionFile: failedFile, statsDb })).status, "failed");
    assert.equal((await aggregateSessionUsage({ sessionFile: cancelledFile, statsDb })).status, "cancelled");
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("reports nothing for a session without usage rows or a missing database", async () => {
  const dir = tempDir();
  try {
    const sessionFile = join(dir, "empty-session.jsonl");
    const statsDb = createStatsDb(dir, []);
    assert.equal(await aggregateSessionUsage({ sessionFile, statsDb }), null);
    assert.equal(await aggregateSessionUsage({ sessionFile, statsDb: join(dir, "absent.db") }), null);
    assert.equal(await aggregateSessionUsage({ sessionFile: "", statsDb }), null);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("posts the aggregated execution to the ingest endpoint with the credential in the header", async () => {
  const dir = tempDir();
  const server = await serve((req, res) => {
    res.writeHead(201, { "Content-Type": "application/json" });
    res.end('{"status":"accepted"}');
  });
  try {
    const sessionFile = join(dir, "posted-session.jsonl");
    const statsDb = createStatsDb(dir, [
      {
        sessionFile,
        model: "model-a",
        provider: "provider-1",
        timestamp: BASE,
        duration: 250,
        input: 100,
        output: 10,
        cacheRead: 400,
        cost: 0.01,
      },
    ]);

    const result = await reportSessionUsage({
      sessionFile,
      config: config({ url: server.url, statsDb }),
    });

    assert.equal(result, "posted");
    assert.equal(server.requests.length, 1);
    const request = server.requests[0];
    assert.equal(request.method, "POST");
    assert.equal(request.url, "/api/insights/v1/ingest");
    assert.equal(request.headers.authorization, "Bearer afi_test");
    assert.match(request.headers["content-type"], /application\/json/);
    assert.equal(request.body.id, "omp:posted-session");
    assert.equal(request.body.type, "workflow_execution");
    assert.deepEqual(request.body.usage[0], {
      provider: "provider-1",
      model: "model-a",
      inputTokens: 500,
      outputTokens: 10,
      cacheReadTokens: 400,
      cacheWriteTokens: 0,
      reasoningTokens: 0,
      costUsd: 0.01,
      costStatus: "reported",
    });
  } finally {
    await server.close();
    rmSync(dir, { recursive: true, force: true });
  }
});

test("reports nothing at all when no credential is configured", async () => {
  const dir = tempDir();
  let calls = 0;
  const neverCalled = () => {
    calls += 1;
    throw new Error("network must not be touched without a credential");
  };
  try {
    const sessionFile = join(dir, "unconfigured-session.jsonl");
    const statsDb = createStatsDb(dir, [
      { sessionFile, model: "model-a", provider: "provider", timestamp: BASE, input: 1, output: 1 },
    ]);

    for (const token of [undefined, "", "   "]) {
      assert.equal(
        await reportSessionUsage({
          sessionFile,
          config: config({ token, statsDb }),
          fetchImpl: neverCalled,
        }),
        "skipped",
      );
    }
    assert.equal(calls, 0);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("a failing endpoint never throws and never hangs the session", async () => {
  const dir = tempDir();
  const failing = await serve((req, res) => {
    res.writeHead(500, { "Content-Type": "application/json" });
    res.end('{"error":"boom"}');
  });
  const hanging = await serve(() => {});
  const redirecting = await serve((req, res) => {
    res.writeHead(307, { Location: failing.url + "/elsewhere" });
    res.end();
  });
  try {
    const sessionFile = join(dir, "failing-session.jsonl");
    const statsDb = createStatsDb(dir, [
      { sessionFile, model: "model-a", provider: "provider", timestamp: BASE, input: 1, output: 1 },
    ]);

    assert.equal(
      await reportSessionUsage({ sessionFile, config: config({ url: failing.url, statsDb }) }),
      "failed",
    );
    assert.equal(
      await reportSessionUsage({
        sessionFile,
        config: config({ url: "http://127.0.0.1:1", statsDb }),
      }),
      "failed",
    );
    assert.equal(
      await reportSessionUsage({
        sessionFile,
        config: config({ url: redirecting.url, statsDb }),
      }),
      "failed",
    );
    assert.equal(failing.requests.length, 1, "a redirect must not be followed to a second request");

    const started = Date.now();
    assert.equal(
      await reportSessionUsage({
        sessionFile,
        config: config({ url: hanging.url, statsDb, timeoutMs: 150 }),
      }),
      "failed",
    );
    assert.ok(Date.now() - started < 2000, "a hanging endpoint must be abandoned by the timeout");
  } finally {
    await failing.close();
    await hanging.close();
    await redirecting.close();
    rmSync(dir, { recursive: true, force: true });
  }
});

test("treats a duplicate id and a conflicting id as quiet outcomes", async () => {
  const dir = tempDir();
  const duplicate = await serve((req, res) => {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end('{"status":"duplicate"}');
  });
  const conflict = await serve((req, res) => {
    res.writeHead(409, { "Content-Type": "application/json" });
    res.end('{"error":"conflict"}');
  });
  try {
    const sessionFile = join(dir, "duplicate-session.jsonl");
    const statsDb = createStatsDb(dir, [
      { sessionFile, model: "model-a", provider: "provider", timestamp: BASE, input: 1, output: 1 },
    ]);

    assert.equal(
      await reportSessionUsage({ sessionFile, config: config({ url: duplicate.url, statsDb }) }),
      "duplicate",
    );
    assert.equal(
      await reportSessionUsage({ sessionFile, config: config({ url: conflict.url, statsDb }) }),
      "conflict",
    );
  } finally {
    await duplicate.close();
    await conflict.close();
    rmSync(dir, { recursive: true, force: true });
  }
});

test("reads configuration from the user file, the project file, then the environment", () => {
  const dir = tempDir();
  try {
    const home = join(dir, "home");
    const project = join(dir, "project");
    const userConfig = join(home, ".config", "museforge", "omp-usage.json");
    const projectConfig = join(project, ".omp", "museforge-usage.json");
    mkdirSync(dirname(userConfig), { recursive: true });
    mkdirSync(dirname(projectConfig), { recursive: true });

    writeFileSync(
      userConfig,
      JSON.stringify({ url: "http://127.0.0.1:9000", token: "afi_user", statsDb: "~/stats.db" }),
    );
    writeFileSync(projectConfig, JSON.stringify({ token: "afi_project", timeoutMs: 250 }));

    const merged = loadUsageConfig({ cwd: project, home, env: {} });
    assert.equal(merged.statsDb, join(home, "stats.db"));
    assert.equal(merged.token, "afi_project");
    assert.equal(merged.url, "http://127.0.0.1:9000");
    assert.equal(merged.timeoutMs, 250);

    const fromEnv = loadUsageConfig({
      cwd: project,
      home,
      env: { MUSEFORGE_INSIGHT_TOKEN: "afi_env", MUSEFORGE_URL: "http://127.0.0.1:9100" },
    });
    assert.equal(fromEnv.token, "afi_env");
    assert.equal(fromEnv.url, "http://127.0.0.1:9100");

    const empty = loadUsageConfig({ cwd: join(dir, "nowhere"), home: join(dir, "nohome"), env: {} });
    assert.equal(empty.token, undefined);
    assert.ok(empty.statsDb.endsWith(join(".omp", "stats.db")));
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("registers a session_shutdown handler that never throws out of the event", async () => {
  const dir = tempDir();
  // The handler reads configuration itself, so pin the environment to a
  // statistics database that does not exist: nothing may reach the network.
  const saved = {
    url: process.env.MUSEFORGE_URL,
    token: process.env.MUSEFORGE_INSIGHT_TOKEN,
    statsDb: process.env.MUSEFORGE_STATS_DB,
  };
  process.env.MUSEFORGE_URL = "http://127.0.0.1:1";
  process.env.MUSEFORGE_INSIGHT_TOKEN = "afi_registration";
  process.env.MUSEFORGE_STATS_DB = join(dir, "absent.db");
  try {
    const events = [];
    registerUsageReporting({ on: (name, handler) => events.push([name, handler]) });
    assert.deepEqual(
      events.map(([name]) => name),
      ["session_shutdown"],
    );

    const [, handler] = events[0];
    // No session file: nothing to report.
    const outcome = await handler({}, { cwd: dir, sessionManager: { getSessionFile: () => "" } });
    assert.equal(outcome, undefined);

    // A statistics database that does not exist, and a handler-less context,
    // must not escape the handler either.
    await handler({}, { cwd: dir, sessionManager: { getSessionFile: () => join(dir, "s.jsonl") } });
    await handler({}, undefined);
  } finally {
    for (const [key, value] of [
      ["MUSEFORGE_URL", saved.url],
      ["MUSEFORGE_INSIGHT_TOKEN", saved.token],
      ["MUSEFORGE_STATS_DB", saved.statsDb],
    ]) {
      if (value === undefined) delete process.env[key];
      else process.env[key] = value;
    }
    rmSync(dir, { recursive: true, force: true });
  }
});

test("the installer copies the extension to the project or user extension directory", () => {
  const dir = tempDir();
  try {
    assert.equal(
      execFileSync(
        "bash",
        [join(ROOT, "omp-extension", "scripts", "install.sh"), "--project", dir],
        { cwd: ROOT, encoding: "utf8" },
      ).trim(),
      join(dir, ".omp", "extensions", "museforge-usage.ts"),
    );
    const installed = readFileSync(join(dir, ".omp", "extensions", "museforge-usage.ts"), "utf8");
    assert.match(installed, /session_shutdown/);
    assert.match(installed, /export default/);

    const fakeHome = join(dir, "home");
    execFileSync(
      "bash",
      [join(ROOT, "omp-extension", "scripts", "install.sh"), "--user"],
      { cwd: ROOT, encoding: "utf8", env: { ...process.env, HOME: fakeHome } },
    );
    assert.match(
      readFileSync(join(fakeHome, ".omp", "agent", "extensions", "museforge-usage.ts"), "utf8"),
      /export default/,
    );
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
