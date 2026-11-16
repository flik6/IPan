// Package store 提供基于 SQLite 的本地持久化层。
//
// 单文件数据库（data/ipan.db）即可承载全部状态：
//   - settings      单例配置（访问密码、签名密钥、显示偏好）
//   - wopan_tokens  云盘令牌
//   - refresh_state 令牌保活状态
//   - shares        平台内分享
//   - passkey_creds WebAuthn 凭证
//   - sessions      Passkey 会话令牌
//   - detail_cache  账号信息缓存
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound 表示记录不存在。
var ErrNotFound = errors.New("not found")

// Store 封装数据库连接。
type Store struct {
	db   *sql.DB
	mu   sync.Mutex // 串行化写操作，SQLite 单写者更稳
	Path string
}

const schema = `
CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wopan_tokens (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  access_token  TEXT NOT NULL DEFAULT '',
  refresh_token TEXT NOT NULL DEFAULT '',
  updated_at    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS refresh_state (
  id           INTEGER PRIMARY KEY CHECK (id = 1),
  ok           INTEGER NOT NULL,
  attempted_at INTEGER NOT NULL,
  success_at   INTEGER NOT NULL DEFAULT 0,
  error        TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS shares (
  id         TEXT PRIMARY KEY,
  code       TEXT NOT NULL,
  items      TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_shares_created ON shares(created_at DESC);

CREATE TABLE IF NOT EXISTS passkey_creds (
  id          TEXT PRIMARY KEY,
  credential  TEXT NOT NULL,
  created_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  token      TEXT PRIMARY KEY,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS detail_cache (
  id        INTEGER PRIMARY KEY CHECK (id = 1),
  payload   TEXT NOT NULL,
  cached_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS dav_properties (
  resource TEXT PRIMARY KEY,
  properties BLOB NOT NULL
);
`

// Open 打开（必要时创建）数据目录与数据库。
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}
	path := filepath.Join(dataDir, "ipan.db")
	dsn := path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// modernc 的 sqlite 驱动在单连接下最稳，写并发由 mu 串行化
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化数据库结构失败（%s，请检查数据目录写权限）: %w", path, err)
	}
	return &Store{db: db, Path: path}, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// DB 返回底层连接（供健康检查等使用）。
func (s *Store) DB() *sql.DB { return s.db }

// ---------- settings ----------

// GetSetting 读取配置项，不存在时返回空串。
func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// SetSetting 写入配置项。
func (s *Store) SetSetting(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// Settings 是应用配置快照。
type Settings struct {
	AppToken             string `json:"appToken"`
	SignParam            string `json:"signParam"`
	ShowHidden           bool   `json:"showHidden"`
	WebDAVDirectDownload bool   `json:"webdavDirectDownload"`
}

// LoadSettings 读取全部应用配置。
func (s *Store) LoadSettings() (Settings, error) {
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return Settings{}, err
	}
	defer rows.Close()
	out := Settings{WebDAVDirectDownload: true}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return Settings{}, err
		}
		switch k {
		case "appToken":
			out.AppToken = v
		case "signParam":
			out.SignParam = v
		case "showHidden":
			out.ShowHidden = v == "1" || v == "true"
		case keyWebDAVDirectDownload:
			out.WebDAVDirectDownload = v == "1" || v == "true"
		}
	}
	return out, rows.Err()
}

// SaveSetting 是 SetSetting 的别名，语义更清晰。
func (s *Store) SaveSetting(key, value string) error { return s.SetSetting(key, value) }

// WebDAVCredentials 保存 WebDAV 用户名及 Basic/Digest 校验所需的 HA1 摘要。
type WebDAVCredentials struct {
	Username  string
	MD5HA1    string
	SHA256HA1 string
	Managed   bool
}

// Enabled 判断 WebDAV 凭据是否完整。
func (c WebDAVCredentials) Enabled() bool {
	return c.Username != "" && c.MD5HA1 != "" && c.SHA256HA1 != ""
}

const (
	keyWebDAVUser           = "webdavUser"
	keyWebDAVMd5            = "webdavMd5"
	keyWebDAVSha256         = "webdavSha256"
	keyWebDAVManaged        = "webdavManaged"
	keyWebDAVDirectDownload = "webdavDirectDownload"
)

// LoadWebDAVDirectDownload 读取下载直连开关。未配置时默认开启。
func (s *Store) LoadWebDAVDirectDownload() (bool, error) {
	value, err := s.GetSetting(keyWebDAVDirectDownload)
	if err != nil {
		return false, err
	}
	return value == "" || value == "1" || value == "true", nil
}

// SaveWebDAVDirectDownload 保存下载直连开关。
func (s *Store) SaveWebDAVDirectDownload(enabled bool) error {
	value := "0"
	if enabled {
		value = "1"
	}
	return s.SaveSetting(keyWebDAVDirectDownload, value)
}

// LoadWebDAV 读取后台配置的 WebDAV 凭据。
func (s *Store) LoadWebDAV() (WebDAVCredentials, error) {
	rows, err := s.db.Query(
		`SELECT key, value FROM settings WHERE key IN (?, ?, ?, ?)`,
		keyWebDAVUser, keyWebDAVMd5, keyWebDAVSha256, keyWebDAVManaged,
	)
	if err != nil {
		return WebDAVCredentials{}, err
	}
	defer rows.Close()

	var out WebDAVCredentials
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return WebDAVCredentials{}, err
		}
		switch key {
		case keyWebDAVUser:
			out.Username = value
		case keyWebDAVMd5:
			out.MD5HA1 = value
		case keyWebDAVSha256:
			out.SHA256HA1 = value
		case keyWebDAVManaged:
			out.Managed = value == "1"
		}
	}
	return out, rows.Err()
}

// SaveWebDAV 原子写入 WebDAV 凭据。
func (s *Store) SaveWebDAV(creds WebDAVCredentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for key, value := range map[string]string{
		keyWebDAVUser:    creds.Username,
		keyWebDAVMd5:     creds.MD5HA1,
		keyWebDAVSha256:  creds.SHA256HA1,
		keyWebDAVManaged: "1",
	} {
		if _, err := tx.Exec(
			`INSERT INTO settings (key, value) VALUES (?, ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			key, value,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClearWebDAV 删除后台配置的 WebDAV 凭据。
func (s *Store) ClearWebDAV() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM settings WHERE key IN (?, ?, ?)`,
		keyWebDAVUser, keyWebDAVMd5, keyWebDAVSha256,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO settings (key, value) VALUES (?, '1')
		 ON CONFLICT(key) DO UPDATE SET value = '1'`,
		keyWebDAVManaged,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------- 云盘令牌 ----------

// Tokens 是云盘令牌。
type Tokens struct {
	AccessToken  string
	RefreshToken string
	UpdatedAt    time.Time
}

// LoadTokens 读取令牌，不存在时返回空令牌（不视为错误）。
func (s *Store) LoadTokens() (Tokens, error) {
	var t Tokens
	var updated int64
	err := s.db.QueryRow(`SELECT access_token, refresh_token, updated_at FROM wopan_tokens WHERE id = 1`).
		Scan(&t.AccessToken, &t.RefreshToken, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Tokens{}, nil
	}
	if err != nil {
		return Tokens{}, err
	}
	if updated > 0 {
		t.UpdatedAt = time.UnixMilli(updated)
	}
	return t, nil
}

// SaveTokens 写入令牌。
func (s *Store) SaveTokens(accessToken, refreshToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO wopan_tokens (id, access_token, refresh_token, updated_at) VALUES (1, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET access_token = excluded.access_token,
		   refresh_token = excluded.refresh_token, updated_at = excluded.updated_at`,
		accessToken, refreshToken, time.Now().UnixMilli(),
	)
	return err
}

// ---------- 令牌刷新状态 ----------

// RefreshState 记录上一次令牌保活结果。
type RefreshState struct {
	OK          bool   `json:"ok"`
	AttemptedAt int64  `json:"attemptedAt"`
	SuccessAt   int64  `json:"successAt,omitempty"`
	Error       string `json:"error,omitempty"`
}

// LoadRefreshState 读取刷新状态，不存在时返回 nil。
func (s *Store) LoadRefreshState() (*RefreshState, error) {
	var st RefreshState
	var ok int
	var errText string
	err := s.db.QueryRow(`SELECT ok, attempted_at, success_at, error FROM refresh_state WHERE id = 1`).
		Scan(&ok, &st.AttemptedAt, &st.SuccessAt, &errText)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	st.OK = ok == 1
	st.Error = errText
	return &st, nil
}

// SaveRefreshState 写入刷新状态。
func (s *Store) SaveRefreshState(st RefreshState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ok := 0
	if st.OK {
		ok = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO refresh_state (id, ok, attempted_at, success_at, error) VALUES (1, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET ok = excluded.ok, attempted_at = excluded.attempted_at,
		   success_at = excluded.success_at, error = excluded.error`,
		ok, st.AttemptedAt, st.SuccessAt, st.Error,
	)
	return err
}

// ---------- 平台分享 ----------

// ShareItem 是分享条目。
type ShareItem struct {
	ID   string `json:"id"`
	Fid  string `json:"fid"`
	Type int    `json:"type"` // 0=目录 1=文件
	Name string `json:"name"`
	Size int64  `json:"size,omitempty"`
}

// Share 是平台内分享。
type Share struct {
	ID        string      `json:"id"`
	Code      string      `json:"code"`
	Items     []ShareItem `json:"items"`
	CreatedAt int64       `json:"createdAt"`
	// ExpiresAt 为 0 表示永久
	ExpiresAt int64 `json:"expiresAt"`
}

// CreateShare 写入分享记录。
func (s *Store) CreateShare(sh Share) error {
	items, err := json.Marshal(sh.Items)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(
		`INSERT INTO shares (id, code, items, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		sh.ID, sh.Code, string(items), sh.CreatedAt, sh.ExpiresAt,
	)
	return err
}

// GetShare 读取分享，过期视为不存在。
func (s *Store) GetShare(id string) (*Share, error) {
	var raw string
	var sh Share
	err := s.db.QueryRow(`SELECT id, code, items, created_at, expires_at FROM shares WHERE id = ?`, id).
		Scan(&sh.ID, &sh.Code, &raw, &sh.CreatedAt, &sh.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), &sh.Items); err != nil {
		return nil, fmt.Errorf("分享数据损坏: %w", err)
	}
	if sh.ExpiresAt > 0 && sh.ExpiresAt <= time.Now().UnixMilli() {
		return nil, ErrNotFound
	}
	return &sh, nil
}

// ListShares 返回未过期的全部分享，按创建时间倒序。
func (s *Store) ListShares() ([]Share, error) {
	now := time.Now().UnixMilli()
	rows, err := s.db.Query(
		`SELECT id, code, items, created_at, expires_at FROM shares
		 WHERE expires_at = 0 OR expires_at > ? ORDER BY created_at DESC`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Share, 0, 16)
	for rows.Next() {
		var sh Share
		var raw string
		if err := rows.Scan(&sh.ID, &sh.Code, &raw, &sh.CreatedAt, &sh.ExpiresAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &sh.Items); err != nil {
			continue
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// DeleteShare 删除分享，返回是否确实删除了一条记录。
func (s *Store) DeleteShare(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM shares WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ---------- Passkey ----------

// PasskeyCredential 存储一条 WebAuthn 凭证（序列化后的 JSON）。
type PasskeyCredential struct {
	ID         string
	Credential string
	CreatedAt  int64
}

// SavePasskeyCredential 保存（或更新）凭证。
func (s *Store) SavePasskeyCredential(cred PasskeyCredential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO passkey_creds (id, credential, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET credential = excluded.credential`,
		cred.ID, cred.Credential, cred.CreatedAt,
	)
	return err
}

// GetPasskeyCredential 读取单条凭证。
func (s *Store) GetPasskeyCredential(id string) (*PasskeyCredential, error) {
	var c PasskeyCredential
	err := s.db.QueryRow(`SELECT id, credential, created_at FROM passkey_creds WHERE id = ?`, id).
		Scan(&c.ID, &c.Credential, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListPasskeyCredentials 返回全部凭证。
func (s *Store) ListPasskeyCredentials() ([]PasskeyCredential, error) {
	rows, err := s.db.Query(`SELECT id, credential, created_at FROM passkey_creds ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PasskeyCredential, 0, 4)
	for rows.Next() {
		var c PasskeyCredential
		if err := rows.Scan(&c.ID, &c.Credential, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------- 会话 ----------

// CreateSession 签发一个带过期时间的会话令牌。
func (s *Store) CreateSession(token string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	_, err := s.db.Exec(
		`INSERT INTO sessions (token, created_at, expires_at) VALUES (?, ?, ?)`,
		token, now.UnixMilli(), now.Add(ttl).UnixMilli(),
	)
	return err
}

// SessionValid 判断会话是否有效。
func (s *Store) SessionValid(token string) bool {
	if token == "" {
		return false
	}
	var expires int64
	err := s.db.QueryRow(`SELECT expires_at FROM sessions WHERE token = ?`, token).Scan(&expires)
	if err != nil {
		return false
	}
	return expires > time.Now().UnixMilli()
}

// DeleteSession 删除会话（登出）。
func (s *Store) DeleteSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}

// PurgeExpired 清理过期数据（会话、分享）。
func (s *Store) PurgeExpired() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, now); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM shares WHERE expires_at > 0 AND expires_at <= ?`, now)
	return err
}

// ---------- 账号信息缓存 ----------

// LoadDetailCache 读取账号信息缓存。
func (s *Store) LoadDetailCache() ([]byte, int64, error) {
	var payload string
	var cachedAt int64
	err := s.db.QueryRow(`SELECT payload, cached_at FROM detail_cache WHERE id = 1`).Scan(&payload, &cachedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	return []byte(payload), cachedAt, nil
}

// SaveDetailCache 写入账号信息缓存。
func (s *Store) SaveDetailCache(payload []byte, cachedAt int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO detail_cache (id, payload, cached_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET payload = excluded.payload, cached_at = excluded.cached_at`,
		string(payload), cachedAt,
	)
	return err
}

// ClearDetailCache 清除账号信息缓存。
func (s *Store) ClearDetailCache() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM detail_cache WHERE id = 1`)
	return err
}
