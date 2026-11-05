package wopan

import (
	"context"
	"os"
	"testing"
	"time"
)

// 这些用例会真正访问联通云盘，只在提供了令牌时运行：
//
//	IPAN_LIVE_TEST=1 WOPAN_ACCESS_TOKEN=... go test ./internal/wopan -run Live -v
//
// 主要用于验证加密与请求签名在上游是被接受的
// （头字段或参数拼写有误时会拿到业务错误码，而不是网络错误）。
func liveToken(t *testing.T) string {
	t.Helper()
	if os.Getenv("IPAN_LIVE_TEST") != "1" {
		t.Skip("未开启线上测试（设置 IPAN_LIVE_TEST=1）")
	}
	token := os.Getenv("WOPAN_ACCESS_TOKEN")
	if token == "" {
		t.Skip("缺少 WOPAN_ACCESS_TOKEN")
	}
	return token
}

// memTokens 是测试用的内存令牌存储。
type memTokens struct {
	access  string
	refresh string
}

func (m *memTokens) LoadTokens() (string, string, error) { return m.access, m.refresh, nil }
func (m *memTokens) SaveTokens(a, r string) error        { m.access, m.refresh = a, r; return nil }

func liveClient(token string) *Client {
	store := &memTokens{access: token, refresh: os.Getenv("WOPAN_REFRESH_TOKEN")}
	return New(store, Options{
		BaseURL:   os.Getenv("WOPAN_BASE_URL"),
		ZoneURL:   os.Getenv("WOPAN_ZONE_URL"),
		SpaceType: "0",
	})
}

func TestLiveListRoot(t *testing.T) {
	c := liveClient(liveToken(t))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data, err := c.List(ctx, "0", 0, 10)
	if err != nil {
		t.Fatalf("列目录失败: %v", err)
	}
	files, _ := data["files"].([]any)
	t.Logf("根目录返回 %d 个条目", len(files))
}

func TestLiveQueryUser(t *testing.T) {
	c := liveClient(liveToken(t))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	user, err := c.AppQueryUser(ctx)
	if err != nil {
		t.Fatalf("查询账号失败: %v", err)
	}
	phone, _ := PickString(user, "userId", "phone", "userAccount")
	if phone == "" {
		t.Logf("响应字段: %v", user)
		t.Fatal("未能解析出手机号，字段映射可能需要调整")
	}
	t.Logf("账号手机号: %s", phone)
}
