package httpapi

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"ipan/internal/wopan"
)

const davTestToken = "test-token-0123456789"
const davTestOrigin = "https://pan.example.test"

type davTestFile struct {
	parent, name, data string
	dir                bool
}

type davTestCloud struct {
	mu    sync.Mutex
	files map[string]davTestFile
	next  int
}

func (c *davTestCloud) add(file davTestFile) string {
	c.next++
	id := fmt.Sprintf("id%04d", c.next)
	c.files[id] = file
	return id
}

func (c *davTestCloud) children(parent string) []string {
	var ids []string
	for id, file := range c.files {
		if file.parent == parent {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (c *davTestCloud) copy(id, parent string) {
	file := c.files[id]
	file.parent = parent
	dst := c.add(file)
	for _, child := range c.children(id) {
		c.copy(child, dst)
	}
}

func (c *davTestCloud) remove(id string) {
	for _, child := range c.children(id) {
		c.remove(child)
	}
	delete(c.files, id)
}

func (c *davTestCloud) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case strings.HasPrefix(r.URL.Path, "/download/"):
		file, ok := c.files[strings.TrimPrefix(r.URL.Path, "/download/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, file.name, time.Time{}, strings.NewReader(file.data))
	case r.URL.Path == "/openapi/client/upload2C":
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		body, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		defer body.Close()
		data, err := io.ReadAll(body)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		c.add(davTestFile{parent: r.FormValue("directoryId"), name: r.FormValue("fileName"), data: string(data)})
		_, _ = io.WriteString(w, `{"code":"0000"}`)
	case r.URL.Path == "/wohome/dispatcher":
		c.dispatch(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (c *davTestCloud) dispatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Header struct{ Key string }
		Body   struct{ Param string }
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	plain, err := wopan.DecryptWithToken(davTestToken, req.Body.Param)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var param map[string]any
	if err := json.Unmarshal([]byte(plain), &param); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	str := func(k string) string { return fmt.Sprint(param[k]) }
	var data any
	switch req.Header.Key {
	case "FamilyUserCurrentEncode":
		data = map[string]string{"defaultHomeId": "0"}
	case "QueryAllFiles":
		children := c.children(str("parentDirectoryId"))
		page, size := int(param["pageNum"].(float64)), int(param["pageSize"].(float64))
		files := make([]map[string]any, 0)
		for _, id := range children[min(page*size, len(children)):min((page+1)*size, len(children))] {
			file := c.files[id]
			typ := 1
			if file.dir {
				typ = 0
			}
			files = append(files, map[string]any{
				"id": id, "fid": id, "name": file.name, "type": typ, "size": len(file.data),
				"createTime": "20261110120000", "fileVersion": 1,
			})
		}
		data = map[string]any{"files": files}
	case "CreateDirectory":
		c.add(davTestFile{parent: str("parentDirectoryId"), name: str("directoryName"), dir: true})
	case "RenameFileOrDirectory":
		id := str("id")
		file := c.files[id]
		file.name = str("name")
		c.files[id] = file
	case "DeleteFile", "MoveFile", "CopyFile":
		for _, field := range []string{"dirList", "fileList"} {
			for _, raw := range toArray(param[field]) {
				id := raw.(string)
				switch req.Header.Key {
				case "DeleteFile":
					c.remove(id)
				case "MoveFile":
					file := c.files[id]
					file.parent = str("targetDirId")
					c.files[id] = file
				case "CopyFile":
					c.copy(id, str("targetDirId"))
				}
			}
		}
	case "GetDownloadUrlV2":
		var list []map[string]string
		for _, raw := range toArray(param["fidList"]) {
			id := raw.(string)
			if _, ok := c.files[id]; ok {
				list = append(list, map[string]string{"fid": id, "downloadUrl": "http://" + r.Host + "/download/" + id})
			}
		}
		data = map[string]any{"list": list}
	default:
		http.Error(w, "unexpected command "+req.Header.Key, 400)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"STATUS": "200", "RSP": map[string]any{"RSP_CODE": "0000", "DATA": data},
	})
}

func newDAVTest(t *testing.T) (http.Handler, *davTestCloud) {
	t.Helper()
	cloud := &davTestCloud{files: make(map[string]davTestFile)}
	upstream := httptest.NewServer(cloud)
	t.Cleanup(upstream.Close)
	server, _, _ := newTestServer(t)
	server.cfg.WebDAVUser = "dav"
	server.cfg.WebDAVPassword = "dav-test-password"
	server.cfg.WopanAccessToken = davTestToken
	server.cfg.WopanBaseURL = upstream.URL
	server.cfg.WopanZoneURL = upstream.URL
	server.cfg.PublicBaseURL = davTestOrigin
	return server.Handler(), cloud
}

func davTestRequest(t *testing.T, handler http.Handler, code int, method, name, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, davTestOrigin+name, strings.NewReader(body))
	req.SetBasicAuth("dav", "dav-test-password")
	for i := 0; i < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != code {
		t.Fatalf("%s %s: got %d, want %d, body %s", method, name, rec.Code, code, rec.Body)
	}
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/xml") {
		if err := xml.Unmarshal(rec.Body.Bytes(), new(any)); err != nil {
			t.Fatalf("invalid response XML: %v, body %s", err, rec.Body)
		}
	}
	return rec
}

func TestDAVFilesAndConditions(t *testing.T) {
	handler, cloud := newDAVTest(t)
	davTestRequest(t, handler, 401, "PROPFIND", "/dav/", "", "Authorization", "")
	davTestRequest(t, handler, 200, "OPTIONS", "/dav", "", "Authorization", "")
	davTestRequest(t, handler, 403, "PROPFIND", "/dav", "")
	findUTF16 := []byte{0xff, 0xfe}
	for _, unit := range utf16.Encode([]rune(`<?xml version="1.0" encoding="UTF-16"?><D:propfind xmlns:D="DAV:"><D:propname/></D:propfind>`)) {
		findUTF16 = binary.LittleEndian.AppendUint16(findUTF16, unit)
	}
	davTestRequest(t, handler, 207, "PROPFIND", "/dav/", string(findUTF16), "Depth", "0", "Content-Type", "text/xml; charset=utf-16")
	include := `<D:propfind xmlns:D="DAV:" xmlns:X="urn:test"><D:allprop/><D:include><D:getlastmodified/><X:missing/></D:include></D:propfind>`
	rootProps := davTestRequest(t, handler, 207, "PROPFIND", "/dav/", include, "Depth", "0")
	if body := rootProps.Body.String(); !strings.Contains(body, "404 Not Found") || !strings.Contains(body, "missing") || strings.Contains(body, "0001") {
		t.Fatalf("unexpected allprop/include status: %s", body)
	}
	davTestRequest(t, handler, 400, "GET", "/dav/a%2Fb", "")
	davTestRequest(t, handler, 409, "MKCOL", "/dav/missing/child", "")
	davTestRequest(t, handler, 415, "MKCOL", "/dav/album", "<x/>")
	davTestRequest(t, handler, 201, "MKCOL", "/dav/album", "")
	file := "/dav/album/%E4%B8%AD%E6%96%87%20%25%23.txt"
	davTestRequest(t, handler, 201, "PUT", file, "abcdef", "If-None-Match", "*")
	listed := davTestRequest(t, handler, 207, "PROPFIND", "/dav/album", "", "Depth", "1")
	if !strings.Contains(listed.Body.String(), "<D:href>"+file+"</D:href>") {
		t.Fatalf("missing escaped resource href %s: %s", file, listed.Body)
	}
	davTestRequest(t, handler, 412, "PUT", file, "no", "If-None-Match", "*")
	davTestRequest(t, handler, 400, "PUT", file, "no", "Content-Range", "bytes 0-1/6")
	head := davTestRequest(t, handler, 200, "HEAD", file, "")
	if head.Header().Get("Content-Length") != "6" || head.Header().Get("ETag") == "" || head.Body.Len() != 0 {
		t.Fatalf("unexpected HEAD %v %q", head.Header(), head.Body)
	}
	part := davTestRequest(t, handler, 206, "GET", file, "", "Range", "bytes=1-3")
	if part.Body.String() != "bcd" || part.Header().Get("Content-Range") != "bytes 1-3/6" {
		t.Fatalf("unexpected Range response %v %q", part.Header(), part.Body)
	}
	davTestRequest(t, handler, 304, "GET", file, "", "If-None-Match", head.Header().Get("ETag"))
	davTestRequest(t, handler, 206, "GET", file, "", "Range", "bytes=1-3", "If-Range", head.Header().Get("ETag"))
	davTestRequest(t, handler, 200, "GET", file, "", "Range", "bytes=1-3", "If-Range", "W/"+head.Header().Get("ETag"))

	// Simulate a truncated fixed-length body. The existing cloud file survives.
	broken := httptest.NewRequest("PUT", davTestOrigin+file, strings.NewReader("short"))
	broken.ContentLength = 99
	broken.SetBasicAuth("dav", "dav-test-password")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, broken)
	if rec.Code != 400 {
		t.Fatalf("truncated PUT: %d %s", rec.Code, rec.Body)
	}
	old := davTestRequest(t, handler, 200, "GET", file, "")
	if old.Body.String() != "abcdef" {
		t.Fatalf("truncated PUT replaced original with %q", old.Body)
	}

	// Chunked PUT must supply the known length required by the upstream upload API.
	chunked := httptest.NewRequest("PUT", davTestOrigin+file, strings.NewReader("updated"))
	chunked.ContentLength = -1
	chunked.SetBasicAuth("dav", "dav-test-password")
	chunked.Header.Set("If-Match", head.Header().Get("ETag"))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, chunked)
	if rec.Code != 204 {
		t.Fatalf("chunked PUT: %d %s", rec.Code, rec.Body)
	}
	copy := "/dav/copy.txt"
	davTestRequest(t, handler, 201, "COPY", file, "", "Destination", davTestOrigin+copy)
	davTestRequest(t, handler, 412, "COPY", file, "", "Destination", copy, "Overwrite", "F")
	davTestRequest(t, handler, 415, "COPY", file, "<x/>", "Destination", copy)
	davTestRequest(t, handler, 502, "MOVE", file, "", "Destination", "https://example.test/dav/other")
	davTestRequest(t, handler, 204, "MOVE", copy, "", "Destination", file, "Overwrite", "T")
	if got := davTestRequest(t, handler, 200, "GET", file, "").Body.String(); got != "updated" {
		t.Fatalf("MOVE content = %q", got)
	}
	davTestRequest(t, handler, 201, "COPY", "/dav/album", "", "Destination", "/dav/shallow", "Depth", "0")
	shallow := davTestRequest(t, handler, 207, "PROPFIND", "/dav/shallow", "", "Depth", "1")
	if strings.Count(shallow.Body.String(), "<D:response>") != 1 {
		t.Fatalf("Depth: 0 COPY included members: %s", shallow.Body)
	}
	davTestRequest(t, handler, 204, "DELETE", "/dav/album", "")
	davTestRequest(t, handler, 404, "GET", file, "")
	cloud.mu.Lock()
	defer cloud.mu.Unlock()
	for _, entry := range cloud.files {
		if strings.HasPrefix(entry.name, ".ipan-dav-") {
			t.Fatalf("left a staging resource: %+v", entry)
		}
	}
}

func TestDAVPropertiesAndLocks(t *testing.T) {
	handler, _ := newDAVTest(t)
	davTestRequest(t, handler, 201, "MKCOL", "/dav/locked", "")
	const file = "/dav/locked/test.txt"
	const lockBody = `<D:lockinfo xmlns:D="DAV:"><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype><D:owner>test &amp; user</D:owner></D:lockinfo>`
	created := davTestRequest(t, handler, 201, "LOCK", file, lockBody, "Timeout", "Second-300")
	token := created.Header().Get("Lock-Token")
	if token == "" || !strings.Contains(created.Body.String(), "test &amp; user") {
		t.Fatalf("missing lock token or owner: %s", created.Body)
	}
	davTestRequest(t, handler, 200, "GET", file, "")
	davTestRequest(t, handler, 423, "PUT", file, "blocked")
	davTestRequest(t, handler, 412, "PUT", file, "blocked", "If", "(<opaquelocktoken:unknown>)")
	davTestRequest(t, handler, 412, "PUT", file, "blocked", "If", "(Not "+token+")")
	davTestRequest(t, handler, 423, "PUT", file, "blocked", "If", "(Not <DAV:no-lock>)")
	davTestRequest(t, handler, 207, "DELETE", "/dav/locked", "")
	davTestRequest(t, handler, 207, "LOCK", "/dav/locked", lockBody)
	condition := "(" + token + ")"
	davTestRequest(t, handler, 200, "LOCK", file, "", "If", condition, "Timeout", "Second-90")
	davTestRequest(t, handler, 204, "PUT", file, "written", "If", condition)

	patch := `<D:propertyupdate xmlns:D="DAV:" xmlns:X="urn:custom"><D:set><D:prop><X:label X:kind="X:name">before<child/>after</X:label><D:displayname>My file</D:displayname></D:prop></D:set></D:propertyupdate>`
	davTestRequest(t, handler, 423, "PROPPATCH", file, patch)
	davTestRequest(t, handler, 207, "PROPPATCH", file, patch, "If", condition)
	protected := `<D:propertyupdate xmlns:D="DAV:" xmlns:X="urn:custom"><D:set><D:prop><D:getetag>bad</D:getetag><X:label>discard</X:label><D:displayname>discard</D:displayname></D:prop></D:set></D:propertyupdate>`
	failed := davTestRequest(t, handler, 207, "PROPPATCH", file, protected, "If", condition)
	if body := failed.Body.String(); !strings.Contains(body, "424 Failed Dependency") ||
		!strings.Contains(body, "cannot-modify-protected-property") ||
		!strings.Contains(body, "displayname") || strings.Contains(body, "urn:ipan:dav:edit:") {
		t.Fatalf("property transaction was not rejected: %s", failed.Body)
	}
	find := `<D:propfind xmlns:D="DAV:" xmlns:X="urn:custom"><D:prop><X:label/><X:missing/><D:displayname/><D:lockdiscovery/></D:prop></D:propfind>`
	found := davTestRequest(t, handler, 207, "PROPFIND", file, find, "Depth", "0")
	if body := found.Body.String(); !strings.Contains(body, "404 Not Found") ||
		!strings.Contains(body, "before<child></child>after") ||
		!strings.Contains(body, `X:kind="X:name"`) || !strings.Contains(body, `xmlns:X="urn:custom"`) ||
		!strings.Contains(body, "My file") || !strings.Contains(body, strings.Trim(token, "<>")) {
		t.Fatalf("unexpected named properties: %s", found.Body)
	}
	decoder := xml.NewDecoder(strings.NewReader(found.Body.String()))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if element, ok := token.(xml.StartElement); ok && element.Name.Local == "child" && element.Name.Space != "" {
			t.Fatalf("unqualified child gained namespace %q", element.Name.Space)
		}
	}
	davTestRequest(t, handler, 201, "COPY", "/dav/locked", "", "Destination", "/dav/copied")
	copied := davTestRequest(t, handler, 207, "PROPFIND", "/dav/copied/test.txt", find, "Depth", "0")
	if !strings.Contains(copied.Body.String(), "before") || strings.Contains(copied.Body.String(), "opaquelocktoken") {
		t.Fatalf("COPY should preserve properties without propagating locks: %s", copied.Body)
	}
	davTestRequest(t, handler, 409, "UNLOCK", "/dav/copied/test.txt", "", "Lock-Token", token)
	davTestRequest(t, handler, 204, "UNLOCK", file, "", "Lock-Token", token)
	shared := strings.ReplaceAll(lockBody, "exclusive", "shared")
	first := davTestRequest(t, handler, 200, "LOCK", "/dav/locked", shared, "Depth", "0")
	second := davTestRequest(t, handler, 200, "LOCK", "/dav/locked", shared, "Depth", "0")
	if first.Header().Get("Lock-Token") == second.Header().Get("Lock-Token") {
		t.Fatal("shared locks reused a token")
	}
	davTestRequest(t, handler, 423, "MKCOL", "/dav/locked/sub", "")
	tagged := "<" + davTestOrigin + "/dav/locked/> (" + second.Header().Get("Lock-Token") + ")"
	davTestRequest(t, handler, 201, "MKCOL", "/dav/locked/sub", "", "If", tagged)
	davTestRequest(t, handler, 204, "PUT", file, "updated independently")
}

func TestDAVDigestOnHTTP(t *testing.T) {
	handler, _ := newDAVTest(t)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	request := func(method, target, auth string) *http.Response {
		t.Helper()
		r, err := http.NewRequest(method, server.URL+target, nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Depth", "0")
		r.Header.Set("Authorization", auth)
		// Forwarded headers only establish a secure origin when explicitly trusted.
		r.Header.Set("X-Forwarded-Proto", "https")
		res, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)
		return res
	}
	unauthorized := request("PROPFIND", "/dav/", "Basic ZGF2OmRhdi10ZXN0LXBhc3N3b3Jk")
	if unauthorized.StatusCode != 401 {
		t.Fatalf("accepted Basic over HTTP: %d", unauthorized.StatusCode)
	}
	challenges := unauthorized.Header.Values("WWW-Authenticate")
	if len(challenges) != 2 || !strings.HasPrefix(challenges[0], "Digest ") {
		t.Fatalf("unexpected HTTP challenge: %v", challenges)
	}
	challenge := parseDAVDigest(strings.TrimPrefix(challenges[0], "Digest "))
	nonce, realm := challenge["nonce"], challenge["realm"]
	authorization := func(nc string) string {
		hash := func(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
		ha1, ha2 := hash("dav:"+realm+":dav-test-password"), hash("PROPFIND:/dav/")
		response := hash(ha1 + ":" + nonce + ":" + nc + ":client-nonce:auth:" + ha2)
		return fmt.Sprintf(`Digest username="dav", realm="%s", nonce="%s", uri="/dav/", algorithm=SHA-256, response="%s", qop=auth, nc=%s, cnonce="client-nonce"`, realm, nonce, response, nc)
	}
	first := authorization("00000001")
	if got := request("PROPFIND", "/dav/file", first).StatusCode; got != 401 {
		t.Fatalf("digest authenticated a different target: %d", got)
	}
	if got := request("PROPFIND", "/dav/", first).StatusCode; got != 207 {
		t.Fatalf("digest authentication failed: %d", got)
	}
	if got := request("PROPFIND", "/dav/", first).StatusCode; got != 401 {
		t.Fatalf("digest replay accepted: %d", got)
	}
	for _, count := range []string{"00000003", "00000002"} {
		if got := request("PROPFIND", "/dav/", authorization(count)).StatusCode; got != 207 {
			t.Fatalf("out-of-order Digest request %s failed: %d", count, got)
		}
	}
}
