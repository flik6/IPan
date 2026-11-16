// Package httpapi 提供 HTTP 接口层，路由与响应结构与原 Worker 版本保持一致，
// 使现成的 Vue 前端可以原样复用。
package httpapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ipan/internal/config"
	"ipan/internal/store"
	"ipan/internal/wopan"
)

// Server 聚合所有依赖。
type Server struct {
	cfg   *config.Config
	store *store.Store

	// 静态资源目录与 SPA 入口
	staticDir string

	// 云盘令牌保活相关的进程内互斥
	refreshMu sync.Mutex
	// 账号信息回源互斥，避免并发请求同时打上游
	detailMu sync.Mutex

	davMu    sync.RWMutex
	davLocks map[string]davLock
	davAuth  davDigest
}

// ---------- 通用响应 ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	buf, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"编码响应失败"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, wopan.StatusOf(err), map[string]string{"error": err.Error()})
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
	dec.UseNumber()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil // 空请求体按「全部缺省」处理
		}
		return wopan.Errorf(http.StatusBadRequest, "请求体解析失败: %v", err)
	}
	return nil
}

// ---------- 鉴权 ----------

const sessionCookieName = "ipan_session"
const sessionTTL = 180 * 24 * time.Hour

// accessPassword 返回当前访问密码。数据库优先，环境变量仅作为未初始化时的预置值。
func (s *Server) accessPassword() (string, error) {
	settings, err := s.store.LoadSettings()
	if err != nil {
		return "", err
	}
	if pw := firstToken(settings.AppToken); pw != "" {
		return pw, nil
	}
	return firstToken(s.cfg.AppToken), nil
}

// firstToken 取逗号分隔列表中的第一个有效值（兼容历史配置）。
func firstToken(raw string) string {
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			return v
		}
	}
	return ""
}

func (s *Server) isInitialized() bool {
	pw, err := s.accessPassword()
	return err == nil && pw != ""
}

// extractToken 依次尝试 Authorization、Cookie 与 ?auth= 查询参数。
func extractToken(r *http.Request) string {
	if v := r.Header.Get("Authorization"); v != "" {
		if len(v) > 7 && strings.EqualFold(v[:7], "Bearer ") {
			return strings.TrimSpace(v[7:])
		}
		return strings.TrimSpace(v)
	}
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	return r.URL.Query().Get("auth")
}

// isSessionToken 判断是否为有效的 Passkey 会话令牌。
func (s *Server) isSessionToken(token string) bool {
	return strings.HasPrefix(token, "sess_") && s.store.SessionValid(token)
}

// requireAuth 校验请求是否携带有效凭据。
func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	password, err := s.accessPassword()
	if err != nil {
		writeError(w, err)
		return false
	}
	if password == "" {
		writeError(w, wopan.Errorf(http.StatusUnauthorized, "尚未初始化访问密码"))
		return false
	}
	token := extractToken(r)
	if token == password {
		return true
	}
	if s.isSessionToken(token) {
		return true
	}
	writeError(w, wopan.Errorf(http.StatusUnauthorized, "unauthorized"))
	return false
}

// ---------- 签名密钥 ----------

// ensureSignParam 惰性生成并持久化文件床签名密钥。
func (s *Server) ensureSignParam() (string, error) {
	settings, err := s.store.LoadSettings()
	if err != nil {
		return "", err
	}
	if settings.SignParam != "" {
		return settings.SignParam, nil
	}
	generated := config.RandomSignParam()
	if err := s.store.SaveSetting("signParam", generated); err != nil {
		return generated, nil
	}
	return generated, nil
}

func (s *Server) bedSign(fid, signParam string) string {
	return wopan.MD5(fid + signParam)
}

// ---------- 云盘客户端 ----------

// tokenStore 适配 store.Store 到 wopan.Tokens，并在环境变量提供初始值时播种。
type tokenStore struct {
	store *store.Store
	cfg   *config.Config
}

func (t tokenStore) LoadTokens() (string, string, error) {
	tokens, err := t.store.LoadTokens()
	if err != nil {
		return "", "", err
	}
	if tokens.AccessToken != "" {
		return tokens.AccessToken, tokens.RefreshToken, nil
	}
	// 库中还没有令牌时使用环境变量的预置值，并立即持久化
	access := t.cfg.WopanAccessToken
	refresh := t.cfg.WopanRefreshToken
	if access != "" {
		_ = t.store.SaveTokens(access, refresh)
	}
	return access, refresh, nil
}

func (t tokenStore) SaveTokens(access, refresh string) error {
	return t.store.SaveTokens(access, refresh)
}

// client 创建一个绑定当前令牌的云盘客户端。
func (s *Server) client() *wopan.Client {
	return wopan.New(tokenStore{store: s.store, cfg: s.cfg}, wopan.Options{
		BaseURL:   s.cfg.WopanBaseURL,
		ZoneURL:   s.cfg.WopanZoneURL,
		SpaceType: s.cfg.WopanSpaceType,
		FamilyID:  s.cfg.WopanFamilyID,
	})
}

// ---------- 小工具 ----------

func randomBase64URL(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

func hmacSHA256(secret, msg string) []byte {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(msg))
	return m.Sum(nil)
}

func queriedInt(r *http.Request, key string, fallback int) (int, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return 0, wopan.Errorf(http.StatusBadRequest, "参数 %s 非法", key)
	}
	return v, nil
}

func queriedRequired(r *http.Request, key string) (string, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return "", wopan.Errorf(http.StatusBadRequest, "缺少参数 %s", key)
	}
	return v, nil
}

// baseURL 返回外部可访问的站点根地址（用于生成分享链接）。
func (s *Server) baseURL(r *http.Request) string {
	if s.cfg.PublicBaseURL != "" {
		return s.cfg.PublicBaseURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if s.cfg.TrustProxy {
		if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
			scheme = strings.TrimSpace(strings.Split(xf, ",")[0])
		}
	}
	host := r.Host
	if s.cfg.TrustProxy {
		if xh := r.Header.Get("X-Forwarded-Host"); xh != "" {
			host = strings.TrimSpace(strings.Split(xh, ",")[0])
		}
	}
	return scheme + "://" + host
}

// clientIP 返回客户端地址（考虑反向代理）。
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\x00", "")
	name = strings.TrimSpace(name)
	if name == "" {
		return "download"
	}
	if len(name) > 180 {
		r := []rune(name)
		if len(r) > 180 {
			name = string(r[:180])
		}
	}
	return name
}

// contentDisposition 生成兼容中文文件名的响应头。
func contentDisposition(attachment bool, name string) string {
	kind := "inline"
	if attachment {
		kind = "attachment"
	}
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(name)
	return kind + `; filename="` + escaped + `"; filename*=UTF-8''` + url.PathEscape(name)
}

var errNotFound = errors.New("not found")

func init() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[ipan] ")
}

// NewServer 构造服务实例。
func NewServer(cfg *config.Config, st *store.Store, staticDir string) *Server {
	return &Server{
		cfg: cfg, store: st, staticDir: staticDir, davLocks: make(map[string]davLock),
		davAuth: davDigest{secret: randomBase64URL(32), used: make(map[string]davNonceCount)},
	}
}
