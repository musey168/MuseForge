package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"agentflow/internal/application"
)

// The capability manifest is a live contract: /api/capabilities serves it and
// the MCP mount turns it into tools. Every operation must name a route the
// platform registers, or a caller is told to call something that answers 404.
func TestCapabilityManifestNamesOnlyLiveRoutes(t *testing.T) {
	registered := map[string]bool{}
	for _, entry := range personalRoutes {
		registered[entry.method+" "+entry.path] = true
	}
	placeholder := regexp.MustCompile(`\{[a-zA-Z]+\}`)
	for _, op := range application.PublicOperations() {
		path := placeholder.ReplaceAllString(op.Path, ":param")
		// The registered table spells the parameters with their real names, so
		// compare on the shape.
		shape := regexp.MustCompile(`:[a-zA-Z]+`).ReplaceAllString(path, ":param")
		matched := false
		for entry := range registered {
			method, registeredPath, ok := strings.Cut(entry, " ")
			if !ok || method != op.Method {
				continue
			}
			if regexp.MustCompile(`:[a-zA-Z]+`).ReplaceAllString(registeredPath, ":param") == shape {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("operation %s names %s %s, which no route serves", op.Name, op.Method, op.Path)
		}
	}
}

// Every mutating operation must be able to reach its handler: a manifest entry
// whose method the platform answers with 405 is as broken as a missing route.
func TestCapabilityManifestDeclaresItsReadOnlyFlag(t *testing.T) {
	for _, op := range application.PublicOperations() {
		readOnly := op.Method == http.MethodGet
		if op.ReadOnly != readOnly {
			t.Errorf("operation %s is %s but declares readOnly=%v", op.Name, op.Method, op.ReadOnly)
		}
	}
}
