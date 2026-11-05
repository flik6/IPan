package httpapi

import (
	"crypto/rand"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ipan/internal/store"
	"ipan/internal/wopan"
)

const (
	shareIDChars   = "abcdefghijkmnpqrstuvwxyz23456789"
	shareCodeChars = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
)

func randomString(n int, alphabet string) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	for i, v := range buf {
		buf[i] = alphabet[int(v)%len(alphabet)]
	}
	return string(buf)
}

// shareItemPayload 是创建分享时的条目。
type shareItemPayload struct {
	ID   string  `json:"id"`
	Fid  string  `json:"fid"`
	Type float64 `json:"type"`
	Name string  `json:"name"`
	Size float64 `json:"size"`
}

// POST /api/share 创建分享，可同时请求官方链接 / 平台链接 / 直链。
func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	var body struct {
		Items   []shareItemPayload `json:"items"`
		Days    float64            `json:"days"`
		Methods []string           `json:"methods"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	days := int(body.Days)
	if body.Days == 0 {
		days = 99999
	}
	methods := body.Methods
	if len(methods) == 0 {
		methods = []string{"official"}
	}
	has := func(m string) bool {
		for _, x := range methods {
			if x == m {
				return true
			}
		}
		return false
	}

	var fileIDs, folderIDs []string
	for _, it := range body.Items {
		id := strings.TrimSpace(it.ID)
		if id == "" {
			continue
		}
		if int(it.Type) == 0 {
			folderIDs = append(folderIDs, id)
		} else {
			fileIDs = append(fileIDs, id)
		}
	}
	// 兼容只传 fid 的旧调用
	if len(fileIDs) == 0 && len(folderIDs) == 0 {
		if fid := r.URL.Query().Get("fid"); fid != "" {
			typ, _ := queriedInt(r, "type", 1)
			if typ == 0 {
				folderIDs = splitIDs(fid)
			} else {
				fileIDs = splitIDs(fid)
			}
			if d, err := queriedInt(r, "days", days); err == nil {
				days = d
			}
		}
	}
	if len(fileIDs) == 0 && len(folderIDs) == 0 {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少分享项目"))
		return
	}

	client := s.client()
	origin := s.baseURL(r)
	result := map[string]any{}
	errorsOut := map[string]string{}

	if has("official") {
		data, err := client.ShareFile(r.Context(), fileIDs, folderIDs, days)
		if err != nil {
			errorsOut["official"] = err.Error()
		} else {
			obj := asObject(data)
			shareID, _ := wopan.PickString(obj, "shareId", "id")
			shareCode, _ := wopan.PickString(obj, "shareCode", "code")
			shareURL, _ := wopan.PickString(obj, "shareUrl", "url", "link")
			if shareURL == "" && shareID != "" {
				shareURL = "https://pan.wo.cn/s/" + shareID
			}
			result["official"] = map[string]any{
				"shareId": shareID, "shareCode": shareCode, "shareUrl": shareURL, "days": days,
			}
		}
	}

	if has("platform") {
		items := make([]store.ShareItem, 0, len(body.Items))
		for _, it := range body.Items {
			id := strings.TrimSpace(it.ID)
			if id == "" {
				continue
			}
			fid := it.Fid
			if fid == "" {
				fid = id
			}
			typ := 1
			if int(it.Type) == 0 {
				typ = 0
			}
			items = append(items, store.ShareItem{ID: id, Fid: fid, Type: typ, Name: it.Name, Size: int64(it.Size)})
		}
		share, err := s.createLocalShare(items, days)
		if err != nil {
			errorsOut["platform"] = err.Error()
		} else {
			result["platform"] = map[string]any{
				"id":        share.ID,
				"code":      share.Code,
				"url":       origin + "/s/" + share.ID,
				"expiresAt": nullableTimestamp(share.ExpiresAt),
			}
		}
	}

	if has("direct") {
		var fids []string
		for _, it := range body.Items {
			if int(it.Type) == 0 {
				continue
			}
			fid := it.Fid
			if fid == "" {
				fid = it.ID
			}
			if fid != "" {
				fids = append(fids, fid)
			}
		}
		entries := make([]directEntry, 0, len(fids))
		urls, err := client.GetDownloadURLs(r.Context(), fids)
		if err != nil {
			errorsOut["direct"] = err.Error()
		} else {
			signParam, _ := s.ensureSignParam()
			for _, u := range urls {
				entries = append(entries, directEntry{
					Fid:    u.Fid,
					URL:    u.URL,
					BedURL: origin + "/api/bed?fid=" + escape(u.Fid) + "&sign=" + s.bedSign(u.Fid, signParam),
				})
			}
			packURL := ""
			if len(folderIDs) > 0 || len(fids) != len(body.Items) {
				packURL, _ = client.GetPackDownloadURL(r.Context(), fids, folderIDs)
			}
			result["direct"] = map[string]any{"files": entries, "packUrl": packURL}
		}
	}

	if len(result) == 0 {
		msg := "分享创建失败"
		for _, v := range errorsOut {
			msg = v
			break
		}
		writeError(w, wopan.Errorf(http.StatusBadGateway, "%s", msg))
		return
	}
	if len(errorsOut) > 0 {
		result["errors"] = errorsOut
	}
	writeJSON(w, http.StatusOK, result)
}

func nullableTimestamp(ms int64) any {
	if ms <= 0 {
		return nil
	}
	return ms
}

// createLocalShare 创建平台内分享。
func (s *Server) createLocalShare(items []store.ShareItem, days int) (*store.Share, error) {
	if len(items) == 0 {
		return nil, wopan.Errorf(http.StatusBadRequest, "没有可分享的项目")
	}
	now := time.Now()
	share := store.Share{
		ID:        randomString(10, shareIDChars),
		Code:      randomString(4, shareCodeChars),
		Items:     items,
		CreatedAt: now.UnixMilli(),
	}
	if days > 0 {
		share.ExpiresAt = now.Add(time.Duration(days) * 24 * time.Hour).UnixMilli()
	}
	if err := s.store.CreateShare(share); err != nil {
		return nil, err
	}
	return &share, nil
}

// GET /api/shares 分享中心：官方分享列表 + 平台分享。
func (s *Server) handleShares(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	page, err := queriedInt(r, "page", 0)
	if err != nil {
		writeError(w, err)
		return
	}
	pageSize, err := queriedInt(r, "pageSize", 20)
	if err != nil {
		writeError(w, err)
		return
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	official := map[string]any{"list": []any{}, "total": 0}
	data, err := s.client().AllShareFileList(r.Context(), page+1, pageSize)
	if err != nil {
		official["error"] = err.Error()
	} else {
		official = convertOfficialShares(data)
	}

	local, err := s.store.ListShares()
	if err != nil {
		writeError(w, err)
		return
	}
	platform := make([]map[string]any, 0, len(local))
	for _, sh := range local {
		platform = append(platform, map[string]any{
			"id":        sh.ID,
			"code":      sh.Code,
			"items":     sh.Items,
			"createdAt": sh.CreatedAt,
			"expiresAt": nullableTimestamp(sh.ExpiresAt),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"official": official,
		"platform": platform,
		"page":     page,
		"pageSize": pageSize,
	})
}

// officialTimePattern 匹配官方返回的 yyyyMMddHHmmss 时间格式。
var officialTimePattern = regexp.MustCompile(`^\d{14}$`)

// convertOfficialShares 把上游分享记录转换为前端结构。
func convertOfficialShares(data map[string]any) map[string]any {
	rawList := toArray(data["list"])
	list := make([]map[string]any, 0, len(rawList))
	for _, item := range rawList {
		row := asObject(item)
		shareID, _ := wopan.PickString(row, "shareId", "id")
		if shareID == "" {
			continue
		}
		count := jsonNumberToInt64(row["num"])
		if count <= 0 {
			count = 1
		}
		fileName, _ := wopan.PickString(row, "fileName", "name")
		if fileName == "" {
			fileName = "未命名"
		}
		display := fileName
		if count > 1 {
			display = fileName + " 等 " + itoa(count) + " 个文件"
		}
		list = append(list, map[string]any{
			"shareId":      shareID,
			"name":         display,
			"count":        count,
			"status":       strOr(row["status"], "1"),
			"shareChannel": strOr(row["shareChannel"], "1"),
			"shareTime":    timestampOrNil(row["shareTime"]),
			"invalidTime":  timestampOrNil(row["invalidTime"]),
			"spaceType":    strOr(row["spaceType"], "0"),
		})
	}
	total := jsonNumberToInt64(data["total"])
	if total == 0 {
		total = int64(len(list))
	}
	return map[string]any{"list": list, "total": total}
}

func strOr(v any, fallback string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return fallback
}

func timestampOrNil(v any) any {
	switch x := v.(type) {
	case string:
		text := strings.TrimSpace(x)
		if text == "" {
			return nil
		}
		if len(text) == 13 {
			return jsonNumberToInt64(text)
		}
		if officialTimePattern.MatchString(text) {
			t, err := time.ParseInLocation("20060102150405", text, time.Local)
			if err == nil {
				return t.UnixMilli()
			}
		}
		if t, err := time.Parse(time.RFC3339, text); err == nil {
			return t.UnixMilli()
		}
		return nil
	default:
		if n := jsonNumberToInt64(v); n > 0 {
			return n
		}
		return nil
	}
}

// POST /api/shares/cancel 取消（operType=1）或删除（2）官方分享。
func (s *Server) handleSharesCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	var body struct {
		ShareIDs []string `json:"shareIds"`
		OperType float64  `json:"operType"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	ids := nonEmpty(body.ShareIDs)
	if len(ids) == 0 {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少 shareIds"))
		return
	}
	operType := 1
	if int(body.OperType) == 2 {
		operType = 2
	}
	if _, err := s.client().CancelOrDeleteShare(r.Context(), ids, operType); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "affected": len(ids)})
}

// POST /api/shares/local-delete 删除平台分享。
func (s *Server) handleSharesLocalDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	ids := nonEmpty(body.IDs)
	if len(ids) == 0 {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少 ids"))
		return
	}
	deleted := 0
	for _, id := range ids {
		ok, err := s.store.DeleteShare(id)
		if err != nil {
			writeError(w, err)
			return
		}
		if ok {
			deleted++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": deleted})
}

// POST /api/resolve 解析沃盘分享链接。
func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	var body struct {
		URL  string `json:"url"`
		Code string `json:"code"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	parsed, err := parseShareLink(r, strings.TrimSpace(body.URL))
	if err != nil {
		writeError(w, err)
		return
	}
	code := strings.TrimSpace(body.Code)
	if code == "" {
		code = parsed.ShareCode
	}
	if code == "" {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少提取码，请在链接中附带或单独提供提取码"))
		return
	}
	result, err := s.client().ResolveShareDownload(r.Context(), parsed.ShareID, code)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ---------- 平台分享的公开访问 ----------

// GET /api/public/share?id= 分享元信息。
func (s *Server) handlePublicShareMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	share, err := s.lookupShare(r.URL.Query().Get("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(share.Items))
	for _, it := range share.Items {
		items = append(items, map[string]any{
			"id": it.ID, "fid": it.Fid, "type": it.Type, "name": it.Name, "size": it.Size,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":        share.ID,
		"items":     items,
		"createdAt": share.CreatedAt,
		"expiresAt": nullableTimestamp(share.ExpiresAt),
	})
}

// POST /api/public/share/verify 校验提取码。
func (s *Server) handlePublicShareVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	var body struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	id := body.ID
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	share, err := s.lookupShare(id)
	if err != nil {
		writeError(w, err)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(body.Code), share.Code) {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "提取码不正确"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GET /api/public/share/download?id=&code= 单文件直链 / 多项目打包。
func (s *Server) handlePublicShareDownload(w http.ResponseWriter, r *http.Request) {
	share, err := s.authorizedShare(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var fileFids, dirIDs []string
	for _, it := range share.Items {
		if it.Type == 0 {
			dirIDs = append(dirIDs, it.ID)
		} else {
			fileFids = append(fileFids, it.Fid)
		}
	}
	client := s.client()
	if len(fileFids) == 1 && len(dirIDs) == 0 {
		url, err := client.GetDownloadURL(r.Context(), fileFids[0])
		if err != nil {
			writeError(w, err)
			return
		}
		redirectTo(w, wopan.NormalizeRedirectURL(url))
		return
	}
	url, err := client.GetPackDownloadURL(r.Context(), fileFids, dirIDs)
	if err != nil {
		writeError(w, err)
		return
	}
	redirectTo(w, wopan.NormalizeRedirectURL(url))
}

// GET /api/public/share/file?id=&code=&fid= 分享内单个文件的直链。
func (s *Server) handlePublicShareFile(w http.ResponseWriter, r *http.Request) {
	share, err := s.authorizedShare(r)
	if err != nil {
		writeError(w, err)
		return
	}
	fid := r.URL.Query().Get("fid")
	if fid == "" {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少参数 fid"))
		return
	}
	allowed := false
	for _, it := range share.Items {
		if it.Type != 0 && it.Fid == fid {
			allowed = true
			break
		}
	}
	if !allowed {
		writeError(w, wopan.Errorf(http.StatusNotFound, "该文件不在分享范围内"))
		return
	}
	url, err := s.client().GetDownloadURL(r.Context(), fid)
	if err != nil {
		writeError(w, err)
		return
	}
	redirectTo(w, wopan.NormalizeRedirectURL(url))
}

func (s *Server) lookupShare(id string) (*store.Share, error) {
	if strings.TrimSpace(id) == "" {
		return nil, wopan.Errorf(http.StatusBadRequest, "缺少分享 ID")
	}
	share, err := s.store.GetShare(id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, wopan.Errorf(http.StatusNotFound, "分享不存在或已失效")
	}
	if err != nil {
		return nil, err
	}
	return share, nil
}

// authorizedShare 校验分享存在且提取码正确。
func (s *Server) authorizedShare(r *http.Request) (*store.Share, error) {
	id := r.URL.Query().Get("id")
	share, err := s.lookupShare(id)
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if !strings.EqualFold(code, share.Code) {
		return nil, wopan.Errorf(http.StatusForbidden, "需要输入正确的提取码才能下载")
	}
	return share, nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
