package wopan

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------- 常量 ----------

const (
	ChannelWoHome  = "wohome"
	ChannelWoCloud = "wocloud"
	channelAPIUser = "api-user"

	ClientID     = "1001000021"
	ClientSecret = "XFmi9GS2hzk98jGX"
	// PCClientID 桌面客户端（无 Ice-UUID 的普通 Windows）使用的凭据
	PCClientID     = "1001000209"
	PCClientSecret = "s72g2VY46ahlK588"
	// LoginClientID H5 短信登录使用的凭据
	LoginClientID     = "1001000035"
	LoginClientSecret = "iELf0UL07o6I8eRK"

	DefaultBaseURL = "https://panservice.mail.wo.cn"
	DefaultZoneURL = "https://tjupload.pan.wo.cn"
	AppID          = "10000001"

	// PartSize 分片大小，与原实现保持一致（8MB）
	PartSize = 8 * 1024 * 1024

	aesIV = "wNSOYIB1k1DjY5lA"
)

var httpClient = &http.Client{
	Timeout: 0, // 上传/下载可能很久，由调用方用 context 控制
	Transport: &http.Transport{
		Proxy:                 http.DefaultTransport.(*http.Transport).Proxy,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

// HTTPClient 返回内部复用的客户端（下载代理会克隆它来关闭自动解压等行为）。
func HTTPClient() *http.Client { return httpClient }

// ---------- 加密辅助 ----------

// EncryptParam 用密钥加密 JSON 参数（AES-CBC + base64）。
func EncryptParam(key string, v any) (string, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	// PKCS#7 padding
	padLen := aes.BlockSize - len(plain)%aes.BlockSize
	plain = append(plain, bytes.Repeat([]byte{byte(padLen)}, padLen)...)
	out := make([]byte, len(plain))
	mode := cipher.NewCBCEncrypter(block, []byte(aesIV))
	mode.CryptBlocks(out, plain)
	return base64.StdEncoding.EncodeToString(out), nil
}

// DecryptParam 解密上游返回的参数。
func DecryptParam(key, content string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	if len(raw) == 0 || len(raw)%aes.BlockSize != 0 {
		return "", fmt.Errorf("密文长度非法")
	}
	out := make([]byte, len(raw))
	cipher.NewCBCDecrypter(block, []byte(aesIV)).CryptBlocks(out, raw)
	padLen := int(out[len(out)-1])
	if padLen <= 0 || padLen > aes.BlockSize || padLen > len(out) {
		return "", fmt.Errorf("填充非法")
	}
	return string(out[:len(out)-padLen]), nil
}

// TokenKey 取 access token 的前 16 位作为 AES-128 密钥。
//
// 上游的 access token 长度不固定，过短时补 0 到 16 字节，
// 否则 aes.NewCipher 会因密钥长度非法而失败。
func TokenKey(token string) string {
	if len(token) >= 16 {
		return token[:16]
	}
	return token + strings.Repeat("\x00", 16-len(token))
}

// EncryptWithToken 用 access token 派生的密钥加密。
func EncryptWithToken(token string, v any) (string, error) {
	return EncryptParam(TokenKey(token), v)
}

// DecryptWithToken 用 access token 派生的密钥解密。
func DecryptWithToken(token, content string) (string, error) {
	return DecryptParam(TokenKey(token), content)
}

// MD5 返回十六进制摘要。
func MD5(input string) string {
	sum := md5.Sum([]byte(input))
	return hex.EncodeToString(sum[:])
}

// CalHeader 生成上游请求头中的签名块。
func CalHeader(channel, key string) map[string]any {
	resTime := time.Now().UnixMilli()
	reqSeq := 100000 + randInt(8999)
	sign := MD5(fmt.Sprintf("%s%d%d%s%s", key, resTime, reqSeq, channel, ""))
	return map[string]any{
		"key":     key,
		"resTime": resTime,
		"reqSeq":  reqSeq,
		"channel": channel,
		"sign":    sign,
		"version": "",
	}
}

func randInt(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return int(time.Now().UnixNano() % int64(n))
	}
	return int(v.Int64())
}

// RandomChars 返回指定长度的随机 ASCII 字母串。
func RandomChars(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	for i, v := range buf {
		buf[i] = chars[int(v)%len(chars)]
	}
	return string(buf)
}

// CommonHeaders 是访问上游时的通用请求头。
func CommonHeaders() map[string]string {
	return map[string]string{
		"Origin":     "https://pan.wo.cn",
		"Referer":    "https://pan.wo.cn/",
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/114 Safari/537.36",
	}
}

// formatBatchNo 生成 yyyyMMddHHmmss 形式的批次号。
func formatBatchNo(t time.Time) string {
	return t.Format("20060102150405")
}

// ToInt64 尽力把任意 JSON 值转成整数。
func ToInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	default:
		return 0
	}
}

// readAll 读取响应体并关闭（带大小上限保护可选）。
func readAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }
