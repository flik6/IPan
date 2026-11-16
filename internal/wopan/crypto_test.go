package wopan

import (
	"encoding/json"
	"testing"
)

// 上游使用 AES-CBC + 固定 IV，这里验证加解密与 JS 实现的行为一致。
func TestEncryptDecryptRoundTrip(t *testing.T) {
	const key = "XFmi9GS2hzk98jGX"
	param := map[string]any{"spaceType": "0", "parentDirectoryId": "0", "pageNum": 0, "pageSize": 50}

	encrypted, err := EncryptParam(key, param)
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	plain, err := DecryptParam(key, encrypted)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(plain), &got); err != nil {
		t.Fatalf("解密结果不是合法 JSON: %v", err)
	}
	if got["parentDirectoryId"] != "0" {
		t.Fatalf("解密内容不正确: %v", got)
	}
}

// 加解密必须能处理非 16 字节整数倍的明文（PKCS#7 填充）。
func TestEncryptHandlesBlockUnalignedPlaintext(t *testing.T) {
	for _, s := range []string{"", "a", "0123456789abcde", "0123456789abcdef", "0123456789abcdef0"} {
		enc, err := EncryptParam(ClientSecret, s)
		if err != nil {
			t.Fatalf("%q 加密失败: %v", s, err)
		}
		dec, err := DecryptParam(ClientSecret, enc)
		if err != nil {
			t.Fatalf("%q 解密失败: %v", s, err)
		}
		want, _ := json.Marshal(s)
		if dec != string(want) {
			t.Fatalf("%q 往返不一致: %q", s, dec)
		}
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	enc, err := EncryptParam(ClientSecret, map[string]any{"x": 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptParam(PCClientSecret, enc); err == nil {
		t.Fatal("用错误密钥解密应当失败")
	}
}

// 过短的令牌不能导致 aes.NewCipher 报错。
func TestTokenKeyPadsShortTokens(t *testing.T) {
	for _, token := range []string{"", "short", "0123456789abcdef", "0123456789abcdefLONGER"} {
		key := TokenKey(token)
		if len(key) != 16 {
			t.Fatalf("TokenKey(%q) 长度 = %d，期望 16", token, len(key))
		}
		if _, err := EncryptWithToken(token, map[string]any{"a": 1}); err != nil {
			t.Fatalf("TokenKey(%q) 无法加密: %v", token, err)
		}
	}
}

func TestMD5(t *testing.T) {
	// 固定输入输出，用于校验文件床签名
	if got := MD5("abc"); got != "900150983cd24fb0d6963f7d28e17f72" {
		t.Fatalf("MD5(abc) = %s", got)
	}
}

func TestFileTypeCode(t *testing.T) {
	cases := map[string]string{
		"photo.JPG":    "1",
		"movie.mp4":    "2",
		"song.flac":    "3",
		"report.docx":  "4",
		"archive.zip":  "5",
		"noext":        "5",
		"trailingdot.": "5",
	}
	for name, want := range cases {
		if got := FileTypeCode(name); got != want {
			t.Errorf("FileTypeCode(%q) = %s, 期望 %s", name, got, want)
		}
	}
}

func TestPickString(t *testing.T) {
	data := map[string]any{
		"userId": "",
		"nested": map[string]any{"headUrl": "https://x/y.png"},
		"list":   []any{"first"},
	}
	if v, ok := PickString(data, "userId", "nested.headUrl"); !ok || v != "https://x/y.png" {
		t.Fatalf("PickString 回退失败: %q %v", v, ok)
	}
	if v, ok := PickString(data, "list.0"); !ok || v != "first" {
		t.Fatalf("PickString 数组取值失败: %q %v", v, ok)
	}
	if _, ok := PickString(data, "missing"); ok {
		t.Fatal("缺失字段不应命中")
	}
}

func TestNormalizeRedirectURL(t *testing.T) {
	in := "https://cdn.example.com/a.zip?x=1&y=a b&z=%2f"
	got := NormalizeRedirectURL(in)
	want := "https://cdn.example.com/a.zip?x=1&y=a%20b&z=%2F"
	if got != want {
		t.Fatalf("NormalizeRedirectURL = %q, 期望 %q", got, want)
	}
}

func TestShareFilesCollectsNestedEntries(t *testing.T) {
	payload := map[string]any{
		"files": []any{
			map[string]any{"fid": "f1", "name": "a.txt", "type": float64(1)},
			map[string]any{"id": "d1", "name": "dir", "type": float64(0)},
		},
	}
	entries := ShareFiles(payload)
	if len(entries) != 2 {
		t.Fatalf("收集到 %d 个条目，期望 2", len(entries))
	}
	if entries[0].IsDir || !entries[1].IsDir {
		t.Fatalf("目录/文件类型判断错误: %+v", entries)
	}
}
