package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ipan/internal/config"
	"ipan/internal/store"
)

func newTestServer(t *testing.T) (*Server, *store.Store, string) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<!doctype html><title>iPan</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{DataDir: t.TempDir(), WopanSpaceType: "0", TokenCheckInterval: 0}
	return NewServer(cfg, st, staticDir), st, staticDir
}

func do(t *testing.T, h http.Handler, method, target string, body string, auth string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是合法 JSON (%d): %s", rec.Code, rec.Body.String())
	}
	return out
}

// 完整走一遍「未初始化 → 设置密码 → 鉴权」流程。
func TestSetupThenAuthFlow(t *testing.T) {
	srv, _, _ := newTestServer(t)
	h := srv.Handler()

	rec := do(t, h, http.MethodGet, "/api/setup/status", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("setup/status 状态码 = %d", rec.Code)
	}
	if body := decode(t, rec); body["initialized"] != false {
		t.Fatalf("尚未初始化: %v", body)
	}

	// 未初始化时业务接口应提示初始化
	rec = do(t, h, http.MethodGet, "/api/list?parentId=0", "", "anything")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未初始化时访问业务接口状态码 = %d", rec.Code)
	}

	// 密码过短被拒绝
	rec = do(t, h, http.MethodPost, "/api/setup", `{"password":"ab"}`, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("短密码应被拒绝，得到 %d: %s", rec.Code, rec.Body)
	}

	rec = do(t, h, http.MethodPost, "/api/setup", `{"password":"test1234"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("设置密码失败 %d: %s", rec.Code, rec.Body)
	}
	if body := decode(t, rec); body["token"] != "test1234" {
		t.Fatalf("初始化未返回令牌: %v", body)
	}
	// 初始化后应下发会话 Cookie，方便浏览器直接进入应用
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("初始化后未下发会话 Cookie")
	}

	// 重复初始化被拒绝
	rec = do(t, h, http.MethodPost, "/api/setup", `{"password":"another"}`, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("重复初始化应失败，得到 %d", rec.Code)
	}

	// 错误密码
	rec = do(t, h, http.MethodGet, "/api/auth-check", "", "wrong")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("错误密码状态码 = %d", rec.Code)
	}

	// 正确密码：auth-check 应同时返回云盘登录态
	rec = do(t, h, http.MethodGet, "/api/auth-check", "", "test1234")
	if rec.Code != http.StatusOK {
		t.Fatalf("auth-check 失败 %d: %s", rec.Code, rec.Body)
	}
	body := decode(t, rec)
	login, ok := body["login"].(map[string]any)
	if !ok {
		t.Fatalf("auth-check 缺少 login 字段: %v", body)
	}
	if login["configured"] != false {
		t.Fatalf("未绑定云盘时 configured 应为 false: %v", login)
	}
	if _, ok := login["autoRefresh"]; !ok {
		t.Fatalf("缺少 autoRefresh 字段: %v", login)
	}

	// 密码同时作为 Bearer token 可通过 ?auth= 传递
	rec = do(t, h, http.MethodGet, "/api/auth-check?auth=test1234", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("?auth= 鉴权失败 %d", rec.Code)
	}
}

func TestSettingsAndPasswordChange(t *testing.T) {
	srv, _, _ := newTestServer(t)
	h := srv.Handler()
	do(t, h, http.MethodPost, "/api/setup", `{"password":"test1234"}`, "")

	rec := do(t, h, http.MethodGet, "/api/settings", "", "test1234")
	body := decode(t, rec)
	if body["signParamConfigured"] != true {
		t.Fatalf("初始化时应生成签名密钥: %v", body)
	}

	rec = do(t, h, http.MethodPost, "/api/settings/show-hidden", `{"showHidden":true}`, "test1234")
	if rec.Code != http.StatusOK {
		t.Fatalf("切换隐藏文件失败 %d", rec.Code)
	}
	rec = do(t, h, http.MethodGet, "/api/settings", "", "test1234")
	if decode(t, rec)["showHidden"] != true {
		t.Fatal("showHidden 未持久化")
	}

	// 旧密码错误时拒绝修改
	rec = do(t, h, http.MethodPost, "/api/settings/password", `{"password":"newpass1","oldPassword":"bad"}`, "test1234")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("旧密码错误应被拒绝，得到 %d", rec.Code)
	}
	rec = do(t, h, http.MethodPost, "/api/settings/password", `{"password":"newpass1","oldPassword":"test1234"}`, "test1234")
	if rec.Code != http.StatusOK {
		t.Fatalf("修改密码失败 %d: %s", rec.Code, rec.Body)
	}

	// 旧密码失效、新密码生效
	if rec := do(t, h, http.MethodGet, "/api/auth-check", "", "test1234"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("旧密码应失效，得到 %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/auth-check", "", "newpass1"); rec.Code != http.StatusOK {
		t.Fatalf("新密码应生效，得到 %d", rec.Code)
	}
}

func TestCloudRoutesRequireBinding(t *testing.T) {
	srv, _, _ := newTestServer(t)
	h := srv.Handler()
	do(t, h, http.MethodPost, "/api/setup", `{"password":"test1234"}`, "")

	cases := []struct {
		method string
		target string
		body   string
	}{
		{http.MethodGet, "/api/list?parentId=0", ""},
		{http.MethodPost, "/api/mkdir?parentId=0&name=x", ""},
		{http.MethodGet, "/api/download?fid=abc", ""},
		{http.MethodPost, "/api/delete", `{"fileList":["a"]}`},
		{http.MethodPost, "/api/upload/init", `{"directoryId":"0","fileName":"a.txt","fileSize":10}`},
		{http.MethodGet, "/api/detail", ""},
		{http.MethodGet, "/api/user/profile", ""},
	}
	for _, c := range cases {
		rec := do(t, h, c.method, c.target, c.body, "test1234")
		// 未绑定云盘时统一返回 409，前端据此引导用户去设置页绑定
		if rec.Code != http.StatusConflict {
			t.Errorf("%s %s 状态码 = %d，期望 409（响应: %s）", c.method, c.target, rec.Code, rec.Body)
		}
	}

	// 分享中心的官方列表失败时不影响平台分享列表返回
	rec := do(t, h, http.MethodGet, "/api/shares?page=0&pageSize=20", "", "test1234")
	if rec.Code != http.StatusOK {
		t.Fatalf("shares 状态码 = %d", rec.Code)
	}
	body := decode(t, rec)
	official := body["official"].(map[string]any)
	if official["error"] == nil {
		t.Fatalf("未绑定云盘时官方分享应带 error: %v", official)
	}
	if platform, ok := body["platform"].([]any); !ok || len(platform) != 0 {
		t.Fatalf("平台分享初始应为空数组: %v", body["platform"])
	}
}

func TestValidationErrors(t *testing.T) {
	srv, _, _ := newTestServer(t)
	h := srv.Handler()
	do(t, h, http.MethodPost, "/api/setup", `{"password":"test1234"}`, "")

	cases := []struct {
		name   string
		method string
		target string
		body   string
		status int
	}{
		{"缺 fid", http.MethodGet, "/api/download", "", http.StatusBadRequest},
		{"手机号非法", http.MethodPost, "/api/login/send-code", `{"phone":"123"}`, http.StatusBadRequest},
		{"验证码缺失", http.MethodPost, "/api/login/verify", `{"phone":"13800138000"}`, http.StatusBadRequest},
		{"下载 zip 缺项目", http.MethodPost, "/api/download-zip", `{}`, http.StatusBadRequest},
		{"上传初始化缺文件大小", http.MethodPost, "/api/upload/init", `{"directoryId":"0","fileName":"a"}`, http.StatusBadRequest},
		{"删除缺项目", http.MethodPost, "/api/delete", `{}`, http.StatusBadRequest},
		{"移动缺目标", http.MethodPost, "/api/move", `{"fileList":["a"]}`, http.StatusBadRequest},
		{"解析缺链接", http.MethodPost, "/api/resolve", `{"url":""}`, http.StatusBadRequest},
		{"方法不允许", http.MethodGet, "/api/delete", "", http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		rec := do(t, h, c.method, c.target, c.body, "test1234")
		if rec.Code != c.status {
			t.Errorf("%s: 状态码 = %d，期望 %d（响应: %s）", c.name, rec.Code, c.status, rec.Body)
		}
	}
}

func TestPublicShareLifecycle(t *testing.T) {
	srv, st, _ := newTestServer(t)
	h := srv.Handler()

	// 直接用 store 造一条分享记录（创建接口需要云盘令牌）
	share, err := srv.createLocalShare([]store.ShareItem{
		{ID: "dir1", Fid: "dir1", Type: 0, Name: "目录"},
		{ID: "f1", Fid: "f1", Type: 1, Name: "a.txt"},
	}, 7)
	if err != nil {
		t.Fatalf("创建平台分享失败: %v", err)
	}
	if len(share.ID) != 10 || len(share.Code) != 4 {
		t.Fatalf("分享 ID/提取码长度异常: %q %q", share.ID, share.Code)
	}

	// 公开元信息无需登录
	rec := do(t, h, http.MethodGet, "/api/public/share?id="+share.ID, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("读取分享元信息失败 %d: %s", rec.Code, rec.Body)
	}
	body := decode(t, rec)
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("分享条目数量 = %d", len(items))
	}

	// 提取码大小写不敏感
	rec = do(t, h, http.MethodPost, "/api/public/share/verify",
		`{"id":"`+share.ID+`","code":"`+strings.ToLower(share.Code)+`"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("正确提取码校验失败 %d: %s", rec.Code, rec.Body)
	}
	rec = do(t, h, http.MethodPost, "/api/public/share/verify",
		`{"id":"`+share.ID+`","code":"ZZZZ"}`, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("错误提取码应返回 400，得到 %d", rec.Code)
	}

	// 下载接口必须带正确提取码；未绑定云盘时应是 409 而不是 403
	rec = do(t, h, http.MethodGet, "/api/public/share/download?id="+share.ID, "", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("缺提取码下载应返回 403，得到 %d", rec.Code)
	}
	rec = do(t, h, http.MethodGet, "/api/public/share/download?id="+share.ID+"&code="+share.Code, "", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("绑定缺失时应返回 409，得到 %d: %s", rec.Code, rec.Body)
	}

	// 不存在的分享
	rec = do(t, h, http.MethodGet, "/api/public/share?id=nope", "", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在的分享应返回 404，得到 %d", rec.Code)
	}

	// 分享页路由对不存在的分享返回 404，对存在的返回 index.html
	rec = do(t, h, http.MethodGet, "/s/nope", "", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在的分享页应返回 404，得到 %d", rec.Code)
	}
	rec = do(t, h, http.MethodGet, "/s/"+share.ID, "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "iPan") {
		t.Fatalf("存在的分享页应返回 SPA 入口，得到 %d: %s", rec.Code, rec.Body)
	}

	// 删除分享（需要登录）
	do(t, h, http.MethodPost, "/api/setup", `{"password":"test1234"}`, "")
	rec = do(t, h, http.MethodPost, "/api/shares/local-delete",
		`{"ids":["`+share.ID+`"]}`, "test1234")
	if rec.Code != http.StatusOK {
		t.Fatalf("删除平台分享失败 %d: %s", rec.Code, rec.Body)
	}
	if _, err := st.GetShare(share.ID); err != store.ErrNotFound {
		t.Fatalf("删除后仍可读到分享: %v", err)
	}
}

func TestBedSignature(t *testing.T) {
	srv, _, _ := newTestServer(t)
	h := srv.Handler()
	do(t, h, http.MethodPost, "/api/setup", `{"password":"test1234"}`, "")

	// 缺少参数
	if rec := do(t, h, http.MethodGet, "/api/bed?fid=x", "", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("缺签名应返回 400，得到 %d", rec.Code)
	}
	// 错误签名
	if rec := do(t, h, http.MethodGet, "/api/bed?fid=x&sign=bad", "", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("错误签名应返回 403，得到 %d", rec.Code)
	}
	// 正确签名后进入云盘调用，未绑定时 409
	signParam, err := srv.ensureSignParam()
	if err != nil {
		t.Fatal(err)
	}
	sign := srv.bedSign("fid123", signParam)
	rec := do(t, h, http.MethodGet, "/api/bed?fid=fid123&sign="+sign, "", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("签名正确时应走到云盘调用（409），得到 %d: %s", rec.Code, rec.Body)
	}
}

func TestStaticAndSPAFallback(t *testing.T) {
	srv, _, staticDir := newTestServer(t)
	h := srv.Handler()

	if err := os.MkdirAll(filepath.Join(staticDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := do(t, h, http.MethodGet, "/", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "iPan") {
		t.Fatalf("首页返回异常 %d", rec.Code)
	}

	rec = do(t, h, http.MethodGet, "/assets/app.js", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("静态资源返回 %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("带哈希的资源应长期缓存，得到 %q", cc)
	}

	// 前端路由回退
	if rec := do(t, h, http.MethodGet, "/settings/deep/link", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("SPA 回退失败 %d", rec.Code)
	}
	// 未知 API 不应回退成 HTML
	rec = do(t, h, http.MethodGet, "/api/unknown", "", "test1234")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		t.Fatalf("未知 API 应返回 JSON 404，得到 %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	// 任何 /dav 前缀的非法路径都不能被 SPA 回退伪装成 WebDAV。
	for _, path := range []string{"/dava", "/davfoo", "/davx/y", "/dav;foo"} {
		if rec := do(t, h, http.MethodGet, path, "", ""); rec.Code != http.StatusNotFound {
			t.Fatalf("非法 WebDAV 前缀 %s 应返回 404，得到 %d", path, rec.Code)
		}
	}
	// 路径穿越
	if rec := do(t, h, http.MethodGet, "/../go.mod", "", ""); rec.Code == http.StatusOK &&
		strings.Contains(rec.Body.String(), "module ipan") {
		t.Fatal("存在路径穿越风险")
	}
}

func TestSessionCookieAuth(t *testing.T) {
	srv, st, _ := newTestServer(t)
	h := srv.Handler()
	do(t, h, http.MethodPost, "/api/setup", `{"password":"test1234"}`, "")

	if err := st.CreateSession("sess_token123", 24*60*60*1e9); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth-check", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "sess_token123"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("会话 Cookie 鉴权失败 %d: %s", rec.Code, rec.Body)
	}

	// 登出后会话失效
	rec = do(t, h, http.MethodPost, "/api/logout", "", "sess_token123")
	if rec.Code != http.StatusOK {
		t.Fatalf("登出失败 %d", rec.Code)
	}
	if st.SessionValid("sess_token123") {
		t.Fatal("登出后会话仍然有效")
	}
}

func TestParseShareLink(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/resolve", nil)
	cases := []struct {
		input     string
		wantID    string
		wantCode  string
		wantError bool
	}{
		{"https://pan.wo.cn/share_code?shareId=abcdef0123456789ab&shareCode=AB12", "abcdef0123456789ab", "AB12", false},
		{"链接：https://pan.wo.cn/share_code?shareId=abcdef0123456789ab 提取码：XY99", "abcdef0123456789ab", "XY99", false},
		{"abcdef0123456789ab", "", "", true},
		{"", "", "", true},
	}
	for _, c := range cases {
		got, err := parseShareLink(req, c.input)
		if c.wantError {
			if err == nil {
				t.Errorf("%q 应当解析失败，得到 %+v", c.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q 解析失败: %v", c.input, err)
			continue
		}
		if got.ShareID != c.wantID || got.ShareCode != c.wantCode {
			t.Errorf("%q 解析结果 = %+v，期望 %s/%s", c.input, got, c.wantID, c.wantCode)
		}
	}
}

func TestBaseURLUsesForwardedHeaders(t *testing.T) {
	srv, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "http://internal:8080/api/direct", nil)
	req.Host = "internal:8080"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "pan.example.com")
	if got := srv.baseURL(req); got != "https://pan.example.com" {
		t.Fatalf("forwarded base URL = %q", got)
	}

	req.Header.Set("X-Forwarded-Host", "bad host/path")
	if got := srv.baseURL(req); got != "https://internal:8080" {
		t.Fatalf("invalid forwarded host fallback = %q", got)
	}
}
