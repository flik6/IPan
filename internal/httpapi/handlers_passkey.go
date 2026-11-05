package httpapi

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"ipan/internal/store"
	"ipan/internal/wopan"
)

const rpName = "iPan"

// webauthnUser 适配 go-webauthn 的用户接口。
type webauthnUser struct {
	id          []byte
	name        string
	credentials []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte                         { return u.id }
func (u *webauthnUser) WebAuthnName() string                       { return u.name }
func (u *webauthnUser) WebAuthnDisplayName() string                { return u.name }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// rpID 取请求域名；反向代理部署时需开启 IPAN_TRUST_PROXY 才能得到外部域名。
func (s *Server) rpID(r *http.Request) string {
	host := r.Host
	if s.cfg.TrustProxy {
		if xh := r.Header.Get("X-Forwarded-Host"); xh != "" {
			host = strings.TrimSpace(strings.Split(xh, ",")[0])
		}
	}
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

func (s *Server) newWebAuthn(r *http.Request) (*webauthn.WebAuthn, error) {
	origin := s.baseURL(r)
	rpID := s.rpID(r)
	// WebAuthn 要求 RP ID 是域名，局域网里用 IP 直接访问时无法注册 Passkey
	if ip := net.ParseIP(rpID); ip != nil {
		return nil, wopan.Errorf(http.StatusBadRequest,
			"Passkey 需要域名访问（当前为 IP 地址 %s），请改用域名或配置 HTTPS 反向代理", rpID)
	}
	return webauthn.New(&webauthn.Config{
		RPDisplayName: rpName,
		RPID:          rpID,
		RPOrigins:     []string{origin},
	})
}

// loadUser 读取全部已注册凭证并构造 WebAuthn 用户。
func (s *Server) loadUser() (*webauthnUser, error) {
	records, err := s.store.ListPasskeyCredentials()
	if err != nil {
		return nil, err
	}
	user := &webauthnUser{id: []byte("ipan-user"), name: "ipan"}
	for _, rec := range records {
		var cred webauthn.Credential
		if err := json.Unmarshal([]byte(rec.Credential), &cred); err != nil {
			continue
		}
		user.credentials = append(user.credentials, cred)
	}
	return user, nil
}

// passkeyChallengeSession 把挑战码暂存到会话里，随响应下发后续校验使用。
//
// 与原 Worker 版本的无状态 HMAC 挑战码不同，这里用 go-webauthn 内置的
// SessionData（含 challenge），因此需要临时保存。为保持无状态部署体验，
// 把 SessionData 编码后放进 HttpOnly Cookie，服务端不落库。
const passkeySessionCookie = "ipan_passkey_session"

func (s *Server) setPasskeySession(w http.ResponseWriter, r *http.Request, session *webauthn.SessionData) error {
	if session == nil {
		return wopan.Errorf(http.StatusInternalServerError, "缺少 Passkey 会话")
	}
	// 只保留完成校验必需的字段
	payload := map[string]any{
		"c":   session.Challenge,
		"u":   session.UserID,
		"uv":  session.UserVerification,
		"exp": time.Now().Add(5 * time.Minute).Unix(),
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     passkeySessionCookie,
		Value:    encodeSessionCookie(buf),
		Path:     "/api/passkey/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
		MaxAge:   300,
	})
	return nil
}

func (s *Server) takePasskeySession(r *http.Request) (*webauthn.SessionData, error) {
	c, err := r.Cookie(passkeySessionCookie)
	if err != nil || c.Value == "" {
		return nil, wopan.Errorf(http.StatusBadRequest, "Passkey 会话已过期，请重试")
	}
	raw, ok := decodeSessionCookie(c.Value)
	if !ok {
		return nil, wopan.Errorf(http.StatusBadRequest, "Passkey 会话无效，请重试")
	}
	var payload struct {
		C   string `json:"c"`
		U   []byte `json:"u"`
		UV  bool   `json:"uv"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, wopan.Errorf(http.StatusBadRequest, "Passkey 会话无效，请重试")
	}
	if payload.Exp < time.Now().Unix() {
		return nil, wopan.Errorf(http.StatusBadRequest, "Passkey 会话已过期，请重试")
	}
	uv := protocol.VerificationPreferred
	if payload.UV {
		uv = protocol.VerificationRequired
	}
	return &webauthn.SessionData{
		Challenge:        payload.C,
		UserID:           payload.U,
		UserVerification: uv,
	}, nil
}

// POST /api/passkey/register/options 生成注册选项。
func (s *Server) handlePasskeyRegisterOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	wa, err := s.newWebAuthn(r)
	if err != nil {
		writeError(w, wopan.Errorf(http.StatusInternalServerError, "初始化 Passkey 失败: %v", err))
		return
	}
	user, err := s.loadUser()
	if err != nil {
		writeError(w, err)
		return
	}
	options, session, err := wa.BeginRegistration(user,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		}),
		webauthn.WithExclusions(credentialsToDescriptors(user.credentials)),
	)
	if err != nil {
		writeError(w, wopan.Errorf(http.StatusInternalServerError, "生成注册选项失败: %v", err))
		return
	}
	if err := s.setPasskeySession(w, r, session); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"options": options})
}

// POST /api/passkey/register/verify 校验注册响应并保存凭证。
func (s *Server) handlePasskeyRegisterVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	wa, err := s.newWebAuthn(r)
	if err != nil {
		writeError(w, wopan.Errorf(http.StatusInternalServerError, "初始化 Passkey 失败: %v", err))
		return
	}
	session, err := s.takePasskeySession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	user, err := s.loadUser()
	if err != nil {
		writeError(w, err)
		return
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(r.Body)
	if err != nil {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "解析注册响应失败: %v", err))
		return
	}
	credential, err := wa.CreateCredential(user, *session, parsed)
	if err != nil {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "Passkey 注册校验失败: %v", err))
		return
	}
	record, err := json.Marshal(credential)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.store.SavePasskeyCredential(store.PasskeyCredential{
		ID:         string(credential.ID),
		Credential: string(record),
		CreatedAt:  time.Now().UnixMilli(),
	}); err != nil {
		writeError(w, err)
		return
	}
	clearPasskeySessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /api/passkey/auth/options 生成认证选项（无需登录）。
func (s *Server) handlePasskeyAuthOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	user, err := s.loadUser()
	if err != nil {
		writeError(w, err)
		return
	}
	if len(user.credentials) == 0 {
		writeError(w, wopan.Errorf(http.StatusNotFound, "尚未注册 Passkey，请先用访问密码登录"))
		return
	}
	wa, err := s.newWebAuthn(r)
	if err != nil {
		writeError(w, wopan.Errorf(http.StatusInternalServerError, "初始化 Passkey 失败: %v", err))
		return
	}
	options, session, err := wa.BeginLogin(user,
		webauthn.WithUserVerification(protocol.VerificationPreferred),
	)
	if err != nil {
		writeError(w, wopan.Errorf(http.StatusInternalServerError, "生成认证选项失败: %v", err))
		return
	}
	if err := s.setPasskeySession(w, r, session); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"options": options})
}

// POST /api/passkey/auth/verify 校验认证并签发会话令牌。
func (s *Server) handlePasskeyAuthVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	wa, err := s.newWebAuthn(r)
	if err != nil {
		writeError(w, wopan.Errorf(http.StatusInternalServerError, "初始化 Passkey 失败: %v", err))
		return
	}
	session, err := s.takePasskeySession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	user, err := s.loadUser()
	if err != nil {
		writeError(w, err)
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(r.Body)
	if err != nil {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "解析认证响应失败: %v", err))
		return
	}
	credential, err := wa.ValidateLogin(user, *session, parsed)
	if err != nil {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "Passkey 认证校验失败: %v", err))
		return
	}
	// 计数器变更需写回，否则下次校验会被判为重放
	record, err := json.Marshal(credential)
	if err == nil {
		_ = s.store.SavePasskeyCredential(store.PasskeyCredential{
			ID:         string(credential.ID),
			Credential: string(record),
			CreatedAt:  time.Now().UnixMilli(),
		})
	}

	token := "sess_" + randomBase64URL(32)
	if err := s.store.CreateSession(token, sessionTTL); err != nil {
		writeError(w, err)
		return
	}
	clearPasskeySessionCookie(w)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "expiresInDays": 180})
}

func credentialsToDescriptors(creds []webauthn.Credential) []protocol.CredentialDescriptor {
	out := make([]protocol.CredentialDescriptor, 0, len(creds))
	for _, c := range creds {
		out = append(out, c.Descriptor())
	}
	return out
}

func clearPasskeySessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     passkeySessionCookie,
		Value:    "",
		Path:     "/api/passkey/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}
