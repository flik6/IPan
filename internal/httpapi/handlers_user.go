package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"ipan/internal/wopan"
)

// detailCacheTTL 是账号信息缓存的过期时间。
const detailCacheTTL = 10 * time.Minute

// detailSnapshot 抓取一份完整账号信息（两次串行上游请求）。
func (s *Server) detailSnapshot(r *http.Request) (map[string]any, error) {
	client := s.client()
	user, err := client.AppQueryUser(r.Context())
	if err != nil {
		return nil, err
	}
	phone, _ := wopan.PickString(user, "userId", "phone", "userAccount")
	usageAny, err := client.QueryCloudUsageInfo(r.Context(), phone)
	if err != nil {
		return nil, err
	}
	usage := asObject(usageAny)
	usageInfo := asObject(usage["usageInfo"])
	nickName, _ := wopan.PickString(user, "userName", "nickName", "nickname", "name")
	avatarURL, _ := wopan.PickString(user, "headUrl", "avatarUrl", "headImgUrl", "userIcon", "iconUrl", "photoUrl")
	vipLevel, _ := wopan.PickString(usage, "vipLevel")
	expireTime, _ := wopan.PickString(usage, "expireTime")
	return map[string]any{
		"phone":      phone,
		"nickName":   nickName,
		"avatarUrl":  avatarURL,
		"totalSize":  jsonNumberToInt64(usageInfo["byteTotalSize"]),
		"usedSize":   jsonNumberToInt64(usageInfo["byteUsedSize"]),
		"imageSize":  jsonNumberToInt64(usageInfo["imageSize"]),
		"videoSize":  jsonNumberToInt64(usageInfo["videoSize"]),
		"audioSize":  jsonNumberToInt64(usageInfo["audioSize"]),
		"otherSize":  jsonNumberToInt64(usageInfo["otherSize"]),
		"vipLevel":   vipLevel,
		"expireTime": expireTime,
	}, nil
}

// GET /api/detail 账号信息。
//
// 缓存未过期直接返回；过期后由服务端自动回源，前端无需手动触发刷新。
// refresh=1 仅用于强制回源（例如资料编辑保存后立刻要用新数据）。
func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	force := r.URL.Query().Get("refresh") == "1"

	// 快路径：缓存仍新鲜就直接返回，不进入锁
	if payload, cachedAt, ok := s.cachedDetail(); ok && !force && !s.detailStale(cachedAt) {
		s.writeCachedDetail(w, payload, cachedAt, false)
		return
	}

	// 过期回源串行化：并发请求里只有一个真正打上游
	s.detailMu.Lock()
	defer s.detailMu.Unlock()

	// 等锁期间可能已被别的请求回源过
	if payload, cachedAt, ok := s.cachedDetail(); ok && !force && !s.detailStale(cachedAt) {
		s.writeCachedDetail(w, payload, cachedAt, false)
		return
	}

	fresh, err := s.detailSnapshot(r)
	if err != nil {
		// 回源失败时退回旧缓存，页面不至于变空白
		if payload, cachedAt, ok := s.cachedDetail(); ok {
			s.writeCachedDetail(w, payload, cachedAt, true)
			return
		}
		writeError(w, err)
		return
	}
	s.respondFreshDetail(w, fresh)
}

// cachedDetail 读取账号信息缓存原文。
func (s *Server) cachedDetail() ([]byte, int64, bool) {
	payload, cachedAt, err := s.store.LoadDetailCache()
	if err != nil || payload == nil {
		return nil, 0, false
	}
	return payload, cachedAt, true
}

func (s *Server) detailStale(cachedAt int64) bool {
	return time.Now().UnixMilli()-cachedAt > int64(detailCacheTTL/time.Millisecond)
}

// writeCachedDetail 返回缓存快照，附加缓存时间与新鲜度标记。
func (s *Server) writeCachedDetail(w http.ResponseWriter, payload []byte, cachedAt int64, stale bool) {
	var snapshot map[string]any
	if json.Unmarshal(payload, &snapshot) != nil {
		writeError(w, wopan.Errorf(http.StatusInternalServerError, "账号信息缓存解析失败"))
		return
	}
	snapshot["cachedAt"] = cachedAt
	snapshot["fromCache"] = true
	snapshot["stale"] = stale
	writeJSON(w, http.StatusOK, snapshot)
}

// respondFreshDetail 回写缓存并返回刚抓取的账号信息。
func (s *Server) respondFreshDetail(w http.ResponseWriter, fresh map[string]any) {
	cachedAt := time.Now().UnixMilli()
	if payload, merr := json.Marshal(fresh); merr == nil {
		_ = s.store.SaveDetailCache(payload, cachedAt)
	}
	fresh["cachedAt"] = cachedAt
	fresh["fromCache"] = false
	fresh["stale"] = false
	writeJSON(w, http.StatusOK, fresh)
}

// GET /api/user/profile 可编辑资料。
func (s *Server) handleUserProfile(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetProfile(w, r)
	case http.MethodPost:
		s.handleUpdateProfile(w, r)
	default:
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
	}
}

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	user, err := s.client().AppQueryUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	phone, _ := wopan.PickString(user, "userId", "phone", "userAccount")
	userName, _ := wopan.PickString(user, "userName", "nickName", "nickname", "name")
	headURL, _ := wopan.PickString(user, "headUrl", "avatarUrl", "headImgUrl", "userIcon", "iconUrl", "photoUrl")
	sex, _ := wopan.PickString(user, "sex", "gender")
	writeJSON(w, http.StatusOK, map[string]any{
		"phone":    phone,
		"userName": userName,
		"headUrl":  headURL,
		"sex":      sex,
		"birthday": parseBirthday(user["birthday"]),
	})
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserName *string  `json:"userName"`
		Sex      *string  `json:"sex"`
		HeadURL  *string  `json:"headUrl"`
		Birthday *float64 `json:"birthday"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	var userName, sex, headURL string
	var birthday int64
	if body.UserName != nil {
		userName = *body.UserName
	}
	if body.Sex != nil {
		sex = *body.Sex
	}
	if body.HeadURL != nil {
		headURL = *body.HeadURL
	}
	if body.Birthday != nil {
		birthday = int64(*body.Birthday)
	}
	result, err := s.client().UpdateUserInfo(r.Context(), userName, sex, headURL, birthday)
	if err != nil {
		// 上游 1000 只说「请求参数错误」，对用户没有可操作性，换成能指导操作的说明
		if he, ok := err.(*wopan.HttpError); ok && len(he.Msg) > 0 && containsAll(he.Msg, "1000") {
			writeError(w, wopan.Errorf(http.StatusBadRequest, "云盘拒绝了本次修改：请确认昵称、性别、生日都已填写后再保存"))
			return
		}
		writeError(w, err)
		return
	}
	_ = s.store.ClearDetailCache()
	writeJSON(w, http.StatusOK, result)
}

// POST /api/user/avatar 上传头像（multipart 字段 file）。
func (s *Server) handleUserAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, wopan.Errorf(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "解析上传表单失败: %v", err))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	headers := r.MultipartForm.File["file"]
	if len(headers) == 0 {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "缺少头像文件"))
		return
	}
	fh := headers[0]
	if fh.Size > 10<<20 {
		writeError(w, wopan.Errorf(http.StatusBadRequest, "头像文件不能超过 10MB"))
		return
	}
	f, err := fh.Open()
	if err != nil {
		writeError(w, err)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		writeError(w, err)
		return
	}
	headURL, err := s.client().UploadAvatar(r.Context(), fh.Filename, fh.Header.Get("Content-Type"), data)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"headUrl": headURL})
}

// parseBirthday 把生日统一成毫秒时间戳，非法值返回 nil。
func parseBirthday(v any) any {
	var n int64
	switch x := v.(type) {
	case float64:
		n = int64(x)
	case string:
		parsed, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return nil
		}
		n = parsed
	case json.Number:
		parsed, err := x.Int64()
		if err != nil {
			return nil
		}
		n = parsed
	default:
		return nil
	}
	if n <= 0 {
		return nil
	}
	return n
}

func containsAll(s string, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
