-- Trimmed from ~/.omp/stats.db: the columns the connector reads, and three
-- sessions (succeeded with two models, cancelled with an unpriced row, failed).
CREATE TABLE messages (
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

INSERT INTO messages VALUES
('/sessions/-proj-a/2026-10-01T00-00-00-000Z_aaaa.jsonl', 'a1', '-proj-a', 'MiniMax-M3.1-Flash', 'minimax-code', 'anthropic-messages', 1790812800000, 1000, 200, 'stop', NULL, 100, 10, 40, 0, 150, 0, 0, 0, 0, 0, 0.01, NULL, 0, 'main', NULL),
('/sessions/-proj-a/2026-10-01T00-00-00-000Z_aaaa.jsonl', 'a2', '-proj-a', 'MiniMax-M3.1-Flash', 'minimax-code', 'anthropic-messages', 1790814600000, 2000, 300, 'toolUse', NULL, 200, 20, 60, 0, 280, 0, 0, 0, 0, 0, 0.02, NULL, 0, 'main', NULL),
('/sessions/-proj-a/2026-10-01T00-00-00-000Z_aaaa.jsonl', 'a3', '-proj-a', 'deepseek-flash', 'deepseek', 'openai-completions', 1790814600000, NULL, NULL, 'stop', NULL, 50, 7, 0, 5, 62, 0, 0, 0, 0, 0, 0.5, NULL, 0, 'main', NULL),
('/sessions/-proj-b/2026-10-02T00-00-00-000Z_bbbb.jsonl', 'b1', '-proj-b', 'MiniMax-M3.1-Flash', 'minimax-code', 'anthropic-messages', 1790942400000, 500, 100, 'aborted', NULL, 10, 2, 4, 0, 16, 0, 0, 0, 0, 0, 0, NULL, 1, 'main', NULL),
('/sessions/-proj-c/2026-10-03T00-00-00-000Z_cccc.jsonl', 'c1', '-proj-c', 'MiniMax-M3.1-Flash', 'minimax-cn', 'anthropic-messages', 1791018000000, 250, 50, 'error', 'upstream refused', 8, 1, 2, 0, 11, 0, 0, 0, 0, 0, 0.001, NULL, 0, 'main', NULL);
