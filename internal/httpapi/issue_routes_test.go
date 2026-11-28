package httpapi

import (
	"testing"

)

func TestIssueRoutesCoverRegisteredPaths(t *testing.T) {
	for path, route := range issueRoutes {
		if route.handle == nil || len(route.methods) == 0 {
			t.Fatalf("%s: incomplete route", path)
		}
	}
	if len(issueRoutes) != 11 {
		t.Fatalf("expected 11 issue sub-routes, got %d", len(issueRoutes))
	}
}
