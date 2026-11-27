package web

import (
	"os"
	"testing"
)

// readThemeSource reads a frontend source file that owns the theme key.
func readThemeSource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
