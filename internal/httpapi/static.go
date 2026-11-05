package httpapi

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// handleStatic 提供前端构建产物，并对 Vue 管理的路由回退到 index.html。
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if s.staticDir == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "前端资源未构建"})
		return
	}

	cleanPath := path.Clean("/" + r.URL.Path)
	filePath := filepath.Join(s.staticDir, filepath.FromSlash(cleanPath))

	// 阻止路径穿越
	if !strings.HasPrefix(filePath, filepath.Clean(s.staticDir)+string(os.PathSeparator)) &&
		filePath != filepath.Clean(s.staticDir) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		// 公开分享页的分享不存在时返回真正的 404，避免 SPA 入口伪装成有效页面
		if strings.HasPrefix(cleanPath, "/s/") {
			id := strings.Trim(strings.TrimPrefix(cleanPath, "/s/"), "/")
			if _, serr := s.store.GetShare(id); serr != nil {
				s.serveIndexWithStatus(w, r, http.StatusNotFound)
				return
			}
		}
		// 其余前端路由（Vue 管理）回退到 index.html
		s.serveIndex(w, r)
		return
	}

	// 带内容哈希的静态资源可长期缓存；HTML 入口每次校验
	if strings.HasPrefix(cleanPath, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	if strings.HasSuffix(cleanPath, ".html") {
		w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	}
	http.ServeFile(w, r, filePath)
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	s.serveIndexWithStatus(w, r, http.StatusOK)
}

func (s *Server) serveIndexWithStatus(w http.ResponseWriter, r *http.Request, status int) {
	indexPath := filepath.Join(s.staticDir, "index.html")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "前端资源未构建，请先运行 npm run build（或使用包含前端产物的 Docker 镜像）",
		})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
