package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"agentflow/internal/domain"
)

func TestDispatchEarlyErrorsAndHandoff(t *testing.T) {
	f := cvBNewHTTPFixture(t)
	d := &deps{repo: f.s, engine: f.e}
	if _, err := dispatchIssue(d, dispatchRequest{IssueID: "missing"}); cvBHTTPStatusRound2(err) != http.StatusNotFound {
		t.Fatalf("missing issue = %v", err)
	}
	blocked := domain.Issue{ID: "cvb-r2-blocked", ProjectID: "platform", Title: "Blocked", Status: "closed", AssigneeUserID: f.user.ID}
	if err := f.s.PutIssue(blocked); err != nil {
		t.Fatal(err)
	}
	if _, err := dispatchIssue(d, dispatchRequest{IssueID: blocked.ID}); cvBHTTPStatusRound2(err) != http.StatusConflict {
		t.Fatalf("blocked issue = %v", err)
	}
	noOwner := domain.Issue{ID: "cvb-r2-no-owner", ProjectID: "platform", Title: "No owner", Status: "todo"}
	if err := f.s.PutIssue(noOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := dispatchIssue(d, dispatchRequest{IssueID: noOwner.ID}); err == nil {
		t.Fatal("ownerless issue dispatched")
	}
	invalidRepo := domain.Issue{ID: "cvb-r2-invalid-repo", ProjectID: "platform", Title: "Bad repo", Status: "todo",
		Kind: "bug", AssigneeUserID: f.user.ID, WorkflowID: "default", RepositoryIDs: []string{"missing"}}
	if err := f.s.PutIssue(invalidRepo); err != nil {
		t.Fatal(err)
	}
	if _, err := dispatchIssue(d, dispatchRequest{IssueID: invalidRepo.ID}); cvBHTTPStatusRound2(err) != http.StatusBadRequest {
		t.Fatalf("invalid repository = %v", err)
	}
	// 存量需求没有责任人（保存与项目设 Owner 都会回填，这里直接清空列模拟旧数据）：
	// 派发时按项目 Owner 回填责任人（PatchIssue 写回已存在的行）。
	if err := f.s.PutProject(domain.Project{ID: "cvb-r2-owned", Name: "Owned", OwnerID: f.user.ID}); err != nil {
		t.Fatal(err)
	}
	unassigned := domain.Issue{ID: "cvb-r2-unassigned", ProjectID: "cvb-r2-owned", Title: "Unassigned", Status: "todo",
		Kind: "bug", WorkflowID: "default", RepositoryIDs: []string{"missing"}}
	if err := f.s.PutIssue(unassigned); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.Table("issues").Where("id = ?", unassigned.ID).Update("assignee_user_id", nil).Error; err != nil {
		t.Fatal(err)
	}
	if stored, _ := f.s.Issue(unassigned.ID); stored.AssigneeUserID != 0 {
		t.Fatalf("precondition: legacy issue assignee = %d", stored.AssigneeUserID)
	}
	if _, err := dispatchIssue(d, dispatchRequest{IssueID: unassigned.ID}); cvBHTTPStatusRound2(err) != http.StatusBadRequest {
		t.Fatalf("unassigned issue with invalid repository = %v", err)
	}
	if stored, ok := f.s.Issue(unassigned.ID); !ok || stored.AssigneeUserID != f.user.ID {
		t.Fatalf("dispatch assignee = %+v (found=%v), want project owner %d", stored.AssigneeUserID, ok, f.user.ID)
	}
	rerun := invalidRepo
	rerun.ID, rerun.RepositoryIDs = "cvb-r2-rerun", nil
	if err := f.s.PutIssue(rerun); err != nil {
		t.Fatal(err)
	}
	if _, err := dispatchIssue(d, dispatchRequest{IssueID: rerun.ID, Rerun: true}); cvBHTTPStatusRound2(err) != http.StatusBadRequest {
		t.Fatalf("premature rerun = %v", err)
	}

	issue := domain.Issue{SOPBindings: []domain.IssueSOPBinding{
		{ID: "first", WorkflowID: "wf-one"},
		{ID: "second", WorkflowID: "wf-two", DependsOn: []string{"first"}, Handoff: []string{"plan.md"}},
	}}
	text := appendSOPHandoff(issue, domain.Project{}, "wf-two", "input")
	if !strings.Contains(text, "wf-one") || !strings.Contains(text, "plan.md") {
		t.Fatalf("handoff = %q", text)
	}
	if appendSOPHandoff(issue, domain.Project{}, "missing", "input") != "input" {
		t.Fatal("unbound workflow changed input")
	}
	if primaryTaskID("issue", "workflow") != primaryTaskID("issue", "workflow") {
		t.Fatal("primary task id is not deterministic")
	}
}

func cvBHTTPStatusRound2(err error) int {
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Status
	}
	return 0
}
