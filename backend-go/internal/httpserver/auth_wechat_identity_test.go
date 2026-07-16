package httpserver

import (
	"path/filepath"
	"testing"
)

func TestCodeOnlyWeChatLoginReusesLinkedAccount(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	demo, ok := findUserByEmail(data.Users, "demo@xianzhi.ai")
	if !ok {
		t.Fatal("demo user is missing")
	}
	accountBefore, err := store.PointAccount(demo.ID)
	if err != nil {
		t.Fatal(err)
	}
	const openID = "wechat-openid-linked-to-demo"
	if _, err := store.UpdateAdminCustomer(demo.ID, adminCustomerMutation{WeChatOpenID: openID}); err != nil {
		t.Fatal(err)
	}
	accountAfter, err := store.PointAccount(demo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if accountAfter.Available != accountBefore.Available {
		t.Fatalf("linking WeChat changed available points: before=%d after=%d", accountBefore.Available, accountAfter.Available)
	}
	data, err = store.AdminData()
	if err != nil {
		t.Fatal(err)
	}

	auth := newAuthAPI(store, newLocalAuthSessions())
	updatedData, user, err := auth.userForWeChatMiniProgramSession(data, wechatMiniProgramSession{OpenID: openID, SessionKey: "session-key"})
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != demo.ID {
		t.Fatalf("linked login user = %s, want %s", user.ID, demo.ID)
	}
	if len(updatedData.Users) != len(data.Users) {
		t.Fatalf("linked login created a duplicate user: before=%d after=%d", len(data.Users), len(updatedData.Users))
	}
}
