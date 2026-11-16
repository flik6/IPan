package httpapi

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"ipan/internal/wopan"
)

func sha256DAV(parts ...string) [32]byte {
	return sha256.Sum256([]byte(strings.Join(parts, "\x00")))
}

type davLock struct {
	Root, Owner  string
	Directory    bool
	Deep, Shared bool
	Expires      time.Time
}

func (l davLock) covers(name string) bool {
	return name == l.Root || l.Deep && davWithin(name, l.Root)
}

type davCondition struct {
	value string
	tag   bool
	not   bool
}

type davIfList struct {
	resource string
	tests    []davCondition
}

func parseDAVIf(value string) ([]davIfList, error) {
	var lists []davIfList
	tagged := strings.HasPrefix(value, "<")
	for value != "" {
		var resource string
		if tagged {
			resource, value = davDelimited(value, '<', '>')
			if resource == "" {
				return nil, davStatus(http.StatusBadRequest)
			}
		}
		if !strings.HasPrefix(value, "(") {
			return nil, davStatus(http.StatusBadRequest)
		}
		for strings.HasPrefix(value, "(") {
			value = strings.TrimSpace(value[1:])
			list := davIfList{resource: resource}
			for value != "" && value[0] != ')' {
				condition := davCondition{}
				if strings.HasPrefix(value, "Not") {
					condition.not = true
					value = strings.TrimSpace(value[3:])
				}
				if strings.HasPrefix(value, "[") {
					condition.tag = true
					condition.value, value = davDelimited(value, '[', ']')
					if !davValidETag(condition.value) {
						return nil, davStatus(http.StatusBadRequest)
					}
				} else {
					condition.value, value = davDelimited(value, '<', '>')
					if !davValidToken(condition.value) {
						return nil, davStatus(http.StatusBadRequest)
					}
				}
				list.tests = append(list.tests, condition)
			}
			if value == "" || len(list.tests) == 0 {
				return nil, davStatus(http.StatusBadRequest)
			}
			lists = append(lists, list)
			value = strings.TrimSpace(value[1:])
		}
	}
	return lists, nil
}

func davDelimited(value string, open, close byte) (string, string) {
	if len(value) < 2 || value[0] != open {
		return "", value
	}
	if end := strings.IndexByte(value[1:], close); end >= 0 {
		return value[1 : end+1], strings.TrimSpace(value[end+2:])
	}
	return "", value
}

func davValidToken(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.IsAbs() && !strings.ContainsAny(value, " \t\r\n<>")
}

func davValidETag(value string) bool {
	value = strings.TrimPrefix(value, "W/")
	return len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' &&
		strings.IndexFunc(value[1:len(value)-1], func(r rune) bool { return r < 33 || r == '"' || r == 127 }) < 0
}

func (d *davRequest) conditions(entry *davEntry) error {
	if d.request.Method != http.MethodGet && d.request.Method != http.MethodHead {
		if status := davHTTPPrecondition(d.request, entry); status != 0 {
			return davStatus(status)
		}
	}
	value := strings.TrimSpace(strings.Join(d.request.Header.Values("If"), " "))
	if value == "" {
		return nil
	}
	lists, err := parseDAVIf(value)
	if err != nil {
		return err
	}
	d.tokens = make(map[string]bool)
	matched := false
	for _, list := range lists {
		name, resource := d.name, entry
		if list.resource != "" {
			name, err = d.reference(list.resource)
			if err != nil && wopan.StatusOf(err) != http.StatusBadGateway {
				return err
			}
			if name != d.name {
				resource = nil
				if name != "" {
					resource, err = d.resolve(name)
					if err != nil {
						return err
					}
				}
			}
		}
		all := true
		for _, test := range list.tests {
			var ok bool
			if test.tag {
				ok = resource.etag() != "" && davWeakTag(test.value) == davWeakTag(resource.etag())
			} else {
				lock, exists := d.server.davLocks[test.value]
				ok = resource != nil && exists && d.now.Before(lock.Expires) && lock.covers(name)
				d.tokens[test.value] = true
			}
			all = all && ok != test.not
		}
		matched = matched || all
	}
	if !matched {
		return davStatus(http.StatusPreconditionFailed)
	}
	return nil
}

func davWeakTag(value string) string {
	return strings.TrimPrefix(value, "W/")
}

func davMatchTags(value string, entry *davEntry, weak bool) bool {
	value = strings.TrimSpace(value)
	if value == "*" {
		return entry != nil
	}
	tag := entry.etag()
	for value != "" && tag != "" {
		quoted := strings.TrimPrefix(value, "W/")
		if !strings.HasPrefix(quoted, "\"") {
			return false
		}
		end := strings.IndexByte(quoted[1:], '"')
		if end < 0 {
			return false
		}
		current := value[:len(value)-len(quoted)+end+2]
		if weak && davWeakTag(current) == davWeakTag(tag) || !weak && current == tag && !strings.HasPrefix(tag, "W/") {
			return true
		}
		rest := strings.TrimSpace(value[len(current):])
		if !strings.HasPrefix(rest, ",") {
			return false
		}
		value = strings.TrimSpace(rest[1:])
	}
	return false
}

func davHTTPPrecondition(r *http.Request, entry *davEntry) int {
	modified := time.Time{}
	if entry != nil {
		modified = entry.Modified.Truncate(time.Second)
	}
	if match := r.Header.Get("If-Match"); match != "" {
		if !davMatchTags(match, entry, false) {
			return http.StatusPreconditionFailed
		}
	} else if date, err := http.ParseTime(r.Header.Get("If-Unmodified-Since")); err == nil && modified.After(date) {
		return http.StatusPreconditionFailed
	}
	safe := r.Method == http.MethodGet || r.Method == http.MethodHead
	if match := r.Header.Get("If-None-Match"); match != "" {
		if davMatchTags(match, entry, true) {
			if safe {
				return http.StatusNotModified
			}
			return http.StatusPreconditionFailed
		}
	} else if safe && !modified.IsZero() {
		if date, err := http.ParseTime(r.Header.Get("If-Modified-Since")); err == nil && !modified.After(date) {
			return http.StatusNotModified
		}
	}
	return 0
}

func davRangeAllowed(r *http.Request, entry *davEntry) bool {
	value := r.Header.Get("If-Range")
	if value == "" {
		return true
	}
	if davValidETag(value) {
		return !strings.HasPrefix(value, "W/") && value == entry.etag()
	}
	t, err := http.ParseTime(value)
	return err == nil && t.Equal(entry.Modified.Truncate(time.Second))
}

func (d *davRequest) requireLocks(name string, subtree, membership bool) error {
	// At least one token is required per independently locked root. Shared locks
	// rooted at the same URL grant any holder permission to modify that resource.
	matched := make(map[string]bool)
	for token, lock := range d.server.davLocks {
		if lock.covers(name) || subtree && davWithin(lock.Root, name) || membership && lock.covers(path.Dir(name)) {
			matched[lock.Root] = matched[lock.Root] || d.tokens[token]
		}
	}
	for root, ok := range matched {
		if !ok {
			failure := &davProtocolError{status: http.StatusLocked, condition: "lock-token-submitted", href: davHref(root, false)}
			if root != name && subtree && davWithin(root, name) {
				failure.member = failure.href
			}
			return failure
		}
	}
	return nil
}

func (d *davRequest) removeLocks(name string, keepRoot bool) {
	for token, lock := range d.server.davLocks {
		if davWithin(lock.Root, name) {
			if !keepRoot || lock.Root != name {
				delete(d.server.davLocks, token)
			}
		}
	}
}

func (d *davRequest) lockDiscovery(name string) string {
	var tokens []string
	for token, lock := range d.server.davLocks {
		if lock.covers(name) && d.now.Before(lock.Expires) {
			tokens = append(tokens, token)
		}
	}
	slices.Sort(tokens)
	var body strings.Builder
	for _, token := range tokens {
		lock := d.server.davLocks[token]
		scope, depth := "exclusive", "0"
		if lock.Shared {
			scope = "shared"
		}
		if lock.Deep {
			depth = "infinity"
		}
		fmt.Fprintf(&body, "<D:activelock><D:lockscope><D:%s/></D:lockscope><D:locktype><D:write/></D:locktype><D:depth>%s</D:depth>", scope, depth)
		body.WriteString(lock.Owner)
		fmt.Fprintf(&body, "<D:timeout>Second-%d</D:timeout><D:locktoken><D:href>%s</D:href></D:locktoken><D:lockroot><D:href>%s</D:href></D:lockroot></D:activelock>",
			davSeconds(d.now, lock.Expires), token, davEscape(davHref(lock.Root, lock.Directory)))
	}
	return body.String()
}

func davSeconds(now, expires time.Time) int64 {
	return max(0, int64((expires.Sub(now)+time.Second-1)/time.Second))
}

func davTimeout(value string) time.Duration {
	const limit = time.Hour
	for raw := range strings.SplitSeq(value, ",") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(raw), "Second-"); ok {
			if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
				return min(max(1, time.Duration(seconds))*time.Second, limit)
			}
		}
		if strings.TrimSpace(raw) == "Infinite" {
			break
		}
	}
	return limit
}

func (d *davRequest) lock(w http.ResponseWriter, entry *davEntry) error {
	body, err := readDAVXML(w, d.request)
	if err != nil {
		return err
	}
	duration := davTimeout(d.request.Header.Get("Timeout"))
	if body == nil {
		if len(d.tokens) != 1 {
			return davStatus(http.StatusBadRequest)
		}
		for token := range d.tokens {
			lock, ok := d.server.davLocks[token]
			if !ok || !lock.covers(d.name) || entry == nil {
				return davStatus(http.StatusPreconditionFailed)
			}
			lock.Expires = d.now.Add(duration)
			d.server.davLocks[token] = lock
		}
		d.writeLock(w, http.StatusOK, duration)
		return nil
	}
	if body.name != davName("lockinfo") {
		return davStatus(http.StatusBadRequest)
	}
	shared := false
	var owner string
	var hasScope, hasType bool
	for _, child := range body.children() {
		switch child.name {
		case davName("lockscope"):
			scopes := child.children()
			if hasScope || len(scopes) != 1 || scopes[0].name != davName("exclusive") && scopes[0].name != davName("shared") {
				return davStatus(http.StatusBadRequest)
			}
			hasScope = true
			shared = scopes[0].name == davName("shared")
		case davName("locktype"):
			if hasType || !child.hasOnly(davName("write")) {
				return davStatus(http.StatusBadRequest)
			}
			hasType = true
		case davName("owner"):
			owner = child.serialize()
		}
	}
	if !hasScope || !hasType {
		return davStatus(http.StatusBadRequest)
	}
	depth := d.request.Header.Get("Depth")
	if depth != "" && depth != "0" && depth != "infinity" {
		return davStatus(http.StatusBadRequest)
	}
	deep := depth != "0"
	for _, lock := range d.server.davLocks {
		if (lock.covers(d.name) || deep && davWithin(lock.Root, d.name)) && (!shared || !lock.Shared) {
			failure := &davProtocolError{status: http.StatusLocked, condition: "no-conflicting-lock", href: davHref(lock.Root, lock.Directory)}
			if lock.Root != d.name && davWithin(lock.Root, d.name) {
				failure.member = failure.href
				failure.dependency = davHref(d.name, true)
			}
			return failure
		}
	}
	status := http.StatusOK
	if entry == nil {
		parent, err := d.parent(d.name)
		if err != nil {
			return err
		}
		if err := d.requireLocks(d.name, false, true); err != nil {
			return err
		}
		if err := d.put(parent.ID, nil, strings.NewReader(""), 0, ""); err != nil {
			return err
		}
		status = http.StatusCreated
		w.Header().Set("Location", davHref(d.name, false))
	}
	token := "opaquelocktoken:" + randomBase64URL(24)
	d.now = time.Now()
	d.server.davLocks[token] = davLock{
		Root: d.name, Owner: owner, Shared: shared, Deep: deep,
		Directory: entry != nil && entry.Directory, Expires: d.now.Add(duration),
	}
	w.Header().Set("Lock-Token", "<"+token+">")
	d.writeLock(w, status, duration)
	return nil
}

func (d *davRequest) writeLock(w http.ResponseWriter, status int, duration time.Duration) {
	w.Header().Set("Timeout", fmt.Sprintf("Second-%d", duration/time.Second))
	davXML(w, status, `<D:prop xmlns:D="DAV:"><D:lockdiscovery>`+d.lockDiscovery(d.name)+`</D:lockdiscovery></D:prop>`)
}

func (d *davRequest) unlock(w http.ResponseWriter, entry *davEntry) error {
	token, rest := davDelimited(strings.TrimSpace(d.request.Header.Get("Lock-Token")), '<', '>')
	if !davValidToken(token) || rest != "" {
		return davStatus(http.StatusBadRequest)
	}
	lock, ok := d.server.davLocks[token]
	if !ok || entry == nil || !lock.covers(d.name) {
		return &davProtocolError{status: http.StatusConflict, condition: "lock-token-matches-request-uri"}
	}
	delete(d.server.davLocks, token)
	w.WriteHeader(http.StatusNoContent)
	return nil
}
