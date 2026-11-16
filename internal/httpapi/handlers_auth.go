package httpapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ipan/internal/config"
	"ipan/internal/wopan"
)

var phonePattern = regexp.MustCompile(`^1\d{10}$`)

// ---------- 初始化 ----------

// GET /api/setup/status 首次部署引导：是否已设置访问密码。
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"initialized": s.isInitialized(),
		"version":     Version,
	})
}

type setupRequest struct {
	Password  string `json:"password"`
	SignParam string `json:"signParam"`
}

// POST /api/setup 设置访问密码与（可选的）签名密钥。
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	if s.isInitialized() {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "已完成初始化，请直接登录"))
		return
	}
	var req setupRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	password := strings.TrimSpace(req.Password)
	if len([]rune(password)) < 4 {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "访问密码至少 4 位"))
		return
	}
	signParam := strings.TrimSpace(req.SignParam)
	if signParam == "" {
		signParam = config.RandomSignParam()
	}
	if err := s.store.SaveSetting("appToken", password); err != nil {
		writeError(w, err)
		return
	}
	if err := s.store.SaveSetting("signParam", signParam); err != nil {
		writeError(w, err)
		return
	}
	// 初始化成功后直接下发会话 Cookie，便于浏览器直接进入应用
	s.issueSessionCookie(w, r, password)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": password})
}

// issueSessionCookie 把访问密码写入会话 Cookie（浏览器刷新后仍保持登录）。
func (s *Server) issueSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.isSecureRequest(r),
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

// ---------- 登录状态 ----------

// loginStatus 汇总当前云盘绑定情况与自动保活状态（只读本地库，不访问上游）。
func (s *Server) loginStatus() (map[string]any, error) {
	tokens, err := s.store.LoadTokens()
	if err != nil {
		return nil, err
	}
	access := tokens.AccessToken
	if access == "" {
		access = s.cfg.WopanAccessToken
	}
	refresh := tokens.RefreshToken
	if refresh == "" {
		refresh = s.cfg.WopanRefreshToken
	}
	state, err := s.store.LoadRefreshState()
	if err != nil {
		return nil, err
	}
	settings, err := s.store.LoadSettings()
	if err != nil {
		return nil, err
	}

	locked := state != nil && !state.OK && (strings.Contains(state.Error, "8105") || strings.Contains(state.Error, "账户被锁定"))
	// 展示的是令牌续期间隔（refresh token 滑动过期，2 天换一次），
	// 而不是后台检查频率：检查可以更频繁，但真正换令牌按这个间隔。
	intervalHours := refreshInterval.Hours()

	var lastSuccess any
	var nextRefresh any
	var lastAttempt any
	var lastError any
	if state != nil {
		if state.SuccessAt > 0 {
			lastSuccess = state.SuccessAt
			nextRefresh = state.SuccessAt + int64(refreshInterval/time.Millisecond)
		}
		lastAttempt = state.AttemptedAt
		if !state.OK {
			lastError = state.Error
			if lastError == "" {
				lastError = "刷新失败"
			}
		}
	}

	return map[string]any{
		"configured": access != "",
		"phone":      "",
		"showHidden": settings.ShowHidden,
		"autoRefresh": map[string]any{
			"enabled":       refresh != "" && !locked,
			"paused":        locked,
			"intervalHours": intervalHours,
			"lastAttemptAt": lastAttempt,
			"lastSuccessAt": lastSuccess,
			"nextRefreshAt": nextRefresh,
			"lastError":     lastError,
		},
	}, nil
}

// GET /api/auth-check 首屏把鉴权与云盘登录态合成一次往返。
func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	login, err := s.loginStatus()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "login": login})
}

// GET /api/login/status 云盘登录态。
func (s *Server) handleLoginStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	login, err := s.loginStatus()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, login)
}

// POST /api/logout 清除会话 Cookie（不影响访问密码）。
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	if token := extractToken(r); s.isSessionToken(token) {
		_ = s.store.DeleteSession(token)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- 云盘手机号登录 ----------

// POST /api/login/send-code 发送短信验证码。
func (s *Server) handleSendCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	var req struct {
		Phone string `json:"phone"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	phone := strings.TrimSpace(req.Phone)
	if !phonePattern.MatchString(phone) {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "手机号格式不正确"))
		return
	}
	if err := wopan.SendSmsCode(r.Context(), phone); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /api/login/verify 校验验证码并保存云盘令牌（令牌不回传前端）。
func (s *Server) handleLoginVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	var req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	phone := strings.TrimSpace(req.Phone)
	code := strings.TrimSpace(req.Code)
	if !phonePattern.MatchString(phone) {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "手机号格式不正确"))
		return
	}
	if code == "" {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少验证码"))
		return
	}
	access, refresh, err := wopan.LoginVerify(r.Context(), phone, code)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.store.SaveTokens(access, refresh); err != nil {
		writeError(w, err)
		return
	}
	_ = s.store.SaveRefreshState(refreshStateOK())
	_ = s.store.ClearDetailCache()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- 设置 ----------

// GET /api/settings 应用设置。
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.LoadSettings()
	if err != nil {
		writeError(w, err)
		return
	}
	webdav, err := s.webDAVCredentials()
	if err != nil {
		writeError(w, err)
		return
	}
	directDownload, err := s.store.LoadWebDAVDirectDownload()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"signParamConfigured":  settings.SignParam != "",
		"showHidden":           settings.ShowHidden,
		"webdavConfigured":     webdav.enabled(),
		"webdavUsername":       webdav.Username,
		"webdavDirectDownload": directDownload,
	})
}

// POST /api/settings/show-hidden 切换隐藏文件显示。
func (s *Server) handleShowHidden(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	var req struct {
		ShowHidden bool `json:"showHidden"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	value := "0"
	if req.ShowHidden {
		value = "1"
	}
	if err := s.store.SaveSetting("showHidden", value); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "showHidden": req.ShowHidden})
}

// POST /api/settings/password 修改访问密码。
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	var req struct {
		Password    string `json:"password"`
		OldPassword string `json:"oldPassword"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	password := strings.TrimSpace(req.Password)
	if len([]rune(password)) < 4 {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "访问密码至少 4 位"))
		return
	}
	// Passkey 会话已通过生物识别，允许不校验旧密码直接轮换
	if !s.isSessionToken(extractToken(r)) {
		current, err := s.accessPassword()
		if err != nil {
			writeError(w, err)
			return
		}
		if strings.TrimSpace(req.OldPassword) != current {
			writeError(w, wopan.Errorf(http.StatusBadRequest, "当前密码不正确"))
			return
		}
	}
	if err := s.store.SaveSetting("appToken", password); err != nil {
		writeError(w, err)
		return
	}
	s.issueSessionCookie(w, r, password)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": password})
}

// ---------- 令牌刷新 ----------

// POST /api/refresh 手动刷新云盘令牌。
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	result, err := s.refreshWithContext(r.Context(), true)
	if err != nil {
		writeError(w, err)
		return
	}
	if !result.Refreshed {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "未执行刷新：%s", result.Skipped))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "refreshedAt": time.Now().UnixMilli()})
}

func init() {
	_ = json.Marshal
}
