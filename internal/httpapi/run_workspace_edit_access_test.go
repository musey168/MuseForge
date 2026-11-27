package httpapi

import (
	"agentflow/internal/domain"
	"bytes"
	"net/http"
	"testing"
)

// Workspace edit access follows the project owner: the account that holds
// the project may edit, another account may not.
func TestWorkspaceEditAllowedFollowsTheProjectOwner(t *testing.T) {
	h, owner := workbenchFixture(t)
	users := map[string]domain.User{"owner": owner}
	for _, name := range []string{"other", "outsider"} {
		if err := h.repo.PutUser(domain.User{LoginName: "ws-" + name}); err != nil {
			t.Fatal(err)
		}
		users[name], _ = h.repo.UserByLoginName("ws-" + name)
	}
	if err := h.repo.PutProject(domain.Project{ID: "unowned", Name: "Unowned"}); err != nil {
		t.Fatal(err)
	}
	runs := &runsHandlers{deps: h}
	runIn := func(projectID string) domain.Run {
		return domain.Run{ID: "run-" + projectID, ExecutionContext: domain.TelemetryContext{ProjectID: projectID}}
	}
	cases := []struct {
		name, user, project string
		want                bool
	}{
		{"the account that holds the project", "owner", "workbench", true},
		{"another account", "other", "workbench", false},
		{"outsider", "outsider", "workbench", false},
		{"unowned project", "outsider", "unowned", true},
		{"missing project", "outsider", "missing", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := workbenchContext(users[tc.user], http.MethodGet, "/", bytes.NewBuffer(nil))
			if got := runs.workspaceEditAllowed(c.Request, runIn(tc.project), users[tc.user].ID); got != tc.want {
				t.Fatalf("workspaceEditAllowed = %v, want %v", got, tc.want)
			}
		})
	}
}
