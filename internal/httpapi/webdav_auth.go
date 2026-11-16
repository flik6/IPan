package httpapi

import (
	"crypto/md5"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const davRealm = "iPan WebDAV"
const davNonceTTL = 5 * time.Minute

type davCredentials struct {
	Username  string
	MD5HA1    string
	SHA256HA1 string
}

func (c davCredentials) enabled() bool {
	return c.Username != "" && c.MD5HA1 != "" && c.SHA256HA1 != ""
}

func makeDAVCredentials(username, password string) davCredentials {
	base := username + ":" + davRealm + ":" + password
	return davCredentials{
		Username:  username,
		MD5HA1:    fmt.Sprintf("%x", md5.Sum([]byte(base))),
		SHA256HA1: fmt.Sprintf("%x", sha256.Sum256([]byte(base))),
	}
}

// webDAVCredentials 优先读取后台配置，环境变量仅作为兼容回退。
func (s *Server) webDAVCredentials() (davCredentials, error) {
	stored, err := s.store.LoadWebDAV()
	if err != nil {
		return davCredentials{}, err
	}
	if stored.Managed {
		if stored.Enabled() {
			return davCredentials{
				Username:  stored.Username,
				MD5HA1:    stored.MD5HA1,
				SHA256HA1: stored.SHA256HA1,
			}, nil
		}
		return davCredentials{}, nil
	}
	if stored.Enabled() {
		return davCredentials{
			Username:  stored.Username,
			MD5HA1:    stored.MD5HA1,
			SHA256HA1: stored.SHA256HA1,
		}, nil
	}
	if s.cfg.WebDAVUser != "" && s.cfg.WebDAVPassword != "" && !strings.Contains(s.cfg.WebDAVUser, ":") {
		return makeDAVCredentials(s.cfg.WebDAVUser, s.cfg.WebDAVPassword), nil
	}
	return davCredentials{}, nil
}

type davDigest struct {
	secret string
	mu     sync.Mutex
	used   map[string]davNonceCount
	pruned time.Time
}

type davNonceCount struct {
	expires time.Time
	high    uint32
	window  uint64
}

func (s *Server) authenticateDAV(w http.ResponseWriter, r *http.Request, creds davCredentials) bool {
	gotUser, gotPassword, ok := r.BasicAuth()
	supplied := fmt.Sprintf("%x", sha256.Sum256([]byte(gotUser+":"+davRealm+":"+gotPassword)))
	if ok && gotUser == creds.Username && subtle.ConstantTimeCompare([]byte(supplied), []byte(creds.SHA256HA1)) == 1 {
		return true
	}
	now := time.Now()
	ok, stale := s.davAuth.authenticate(r, creds, now)
	if ok {
		return true
	}
	w.Header().Add("WWW-Authenticate", `Basic realm="`+davRealm+`", charset="UTF-8"`)
	nonce := s.davAuth.nonce(now)
	for _, algorithm := range []string{"MD5", "SHA-256"} {
		challenge := fmt.Sprintf(`Digest realm="%s", nonce="%s", algorithm=%s, qop="auth"`, davRealm, nonce, algorithm)
		if stale {
			challenge += ", stale=true"
		}
		w.Header().Add("WWW-Authenticate", challenge)
	}
	w.WriteHeader(http.StatusUnauthorized)
	return false
}

func (d *davDigest) nonce(now time.Time) string {
	value := strconv.FormatInt(now.Unix(), 10) + "." + randomBase64URL(16)
	mac := hmacSHA256(d.secret, value)
	return value + "." + base64.RawURLEncoding.EncodeToString(mac)
}

func (d *davDigest) authenticate(r *http.Request, creds davCredentials, now time.Time) (ok, stale bool) {
	scheme, value, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	if !strings.EqualFold(scheme, "Digest") || len(value) > 8192 {
		return false, false
	}
	params := parseDAVDigest(value)
	uri := params["uri"]
	if params["username"] != creds.Username || params["realm"] != davRealm || params["qop"] != "auth" ||
		uri == "" || uri != r.URL.RequestURI() && uri != r.RequestURI {
		return false, false
	}
	count, err := strconv.ParseUint(params["nc"], 16, 32)
	if err != nil || count == 0 || len(params["nc"]) != 8 || params["cnonce"] == "" {
		return false, false
	}
	nonce := params["nonce"]
	lastDot := strings.LastIndexByte(nonce, '.')
	if lastDot <= 0 {
		return false, false
	}
	payload := nonce[:lastDot]
	mac, err := base64.RawURLEncoding.DecodeString(nonce[lastDot+1:])
	if err != nil || subtle.ConstantTimeCompare(mac, hmacSHA256(d.secret, payload)) != 1 {
		return false, false
	}
	issued, _, _ := strings.Cut(payload, ".")
	unix, err := strconv.ParseInt(issued, 10, 64)
	if err != nil {
		return false, false
	}
	expires := time.Unix(unix, 0).Add(davNonceTTL)
	var hash func(string) string
	var ha1 string
	switch strings.ToUpper(params["algorithm"]) {
	case "SHA-256":
		hash = func(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
		ha1 = creds.SHA256HA1
	case "", "MD5":
		// RFC 2617 clients use MD5. Newer clients can select SHA-256.
		hash = func(s string) string { return fmt.Sprintf("%x", md5.Sum([]byte(s))) }
		ha1 = creds.MD5HA1
	default:
		return false, false
	}
	ha2 := hash(r.Method + ":" + uri)
	expected := hash(strings.Join([]string{ha1, nonce, params["nc"], params["cnonce"], "auth", ha2}, ":"))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(params["response"])) != 1 {
		return false, false
	}
	if !now.Before(expires) {
		return false, true
	}
	return d.acceptCount(nonce+"\x00"+params["cnonce"], uint32(count), now, expires), false
}

func (d *davDigest) acceptCount(key string, count uint32, now, expires time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if now.Sub(d.pruned) >= davNonceTTL {
		for k, state := range d.used {
			if !now.Before(state.expires) {
				delete(d.used, k)
			}
		}
		d.pruned = now
	}
	state := d.used[key]
	if count > state.high {
		state.window <<= count - state.high
		state.high = count
	}
	offset := state.high - count
	if offset >= 64 || state.window&(1<<offset) != 0 {
		return false
	}
	// Parallel requests can arrive out of order. Reject repeated counts without
	// requiring strict arrival order.
	state.window |= 1 << offset
	state.expires = expires
	d.used[key] = state
	return true
}

func parseDAVDigest(value string) map[string]string {
	params := make(map[string]string)
	for value = strings.TrimSpace(value); value != ""; value = strings.TrimSpace(value[1:]) {
		key, rest, ok := strings.Cut(value, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if !ok || key == "" || strings.ContainsAny(key, ", \t") {
			return nil
		}
		if _, duplicate := params[key]; duplicate {
			return nil
		}
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "\"") {
			var quoted strings.Builder
			i := 1
			for i < len(rest) && rest[i] != '"' {
				if rest[i] == '\\' {
					i++
					if i == len(rest) {
						return nil
					}
				}
				quoted.WriteByte(rest[i])
				i++
			}
			if i == len(rest) {
				return nil
			}
			params[key] = quoted.String()
			value = strings.TrimSpace(rest[i+1:])
		} else {
			token, remainder, found := strings.Cut(rest, ",")
			params[key] = strings.TrimSpace(token)
			value = ""
			if found {
				value = "," + remainder
			}
		}
		if value == "" {
			break
		}
		if value[0] != ',' {
			return nil
		}
	}
	return params
}
