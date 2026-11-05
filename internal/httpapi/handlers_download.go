package httpapi

import (
	"io"
	"net/http"
	"strings"

	"ipan/internal/wopan"
)

// passThroughHeaders 是从上游响应中允许透传给浏览器的头白名单。
var passThroughHeaders = []string{
	"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag",
}

// upstreamClient 是用于代理下载的 HTTP 客户端。
// 相比默认客户端，这里禁用了自动解压，避免 Content-Length 与实际字节数不一致。
func upstreamClient() *http.Client {
	base := *wopan.HTTPClient()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	base.Transport = transport
	return &base
}

var proxyHTTPClient = upstreamClient()

// proxyStream 以流式方式转发上游内容（支持 Range 断点续传）。
func proxyStream(w http.ResponseWriter, r *http.Request, upstreamURL string, attachment bool, name string) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstreamURL, nil)
	if err != nil {
		writeError(w, wopan.Errorf(http.StatusBadGateway, "构建下载请求失败: %v", err))
		return
	}
	for k, v := range wopan.CommonHeaders() {
		req.Header.Set(k, v)
	}
	for _, h := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	res, err := proxyHTTPClient.Do(req)
	if err != nil {
		if r.Context().Err() != nil {
			return // 客户端已断开
		}
		writeError(w, wopan.Errorf(http.StatusBadGateway, "获取文件失败: %v", err))
		return
	}
	defer res.Body.Close()

	for _, h := range passThroughHeaders {
		if v := res.Header.Get(h); v != "" && h != "Content-Length" && h != "Content-Type" {
			w.Header().Set(h, v)
		}
	}
	if ct := res.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cl := res.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	w.Header().Set("Cache-Control", "private, max-age=0, no-store")
	w.Header().Set("Content-Disposition", contentDisposition(attachment, name))
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
}

// redirectTo 302 跳转到上游直链，不携带 Referer。
func redirectTo(w http.ResponseWriter, location string) {
	w.Header().Set("Location", location)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusFound)
}

// GET /api/download?fid= 302 跳转到联通云盘直链。
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	fid, err := queriedRequired(r, "fid")
	if err != nil {
		writeError(w, err)
		return
	}
	url, err := s.client().GetDownloadURL(r.Context(), fid)
	if err != nil {
		writeError(w, err)
		return
	}
	name := sanitizeFilename(r.URL.Query().Get("name"))
	if r.URL.Query().Get("name") == "" {
		name = "download"
	}
	w.Header().Set("Content-Disposition", contentDisposition(true, name))
	redirectTo(w, wopan.NormalizeRedirectURL(url))
}

// GET /api/view?fid= 流式预览（图片/音视频/文本）。
func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	fid, err := queriedRequired(r, "fid")
	if err != nil {
		writeError(w, err)
		return
	}
	url, err := s.client().GetDownloadURL(r.Context(), fid)
	if err != nil {
		writeError(w, err)
		return
	}
	name := sanitizeFilename(r.URL.Query().Get("name"))
	proxyStream(w, r, url, false, name)
}

// GET /api/proxy-download?fid= 流式下载（附件）。
func (s *Server) handleProxyDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	fid, err := queriedRequired(r, "fid")
	if err != nil {
		writeError(w, err)
		return
	}
	url, err := s.client().GetDownloadURL(r.Context(), fid)
	if err != nil {
		writeError(w, err)
		return
	}
	name := sanitizeFilename(r.URL.Query().Get("name"))
	proxyStream(w, r, url, true, name)
}

// GET /api/bed?fid=&sign= 文件床：签名校验后 302 或流式转发。
func (s *Server) handleBed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	fid := r.URL.Query().Get("fid")
	sign := r.URL.Query().Get("sign")
	if fid == "" || sign == "" {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少 fid 或 sign"))
		return
	}
	signParam, err := s.ensureSignParam()
	if err != nil {
		writeError(w, err)
		return
	}
	if s.bedSign(fid, signParam) != sign {
		writeError(w, wopan.Errorf(http.StatusForbidden, "签名无效"))
		return
	}
	url, err := s.client().GetDownloadURL(r.Context(), fid)
	if err != nil {
		writeError(w, err)
		return
	}
	name := sanitizeFilename(r.URL.Query().Get("name"))
	if r.URL.Query().Get("mode") == "proxy" {
		proxyStream(w, r, url, false, name)
		return
	}
	w.Header().Set("Content-Disposition", contentDisposition(false, name))
	redirectTo(w, wopan.NormalizeRedirectURL(url))
}

// directEntry 是直链结果条目。
type directEntry struct {
	Fid    string `json:"fid"`
	URL    string `json:"url"`
	BedURL string `json:"bedUrl"`
}

// GET /api/direct?fid= 返回单个直链。
func (s *Server) handleDirect(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		fid, err := queriedRequired(r, "fid")
		if err != nil {
			writeError(w, err)
			return
		}
		url, err := s.client().GetDownloadURL(r.Context(), fid)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"url": url})
	case http.MethodPost:
		var req struct {
			Fids []string `json:"fids"`
		}
		if err := readJSON(r, &req); err != nil {
			writeError(w, err)
			return
		}
		fids := nonEmpty(req.Fids)
		if len(fids) == 0 {
			writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少 fids"))
			return
		}
		urls, err := s.client().GetDownloadURLs(r.Context(), fids)
		if err != nil {
			writeError(w, err)
			return
		}
		signParam, err := s.ensureSignParam()
		if err != nil {
			writeError(w, err)
			return
		}
		origin := s.baseURL(r)
		out := make([]directEntry, 0, len(urls))
		for _, u := range urls {
			out = append(out, directEntry{
				Fid:    u.Fid,
				URL:    u.URL,
				BedURL: origin + "/api/bed?fid=" + escape(u.Fid) + "&sign=" + s.bedSign(u.Fid, signParam),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"urls": out})
	default:
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
	}
}

// packItems 是打包下载的条目。
type packItems struct {
	FileFids []string
	DirIDs   []string
}

// packItemsFromBody 解析 { items: [{ id, fid, type }] }。
func packItemsFromBody(body struct {
	Items []struct {
		ID   string  `json:"id"`
		Fid  string  `json:"fid"`
		Type float64 `json:"type"`
	} `json:"items"`
}) (packItems, error) {
	var out packItems
	for _, it := range body.Items {
		fid := it.Fid
		if fid == "" {
			fid = it.ID
		}
		if int(it.Type) == 0 {
			if it.ID != "" {
				out.DirIDs = append(out.DirIDs, it.ID)
			}
		} else if fid != "" {
			out.FileFids = append(out.FileFids, fid)
		}
	}
	if len(out.FileFids) == 0 && len(out.DirIDs) == 0 {
		return out, wopan.Errorf(http.StatusBadRequest, "缺少待打包的项目")
	}
	return out, nil
}

// POST /api/download-zip 流式下载 zip 打包。
func (s *Server) handleDownloadZip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	var body struct {
		Items []struct {
			ID   string  `json:"id"`
			Fid  string  `json:"fid"`
			Type float64 `json:"type"`
		} `json:"items"`
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	items, err := packItemsFromBody(struct {
		Items []struct {
			ID   string  `json:"id"`
			Fid  string  `json:"fid"`
			Type float64 `json:"type"`
		} `json:"items"`
	}{Items: body.Items})
	if err != nil {
		writeError(w, err)
		return
	}
	packURL, err := s.client().GetPackDownloadURL(r.Context(), items.FileFids, items.DirIDs)
	if err != nil {
		writeError(w, err)
		return
	}
	zipName := sanitizeFilename(body.Name)
	if body.Name == "" {
		zipName = "wopan-download"
	}
	// 打包下载走 302 会让浏览器丢掉文件名，因此这里流式转发
	proxyStream(w, r, packURL, true, zipName+".zip")
}

// POST /api/pack-url 返回打包直链。
func (s *Server) handlePackURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	var body struct {
		Items []struct {
			ID   string  `json:"id"`
			Fid  string  `json:"fid"`
			Type float64 `json:"type"`
		} `json:"items"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	items, err := packItemsFromBody(body)
	if err != nil {
		writeError(w, err)
		return
	}
	url, err := s.client().GetPackDownloadURL(r.Context(), items.FileFids, items.DirIDs)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": url})
}

func nonEmpty(items []string) []string {
	out := make([]string, 0, len(items))
	for _, v := range items {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func escape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "&", "%26"), "#", "%23")
}
