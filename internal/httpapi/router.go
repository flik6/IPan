package httpapi

import (
	"net/http"
	"strings"
)

// Handler 组装路由：API 走 mux，其余请求交给静态文件 / SPA 回退。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// ---- 公开接口 ----
	mux.HandleFunc("/api/setup/status", s.handleSetupStatus)
	mux.HandleFunc("/api/setup", s.handleSetup)
	mux.HandleFunc("/api/bed", s.handleBed)
	mux.HandleFunc("/api/public/share", s.handlePublicShareMeta)
	mux.HandleFunc("/api/public/share/verify", s.handlePublicShareVerify)
	mux.HandleFunc("/api/public/share/download", s.handlePublicShareDownload)
	mux.HandleFunc("/api/public/share/file", s.handlePublicShareFile)

	// ---- Passkey（部分接口无需登录）----
	mux.HandleFunc("/api/passkey/register/options", s.auth(s.handlePasskeyRegisterOptions))
	mux.HandleFunc("/api/passkey/register/verify", s.auth(s.handlePasskeyRegisterVerify))
	mux.HandleFunc("/api/passkey/auth/options", s.handlePasskeyAuthOptions)
	mux.HandleFunc("/api/passkey/auth/verify", s.handlePasskeyAuthVerify)

	// ---- 需要登录的接口 ----
	authed := map[string]http.HandlerFunc{
		"/api/auth-check":           s.handleAuthCheck,
		"/api/login/status":         s.handleLoginStatus,
		"/api/login/send-code":      s.handleSendCode,
		"/api/login/verify":         s.handleLoginVerify,
		"/api/logout":               s.handleLogout,
		"/api/settings":             s.handleSettings,
		"/api/settings/webdav":      s.handleWebDAVSettings,
		"/api/settings/show-hidden": s.handleShowHidden,
		"/api/settings/password":    s.handleChangePassword,
		"/api/list":                 s.handleList,
		"/api/mkdir":                s.handleMkdir,
		"/api/rename":               s.handleRename,
		"/api/delete":               s.handleDelete,
		"/api/move":                 s.handleMove,
		"/api/copy":                 s.handleCopy,
		"/api/upload/init":          s.handleUploadInit,
		"/api/upload/part":          s.handleUploadPart,
		"/api/upload":               s.handleUpload,
		"/api/download":             s.handleDownload,
		"/api/view":                 s.handleView,
		"/api/proxy-download":       s.handleProxyDownload,
		"/api/direct":               s.handleDirect,
		"/api/download-zip":         s.handleDownloadZip,
		"/api/pack-url":             s.handlePackURL,
		"/api/resolve":              s.handleResolve,
		"/api/share":                s.handleShare,
		"/api/shares":               s.handleShares,
		"/api/shares/cancel":        s.handleSharesCancel,
		"/api/shares/local-delete":  s.handleSharesLocalDelete,
		"/api/detail":               s.handleDetail,
		"/api/user/profile":         s.handleUserProfile,
		"/api/user/avatar":          s.handleUserAvatar,
		"/api/refresh":              s.handleRefresh,
	}
	for path, h := range authed {
		mux.HandleFunc(path, s.auth(h))
	}

	// 静态资源 + SPA 回退
	mux.HandleFunc("/", s.handleStatic)

	return withMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Dispatch before ServeMux cleans paths or redirects non-GET requests.
		if r.URL.Path == davPrefix || strings.HasPrefix(r.URL.Path, davPrefix+"/") {
			s.handleDAV(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, davPrefix) {
			http.NotFound(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	}))
}

// auth 包装需要登录的处理函数。
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.requireAuth(w, r) {
			return
		}
		next(w, r)
	}
}

// withMiddleware 添加通用中间件：panic 恢复与基础安全响应头。
func withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "服务内部错误"})
			}
		}()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("X-Frame-Options", "DENY")
		}
		next.ServeHTTP(w, r)
	})
}
