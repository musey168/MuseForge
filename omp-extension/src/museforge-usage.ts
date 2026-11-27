// MuseForge usage reporting for omp (oh-my-pi).
//
// omp already records per-message usage in `~/.omp/stats.db`. This extension
// listens for `session_shutdown`, aggregates the finished session's rows into
// one `workflow_execution`, and posts it to MuseForge's ingest endpoint, so the
// usage console shows the session without any manual step.
//
// Design rules (docs/adr/0003-omp-usage-console.md):
// - It is a single self-contained file: it is copied into an extension
//   directory as-is and never imports a sibling module.
// - It reports only when a data-source credential is configured. Without one it
//   stays silent: no network call, no log line.
// - It never throws out of the event handler and never retries. A report that
//   fails is dropped; the next session reports normally. The report id is
//   derived from the session, so re-sending the same session is a no-op on the
//   platform (HTTP 200 duplicate) instead of a second counted execution.
//
// Configuration is read from, in increasing precedence: the user file
// `~/.config/museforge/omp-usage.json`, the project file
// `<cwd>/.omp/museforge-usage.json`, and the environment. Keys: `url`, `token`,
// `statsDb`, `timeoutMs`, `debug`; the environment uses `MUSEFORGE_URL`,
// `MUSEFORGE_INSIGHT_TOKEN`, `MUSEFORGE_STATS_DB`, `MUSEFORGE_TIMEOUT_MS` and
// `MUSEFORGE_DEBUG`. See omp-extension/README.md.

import { existsSync, readFileSync } from "node:fs";
import { homedir } from "node:os";
import { basename, join } from "node:path";

const INGEST_PATH = "/api/insights/v1/ingest";
const WORKFLOW_ID = "omp-session";
const WORKFLOW_NAME = "omp 会话";
const DEFAULT_TIMEOUT_MS = 1500;
const MAX_TIMEOUT_MS = 10_000;
const DEFAULT_STATS_DB = [".omp", "stats.db"];
const USER_CONFIG_PATH = [".config", "museforge", "omp-usage.json"];
const PROJECT_CONFIG_PATH = [".omp", "museforge-usage.json"];
const MAX_USAGE_ENTRIES = 64;
// The platform caps record IDs at 256 bytes; the prefix leaves room to spare.
const MAX_SESSION_KEY_BYTES = 200;

// One row set per (model, provider) group. omp's `input_tokens` excludes the
// cache counters and `total_tokens` is their sum, so the reported input adds
// the cache counters back: the ingest contract requires input to include cache.
const SESSION_USAGE_SQL = `
  SELECT
    model,
    provider,
    SUM(input_tokens) AS input_tokens,
    SUM(output_tokens) AS output_tokens,
    SUM(cache_read_tokens) AS cache_read_tokens,
    SUM(cache_write_tokens) AS cache_write_tokens,
    SUM(cost_total) AS cost_total,
    SUM(cost_unpriced) AS cost_unpriced,
    SUM(COALESCE(duration, 0)) AS duration_ms,
    MIN(timestamp) AS started_at,
    MAX(timestamp) AS finished_at,
    SUM(CASE WHEN stop_reason = 'error' OR error_message IS NOT NULL THEN 1 ELSE 0 END) AS failed_rows,
    SUM(CASE WHEN stop_reason = 'aborted' THEN 1 ELSE 0 END) AS aborted_rows
  FROM messages
  WHERE session_file = ?
  GROUP BY model, provider
  ORDER BY model, provider
`;

function text(value) {
  return typeof value === "string" && value.trim() !== "" ? value.trim() : undefined;
}

function positiveInteger(value, fallback) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return fallback;
  return Math.min(Math.round(parsed), MAX_TIMEOUT_MS);
}

function expandHome(value, home) {
  if (value === "~") return home;
  if (value.startsWith("~/")) return join(home, value.slice(2));
  return value;
}

function readConfigFile(file) {
  try {
    const parsed = JSON.parse(readFileSync(file, "utf8"));
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
  } catch {
    // A missing or malformed file is not an error: it means "not configured".
    return {};
  }
}

// loadUsageConfig resolves the reporting configuration. Every input is
// injectable so the resolution can be tested without touching the real home.
export function loadUsageConfig({ cwd, env = process.env, home = homedir() } = {}) {
  const fromUser = readConfigFile(join(home, ...USER_CONFIG_PATH));
  const fromProject = cwd ? readConfigFile(join(cwd, ...PROJECT_CONFIG_PATH)) : {};
  const merged = { ...fromUser, ...fromProject };
  const statsDb = text(env.MUSEFORGE_STATS_DB) ?? text(merged.statsDb);
  return {
    url: text(env.MUSEFORGE_URL) ?? text(merged.url),
    token: text(env.MUSEFORGE_INSIGHT_TOKEN) ?? text(merged.token),
    statsDb: statsDb ? expandHome(statsDb, home) : join(home, ...DEFAULT_STATS_DB),
    timeoutMs: positiveInteger(
      text(env.MUSEFORGE_TIMEOUT_MS) ?? merged.timeoutMs,
      DEFAULT_TIMEOUT_MS,
    ),
    debug: text(env.MUSEFORGE_DEBUG) === "1" || merged.debug === true,
  };
}

function openStatsDb(file) {
  // Loaded lazily so a runtime without the SQLite module still loads the
  // extension; the handler then reports nothing instead of failing.
  return import("node:sqlite").then(({ DatabaseSync }) => {
    try {
      return new DatabaseSync(file, { readOnly: true });
    } catch {
      // Runtimes that do not honour `readOnly` still read the same rows.
      return new DatabaseSync(file);
    }
  });
}

function sessionKey(sessionFile) {
  const name = basename(sessionFile).replace(/\.jsonl$/i, "");
  if (Buffer.byteLength(name) <= MAX_SESSION_KEY_BYTES) return name;
  return Buffer.from(name).subarray(0, MAX_SESSION_KEY_BYTES).toString("utf8");
}

function amount(value) {
  const number = Number(value);
  return Number.isFinite(number) && number >= 0 ? number : 0;
}

// rounded keeps a cost summation independent of SQLite's aggregation order, so
// re-reading the same rows produces byte-identical payloads (and a retry stays
// a duplicate instead of becoming a content conflict).
function rounded(value) {
  return Math.round(amount(value) * 1e10) / 1e10;
}

function usageEntry(row) {
  const cacheRead = Math.round(amount(row.cache_read_tokens));
  const cacheWrite = Math.round(amount(row.cache_write_tokens));
  const input = Math.round(amount(row.input_tokens)) + cacheRead + cacheWrite;
  const output = Math.round(amount(row.output_tokens));
  const cost = rounded(row.cost_total);
  const unpriced = amount(row.cost_unpriced) > 0;
  // A group with unpriced requests has a lower-bound cost. Reporting 0 would
  // claim the work was free, so a zero total becomes unknown instead.
  const costStatus = !unpriced ? "reported" : cost > 0 ? "estimated" : "unknown";
  return {
    provider: String(row.provider),
    model: String(row.model),
    inputTokens: input,
    outputTokens: output,
    cacheReadTokens: cacheRead,
    cacheWriteTokens: cacheWrite,
    // omp records no reasoning-token count; the contract treats it as optional.
    reasoningTokens: 0,
    costUsd: costStatus === "unknown" ? null : cost,
    costStatus,
  };
}

// aggregateSessionUsage reads the finished session's rows and builds the one
// execution the ingest endpoint accepts. It returns null when the session has
// no usable usage (unknown session, absent database, empty row set).
export async function aggregateSessionUsage({ sessionFile, statsDb }) {
  if (!sessionFile || !statsDb || !existsSync(statsDb)) return null;
  let db = null;
  try {
    db = await openStatsDb(statsDb);
    const rows = db.prepare(SESSION_USAGE_SQL).all(sessionFile);
    if (rows.length === 0) return null;

    let startedAt = Number.POSITIVE_INFINITY;
    let finishedAt = 0;
    let duration = 0;
    let failedRows = 0;
    let abortedRows = 0;
    let usage = [];
    for (const row of rows) {
      usage.push(usageEntry(row));
      startedAt = Math.min(startedAt, Number(row.started_at));
      finishedAt = Math.max(finishedAt, Number(row.finished_at));
      duration += amount(row.duration_ms);
      failedRows += amount(row.failed_rows);
      abortedRows += amount(row.aborted_rows);
    }
    if (!Number.isFinite(startedAt) || finishedAt < startedAt) return null;

    if (usage.length > MAX_USAGE_ENTRIES) {
      // The contract accepts at most 64 entries; a session with more distinct
      // models keeps the heaviest ones rather than being refused outright.
      usage = usage
        .sort((left, right) => right.inputTokens + right.outputTokens - (left.inputTokens + left.outputTokens))
        .slice(0, MAX_USAGE_ENTRIES)
        .sort((left, right) => (left.model < right.model ? -1 : left.model > right.model ? 1 : 0));
    }

    const key = sessionKey(sessionFile);
    return {
      id: `omp-session:${key}`,
      type: "workflow_execution",
      externalExecutionId: key,
      workflow: { id: WORKFLOW_ID, name: WORKFLOW_NAME },
      startedAt: new Date(startedAt).toISOString(),
      finishedAt: new Date(finishedAt).toISOString(),
      executionDurationMs: Math.round(duration),
      status: failedRows > 0 ? "failed" : abortedRows > 0 ? "cancelled" : "succeeded",
      usage,
    };
  } catch {
    return null;
  } finally {
    try {
      if (db) db.close();
    } catch {
      // Closing a read-only handle cannot fail the report.
    }
  }
}

async function postExecution({ url, token, execution, timeoutMs, fetchImpl }) {
  const endpoint = url.replace(/\/+$/, "") + INGEST_PATH;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetchImpl(endpoint, {
      method: "POST",
      // The credential rides in the header, so a redirect is never followed.
      redirect: "error",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body: JSON.stringify(execution),
      signal: controller.signal,
    });
    try {
      await response.arrayBuffer();
    } catch {
      // The status is the only part of the reply that matters.
    }
    if (response.status === 201) return "posted";
    if (response.status === 200) return "duplicate";
    if (response.status === 409) return "conflict";
    return "failed";
  } finally {
    clearTimeout(timer);
  }
}

// reportSessionUsage posts the session's aggregate and returns what happened.
// It resolves for every outcome: "skipped" (nothing to report), "posted",
// "duplicate", "conflict" or "failed". A credential-less configuration never
// reaches the network.
export async function reportSessionUsage({ sessionFile, config, fetchImpl = globalThis.fetch, log }) {
  const token = text(config?.token);
  const url = text(config?.url);
  if (!token || !url || !sessionFile) return "skipped";
  try {
    const execution = await aggregateSessionUsage({ sessionFile, statsDb: config.statsDb });
    if (!execution) return "skipped";
    const outcome = await postExecution({
      url,
      token,
      execution,
      timeoutMs: positiveInteger(config.timeoutMs, DEFAULT_TIMEOUT_MS),
      fetchImpl,
    });
    if (outcome === "failed" && typeof log === "function") {
      log(`report for ${execution.externalExecutionId} was not accepted`);
    }
    return outcome;
  } catch (error) {
    if (typeof log === "function") log(`report failed: ${error?.message ?? String(error)}`);
    return "failed";
  }
}

function writeDebug(message) {
  try {
    process.stderr.write(`[museforge-usage] ${message}\n`);
  } catch {
    // Diagnostics must never surface as an error.
  }
}

export default function registerUsageReporting(pi) {
  if (!pi || typeof pi.on !== "function") return;
  pi.on("session_shutdown", async (event, ctx) => {
    // Everything is inside the guard: a reporting problem must not become a
    // session problem.
    try {
      const sessionFile = ctx?.sessionManager?.getSessionFile?.() ?? "";
      const config = loadUsageConfig({ cwd: ctx?.cwd });
      await reportSessionUsage({
        sessionFile,
        config,
        log: config.debug ? writeDebug : undefined,
      });
    } catch {
      // Quiet by design.
    }
  });
}
