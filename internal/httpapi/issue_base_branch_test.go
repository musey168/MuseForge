package httpapi

import (
	"testing"

)

func TestNormalizeIssueBaseBranch(t *testing.T) {
	for _, branch := range []string{"", "feature/continue-work", "release/2026.10", "修复/套餐", "origin/valid-branch", "@"} {
		got, err := normalizeIssueBaseBranch("  " + branch + "  ")
		if err != nil || got != branch {
			t.Errorf("branch %q: got %q, %v", branch, got, err)
		}
	}
	for _, branch := range []string{"HEAD", "--upload-pack=echo", "feature name", "feature\nname", "feature\x7fname", "foo..bar", "foo@{1}", "foo~1", "foo^", "foo:bar", "foo?", "foo*", "foo[bar", `foo\bar`, "/feature", "feature/", "feature//foo", ".feature", "feature/.hidden", "feature/foo.lock/bar", "feature/foo."} {
		if _, err := normalizeIssueBaseBranch(branch); err == nil {
			t.Errorf("invalid branch %q was accepted", branch)
		}
	}
}
