package web

import (
	"regexp"
	"strings"
	"testing"
)

// sidebarCSSBlock returns the body of one CSS rule.
func sidebarCSSBlock(t *testing.T, css, selector string) string {
	t.Helper()
	pattern := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(selector) + `\s*\{([^}]*)\}`)
	m := pattern.FindStringSubmatch(css)
	if m == nil {
		t.Fatalf("missing CSS rule %q", selector)
	}
	return m[1]
}

// sidebarCSSValue reads one property out of a CSS rule body.
func sidebarCSSValue(body, property string) string {
	pattern := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(property) + `\s*:\s*([^;]+);`)
	m := pattern.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}
