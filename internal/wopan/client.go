// Package wopan 是联通云盘（沃盘）开放接口的 Go 客户端。
//
// 移植自 wopan-sdk-go / 本项目原 Cloudflare Worker 实现（src/server/wopan.ts），
// 保留原有的加密方式（AES-CBC + base64）、请求签名与令牌自动刷新逻辑。
package wopan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HttpError 是对外可展示的 HTTP 错误。
type HttpError struct {
	Status int
	Msg    string
}

func (e *HttpError) Error() string { return e.Msg }

// Errorf 构造带状态码的错误。
func Errorf(status int, format string, args ...any) error {
	return &HttpError{Status: status, Msg: fmt.Sprintf(format, args...)}
}

// StatusOf 返回错误对应的 HTTP 状态码，未知错误按 500 处理。
func StatusOf(err error) int {
	var he *HttpError
	if errors.As(err, &he) {
		return he.Status
	}
	return 500
}

// Tokens 是令牌读写接口，由 store 实现。
type Tokens interface {
	LoadTokens() (access, refresh string, err error)
	SaveTokens(access, refresh string) error
}

// Options 是客户端构造参数。
type Options struct {
	BaseURL   string
	ZoneURL   string
	SpaceType string
	FamilyID  string
}

// Client 是云盘 API 客户端。每次请求创建一个实例（轻量），
// 令牌由 Tokens 接口按需读取，支持多实例共享。
type Client struct {
	opts  Options
	store Tokens

	mu          sync.Mutex
	accessToken string
	refreshTok  string

	zoneMu       sync.Mutex
	discoveredZu string
	familyOnce   sync.Once
	defaultFamID string
}

// New 创建客户端并载入当前令牌。
func New(store Tokens, opts Options) *Client {
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	opts.BaseURL = strings.TrimRight(opts.BaseURL, "/")
	if opts.SpaceType == "" {
		opts.SpaceType = "0"
	}
	c := &Client{opts: opts, store: store}
	if store != nil {
		access, refresh, err := store.LoadTokens()
		if err == nil {
			c.accessToken, c.refreshTok = access, refresh
		}
	}
	return c
}

// AccessToken 返回当前使用的 access token。
func (c *Client) AccessToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accessToken
}

// HasToken 判断是否已绑定云盘账号。
func (c *Client) HasToken() bool { return len(c.AccessToken()) >= 16 }

func (c *Client) requireToken() error {
	if !c.HasToken() {
		return Errorf(409, "尚未绑定云盘账号，请先在设置页完成绑定")
	}
	return nil
}

func (c *Client) setTokens(access, refresh string) {
	c.mu.Lock()
	c.accessToken, c.refreshTok = access, refresh
	c.mu.Unlock()
}

func (c *Client) tokens() (string, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accessToken, c.refreshTok
}

// ---------- 底层请求 ----------

type rspEnvelope struct {
	Status  string          `json:"STATUS"`
	MSG     string          `json:"MSG"`
	RSP     json.RawMessage `json:"RSP"`
	RspCode string          `json:"-"`
	RspDesc string          `json:"-"`
	Data    json.RawMessage `json:"-"`
}

type dispatcherRsp struct {
	RSPCode string          `json:"RSP_CODE"`
	RSPDesc string          `json:"RSP_DESC"`
	DATA    json.RawMessage `json:"DATA"`
}

func (c *Client) postJSON(ctx context.Context, endpoint string, headers map[string]string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range CommonHeaders() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, Errorf(502, "请求联通云盘失败: %v", err)
	}
	defer res.Body.Close()
	return readAll(res.Body)
}

// dispatchOptions 描述一次上游请求的报文形态。
//
// 与原实现保持一致：
//   - wohome 通道：body 为 { clientId, secret: true, param }，param 用 access token 前 16 位加密；
//     ShareFileDetail 例外，用 clientSecret 加密并附带 secretType=clientSecret。
//   - api-user 通道：body 为 { clientId, secret?, param }，param 始终用对应客户端的 secret 加密；
//     AppRefreshToken 等接口不带 secret 字段。
type dispatchOptions struct {
	clientID    string
	encKey      string // 加密 param 使用的密钥
	secretMode  string // "true" | "false" | "omit"
	secretType  string // 非空时写入 body.secretType
	extraHeader map[string]string
}

func (c *Client) callDispatcher(ctx context.Context, channel, key string, param any, o dispatchOptions) (json.RawMessage, error) {
	encrypted, err := EncryptParam(o.encKey, param)
	if err != nil {
		return nil, Errorf(500, "参数加密失败: %v", err)
	}

	body := map[string]any{
		"clientId": orDefault(o.clientID, ClientID),
		"param":    encrypted,
	}
	switch o.secretMode {
	case "false":
		body["secret"] = false
	case "omit":
	default:
		body["secret"] = true
	}
	if o.secretType != "" {
		body["secretType"] = o.secretType
	}
	payload := map[string]any{"header": CalHeader(channel, key), "body": body}

	headers := map[string]string{"Accesstoken": c.AccessToken()}
	if channel == channelAPIUser {
		for k, v := range c.apiUserHeaders() {
			headers[k] = v
		}
	}
	for k, v := range o.extraHeader {
		headers[k] = v
	}

	raw, err := c.postJSON(ctx, c.opts.BaseURL+"/"+channel+"/dispatcher", headers, payload)
	if err != nil {
		return nil, err
	}
	return parseDispatcher(raw)
}

func parseDispatcher(raw []byte) (json.RawMessage, error) {
	var env rspEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, Errorf(502, "解析联通云盘响应失败: %v", err)
	}
	if env.Status != "200" {
		return nil, Errorf(502, "联通云盘状态异常 %s: %s", env.Status, env.MSG)
	}
	if len(env.RSP) == 0 {
		return nil, Errorf(502, "联通云盘响应缺少 RSP")
	}
	var rsp dispatcherRsp
	if err := json.Unmarshal(env.RSP, &rsp); err != nil {
		return nil, Errorf(502, "解析联通云盘 RSP 失败: %v", err)
	}
	if rsp.RSPCode != "0000" {
		return nil, &rspError{Code: rsp.RSPCode, Desc: rsp.RSPDesc}
	}
	return rsp.DATA, nil
}

// rspError 表示上游业务错误码，便于判断 9999（令牌失效）。
type rspError struct {
	Code string
	Desc string
}

func (e *rspError) Error() string { return "联通云盘错误 " + e.Code + ": " + e.Desc }

// isTokenExpired 判断是否为令牌过期错误。
func isTokenExpired(err error) bool {
	var re *rspError
	return errors.As(err, &re) && re.Code == "9999"
}

// withRetry 在令牌过期（RSP_CODE=9999）时刷新令牌并重试一次，最后解密 DATA。
func (c *Client) withRetry(ctx context.Context, fn func() (json.RawMessage, error), decode func(json.RawMessage) (any, error)) (any, error) {
	data, err := fn()
	if err != nil && isTokenExpired(err) {
		if rerr := c.refreshOrReload(ctx); rerr == nil {
			data, err = fn()
		}
	}
	if err != nil {
		return nil, c.wrapError(err)
	}
	return decode(data)
}

// decodeWoHome 解密 wohome 通道的 DATA。
func decodeWoHome(key string) func(json.RawMessage) (any, error) {
	return func(data json.RawMessage) (any, error) {
		if len(data) == 0 || string(data) == "null" {
			return nil, nil
		}
		var asString string
		if err := json.Unmarshal(data, &asString); err == nil {
			if asString == "" {
				return nil, nil
			}
			plain, derr := DecryptParam(key, asString)
			if derr != nil {
				return nil, Errorf(502, "解密联通云盘响应失败: %v", derr)
			}
			var out any
			if err := json.Unmarshal([]byte(plain), &out); err != nil {
				return nil, Errorf(502, "解析联通云盘响应失败: %v", err)
			}
			return out, nil
		}
		var out any
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
}

// decodeAPIUser 解密 api-user 通道的 DATA：先用调用方客户端的密钥，
// 失败再试 Web 客户端密钥；都无法解密时原样返回（更新类接口的响应不一定是密文）。
func decodeAPIUser(keys ...string) func(json.RawMessage) (any, error) {
	return func(data json.RawMessage) (any, error) {
		if len(data) == 0 || string(data) == "null" {
			return nil, nil
		}
		var asString string
		if err := json.Unmarshal(data, &asString); err == nil {
			if asString == "" {
				return nil, nil
			}
			for _, k := range keys {
				plain, derr := DecryptParam(k, asString)
				if derr != nil {
					continue
				}
				var out any
				if json.Unmarshal([]byte(plain), &out) == nil {
					return out, nil
				}
			}
			return asString, nil
		}
		var out any
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
}

// wrapError 把上游业务错误转换为带状态码的可展示错误。
func (c *Client) wrapError(err error) error {
	var re *rspError
	if errors.As(err, &re) {
		return Errorf(502, "联通云盘错误 %s: %s", re.Code, re.Desc)
	}
	return err
}

// refreshOrReload 优先用 refresh token 刷新，失败则回读存储（可能已被其他请求轮换）。
func (c *Client) refreshOrReload(ctx context.Context) error {
	if _, refresh := c.tokens(); refresh != "" {
		if err := c.RefreshAccessToken(ctx); err == nil {
			return nil
		}
	}
	return c.reloadFromStore()
}

func (c *Client) reloadFromStore() error {
	if c.store == nil {
		return fmt.Errorf("无可用的令牌存储")
	}
	access, refresh, err := c.store.LoadTokens()
	if err != nil {
		return err
	}
	if access == "" || access == c.AccessToken() {
		return fmt.Errorf("令牌未更新")
	}
	c.setTokens(access, refresh)
	return nil
}

// ---------- wohome 业务接口 ----------

// RequestWoHome 调用 wohome 通道的指定 key（param 用 access token 加密）。
func (c *Client) RequestWoHome(ctx context.Context, key string, param map[string]any) (any, error) {
	if err := c.requireToken(); err != nil {
		return nil, err
	}
	token, _ := c.tokens()
	key16 := TokenKey(token)
	return c.withRetry(ctx, func() (json.RawMessage, error) {
		return c.callDispatcher(ctx, ChannelWoHome, key, param, dispatchOptions{
			encKey:     key16,
			secretMode: "true",
		})
	}, decodeWoHome(key16))
}

func (c *Client) apiUserHeaders() map[string]string {
	token := c.AccessToken()
	return map[string]string{
		"Accesstoken":       token,
		"Access-Token":      token,
		"X-YP-Access-Token": token,
		"Sys-Version":       "Android/10",
		"platform":          "4",
		"pc-version":        "2.8.13",
		"pc-os":             "win",
	}
}

// requestAPIUser 调用 api-user 通道。
//
// secretMode: "true"（默认，附加 secret:true）、"false"（secret:false）或 "omit"（不带该字段）。
func (c *Client) requestAPIUser(ctx context.Context, key string, param map[string]any, clientID, secret, secretMode string) (any, error) {
	if err := c.requireToken(); err != nil {
		return nil, err
	}
	useSecret := orDefault(secret, ClientSecret)
	return c.withRetry(ctx, func() (json.RawMessage, error) {
		return c.callDispatcher(ctx, channelAPIUser, key, param, dispatchOptions{
			clientID:   clientID,
			encKey:     useSecret,
			secretMode: secretMode,
		})
	}, decodeAPIUser(useSecret, ClientSecret))
}

// ---------- 对外业务方法 ----------

func (c *Client) spaceParams() map[string]any {
	p := map[string]any{"spaceType": c.opts.SpaceType, "clientId": ClientID}
	if c.opts.SpaceType == "1" {
		p["familyId"] = c.opts.FamilyID
	}
	return p
}

func (c *Client) merge(base map[string]any, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// List 列出目录内容。
func (c *Client) List(ctx context.Context, parentID string, page, pageSize int) (map[string]any, error) {
	param := c.merge(c.spaceParams(), map[string]any{
		"parentDirectoryId": parentID,
		"pageNum":           page,
		"pageSize":          pageSize,
		"sortRule":          0,
	})
	data, err := c.RequestWoHome(ctx, "QueryAllFiles", param)
	if err != nil {
		return nil, err
	}
	return asObject(data), nil
}

// Mkdir 新建目录。
func (c *Client) Mkdir(ctx context.Context, parentID, name string) (any, error) {
	fam, err := c.resolveFamilyID(ctx)
	if err != nil {
		return nil, err
	}
	return c.RequestWoHome(ctx, "CreateDirectory", c.merge(c.spaceParams(), map[string]any{
		"familyId":          fam,
		"parentDirectoryId": parentID,
		"directoryName":     name,
	}))
}

// Rename 重命名文件或目录。
func (c *Client) Rename(ctx context.Context, id, name string, typ int) (any, error) {
	fam, err := c.resolveFamilyID(ctx)
	if err != nil {
		return nil, err
	}
	fileTypeVal := FileTypeCode(name)
	if typ == 0 {
		fileTypeVal = "0"
	}
	return c.RequestWoHome(ctx, "RenameFileOrDirectory", c.merge(c.spaceParams(), map[string]any{
		"type":     typ,
		"fileType": fileTypeVal,
		"id":       id,
		"name":     name,
		"familyId": fam,
	}))
}

// Delete 删除文件与目录。
func (c *Client) Delete(ctx context.Context, dirList, fileList []string) (any, error) {
	fam, err := c.resolveFamilyID(ctx)
	if err != nil {
		return nil, err
	}
	return c.RequestWoHome(ctx, "DeleteFile", c.merge(c.spaceParams(), map[string]any{
		"vipLevel": "0",
		"dirList":  emptyIfNil(dirList),
		"fileList": emptyIfNil(fileList),
		"familyId": fam,
	}))
}

// Move 移动。
func (c *Client) Move(ctx context.Context, targetID string, dirList, fileList []string) (any, error) {
	return c.moveOrCopy(ctx, "MoveFile", targetID, dirList, fileList)
}

// Copy 复制。
func (c *Client) Copy(ctx context.Context, targetID string, dirList, fileList []string) (any, error) {
	return c.moveOrCopy(ctx, "CopyFile", targetID, dirList, fileList)
}

func (c *Client) moveOrCopy(ctx context.Context, key, targetID string, dirList, fileList []string) (any, error) {
	fam, err := c.resolveFamilyID(ctx)
	if err != nil {
		return nil, err
	}
	return c.RequestWoHome(ctx, key, map[string]any{
		"targetDirId":  targetID,
		"sourceType":   c.opts.SpaceType,
		"targetType":   c.opts.SpaceType,
		"dirList":      emptyIfNil(dirList),
		"fileList":     emptyIfNil(fileList),
		"secret":       false,
		"clientId":     ClientID,
		"fromFamilyId": fam,
		"familyId":     fam,
	})
}

// ShareFile 创建官方分享。
func (c *Client) ShareFile(ctx context.Context, fileIDs, folderIDs []string, days int) (any, error) {
	fam, err := c.resolveFamilyID(ctx)
	if err != nil {
		return nil, err
	}
	permanent := days <= 0
	d := days
	shareType := "0"
	if permanent {
		d = 36500
		shareType = "1"
	}
	return c.RequestWoHome(ctx, "ShareFile", map[string]any{
		"fileIds":       strings.Join(fileIDs, ","),
		"fileFolderIds": strings.Join(folderIDs, ","),
		"days":          d,
		"shareType":     shareType,
		"clientId":      ClientID,
		"familyId":      fam,
	})
}

// AllShareFileList 官方分享列表，pageNo 从 1 开始。
func (c *Client) AllShareFileList(ctx context.Context, pageNo, pageSize int) (map[string]any, error) {
	data, err := c.RequestWoHome(ctx, "AllShareFileList", map[string]any{
		"pageNum":  pageNo,
		"pageSize": pageSize,
		"sortRule": 0,
		"clientId": ClientID,
	})
	if err != nil {
		return nil, err
	}
	return asObject(data), nil
}

// CancelOrDeleteShare 取消（1）或删除（2）官方分享。
func (c *Client) CancelOrDeleteShare(ctx context.Context, shareIDs []string, operType int) (any, error) {
	if len(shareIDs) == 0 {
		return nil, Errorf(400, "缺少分享 ID")
	}
	return c.RequestWoHome(ctx, "CancelOrDelShareFile", map[string]any{
		"shareId":  strings.Join(shareIDs, ","),
		"operType": operType,
		"clientId": ClientID,
	})
}

// ShareFileDetail 查询官方分享详情（param 用 clientSecret 加密）。
func (c *Client) ShareFileDetail(ctx context.Context, shareID, shareCode string) (map[string]any, error) {
	if err := c.requireToken(); err != nil {
		return nil, err
	}
	data, err := c.withRetry(ctx, func() (json.RawMessage, error) {
		return c.callDispatcher(ctx, ChannelWoHome, "ShareFileDetail", map[string]any{
			"shareId":   shareID,
			"shareCode": shareCode,
		}, dispatchOptions{
			encKey:     ClientSecret,
			secretMode: "true",
			secretType: "clientSecret",
		})
	}, decodeWoHome(ClientSecret))
	if err != nil {
		return nil, err
	}
	return asObject(data), nil
}

// QueryCloudUsageInfo 查询容量信息。
func (c *Client) QueryCloudUsageInfo(ctx context.Context, phone string) (any, error) {
	return c.RequestWoHome(ctx, "QueryCloudUsageInfo", map[string]any{
		"phoneNum": phone,
		"clientId": ClientID,
	})
}

// AppQueryUser 查询账号信息。
func (c *Client) AppQueryUser(ctx context.Context) (map[string]any, error) {
	data, err := c.requestAPIUser(ctx, "AppQueryUser", map[string]any{"accessToken": c.AccessToken()}, ClientID, ClientSecret, "true")
	if err != nil {
		return nil, err
	}
	return asObject(data), nil
}

// GetDownloadURLs 批量获取下载直链。
func (c *Client) GetDownloadURLs(ctx context.Context, fids []string) ([]DownloadURL, error) {
	if len(fids) == 0 {
		return nil, nil
	}
	data, err := c.RequestWoHome(ctx, "GetDownloadUrlV2", map[string]any{
		"type":     "1",
		"fidList":  fids,
		"clientId": ClientID,
	})
	if err != nil {
		return nil, err
	}
	obj := asObject(data)
	list, _ := obj["list"].([]any)
	out := make([]DownloadURL, 0, len(list))
	for _, item := range list {
		m := asObject(item)
		fid, _ := m["fid"].(string)
		u, _ := m["downloadUrl"].(string)
		if fid != "" && u != "" {
			out = append(out, DownloadURL{Fid: fid, URL: u})
		}
	}
	return out, nil
}

// DownloadURL 是单个文件的下载直链。
type DownloadURL struct {
	Fid string
	URL string
}

// GetDownloadURL 获取单个文件的下载直链。
func (c *Client) GetDownloadURL(ctx context.Context, fid string) (string, error) {
	urls, err := c.GetDownloadURLs(ctx, []string{fid})
	if err != nil {
		return "", err
	}
	for _, u := range urls {
		if u.Fid == fid {
			return u.URL, nil
		}
	}
	if len(urls) > 0 {
		return urls[0].URL, nil
	}
	return "", Errorf(502, "未获取到下载直链")
}

// GetPackDownloadURL 获取多文件 / 目录的 zip 打包直链。
func (c *Client) GetPackDownloadURL(ctx context.Context, fileFids, dirIDs []string) (string, error) {
	if len(fileFids) == 0 && len(dirIDs) == 0 {
		return "", Errorf(400, "没有可打包的项目")
	}
	param := map[string]any{
		"spaceType": c.opts.SpaceType,
		"fidList":   emptyIfNil(fileFids),
		"dirList":   emptyIfNil(dirIDs),
		"clientId":  ClientID,
	}
	if c.opts.SpaceType == "1" {
		param["familyId"] = c.opts.FamilyID
	}
	data, err := c.RequestWoHome(ctx, "GetDownloadUrlV3", param)
	if err != nil {
		return "", err
	}
	u, _ := asObject(data)["downloadUrl"].(string)
	if u == "" {
		return "", Errorf(502, "未获取到打包下载直链")
	}
	return u, nil
}

// ---------- 上传 ----------

// UploadSession 描述一次分片上传会话。
type UploadSession struct {
	UploadURL string            `json:"uploadUrl"`
	PartSize  int               `json:"partSize"`
	TotalPart int               `json:"totalPart"`
	Fields    map[string]string `json:"fields"`
}

// CreateUploadSession 创建分片上传会话（浏览器端分片上传）。
func (c *Client) CreateUploadSession(ctx context.Context, directoryID, fileName string, fileSize int64) (*UploadSession, error) {
	if err := c.requireToken(); err != nil {
		return nil, err
	}
	if fileSize < 0 {
		return nil, Errorf(400, "文件大小非法")
	}
	fileInfo := c.fileInfo(directoryID, fileName, fileSize)
	encrypted, err := EncryptWithToken(c.AccessToken(), fileInfo)
	if err != nil {
		return nil, Errorf(500, "参数加密失败: %v", err)
	}
	totalPart := int((fileSize + PartSize - 1) / PartSize)
	if totalPart < 1 {
		totalPart = 1
	}
	return &UploadSession{
		UploadURL: "/api/upload/part",
		PartSize:  PartSize,
		TotalPart: totalPart,
		Fields: map[string]string{
			"uniqueId":    fmt.Sprintf("%d_%s", time.Now().UnixMilli(), RandomChars(6)),
			"fileName":    fileName,
			"psToken":     "undefined",
			"fileSize":    strconv.FormatInt(fileSize, 10),
			"totalPart":   strconv.Itoa(totalPart),
			"channel":     ChannelWoCloud,
			"directoryId": directoryID,
			"fileInfo":    encrypted,
		},
	}, nil
}

func (c *Client) fileInfo(directoryID, fileName string, fileSize int64) map[string]any {
	fi := map[string]any{
		"spaceType":   c.opts.SpaceType,
		"directoryId": directoryID,
		"batchNo":     formatBatchNo(time.Now()),
		"fileName":    fileName,
		"fileSize":    fileSize,
		"fileType":    FileTypeCode(fileName),
	}
	if c.opts.SpaceType == "1" {
		fi["familyId"] = c.opts.FamilyID
	}
	return fi
}

// UploadPartForm 上传浏览器提交的一个分片。
func (c *Client) UploadPartForm(ctx context.Context, form *multipart.Form) (map[string]any, error) {
	if err := c.requireToken(); err != nil {
		return nil, err
	}
	fileName := formValue(form, "fileName")
	partIndex, err := strconv.Atoi(formValue(form, "partIndex"))
	if err != nil || partIndex < 1 {
		return nil, Errorf(400, "分片序号非法")
	}
	var fileHeader *multipart.FileHeader
	if len(form.File["file"]) > 0 {
		fileHeader = form.File["file"][0]
	}
	if fileHeader == nil {
		return nil, Errorf(400, "缺少文件分片")
	}

	zone, err := c.uploadBaseURL(ctx)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, key := range []string{"uniqueId", "fileName", "psToken", "fileSize", "totalPart", "channel", "directoryId", "fileInfo", "partSize", "partIndex"} {
		v := formValue(form, key)
		if v == "" {
			return nil, Errorf(400, "缺少表单字段 %s", key)
		}
		_ = mw.WriteField(key, v)
	}
	_ = mw.WriteField("accessToken", c.AccessToken())
	part, err := mw.CreatePart(fileHeader.Header)
	if err != nil {
		return nil, err
	}
	src, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	if _, err := io.Copy(part, src); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	headers := CommonHeaders()
	headers["Content-Type"] = mw.FormDataContentType()
	return c.uploadPart(ctx, zone+"/openapi/client/upload2C", buf.Bytes(), headers, partIndex, fileName)
}

// UploadStream 流式上传（服务端代为转发，按 8MB 分片）。
func (c *Client) UploadStream(ctx context.Context, directoryID, fileName string, r io.Reader, fileSize int64, contentType string) (map[string]any, error) {
	if err := c.requireToken(); err != nil {
		return nil, err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	fileInfo := c.fileInfo(directoryID, fileName, fileSize)
	encrypted, err := EncryptWithToken(c.AccessToken(), fileInfo)
	if err != nil {
		return nil, Errorf(500, "参数加密失败: %v", err)
	}
	totalPart := int((fileSize + PartSize - 1) / PartSize)
	if totalPart < 1 {
		totalPart = 1
	}
	zone, err := c.uploadBaseURL(ctx)
	if err != nil {
		return nil, err
	}
	uniqueID := fmt.Sprintf("%d_%s", time.Now().UnixMilli(), RandomChars(6))

	buf := make([]byte, PartSize)
	last := map[string]any{}
	for partIndex := 1; partIndex <= totalPart; partIndex++ {
		n, rerr := io.ReadFull(r, buf)
		if rerr != nil && !errors.Is(rerr, io.ErrUnexpectedEOF) && !errors.Is(rerr, io.EOF) {
			return nil, Errorf(400, "读取上传数据失败: %v", rerr)
		}
		if n == 0 && partIndex > 1 {
			break
		}
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		mw.WriteField("uniqueId", uniqueID)
		mw.WriteField("accessToken", c.AccessToken())
		mw.WriteField("fileName", fileName)
		mw.WriteField("psToken", "undefined")
		mw.WriteField("fileSize", strconv.FormatInt(fileSize, 10))
		mw.WriteField("totalPart", strconv.Itoa(totalPart))
		mw.WriteField("channel", ChannelWoCloud)
		mw.WriteField("directoryId", directoryID)
		mw.WriteField("fileInfo", encrypted)
		mw.WriteField("partSize", strconv.Itoa(n))
		mw.WriteField("partIndex", strconv.Itoa(partIndex))
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(fileName)))
		h.Set("Content-Type", contentType)
		pw, _ := mw.CreatePart(h)
		if _, err := pw.Write(buf[:n]); err != nil {
			return nil, err
		}
		mw.Close()

		headers := CommonHeaders()
		headers["Content-Type"] = mw.FormDataContentType()
		last, err = c.uploadPart(ctx, zone+"/openapi/client/upload2C", body.Bytes(), headers, partIndex, fileName)
		if err != nil {
			return nil, err
		}
		if err := checkUploadCode(last, partIndex); err != nil {
			return nil, err
		}
	}
	return last, nil
}

func (c *Client) uploadPart(ctx context.Context, url string, body []byte, headers map[string]string, partIndex int, fileName string) (map[string]any, error) {
	var lastErr error
	for attempt := 0; attempt <= 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		res, err := httpClient.Do(req)
		if err != nil {
			lastErr = Errorf(502, "上传分片 %d 失败: %v", partIndex, err)
		} else {
			raw, rerr := readAll(res.Body)
			res.Body.Close()
			if rerr != nil {
				lastErr = Errorf(502, "读取上传响应失败: %v", rerr)
			} else {
				var out map[string]any
				if jerr := json.Unmarshal(raw, &out); jerr != nil {
					lastErr = Errorf(502, "上传分片 %d 响应异常: %s", partIndex, truncate(string(raw), 200))
				} else if res.StatusCode >= 400 {
					lastErr = Errorf(502, "上传分片 %d 失败: HTTP %d", partIndex, res.StatusCode)
				} else {
					return out, nil
				}
			}
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(300*(attempt+1)) * time.Millisecond):
			}
		}
	}
	return nil, lastErr
}

func checkUploadCode(result map[string]any, partIndex int) error {
	if code, ok := result["code"].(string); ok && code != "" && code != "0000" {
		return Errorf(502, "上传分片 %d 失败: %v", partIndex, result)
	}
	return nil
}

func (c *Client) uploadBaseURL(ctx context.Context) (string, error) {
	if z := strings.TrimRight(c.opts.ZoneURL, "/"); z != "" && z != DefaultZoneURL {
		return z, nil
	}
	c.zoneMu.Lock()
	defer c.zoneMu.Unlock()
	if c.discoveredZu != "" {
		return c.discoveredZu, nil
	}
	data, err := c.RequestWoHome(ctx, "GetZoneInfo", map[string]any{"appId": AppID})
	if err == nil {
		if u, _ := asObject(data)["url"].(string); u != "" {
			c.discoveredZu = strings.TrimRight(u, "/")
			return c.discoveredZu, nil
		}
	}
	c.discoveredZu = DefaultZoneURL
	return c.discoveredZu, nil
}

// ---------- 头像与资料 ----------

// UploadAvatar 上传头像文件，返回联通侧生成的头像 URL。
func (c *Client) UploadAvatar(ctx context.Context, fileName, contentType string, data []byte) (string, error) {
	if contentType == "" {
		contentType = "image/png"
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(fileName)))
	h.Set("Content-Type", contentType)
	pw, _ := mw.CreatePart(h)
	pw.Write(data)
	mw.Close()

	headers := c.apiUserHeaders()
	for k, v := range CommonHeaders() {
		headers[k] = v
	}
	headers["Content-Type"] = mw.FormDataContentType()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opts.BaseURL+"/api-user/update/user/head", bytes.NewReader(buf.Bytes()))
	if err != nil {
		return "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return "", Errorf(502, "上传头像失败: %v", err)
	}
	defer res.Body.Close()
	raw, _ := readAll(res.Body)

	var env rspEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", Errorf(502, "解析头像上传响应失败: %v", err)
	}
	if env.Status != "200" {
		return "", Errorf(502, "联通云盘状态异常 %s: %s", env.Status, env.MSG)
	}
	var rsp dispatcherRsp
	if err := json.Unmarshal(env.RSP, &rsp); err != nil {
		return "", Errorf(502, "解析联通云盘 RSP 失败: %v", err)
	}
	if rsp.RSPCode != "0000" {
		return "", Errorf(502, "联通云盘错误 %s: %s", rsp.RSPCode, rsp.RSPDesc)
	}
	var asString string
	if json.Unmarshal(rsp.DATA, &asString) == nil && asString != "" {
		return asString, nil
	}
	obj := asObject(decodeAny(rsp.DATA))
	if u, _ := PickString(obj, "headUrl", "url"); u != "" {
		return u, nil
	}
	return "", Errorf(502, "未获取到头像地址")
}

// UpdateUserInfo 更新账号资料（昵称/性别/生日/头像）。
func (c *Client) UpdateUserInfo(ctx context.Context, userName, sex, headURL string, birthday int64) (any, error) {
	if strings.TrimSpace(userName) == "" {
		return nil, Errorf(400, "昵称不能为空")
	}
	if sex != "男" && sex != "女" {
		return nil, Errorf(400, "请选择性别（男 / 女）后再保存")
	}
	if birthday <= 0 {
		return nil, Errorf(400, "请选择生日后再保存")
	}
	if headURL == "" {
		user, err := c.AppQueryUser(ctx)
		if err == nil {
			headURL, _ = PickString(user, "headUrl", "avatarUrl")
		}
	}
	inner := map[string]any{
		"accessToken": c.AccessToken(),
		"userName":    userName,
		"sex":         sex,
		"headUrl":     headURL,
		"birthday":    birthday,
	}
	clients := []struct{ id, secret string }{
		{PCClientID, PCClientSecret},
		{ClientID, ClientSecret},
		{LoginClientID, LoginClientSecret},
	}
	var lastErr error
	for _, cl := range clients {
		data, err := c.requestAPIUser(ctx, "WebUpdateUserInfo", inner, cl.id, cl.secret, "false")
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !strings.Contains(err.Error(), "8005") && !strings.Contains(err.Error(), "9002") {
			return nil, err
		}
	}
	return nil, lastErr
}

// ---------- 令牌刷新 ----------

// RefreshAccessToken 用 refresh token 换取新令牌并持久化。
func (c *Client) RefreshAccessToken(ctx context.Context) error {
	_, refresh := c.tokens()
	if refresh == "" {
		return Errorf(400, "尚未获取 refresh token，无法自动刷新")
	}
	clients := []struct{ id, secret string }{
		{PCClientID, PCClientSecret},
		{ClientID, ClientSecret},
		{LoginClientID, LoginClientSecret},
	}
	var lastErr error
	for _, cl := range clients {
		data, err := c.requestAPIUser(ctx, "AppRefreshToken", map[string]any{
			"refreshToken": refresh,
			"clientSecret": cl.secret,
		}, cl.id, cl.secret, "omit")
		if err != nil {
			lastErr = err
			if !strings.Contains(err.Error(), "8005") && !strings.Contains(err.Error(), "9002") {
				return err
			}
			continue
		}
		obj := asObject(data)
		accessToken, _ := obj["access_token"].(string)
		newRefresh, _ := obj["refresh_token"].(string)
		if accessToken == "" || newRefresh == "" {
			return Errorf(502, "刷新令牌响应缺少字段")
		}
		c.setTokens(accessToken, newRefresh)
		if c.store != nil {
			_ = c.store.SaveTokens(accessToken, newRefresh)
		}
		return nil
	}
	return lastErr
}

func (c *Client) resolveFamilyID(ctx context.Context) (string, error) {
	if c.opts.SpaceType == "1" {
		return c.opts.FamilyID, nil
	}
	var resolveErr error
	c.familyOnce.Do(func() {
		data, err := c.RequestWoHome(ctx, "FamilyUserCurrentEncode", map[string]any{"clientId": ClientID})
		if err != nil {
			resolveErr = err
			return
		}
		switch v := asObject(data)["defaultHomeId"].(type) {
		case string:
			c.defaultFamID = v
		default:
			c.defaultFamID = strconv.FormatInt(ToInt64(v), 10)
		}
	})
	return c.defaultFamID, resolveErr
}

// ---------- 分享链接解析 ----------

// ShareFile 分享详情里的条目。
type ShareEntry struct {
	ID    string
	Fid   string
	Name  string
	IsDir bool
}

// ShareFiles 递归收集分享详情里的文件条目。
func ShareFiles(data any) []ShareEntry {
	var out []ShareEntry
	var visit func(v any, depth int)
	visit = func(v any, depth int) {
		if v == nil || depth > 6 {
			return
		}
		switch t := v.(type) {
		case []any:
			for _, item := range t {
				visit(item, depth+1)
			}
		case map[string]any:
			id, _ := t["id"].(string)
			fid, _ := t["fid"].(string)
			if id != "" || fid != "" {
				name, _ := t["name"].(string)
				if name == "" {
					name, _ = t["fileName"].(string)
				}
				isDir := false
				switch tv := t["type"].(type) {
				case float64:
					isDir = int(tv) == 0
				case string:
					isDir = tv == "0"
				default:
					if ft, ok := t["fileType"].(string); ok {
						isDir = ft == "0"
					}
				}
				out = append(out, ShareEntry{ID: id, Fid: firstNonEmpty(fid, id), Name: name, IsDir: isDir})
			}
			for _, key := range []string{"files", "list", "fileList", "data", "fileVOList", "content", "records", "items"} {
				if child, ok := t[key]; ok {
					visit(child, depth+1)
				}
			}
		}
	}
	visit(data, 0)
	return out
}

// ResolveShareDownload 解析官方分享的下载直链。
func (c *Client) ResolveShareDownload(ctx context.Context, shareID, shareCode string) (map[string]any, error) {
	detail, err := c.ShareFileDetail(ctx, shareID, shareCode)
	if err != nil {
		return nil, err
	}
	entries := ShareFiles(detail)
	var files, dirs []ShareEntry
	for _, e := range entries {
		if e.IsDir {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}
	if len(files) == 1 && len(dirs) == 0 {
		u, err := c.GetDownloadURL(ctx, files[0].Fid)
		if err != nil {
			return nil, err
		}
		return map[string]any{"url": u, "name": files[0].Name, "packed": false}, nil
	}
	if len(files)+len(dirs) == 0 {
		return nil, Errorf(502, "分享内没有可下载的文件")
	}
	var fileFids, dirIDs []string
	for _, f := range files {
		fileFids = append(fileFids, f.Fid)
	}
	for _, d := range dirs {
		dirIDs = append(dirIDs, d.ID)
	}
	u, err := c.GetPackDownloadURL(ctx, fileFids, dirIDs)
	if err != nil {
		return nil, err
	}
	first := ""
	if len(files) > 0 {
		first = files[0].Name
	} else {
		first = dirs[0].Name
	}
	name := "分享打包_" + shareID + ".zip"
	if first != "" {
		name = fmt.Sprintf("%s 等 %d 项.zip", first, len(files)+len(dirs))
	}
	return map[string]any{"url": u, "name": name, "packed": true}, nil
}

// ---------- 手机号登录 ----------

// SendSmsCode 发送短信验证码。
func SendSmsCode(ctx context.Context, phone string) error {
	param, err := EncryptParam(LoginClientSecret, map[string]any{"operateType": "1", "phone": phone})
	if err != nil {
		return err
	}
	payload := map[string]any{"func": "app_send", "clientId": LoginClientID, "param": param}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DefaultBaseURL+"/api-user/sendMessageCodeBase", bytes.NewReader(body))
	if err != nil {
		return err
	}
	for k, v := range CommonHeaders() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := httpClient.Do(req)
	if err != nil {
		return Errorf(502, "发送验证码失败: %v", err)
	}
	defer res.Body.Close()
	return checkEnvelope(awaitBytes(res))
}

// LoginVerify 用手机号 + 短信验证码登录，返回 access/refresh token。
func LoginVerify(ctx context.Context, phone, smsCode string) (accessToken, refreshToken string, err error) {
	param, err := EncryptParam(LoginClientSecret, map[string]any{
		"clientSecret": LoginClientSecret,
		"phone":        phone,
		"smsCode":      smsCode,
	})
	if err != nil {
		return "", "", err
	}
	payload := map[string]any{
		"header": CalHeader(channelAPIUser, "LoginByMobileV2"),
		"body":   map[string]any{"clientId": LoginClientID, "param": param},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DefaultBaseURL+"/api-user/dispatcher", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	for k, v := range CommonHeaders() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := httpClient.Do(req)
	if err != nil {
		return "", "", Errorf(502, "登录请求失败: %v", err)
	}
	defer res.Body.Close()
	raw := awaitBytes(res)

	var env rspEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", "", Errorf(502, "解析登录响应失败: %v", err)
	}
	if env.Status != "200" {
		return "", "", Errorf(502, "联通云盘状态异常 %s: %s", env.Status, env.MSG)
	}
	var rsp dispatcherRsp
	if err := json.Unmarshal(env.RSP, &rsp); err != nil {
		return "", "", Errorf(502, "解析登录响应失败: %v", err)
	}
	if rsp.RSPCode != "0000" {
		return "", "", Errorf(502, "联通云盘错误 %s: %s", rsp.RSPCode, rsp.RSPDesc)
	}
	var data map[string]any
	if len(rsp.DATA) > 0 {
		var asString string
		if json.Unmarshal(rsp.DATA, &asString) == nil && asString != "" {
			plain, derr := DecryptParam(LoginClientSecret, asString)
			if derr == nil {
				_ = json.Unmarshal([]byte(plain), &data)
			}
		} else {
			_ = json.Unmarshal(rsp.DATA, &data)
		}
	}
	access, _ := PickString(data, "accessToken", "access_token", "token", "Token")
	refresh, _ := PickString(data, "refreshToken", "refresh_token")
	if access == "" {
		return "", "", Errorf(502, "登录响应缺少 accessToken")
	}
	return access, refresh, nil
}

func awaitBytes(res *http.Response) []byte {
	b, _ := readAll(res.Body)
	return b
}

func checkEnvelope(raw []byte) error {
	var env rspEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Errorf(502, "解析响应失败: %v", err)
	}
	if env.Status != "200" {
		return Errorf(502, "联通云盘状态异常 %s: %s", env.Status, env.MSG)
	}
	if len(env.RSP) > 0 {
		var rsp dispatcherRsp
		if json.Unmarshal(env.RSP, &rsp) == nil && rsp.RSPCode != "" && rsp.RSPCode != "0000" {
			return Errorf(502, "联通云盘错误 %s: %s", rsp.RSPCode, rsp.RSPDesc)
		}
	}
	return nil
}

// ---------- 文件类型 ----------

var (
	imageExt = []string{"jpg", "jpeg", "png", "gif", "webp", "bmp", "tiff", "tif", "psd", "raw", "tga", "livp", "heic", "ico"}
	videoExt = []string{"asf", "avi", "dv", "flv", "mkv", "mov", "mp4", "rm", "swf", "ts", "vob", "wmv", "rmvb", "mpg", "webm", "ogv", "m4v", "f4v", "3gp", "m3u8", "dat", "m3us"}
	audioExt = []string{"ac3", "flac", "m4a", "mp2", "mp3", "wav", "wma", "ape", "mpc", "tta", "ogg", "amr", "aac", "aiff", "au", "mka", "wv"}
	docExt   = []string{"txt", "rtf", "doc", "docx", "ppt", "pptx", "xls", "xlsx", "md", "pdf", "hlp", "csv"}
	extSets  = []struct {
		code string
		set  map[string]bool
	}{}
)

func init() {
	for _, e := range []struct {
		code string
		list []string
	}{{"1", imageExt}, {"2", videoExt}, {"3", audioExt}, {"4", docExt}} {
		set := make(map[string]bool, len(e.list))
		for _, x := range e.list {
			set[x] = true
		}
		extSets = append(extSets, struct {
			code string
			set  map[string]bool
		}{e.code, set})
	}
}

// FileTypeCode 根据文件名推断联通云盘的文件类型编码。
func FileTypeCode(name string) string {
	idx := strings.LastIndex(name, ".")
	if idx < 0 || idx == len(name)-1 {
		return "5"
	}
	ext := strings.ToLower(name[idx+1:])
	for _, s := range extSets {
		if s.set[ext] {
			return s.code
		}
	}
	return "5"
}

// ---------- 小工具 ----------

func asObject(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func decodeAny(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var out any
	_ = json.Unmarshal(raw, &out)
	return out
}

func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func escapeQuotes(s string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(s)
}

func formValue(form *multipart.Form, key string) string {
	if len(form.Value[key]) == 0 {
		return ""
	}
	return form.Value[key][0]
}

// PickString 依次尝试多个字段名，返回第一个非空字符串。
func PickString(m map[string]any, paths ...string) (string, bool) {
	for _, p := range paths {
		cur := any(m)
		ok := true
		for _, part := range strings.Split(p, ".") {
			switch node := cur.(type) {
			case map[string]any:
				cur, ok = node[part]
			case []any:
				var idx int
				idx, err := strconv.Atoi(part)
				if err != nil || idx < 0 || idx >= len(node) {
					ok = false
					break
				}
				cur = node[idx]
			default:
				ok = false
			}
			if !ok {
				break
			}
		}
		if !ok {
			continue
		}
		if s, isStr := cur.(string); isStr && s != "" {
			return s, true
		}
	}
	return "", false
}

// NormalizeRedirectURL 对直链中的查询参数做一次规范化编码。
func NormalizeRedirectURL(value string) string {
	if value == "" {
		return value
	}
	hashIdx := strings.Index(value, "#")
	beforeHash := value
	hash := ""
	if hashIdx >= 0 {
		beforeHash = value[:hashIdx]
		hash = value[hashIdx:]
	}
	qIdx := strings.Index(beforeHash, "?")
	if qIdx < 0 {
		return value
	}
	base := beforeHash[:qIdx]
	query := beforeHash[qIdx+1:]
	parts := strings.Split(query, "&")
	for i, part := range parts {
		eq := strings.Index(part, "=")
		if eq < 0 {
			parts[i] = encodeQueryPart(part)
			continue
		}
		parts[i] = encodeQueryPart(part[:eq]) + "=" + encodeQueryPart(part[eq+1:])
	}
	return base + "?" + strings.Join(parts, "&") + hash
}

// encodeQueryPart 与前端实现一致：保留已有的 %XX 转义（统一大写），
// 其余字符按 encodeURIComponent 规则编码（空格编码为 %20 而非 +）。
func encodeQueryPart(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); {
		c := v[i]
		if c == '%' && i+2 < len(v) && isHex(v[i+1]) && isHex(v[i+2]) {
			b.WriteByte('%')
			b.WriteString(strings.ToUpper(v[i+1 : i+3]))
			i += 3
			continue
		}
		if isUnreserved(c) {
			b.WriteByte(c)
			i++
			continue
		}
		// 非 ASCII 字符按 UTF-8 字节整体编码
		size := 1
		if c >= 0xF0 {
			size = 4
		} else if c >= 0xE0 {
			size = 3
		} else if c >= 0xC0 {
			size = 2
		}
		if i+size > len(v) {
			size = 1
		}
		for _, raw := range []byte(v[i : i+size]) {
			b.WriteByte('%')
			b.WriteString(strings.ToUpper(hexPair(raw)))
		}
		i += size
	}
	return b.String()
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

const hexDigits = "0123456789ABCDEF"

func hexPair(b byte) string {
	return string([]byte{hexDigits[b>>4], hexDigits[b&0x0F]})
}

func isUnreserved(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '~' || c == '-'
}

// SortStrings 便于稳定输出。
func SortStrings(s []string) {
	sort.Strings(s)
}
