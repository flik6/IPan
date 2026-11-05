// Package config 负责从环境变量读取运行配置。
//
// 与原来的 Worker 版本不同，Go 版本有本地数据目录，因此访问密码、
// 文件床签名密钥、云盘令牌等都可以直接持久化到 SQLite，
// 不再依赖 KV 绑定。环境变量仅用于首次启动的预置值与部署参数。
package config

import (
	"crypto/rand"
	"os"
	"strconv"
	"strings"
)

// Config 是进程级配置，启动时读取一次。
type Config struct {
	// Listen 监听地址，例如 ":8080"
	Listen string
	// DataDir 数据目录，存放 SQLite 数据库
	DataDir string

	// 预置值（可选）：库中没有对应记录时用这些值初始化
	AppToken           string
	SignParam          string
	WopanAccessToken   string
	WopanRefreshToken  string
	WopanBaseURL       string
	WopanZoneURL       string
	SysConf            string
	WopanSpaceType     string
	WopanFamilyID      string
	PublicBaseURL      string
	TrustProxy         bool
	TokenCheckInterval int // 小时，0 表示禁用后台保活
	EnablePasskey      bool
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return v
}

// Load 读取环境变量并填充默认值。
func Load() *Config {
	interval, err := strconv.Atoi(env("TOKEN_CHECK_INTERVAL_HOURS", "1"))
	if err != nil || interval < 0 {
		interval = 1
	}
	return &Config{
		Listen:             env("IPAN_LISTEN", ":8080"),
		DataDir:            env("IPAN_DATA_DIR", "./data"),
		AppToken:           env("APP_TOKEN", ""),
		SignParam:          env("SIGN_PARAM", ""),
		WopanAccessToken:   env("WOPAN_ACCESS_TOKEN", ""),
		WopanRefreshToken:  env("WOPAN_REFRESH_TOKEN", ""),
		WopanBaseURL:       env("WOPAN_BASE_URL", ""),
		WopanZoneURL:       env("WOPAN_ZONE_URL", ""),
		SysConf:            env("WOPAN_SYS_CONF", ""),
		WopanSpaceType:     env("WOPAN_SPACE_TYPE", "0"),
		WopanFamilyID:      env("WOPAN_FAMILY_ID", ""),
		PublicBaseURL:      strings.TrimSuffix(env("IPAN_PUBLIC_URL", ""), "/"),
		TrustProxy:         envBool("IPAN_TRUST_PROXY", false),
		TokenCheckInterval: interval,
		EnablePasskey:      envBool("IPAN_ENABLE_PASSKEY", true),
	}
}

const signAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

// RandomSignParam 生成 32 位随机签名密钥（base32 风格字符集，可直接拼进 URL）。
func RandomSignParam() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	var b strings.Builder
	for _, v := range buf {
		b.WriteByte(signAlphabet[int(v)%len(signAlphabet)])
	}
	return b.String()
}
