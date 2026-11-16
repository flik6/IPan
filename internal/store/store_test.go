package store

import (
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSettingsRoundTrip(t *testing.T) {
	st := newTestStore(t)
	if err := st.SaveSetting("appToken", "secret"); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSetting("showHidden", "1"); err != nil {
		t.Fatal(err)
	}
	settings, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.AppToken != "secret" || !settings.ShowHidden || !settings.WebDAVDirectDownload {
		t.Fatalf("设置读取不正确: %+v", settings)
	}
	if err := st.SaveWebDAVDirectDownload(false); err != nil {
		t.Fatal(err)
	}
	settings, _ = st.LoadSettings()
	if settings.WebDAVDirectDownload {
		t.Fatalf("下载直连应已关闭: %+v", settings)
	}
	// 覆盖写
	if err := st.SaveSetting("appToken", "next"); err != nil {
		t.Fatal(err)
	}
	settings, _ = st.LoadSettings()
	if settings.AppToken != "next" {
		t.Fatalf("覆盖写失败: %+v", settings)
	}
}

func TestTokens(t *testing.T) {
	st := newTestStore(t)
	tokens, err := st.LoadTokens()
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken != "" {
		t.Fatal("空库应当返回空令牌")
	}
	if err := st.SaveTokens("access-1", "refresh-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveTokens("access-2", "refresh-2"); err != nil {
		t.Fatal(err)
	}
	tokens, _ = st.LoadTokens()
	if tokens.AccessToken != "access-2" || tokens.RefreshToken != "refresh-2" {
		t.Fatalf("令牌写入不正确: %+v", tokens)
	}
}

func TestSharesExpiry(t *testing.T) {
	st := newTestStore(t)
	items := []ShareItem{{ID: "id1", Fid: "fid1", Type: 1, Name: "a.txt"}}

	permanent := Share{ID: "keep", Code: "ABCD", Items: items, CreatedAt: time.Now().UnixMilli()}
	if err := st.CreateShare(permanent); err != nil {
		t.Fatal(err)
	}
	expired := Share{
		ID: "gone", Code: "WXYZ", Items: items,
		CreatedAt: time.Now().Add(-48 * time.Hour).UnixMilli(),
		ExpiresAt: time.Now().Add(-24 * time.Hour).UnixMilli(),
	}
	if err := st.CreateShare(expired); err != nil {
		t.Fatal(err)
	}

	if _, err := st.GetShare("keep"); err != nil {
		t.Fatalf("永久分享应当可读: %v", err)
	}
	if _, err := st.GetShare("gone"); err != ErrNotFound {
		t.Fatalf("过期分享应当视为不存在，得到: %v", err)
	}

	list, err := st.ListShares()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "keep" {
		t.Fatalf("列表应只包含未过期的分享: %+v", list)
	}

	ok, err := st.DeleteShare("keep")
	if err != nil || !ok {
		t.Fatalf("删除失败: %v %v", ok, err)
	}
	if ok, _ := st.DeleteShare("keep"); ok {
		t.Fatal("重复删除应返回 false")
	}
}

func TestSessions(t *testing.T) {
	st := newTestStore(t)
	if err := st.CreateSession("sess_abc", time.Hour); err != nil {
		t.Fatal(err)
	}
	if !st.SessionValid("sess_abc") {
		t.Fatal("新会话应当有效")
	}
	if st.SessionValid("sess_nope") || st.SessionValid("") {
		t.Fatal("未知会话不应有效")
	}
	// 过期会话
	if err := st.CreateSession("sess_old", -time.Minute); err != nil {
		t.Fatal(err)
	}
	if st.SessionValid("sess_old") {
		t.Fatal("过期会话不应有效")
	}
	if err := st.PurgeExpired(); err != nil {
		t.Fatal(err)
	}
	if st.SessionValid("sess_old") {
		t.Fatal("清理后过期会话仍未删除")
	}
}

func TestDetailCache(t *testing.T) {
	st := newTestStore(t)
	if payload, _, err := st.LoadDetailCache(); err != nil || payload != nil {
		t.Fatalf("空缓存应返回 nil: %v %v", payload, err)
	}
	if err := st.SaveDetailCache([]byte(`{"phone":"123"}`), 1000); err != nil {
		t.Fatal(err)
	}
	payload, at, err := st.LoadDetailCache()
	if err != nil || at != 1000 || string(payload) != `{"phone":"123"}` {
		t.Fatalf("缓存读取不正确: %s %d %v", payload, at, err)
	}
	if err := st.ClearDetailCache(); err != nil {
		t.Fatal(err)
	}
	if payload, _, _ := st.LoadDetailCache(); payload != nil {
		t.Fatal("清除后仍能读到缓存")
	}
}

func TestRefreshState(t *testing.T) {
	st := newTestStore(t)
	if st, err := st.LoadRefreshState(); err != nil || st != nil {
		t.Fatalf("空状态应返回 nil: %v %v", st, err)
	}
	want := RefreshState{OK: false, AttemptedAt: 123, SuccessAt: 100, Error: "boom"}
	if err := st.SaveRefreshState(want); err != nil {
		t.Fatal(err)
	}
	got, err := st.LoadRefreshState()
	if err != nil || got == nil {
		t.Fatalf("读取状态失败: %v", err)
	}
	if got.OK != want.OK || got.AttemptedAt != want.AttemptedAt || got.Error != want.Error {
		t.Fatalf("状态不一致: %+v", got)
	}
}

func TestPasskeyCredentials(t *testing.T) {
	st := newTestStore(t)
	creds, err := st.ListPasskeyCredentials()
	if err != nil || len(creds) != 0 {
		t.Fatalf("初始应无凭证: %v", err)
	}
	if err := st.SavePasskeyCredential(PasskeyCredential{ID: "c1", Credential: `{"id":"c1"}`, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	// 同 ID 覆盖
	if err := st.SavePasskeyCredential(PasskeyCredential{ID: "c1", Credential: `{"id":"c1","n":2}`, CreatedAt: 2}); err != nil {
		t.Fatal(err)
	}
	cred, err := st.GetPasskeyCredential("c1")
	if err != nil || cred.Credential != `{"id":"c1","n":2}` {
		t.Fatalf("凭证覆盖失败: %+v %v", cred, err)
	}
	if _, err := st.GetPasskeyCredential("missing"); err != ErrNotFound {
		t.Fatalf("缺失凭证应返回 ErrNotFound: %v", err)
	}
}

func TestWebDAVCredentials(t *testing.T) {
	st := newTestStore(t)
	got, err := st.LoadWebDAV()
	if err != nil || got.Enabled() {
		t.Fatalf("初始 WebDAV 凭据应为空: %+v %v", got, err)
	}
	want := WebDAVCredentials{Username: "dav", MD5HA1: "md5", SHA256HA1: "sha256", Managed: true}
	if err := st.SaveWebDAV(want); err != nil {
		t.Fatal(err)
	}
	got, err = st.LoadWebDAV()
	if err != nil || got != want {
		t.Fatalf("WebDAV 凭据读取不一致: %+v %v", got, err)
	}
	if err := st.ClearWebDAV(); err != nil {
		t.Fatal(err)
	}
	got, err = st.LoadWebDAV()
	if err != nil || got.Enabled() || !got.Managed {
		t.Fatalf("清除后 WebDAV 凭据应为空: %+v %v", got, err)
	}
}
