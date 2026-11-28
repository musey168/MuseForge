package repo

import (
	"testing"
)

// The requirement table carries the labels and nothing of the retired LOOP
// lifecycle: a fresh database creates none of the removed columns.
func TestIssueTableHasNoFlowColumns(t *testing.T) {
	s, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	for _, gone := range []string{
		"workflow_id",
		"assignee_user_id",
		"latest_task_id",
		"automation_state",
		"delivery_state",
		"sop_mode",
		"playbook_id",
		"sop_bindings",
		"conversation_session_id",
	} {
		if s.DB.Migrator().HasColumn(&issueRow{}, gone) {
			t.Errorf("the issues table still has the column %q", gone)
		}
	}
	if !s.DB.Migrator().HasColumn(&issueRow{}, "tags") {
		t.Error("the issues table has no tags column")
	}
}
