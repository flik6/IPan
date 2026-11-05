package httpapi

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"ipan/internal/wopan"
)

// shareLink 是解析出的分享参数。
type shareLink struct {
	ShareID   string
	ShareCode string
}

var (
	codePattern     = regexp.MustCompile(`(?i)(?:提取码|提取碼|password|pwd|code)\s*[:：]?\s*([0-9a-zA-Z]{2,16})`)
	urlPattern      = regexp.MustCompile(`https?://[^\s]+`)
	shareIDPattern  = regexp.MustCompile(`shareId=([0-9a-fA-F]{16,40})`)
	shareCodeRegexp = regexp.MustCompile(`shareCode=([0-9a-zA-Z]{2,16})`)
	shortLinkPath   = regexp.MustCompile(`/s/[0-9a-zA-Z]+`)
)

// parseShareLink 从一段文本中解析分享 ID 与提取码。
//
// 支持两种形式：
//   - 完整链接，例如 https://pan.wo.cn/share_code?shareId=xxx&shareCode=yyy
//   - 短链，例如 https://pan.wo.cn/s/XXXX（需请求一次拿到跳转地址）
func parseShareLink(r *http.Request, raw string) (shareLink, error) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return shareLink{}, wopan.Errorf(http.StatusBadRequest, "请提供分享链接")
	}

	shareCode := ""
	urlPart := input
	if m := codePattern.FindStringSubmatch(input); m != nil {
		shareCode = m[1]
		urlPart = strings.TrimSpace(input[:strings.Index(input, m[0])])
		if urlPart == "" {
			urlPart = input
		}
	}
	target := urlPart
	if m := urlPattern.FindString(urlPart); m != "" {
		target = m
	}

	parsed := extractShareParams(target)
	if parsed.ShareID != "" {
		if parsed.ShareCode == "" {
			parsed.ShareCode = shareCode
		}
		return parsed, nil
	}

	if isShortShareLink(target) {
		location, err := resolveShortLink(r, target)
		if err != nil {
			return shareLink{}, err
		}
		if location != "" {
			parsed = extractShareParams(location)
			if parsed.ShareID != "" {
				if parsed.ShareCode == "" {
					parsed.ShareCode = shareCode
				}
				return parsed, nil
			}
		}
		return shareLink{}, wopan.Errorf(http.StatusBadRequest, "短链未能解析出 shareId，可能已失效或被取消")
	}

	return shareLink{}, wopan.Errorf(http.StatusBadRequest, "无法从链接中解析出 shareId，请粘贴有效的沃盘分享链接")
}

func extractShareParams(target string) shareLink {
	var out shareLink
	if u, err := url.Parse(target); err == nil {
		out.ShareID = u.Query().Get("shareId")
		out.ShareCode = u.Query().Get("shareCode")
	}
	if out.ShareID == "" {
		if m := shareIDPattern.FindStringSubmatch(target); m != nil {
			out.ShareID = m[1]
		}
	}
	if out.ShareCode == "" {
		if m := shareCodeRegexp.FindStringSubmatch(target); m != nil {
			out.ShareCode = m[1]
		}
	}
	return out
}

func isShortShareLink(target string) bool {
	u, err := url.Parse(target)
	if err != nil {
		return false
	}
	return shortLinkPath.MatchString(u.Path) && u.Query().Get("shareId") == ""
}

// resolveShortLink 请求短链并取回跳转地址（不跟随跳转）。
func resolveShortLink(r *http.Request, target string) (string, error) {
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		return "", wopan.Errorf(http.StatusBadGateway, "短链解析失败: %v", err)
	}
	for k, v := range wopan.CommonHeaders() {
		req.Header.Set(k, v)
	}
	res, err := client.Do(req)
	if err != nil {
		return "", wopan.Errorf(http.StatusBadGateway, "短链解析失败，无法访问分享链接")
	}
	defer res.Body.Close()
	return res.Header.Get("Location"), nil
}
