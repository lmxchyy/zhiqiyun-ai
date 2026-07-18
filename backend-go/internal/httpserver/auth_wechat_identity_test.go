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

func TestCodeOnlyWeChatLoginPersistsIdentityOnNewAccount(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	auth := newAuthAPI(store, newLocalAuthSessions())
	const openID = "wechat-openid-new-account"
	const unionID = "wechat-unionid-new-account"
	_, created, err := auth.userForWeChatMiniProgramSession(data, wechatMiniProgramSession{
		OpenID: openID, UnionID: unionID, SessionKey: "session-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.WeChatUnionID != unionID {
		t.Fatalf("created union id = %q, want %q", created.WeChatUnionID, unionID)
	}
	if len(created.WeChatOpenIDs) != 1 || created.WeChatOpenIDs[0] != openID {
		t.Fatalf("created open ids = %#v, want %q", created.WeChatOpenIDs, openID)
	}
	updatedData, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	found, ok := findUserByWechatIdentity(updatedData.Users, wechatMiniProgramSession{OpenID: openID, UnionID: unionID})
	if !ok || found.ID != created.ID {
		t.Fatalf("persisted WeChat identity did not resolve created user: found=%t id=%q created=%q", ok, found.ID, created.ID)
	}
}
