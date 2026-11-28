package repo

import (
	"testing"
)

// removedTables lists every table the personal build retired. The list is
// explicit on purpose: a guard that computed the removed set from the current
// manifest would pass by construction and prove nothing.
//
// The list was taken from the pre-trim schema — the upstream tree at c0b61e05 —
// minus the tables that survive. When a surface is retired its tables are added
// here; nothing is ever taken out of this list.
var removedTables = []string{
	"agents",
	"api_tokens",
	"asset_versions",
	"assistant_diagnostics",
	"assistant_generations",
	"assistant_preferences",
	"assistant_session_reads",
	"assistant_sessions",
	"assistant_tasks",
	"bot_binding_challenges",
	"bot_chat_origins",
	"bot_clarify_sessions",
	"bot_connections",
	"bot_conversations",
	"bot_interactions",
	"bot_native_sessions",
	"bot_pending_drafts",
	"bot_user_bindings",
	"business_domain_assignments",
	"business_domains",
	"chat_messages",
	"chat_rooms",
	"definition_revisions",
	"engine_instances",
	"evo_asset_gates",
	"evo_findings",
	"evo_policies",
	"evo_proposals",
	"evo_queue",
	"evo_replays",
	"evo_samples",
	"evo_signals",
	"evo_sop_policies",
	"executors",
	"generic_templates",
	"knowledge_connector_usage_counts",
	"knowledge_connectors",
	"knowledge_document_usage_counts",
	"knowledge_settings",
	"local_accounts",
	"knowledge_submissions",
	"knowledge_usage_events",
	"mcp_files",
	"mcp_servers",
	"mr_delivery_states",
	"notification_bots",
	"notification_deliveries",
	"platform_codebuddy_keys",
	"platform_git_http_hosts",
	"platform_git_https_hosts",
	"platform_git_ssh_hosts",
	"platform_settings",
	"preview_blobs",
	"preview_sessions",
	"project_join_requests",
	"project_members",
	"requirement_spaces",
	"rules",
	"run_admission_locks",
	"run_artifact_blob_chunks",
	"run_artifact_blobs",
	"run_artifacts",
	"run_asset_pins",
	"run_evals",
	"run_events",
	"run_logs",
	"run_nodes",
	"run_previews",
	"run_telemetry",
	"run_workspace_checkpoint_chunks",
	"run_workspace_checkpoints",
	"runner_task_events",
	"runner_tasks",
	"runs",
	"sandboxes",
	"schedule_job_execution_stats",
	"schedule_job_executions",
	"schedule_jobs",
	"shared_schedule_versions",
	"skill_files",
	"skills",
	"space_artifact_revisions",
	"space_artifacts",
	"space_members",
	"space_threads",
	"sync_config",
	"tasks",
	"user_bot_credentials",
	"workflows",
}

// TestRemovedTablesAreNotCreated asserts that a fresh database creates none of
// the retired tables. The schema is built from allModels(), so a table that is
// still listed there is still created.
func TestRemovedTablesAreNotCreated(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	for _, table := range removedTables {
		if s.DB.Migrator().HasTable(table) {
			t.Errorf("a fresh database still creates the table %q", table)
		}
	}
}
