package httpapi

import (
	"net/http"
	"strings"

	"ipan/internal/store"
	"ipan/internal/wopan"
)

type webDAVSettingsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleWebDAVSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		creds, err := s.webDAVCredentials()
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": creds.enabled(),
			"username":   creds.Username,
			"path":       "/dav/",
		})
	case http.MethodPost:
		var req webDAVSettingsRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, err)
			return
		}
		username := strings.TrimSpace(req.Username)
		password := req.Password
		if username == "" {
			writeError(w, wopan.Errorf(http.StatusBadRequest, "请输入 WebDAV 用户名"))
			return
		}
		if strings.Contains(username, ":") {
			writeError(w, wopan.Errorf(http.StatusBadRequest, "WebDAV 用户名不能包含冒号"))
			return
		}
		if len([]rune(password)) < 4 {
			writeError(w, wopan.Errorf(http.StatusBadRequest, "WebDAV 密码至少 4 位"))
			return
		}
		creds := makeDAVCredentials(username, password)
		if err := s.store.SaveWebDAV(store.WebDAVCredentials{
			Username:  creds.Username,
			MD5HA1:    creds.MD5HA1,
			SHA256HA1: creds.SHA256HA1,
		}); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "configured": true, "username": username})
	case http.MethodDelete:
		if err := s.store.ClearWebDAV(); err != nil {
			writeError(w, err)
			return
		}
		creds, err := s.webDAVCredentials()
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         true,
			"configured": creds.enabled(),
			"username":   creds.Username,
		})
	default:
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
	}
}
