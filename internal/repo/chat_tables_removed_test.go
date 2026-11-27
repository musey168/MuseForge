package repo

import "testing"

// The chat workbench tables are gone: a fresh database must not create them.
func TestFreshSchemaHasNoChatTables(t *testing.T) {
	st, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for _, table := range []string{
		"assistant_sessions",
		"assistant_generations",
		"assistant_tasks",
		"assistant_preferences",
		"assistant_diagnostics",
		"assistant_session_reads",
		"chat_rooms",
		"chat_messages",
		"requirement_spaces",
		"space_members",
		"space_artifacts",
		"space_artifact_revisions",
		"space_threads",
	} {
		if st.DB.Migrator().HasTable(table) {
			t.Errorf("a fresh schema still creates the chat table %q", table)
		}
	}
}
