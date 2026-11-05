package httpapi

import (
	"encoding/json"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"ipan/internal/wopan"
)

// fileEntry 是列表项中与服务端过滤相关的字段。
type fileEntry struct {
	Name string `json:"name"`
}

// GET /api/list 列目录。隐藏文件过滤在服务端完成（与原实现一致）。
func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	page, err := queriedInt(r, "page", 0)
	if err != nil {
		writeError(w, err)
		return
	}
	pageSize, err := queriedInt(r, "pageSize", 50)
	if err != nil {
		writeError(w, err)
		return
	}
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 50
	}
	parentID := r.URL.Query().Get("parentId")
	if parentID == "" {
		parentID = "0"
	}
	settings, err := s.store.LoadSettings()
	if err != nil {
		writeError(w, err)
		return
	}
	data, err := s.client().List(r.Context(), parentID, page, pageSize)
	if err != nil {
		writeError(w, err)
		return
	}
	all := toArray(data["files"])
	files := make([]any, 0, len(all))
	for _, item := range all {
		if !settings.ShowHidden {
			if m, ok := item.(map[string]any); ok {
				if name, _ := m["name"].(string); strings.HasPrefix(name, ".") {
					continue
				}
			}
		}
		files = append(files, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"files":    files,
		"pageNum":  page,
		"pageSize": pageSize,
		"hasMore":  len(all) >= pageSize,
	})
}

// POST /api/mkdir 新建目录。
func (s *Server) handleMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	parentID, err := queriedRequired(r, "parentId")
	if err != nil {
		writeError(w, err)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少参数 name"))
		return
	}
	result, err := s.client().Mkdir(r.Context(), parentID, name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// POST /api/rename 重命名。
func (s *Server) handleRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	fid, err := queriedRequired(r, "fid")
	if err != nil {
		writeError(w, err)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少参数 name"))
		return
	}
	typ, err := queriedInt(r, "type", 1)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := s.client().Rename(r.Context(), fid, name, typ)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// idPayload 是批量操作常用的请求体。
type idPayload struct {
	TargetDirID string   `json:"targetDirId"`
	DirList     []string `json:"dirList"`
	FileList    []string `json:"fileList"`
}

// idsFromRequest 从 body 或 query 解析目录/文件 ID，兼容两种调用方式。
func idsFromRequest(r *http.Request) (dirList, fileList []string, targetID string, err error) {
	var body idPayload
	if derr := readJSON(r, &body); derr != nil {
		err = derr
		return
	}
	targetID = body.TargetDirID
	if targetID == "" {
		targetID = r.URL.Query().Get("targetDirId")
	}
	if len(body.DirList) > 0 || len(body.FileList) > 0 {
		return body.DirList, body.FileList, targetID, nil
	}
	fid := r.URL.Query().Get("fid")
	if fid == "" {
		err = wopan.Errorf(http.StatusBadRequest, "缺少参数 fid")
		return
	}
	typ, ierr := queriedInt(r, "type", 1)
	if ierr != nil {
		err = ierr
		return
	}
	ids := splitIDs(fid)
	if typ == 0 {
		return ids, nil, targetID, nil
	}
	return nil, ids, targetID, nil
}

func splitIDs(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// POST /api/delete 删除。
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	dirList, fileList, _, err := idsFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if len(dirList) == 0 && len(fileList) == 0 {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少待删除的项目"))
		return
	}
	result, err := s.client().Delete(r.Context(), dirList, fileList)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// POST /api/move 移动。
func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
	s.handleMoveOrCopy(w, r, "move")
}

// POST /api/copy 复制。
func (s *Server) handleCopy(w http.ResponseWriter, r *http.Request) {
	s.handleMoveOrCopy(w, r, "copy")
}

func (s *Server) handleMoveOrCopy(w http.ResponseWriter, r *http.Request, action string) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	dirList, fileList, targetID, err := idsFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if targetID == "" {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少目标目录 targetDirId"))
		return
	}
	if len(dirList) == 0 && len(fileList) == 0 {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少待操作的项目"))
		return
	}
	var result any
	if action == "move" {
		result, err = s.client().Move(r.Context(), targetID, dirList, fileList)
	} else {
		result, err = s.client().Copy(r.Context(), targetID, dirList, fileList)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ---------- 上传 ----------

// POST /api/upload/init 创建分片上传会话。
func (s *Server) handleUploadInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	var req struct {
		DirectoryID string   `json:"directoryId"`
		FileName    string   `json:"fileName"`
		FileSize    *float64 `json:"fileSize"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	directoryID := req.DirectoryID
	if directoryID == "" {
		directoryID = r.URL.Query().Get("directoryId")
	}
	if directoryID == "" {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少参数 directoryId"))
		return
	}
	if strings.TrimSpace(req.FileName) == "" {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少文件名"))
		return
	}
	if req.FileSize == nil || *req.FileSize < 0 {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "文件大小非法"))
		return
	}
	session, err := s.client().CreateUploadSession(r.Context(), directoryID, req.FileName, int64(*req.FileSize))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// POST /api/upload/part 上传浏览器切分的一个分片。
func (s *Server) handleUploadPart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	// 浏览器分片默认 8MB，留出表单开销
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "解析上传表单失败: %v", err))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	result, err := s.client().UploadPartForm(r.Context(), r.MultipartForm)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := uploadCodeError(result); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func uploadCodeError(result map[string]any) error {
	if code, ok := result["code"].(string); ok && code != "" && code != "0000" {
		return wopan.Errorf(http.StatusBadGateway, "上传失败: %s", code)
	}
	return nil
}

// PUT /api/upload 流式上传单个文件；POST 走 multipart 多文件上传。
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		s.handleStreamUpload(w, r)
	case http.MethodPost:
		s.handleMultipartUpload(w, r)
	default:
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
	}
}

func (s *Server) handleStreamUpload(w http.ResponseWriter, r *http.Request) {
	directoryID := r.URL.Query().Get("directoryId")
	name := r.URL.Query().Get("name")
	if directoryID == "" || name == "" {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少 directoryId 或 name 参数"))
		return
	}
	size := r.ContentLength
	if size < 0 {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少 Content-Length"))
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	result, err := s.client().UploadStream(r.Context(), directoryID, name, r.Body, size, contentType)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := uploadCodeError(result); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": result})
}

func (s *Server) handleMultipartUpload(w http.ResponseWriter, r *http.Request) {
	directoryID := r.URL.Query().Get("directoryId")
	if directoryID == "" {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少参数 directoryId"))
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "解析上传表单失败: %v", err))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	headers := r.MultipartForm.File["files"]
	if len(headers) == 0 {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少上传文件"))
		return
	}
	results := make([]map[string]any, 0, len(headers))
	for _, fh := range headers {
		file, err := fh.Open()
		if err != nil {
			writeError(w, err)
			return
		}
		contentType := fh.Header.Get("Content-Type")
		if contentType == "" {
			if mt, _, err := mime.ParseMediaType(fh.Header.Get("Content-Type")); err == nil {
				contentType = mt
			}
		}
		result, uerr := s.client().UploadStream(r.Context(), directoryID, fh.Filename, file, fh.Size, contentType)
		file.Close()
		if uerr != nil {
			writeError(w, uerr)
			return
		}
		if cerr := uploadCodeError(result); cerr != nil {
			writeError(w, cerr)
			return
		}
		results = append(results, map[string]any{"name": fh.Filename, "result": result})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "results": results})
}

// ---------- 工具 ----------

func toArray(v any) []any {
	if arr, ok := v.([]any); ok {
		return arr
	}
	return nil
}

func asObject(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func jsonNumberToInt64(v any) int64 {
	switch x := v.(type) {
	case json.Number:
		n, _ := x.Int64()
		return n
	case float64:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}
