package httpapi

import (
	"testing"

	"agentflow/internal/auth"
	"agentflow/internal/repo"
)

func newSyncAuthUserHandlers(t *testing.T) (*authHandlers, *repo.Store) {
	t.Helper()
	st, err := repo.OpenTest()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &authHandlers{deps: &deps{repo: st}}, st
}

func TestSyncAuthUserRecordPersistsMissingUser(t *testing.T) {
	h, st := newSyncAuthUserHandlers(t)
	user := &auth.User{LoginName: "carol", StaffID: 11, ChineseName: "卡罗尔", DeptID: 2, DeptName: "运营"}
	h.syncAuthUserRecord(user, "gateway")

	saved, ok := st.UserByLoginName("carol")
	if !ok {
		t.Fatal("missing user should be persisted")
	}
	if user.ID == 0 || user.ID != saved.ID {
		t.Fatalf("persisted ID not read back: user=%d saved=%d", user.ID, saved.ID)
	}
	if saved.StaffID != 11 || saved.ChineseName != "卡罗尔" || saved.DeptID != 2 || saved.DeptName != "运营" {
		t.Fatalf("persisted row = %+v", saved)
	}
}

func TestSyncAuthUserRecordWithoutRepoIsNoop(t *testing.T) {
	h := &authHandlers{deps: &deps{}}
	user := &auth.User{LoginName: "dave", ChineseName: "dave"}
	h.syncAuthUserRecord(user, "cookie")
	if user.ID != 0 || user.ChineseName != "dave" {
		t.Fatalf("nil repo should leave user untouched: %+v", user)
	}
}
