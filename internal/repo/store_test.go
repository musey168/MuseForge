package repo

import (
	"testing"
)

func TestOpenRequiresDatasource(t *testing.T) {
	t.Setenv("AGENTFLOW_DATASOURCE_DSN", "")
	t.Setenv("AGENTFLOW_MYSQL_DSN", "")
	s, err := Open("")
	if err == nil {
		s.Close()
		t.Fatal("Open without a datasource must fail: the platform does not invent state somewhere else")
	}
}
