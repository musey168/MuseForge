package repo

import "testing"

// The platform administrator tables are gone: a fresh database must not
// create them.
func TestFreshSchemaHasNoPlatformAdministratorTables(t *testing.T) {
	st, err := OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for _, table := range []string{
		"api_tokens",
		"platform_settings",
		"platform_codebuddy_keys",
		"platform_git_ssh_hosts",
		"platform_git_https_hosts",
		"platform_git_http_hosts",
	} {
		if st.DB.Migrator().HasTable(table) {
			t.Errorf("a fresh schema still creates the platform table %q", table)
		}
	}
}
