package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"ipan/internal/wopan"
)

const davPrefix = "/dav"
const davMethods = "OPTIONS, GET, HEAD, PUT, DELETE, PROPFIND, PROPPATCH, MKCOL, COPY, MOVE, LOCK, UNLOCK"

func (s *Server) handleDAV(w http.ResponseWriter, r *http.Request) {
	user, password := s.cfg.WebDAVUser, s.cfg.WebDAVPassword
	if user == "" || password == "" || strings.Contains(user, ":") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("DAV", "1, 2")
	w.Header().Set("MS-Author-Via", "DAV")
	w.Header().Set("Allow", davMethods)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	if r.Method != http.MethodOptions && !s.authenticateDAV(w, r) {
		return
	}
	name, err := davPath(r.URL)
	if err != nil {
		davError(w, err)
		return
	}
	switch r.Method {
	case http.MethodOptions, http.MethodGet, http.MethodHead, http.MethodDelete, "MKCOL", "COPY", "MOVE", "UNLOCK":
		if err := davNoBody(r); err != nil {
			davError(w, err)
			return
		}
	}
	if r.Method == http.MethodOptions {
		if err := serveDAVLibrary(w, r, &davCloudFS{}); err != nil {
			davError(w, err)
		}
		return
	}
	var releaseRead func()
	switch r.Method {
	case http.MethodGet, http.MethodHead, "PROPFIND":
		s.davMu.RLock()
		releaseRead = sync.OnceFunc(s.davMu.RUnlock)
		defer releaseRead()
	case http.MethodPut, http.MethodDelete, "PROPPATCH", "MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK":
		s.davMu.Lock()
		defer s.davMu.Unlock()
		now := time.Now()
		for token, lock := range s.davLocks {
			if !now.Before(lock.Expires) {
				delete(s.davLocks, token)
			}
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	d := &davRequest{
		server: s, request: r, client: s.client(), name: name, now: time.Now(),
		lists:       make(map[string][]davEntry),
		releaseRead: releaseRead,
	}
	if !d.client.HasToken() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	err = d.serve(w)
	if err != nil {
		davError(w, err)
	}
}

func davNoBody(r *http.Request) error {
	if r.ContentLength > 0 {
		return davStatus(http.StatusUnsupportedMediaType)
	}
	if r.ContentLength < 0 {
		var b [1]byte
		if _, err := io.ReadFull(r.Body, b[:]); err != io.EOF {
			return davStatus(http.StatusUnsupportedMediaType)
		}
	}
	return nil
}

type davRequest struct {
	server      *Server
	request     *http.Request
	client      *wopan.Client
	name        string
	now         time.Time
	lists       map[string][]davEntry
	tokens      map[string]bool
	releaseRead func()
}

func (d *davRequest) serve(w http.ResponseWriter) error {
	entry, err := d.resolve(d.name)
	if err != nil {
		return err
	}
	if err := d.conditions(entry); err != nil {
		return err
	}
	switch d.request.Method {
	case http.MethodGet, http.MethodHead:
		return d.get(w, entry)
	case "PROPFIND":
		return d.propfind(w, entry)
	case "PROPPATCH":
		return d.proppatch(w, entry)
	case "COPY", "MOVE":
		return d.copyMove(w, entry)
	case "LOCK":
		return d.lock(w, entry)
	case "UNLOCK":
		return d.unlock(w, entry)
	default:
		return d.writeResource(w, entry)
	}
}

func davPath(u *url.URL) (string, error) {
	if u.Path == davPrefix {
		return "/", nil
	}
	escaped := u.EscapedPath()
	if !strings.HasPrefix(escaped, davPrefix+"/") {
		return "", davStatus(http.StatusBadRequest)
	}
	relative := strings.TrimSuffix(strings.TrimPrefix(escaped, davPrefix+"/"), "/")
	if relative == "" {
		if u.Path != davPrefix+"/" {
			return "", davStatus(http.StatusBadRequest)
		}
		return "/", nil
	}
	segments := strings.Split(relative, "/")
	for i, segment := range segments {
		decoded, err := url.PathUnescape(segment)
		if err != nil || !davValidName(decoded) {
			return "", davStatus(http.StatusBadRequest)
		}
		segments[i] = decoded
	}
	return "/" + strings.Join(segments, "/"), nil
}

func davValidName(name string) bool {
	if name == "" || name == "." || name == ".." || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\") {
		return false
	}
	return strings.IndexFunc(name, func(r rune) bool { return r < 32 || r == 127 }) < 0
}

func davHref(name string, collection bool) string {
	uri := url.URL{Path: davPrefix + name}
	if collection && !strings.HasSuffix(uri.Path, "/") {
		uri.Path += "/"
	}
	return uri.EscapedPath()
}

func davWithin(name, root string) bool {
	return name == root || strings.HasPrefix(name, strings.TrimSuffix(root, "/")+"/")
}

func (d *davRequest) reference(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.RawQuery != "" || u.ForceQuery || u.User != nil || u.Opaque != "" {
		return "", davStatus(http.StatusBadRequest)
	}
	if u.Host != "" || u.Scheme != "" {
		origin, err := url.Parse(d.server.baseURL(d.request))
		if err != nil || !strings.EqualFold(origin.Scheme, u.Scheme) || !strings.EqualFold(origin.Host, u.Host) {
			return "", davStatus(http.StatusBadGateway)
		}
	}
	return davPath(u)
}

func davStatus(code int) error {
	return wopan.Errorf(code, "%s", http.StatusText(code))
}

func davError(w http.ResponseWriter, err error) {
	var protocol *davProtocolError
	if errors.As(err, &protocol) {
		protocol.write(w)
		return
	}
	status := wopan.StatusOf(err)
	// An expired upstream token must not prompt clients for their DAV password again.
	if status == http.StatusUnauthorized {
		status = http.StatusBadGateway
	}
	switch status {
	case http.StatusLocked:
		davXML(w, status, `<D:error xmlns:D="DAV:"><D:lock-token-submitted/></D:error>`)
	case http.StatusNotModified:
		w.WriteHeader(status)
	default:
		http.Error(w, http.StatusText(status), status)
	}
}

func (d *davRequest) get(w http.ResponseWriter, entry *davEntry) error {
	if entry == nil {
		return davStatus(http.StatusNotFound)
	}
	w.Header().Set("ETag", entry.etag())
	if !entry.Modified.IsZero() {
		w.Header().Set("Last-Modified", entry.Modified.UTC().Format(http.TimeFormat))
	}
	if status := davHTTPPrecondition(d.request, entry); status != 0 {
		w.WriteHeader(status)
		return nil
	}
	if entry.Directory {
		// Return a minimal collection representation. PROPFIND enumerates members.
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
		return nil
	}
	w.Header().Set("Content-Type", entry.contentType())
	w.Header().Set("Content-Disposition", contentDisposition(true, entry.Name))
	w.Header().Set("Accept-Ranges", "bytes")
	if d.request.Method == http.MethodHead || entry.Size == 0 {
		if entry.Size >= 0 {
			w.Header().Set("Content-Length", fmt.Sprint(entry.Size))
		}
		if entry.Size == 0 && d.request.Method == http.MethodGet && d.request.Header.Get("Range") != "" &&
			davRangeAllowed(d.request, entry) {
			w.Header().Set("Content-Range", "bytes */0")
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return nil
		}
		w.WriteHeader(http.StatusOK)
		return nil
	}
	link, err := d.client.GetDownloadURL(d.request.Context(), entry.FID)
	if err != nil {
		return err
	}
	upstream, err := http.NewRequestWithContext(d.request.Context(), http.MethodGet, link, nil)
	if err != nil {
		return davStatus(http.StatusBadGateway)
	}
	for k, v := range wopan.CommonHeaders() {
		upstream.Header.Set(k, v)
	}
	if davRangeAllowed(d.request, entry) {
		upstream.Header.Set("Range", d.request.Header.Get("Range"))
	}
	d.releaseRead()
	return davDownload(w, upstream)
}

func (d *davRequest) parent(name string) (*davEntry, error) {
	entry, err := d.resolve(path.Dir(name))
	if err == nil && (entry == nil || !entry.Directory) {
		err = davStatus(http.StatusConflict)
	}
	return entry, err
}
