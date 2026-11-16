package httpapi

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/webdav"
)

// davCloudFS exposes cloud metadata and directory operations to webdav.Handler.
// File contents use iPan's streaming paths and cloud-native COPY/MOVE.
type davCloudFS struct {
	dav       *davRequest
	err       error
	namesOnly bool
	required  []xml.Name
	resources map[string]davResourceProperties
	rewrite   bool
	edits     []davPropertyEdit
	aliases   map[xml.Name]xml.Name
}

type davResourceProperties struct {
	values  davProperties
	omitted []xml.Name
}

var _ webdav.FileSystem = (*davCloudFS)(nil)
var _ webdav.DeadPropsHolder = (*davCloudFile)(nil)

func (f *davCloudFS) record(err error) error {
	if err != nil {
		f.err = err
	}
	return err
}

func (f *davCloudFS) lookup(name string) (*davEntry, error) {
	if f.dav == nil { // OPTIONS advertises capabilities without accessing the cloud.
		return nil, fs.ErrNotExist
	}
	entry, err := f.dav.resolve(path.Clean("/" + name))
	if err != nil {
		return nil, f.record(err)
	}
	if entry == nil {
		return nil, fs.ErrNotExist
	}
	return entry, nil
}

func (f *davCloudFS) Stat(_ context.Context, name string) (os.FileInfo, error) {
	entry, err := f.lookup(name)
	if err != nil {
		return nil, err
	}
	return davFileInfo{entry}, nil
}

func (f *davCloudFS) OpenFile(_ context.Context, name string, flag int, _ os.FileMode) (webdav.File, error) {
	if flag != os.O_RDONLY && flag != os.O_RDWR {
		return nil, errors.ErrUnsupported
	}
	entry, err := f.lookup(name)
	if err != nil {
		return nil, err
	}
	return &davCloudFile{fs: f, name: path.Clean("/" + name), entry: entry}, nil
}

func (f *davCloudFS) Mkdir(ctx context.Context, name string, _ os.FileMode) error {
	parent, err := f.dav.parent(path.Clean("/" + name))
	if err != nil {
		return f.record(err)
	}
	_, err = f.dav.client.Mkdir(ctx, parent.ID, path.Base(name))
	return f.record(err)
}

func (f *davCloudFS) RemoveAll(ctx context.Context, name string) error {
	entry, err := f.lookup(name)
	if err != nil {
		return err
	}
	keys, err := f.dav.propertyKeys(entry)
	if err != nil {
		return f.record(err)
	}
	if err := f.dav.remove(ctx, entry); err != nil {
		return f.record(err)
	}
	f.dav.removeLocks(path.Clean("/"+name), false)
	f.dav.removeProperties(keys)
	return nil
}

func (f *davCloudFS) Rename(context.Context, string, string) error {
	return errors.ErrUnsupported
}

type davFileInfo struct{ entry *davEntry }

func (i davFileInfo) Name() string                         { return i.entry.Name }
func (i davFileInfo) Size() int64                          { return i.entry.Size }
func (i davFileInfo) ModTime() time.Time                   { return i.entry.Modified }
func (i davFileInfo) IsDir() bool                          { return i.entry.Directory }
func (i davFileInfo) Sys() any                             { return nil }
func (i davFileInfo) ETag(context.Context) (string, error) { return i.entry.etag(), nil }
func (i davFileInfo) ContentType(context.Context) (string, error) {
	return i.entry.contentType(), nil
}

func (i davFileInfo) Mode() os.FileMode {
	if i.IsDir() {
		return os.ModeDir | 0o755
	}
	return 0o644
}

type davCloudFile struct {
	fs     *davCloudFS
	name   string
	entry  *davEntry
	offset int
}

func (f *davCloudFile) Close() error                   { return nil }
func (f *davCloudFile) Stat() (os.FileInfo, error)     { return davFileInfo{f.entry}, nil }
func (f *davCloudFile) Read([]byte) (int, error)       { return 0, errors.ErrUnsupported }
func (f *davCloudFile) Write([]byte) (int, error)      { return 0, errors.ErrUnsupported }
func (f *davCloudFile) Seek(int64, int) (int64, error) { return 0, errors.ErrUnsupported }

func (f *davCloudFile) Readdir(count int) ([]os.FileInfo, error) {
	if !f.entry.Directory {
		return nil, fs.ErrInvalid
	}
	entries, err := f.fs.dav.list(f.entry.ID)
	if err != nil {
		return nil, f.fs.record(err)
	}
	end := len(entries)
	if count > 0 {
		if f.offset == end {
			return nil, io.EOF
		}
		end = f.offset + min(count, end-f.offset)
	}
	files := make([]os.FileInfo, 0, end-f.offset)
	for i := f.offset; i < end; i++ {
		files = append(files, davFileInfo{&entries[i]})
	}
	f.offset = end
	return files, nil
}

func (f *davCloudFile) DeadProps() (map[xml.Name]webdav.Property, error) {
	href := davHref(f.name, f.entry.Directory)
	resource, found := f.fs.resources[href]
	if !found {
		values, err := f.fs.dav.properties(f.entry)
		if err != nil {
			return nil, f.fs.record(err)
		}
		resource.values = values
		if f.entry.Modified.IsZero() {
			resource.omitted = append(resource.omitted, davName("getlastmodified"))
		}
		if !f.entry.Directory && f.entry.Size < 0 {
			resource.omitted = append(resource.omitted, davName("getcontentlength"))
		}
		f.fs.resources[href] = resource
		f.fs.rewrite = f.fs.rewrite || len(resource.omitted) > 0 || !f.fs.namesOnly && len(values) > 0
	}
	props := make(map[xml.Name]webdav.Property)
	for key := range resource.values {
		name := davPropertyXMLName(key)
		// The handler already includes the built-in displayname in propname.
		if !f.fs.namesOnly || name != davName("displayname") {
			props[name] = webdav.Property{XMLName: name}
		}
	}
	set := func(local, inner string) {
		name := davName(local)
		props[name] = webdav.Property{XMLName: name, InnerXML: []byte(inner)}
	}
	if !f.entry.Created.IsZero() {
		set("creationdate", f.entry.Created.UTC().Format(time.RFC3339))
	}
	if f.entry.Directory {
		set("getcontentlength", "0")
	}
	set("lockdiscovery", f.fs.dav.lockDiscovery(f.name))
	if !f.fs.namesOnly {
		if _, stored := props[davName("displayname")]; f.name == "/" && !stored {
			set("displayname", "iPan")
		}
		set("supportedlock", "<D:lockentry><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype></D:lockentry>"+
			"<D:lockentry><D:lockscope><D:shared/></D:lockscope><D:locktype><D:write/></D:locktype></D:lockentry>")
	}
	return props, nil
}

type davLibraryResponse struct {
	http.ResponseWriter
	body   bytes.Buffer
	status int
}

func (w *davLibraryResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *davLibraryResponse) Write(body []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(body)
}

func serveDAVLibrary(w http.ResponseWriter, r *http.Request, cloud *davCloudFS) error {
	req := r.Clone(r.Context())
	// The outer handler has already evaluated If and enforced the shared lock
	// table. The library only takes short-lived locks for this delegated call.
	req.Header.Del("If")
	handler := webdav.Handler{Prefix: davPrefix, FileSystem: cloud, LockSystem: webdav.NewMemLS()}
	resp := &davLibraryResponse{ResponseWriter: w}
	handler.ServeHTTP(resp, req)
	if cloud.err != nil {
		return cloud.err
	}
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", davMethods)
	}
	if resp.status == http.StatusCreated && r.Method == "MKCOL" {
		w.Header().Set("Location", davHref(cloud.dav.name, true))
	}
	body := resp.body.Bytes()
	if resp.status == http.StatusMultiStatus {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		var err error
		body, err = cloud.restorePropertyXML(body)
		if err != nil {
			return err
		}
	}
	if resp.status == 0 {
		resp.status = http.StatusOK
	}
	w.WriteHeader(resp.status)
	if len(body) != 0 {
		_, _ = w.Write(body)
	}
	return nil
}

// Preserve full stored elements (including attributes x/net cannot represent),
// omit unavailable cloud metadata, and report missing allprop/include names.
func (f *davCloudFS) restorePropertyXML(body []byte) ([]byte, error) {
	if len(f.aliases) == 0 && !f.rewrite && len(f.required) == 0 {
		return body, nil
	}
	var output bytes.Buffer
	dec := xml.NewDecoder(bytes.NewReader(body))
	var stack []xml.Name
	var href string
	seen := make(map[xml.Name]bool)
	var copied int64
	for {
		offset := dec.InputOffset()
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch node := token.(type) {
		case xml.StartElement:
			if len(stack) == 2 && stack[1] == davName("response") && node.Name == davName("href") {
				if err := dec.DecodeElement(&href, &node); err != nil {
					return nil, err
				}
				clear(seen)
				continue
			}
			if len(stack) == 4 && stack[1] == davName("response") && stack[3] == davName("prop") {
				value, replace := f.propertyReplacement(href, node.Name)
				if value != "" || !replace {
					seen[node.Name] = true
				}
				if err := dec.Skip(); err != nil {
					return nil, err
				}
				if replace {
					output.Write(body[copied:offset])
					output.WriteString(value)
					copied = dec.InputOffset()
				}
				continue
			}
			stack = append(stack, node.Name)
		case xml.EndElement:
			if len(stack) == 2 && node.Name == davName("response") {
				if missing := f.missingProperties(seen); missing != "" {
					output.Write(body[copied:offset])
					output.WriteString(missing)
					copied = offset
				}
			}
			stack = stack[:len(stack)-1]
		}
	}
	if copied == 0 {
		return body, nil
	}
	output.Write(body[copied:])
	return output.Bytes(), nil
}

func (f *davCloudFS) propertyReplacement(href string, name xml.Name) (string, bool) {
	if alias, ok := f.aliases[name]; ok {
		return davEmptyProperty(davPropertyName(alias)), true
	}
	resource := f.resources[href]
	if slices.Contains(resource.omitted, name) {
		return "", true
	}
	if !f.namesOnly {
		value, found := resource.values[davPropertyName(name)]
		return value, found
	}
	return "", false
}

func (f *davCloudFS) missingProperties(seen map[xml.Name]bool) string {
	var missing strings.Builder
	for _, name := range f.required {
		if !seen[name] {
			missing.WriteString(davEmptyProperty(davPropertyName(name)))
		}
	}
	if missing.Len() == 0 {
		return ""
	}
	return "<D:propstat><D:prop>" + missing.String() + "</D:prop><D:status>HTTP/1.1 404 Not Found</D:status></D:propstat>"
}
