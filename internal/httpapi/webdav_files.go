package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"ipan/internal/wopan"
)

type davEntry struct {
	ID, FID, Name, Parent string
	Version               string
	Directory             bool
	Size                  int64
	Created, Modified     time.Time
}

func (e *davEntry) etag() string {
	if e == nil || e.Directory {
		return ""
	}
	tag := fmt.Sprintf("\"%x\"", sha256DAV(e.ID, e.FID, e.Name, e.Version, strconv.FormatInt(e.Size, 10), e.Modified.Format(time.RFC3339Nano)))
	if e.Version == "" {
		// Without the cloud content version, metadata can only serve as a weak validator.
		return "W/" + tag
	}
	return tag
}

func (e *davEntry) contentType() string {
	if ct := mime.TypeByExtension(path.Ext(e.Name)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func (e *davEntry) ids() (dirs, files []string) {
	if e.Directory {
		return []string{e.ID}, nil
	}
	return nil, []string{e.ID}
}

func (d *davRequest) list(id string) ([]davEntry, error) {
	if entries, ok := d.lists[id]; ok {
		return entries, nil
	}
	const pageSize = 100
	var entries []davEntry
	seen := make(map[string]bool)
	for page := 0; ; page++ {
		data, err := d.client.List(d.request.Context(), id, page, pageSize)
		if err != nil {
			return nil, err
		}
		items := toArray(data["files"])
		for _, item := range items {
			raw := asObject(item)
			name, _ := wopan.PickString(raw, "name", "fileName", "directoryName")
			itemID, _ := wopan.PickString(raw, "id", "fid", "directoryId")
			fid, _ := wopan.PickString(raw, "fid", "id")
			typ := fmt.Sprint(raw["type"])
			version := ""
			if raw["fileVersion"] != nil {
				version = fmt.Sprint(raw["fileVersion"])
			}
			if itemID == "" || !davValidName(name) {
				continue
			}
			key := typ + "/" + itemID
			if seen[key] {
				return nil, wopan.Errorf(http.StatusBadGateway, "目录分页重复返回同一条目")
			}
			seen[key] = true
			entries = append(entries, davEntry{
				ID: itemID, FID: fid, Name: name, Parent: id, Directory: typ == "0", Version: version,
				Size:     davEntrySize(raw),
				Created:  davEntryTime(raw, "createTime", "uploadTime"),
				Modified: davEntryTime(raw, "updateTime", "modifyTime", "uploadTime", "createTime"),
			})
		}
		if len(items) < pageSize {
			d.lists[id] = entries
			return entries, nil
		}
	}
}

var davCloudTZ = time.FixedZone("CST", 8*60*60)

func davEntrySize(raw map[string]any) int64 {
	for _, field := range []string{"size", "fileSize"} {
		if value, ok := raw[field]; ok && value != nil {
			if n, err := strconv.ParseInt(fmt.Sprint(value), 10, 64); err == nil {
				return max(-1, n)
			}
			if n, ok := value.(float64); ok && n >= 0 {
				return int64(n)
			}
		}
	}
	return -1
}

func davEntryTime(raw map[string]any, fields ...string) time.Time {
	for _, field := range fields {
		value, isString := raw[field].(string)
		if !isString {
			value = strconv.FormatInt(jsonNumberToInt64(raw[field]), 10)
		}
		for _, format := range []string{"20060102150405", time.RFC3339Nano, "2006-01-02 15:04:05"} {
			if t, err := time.ParseInLocation(format, value, davCloudTZ); err == nil {
				return t
			}
		}
		if n, err := strconv.ParseInt(value, 10, 64); err == nil && n > 0 {
			switch len(value) {
			case 10:
				return time.Unix(n, 0)
			case 13:
				return time.UnixMilli(n)
			}
		}
	}
	return time.Time{}
}

func (d *davRequest) child(parent, name string) (*davEntry, error) {
	entries, err := d.list(parent)
	if err != nil {
		return nil, err
	}
	var match *davEntry
	for i := range entries {
		if entries[i].Name == name {
			if match != nil {
				return nil, davStatus(http.StatusConflict)
			}
			match = &entries[i]
		}
	}
	return match, nil
}

func (d *davRequest) resolve(name string) (*davEntry, error) {
	entry := &davEntry{ID: "0", Directory: true}
	if name == "/" {
		return entry, nil
	}
	for _, segment := range strings.Split(name[1:], "/") {
		if !entry.Directory {
			return nil, nil
		}
		var err error
		entry, err = d.child(entry.ID, segment)
		if err != nil || entry == nil {
			return entry, err
		}
	}
	return entry, nil
}

func davDownload(w http.ResponseWriter, request *http.Request) error {
	res, err := proxyHTTPClient.Do(request)
	if err != nil {
		return davStatus(http.StatusBadGateway)
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusRequestedRangeNotSatisfiable:
		for _, name := range []string{"Content-Length", "Content-Range"} {
			if value := res.Header.Get(name); value != "" {
				w.Header().Set(name, value)
			}
		}
		if ct := res.Header.Get("Content-Type"); strings.HasPrefix(ct, "multipart/byteranges") {
			w.Header().Set("Content-Type", ct)
		}
		w.WriteHeader(res.StatusCode)
		_, _ = io.Copy(w, res.Body)
		return nil
	default:
		return davStatus(http.StatusBadGateway)
	}
}

func davRedirect(w http.ResponseWriter, location string) error {
	u, err := url.Parse(location)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return davStatus(http.StatusBadGateway)
	}
	w.Header().Set("Location", wopan.NormalizeRedirectURL(location))
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusTemporaryRedirect)
	return nil
}

func (d *davRequest) writeResource(w http.ResponseWriter, entry *davEntry) error {
	method := d.request.Method
	if d.name == "/" {
		if method == http.MethodDelete {
			return davStatus(http.StatusForbidden)
		}
		return davStatus(http.StatusMethodNotAllowed)
	}
	if err := d.requireLocks(d.name, method == http.MethodDelete, entry == nil || method == http.MethodDelete); err != nil {
		return err
	}
	switch method {
	case http.MethodDelete:
		if entry == nil {
			return davStatus(http.StatusNotFound)
		}
		if depth := d.request.Header.Get("Depth"); entry.Directory && depth != "" && depth != "infinity" {
			return davStatus(http.StatusBadRequest)
		}
		return serveDAVLibrary(w, d.request, &davCloudFS{dav: d})
	case "MKCOL":
		if entry != nil {
			return davStatus(http.StatusMethodNotAllowed)
		}
		return serveDAVLibrary(w, d.request, &davCloudFS{dav: d})
	case http.MethodPut:
		if entry != nil && entry.Directory {
			return davStatus(http.StatusMethodNotAllowed)
		}
		if d.request.Header.Get("Content-Range") != "" {
			return davStatus(http.StatusBadRequest)
		}
		parent, err := d.parent(d.name)
		if err != nil {
			return err
		}
		if err := d.put(parent.ID, entry, d.request.Body, d.request.ContentLength, d.request.Header.Get("Content-Type")); err != nil {
			return err
		}
		d.writeCreated(w, entry == nil, false, d.name)
	}
	return nil
}

func (d *davRequest) put(parent string, existing *davEntry, body io.Reader, size int64, contentType string) error {
	if size < 0 {
		// The upstream multipart protocol needs the final byte count before the first part.
		f, err := os.CreateTemp("", "ipan-dav-upload-*")
		if err != nil {
			return davStatus(http.StatusInsufficientStorage)
		}
		defer os.Remove(f.Name())
		defer f.Close()
		size, err = io.Copy(f, body)
		if err != nil {
			if errors.Is(err, os.ErrClosed) || errors.Is(err, io.ErrUnexpectedEOF) {
				return davStatus(http.StatusBadRequest)
			}
			return davStatus(http.StatusInsufficientStorage)
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		body = f
	}
	ctx := d.request.Context()
	name := path.Base(d.name)
	if existing == nil {
		_, err := d.client.UploadStream(ctx, parent, name, body, size, contentType)
		return err
	}
	// Upload under a temporary name so a broken transfer leaves the original intact.
	stage := davTemporaryName(name)
	if _, err := d.client.UploadStream(ctx, parent, stage, body, size, contentType); err != nil {
		return err
	}
	delete(d.lists, parent)
	uploaded, err := d.expectChild(parent, stage)
	if err != nil {
		return err
	}
	if err := d.cloneProperties(existing, uploaded, false); err != nil {
		d.cleanup(func(ctx context.Context) error { return d.remove(ctx, uploaded) })
		return err
	}
	if err := d.replace(existing, func() error { return d.rename(ctx, uploaded, name) }); err != nil {
		d.cleanup(func(ctx context.Context) error { return d.remove(ctx, uploaded) })
		d.removeProperties([]string{d.propertyKey(uploaded)})
		return err
	}
	d.removeProperties([]string{d.propertyKey(existing)})
	return nil
}

func davTemporaryName(name string) string {
	return ".ipan-dav-" + randomBase64URL(18) + path.Ext(name)
}

func (d *davRequest) expectChild(parent, name string) (*davEntry, error) {
	for attempt := 0; attempt < 5; attempt++ {
		if attempt != 0 {
			select {
			case <-d.request.Context().Done():
				return nil, d.request.Context().Err()
			case <-time.After(200 * time.Millisecond):
			}
			delete(d.lists, parent)
		}
		entry, err := d.child(parent, name)
		if err != nil || entry != nil {
			return entry, err
		}
	}
	return nil, wopan.Errorf(http.StatusBadGateway, "云盘未返回刚创建的条目")
}

func (d *davRequest) cleanup(fn func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(d.request.Context()), 15*time.Second)
	defer cancel()
	if err := fn(ctx); err != nil {
		log.Printf("WebDAV 云盘操作清理失败: %v", err)
	}
}

func (d *davRequest) rename(ctx context.Context, entry *davEntry, name string) error {
	typ := 1
	if entry.Directory {
		typ = 0
	}
	_, err := d.client.Rename(ctx, entry.ID, name, typ)
	return err
}

func (d *davRequest) remove(ctx context.Context, entry *davEntry) error {
	dirs, files := entry.ids()
	_, err := d.client.Delete(ctx, dirs, files)
	return err
}

func (d *davRequest) move(ctx context.Context, entry *davEntry, parent string) error {
	dirs, files := entry.ids()
	_, err := d.client.Move(ctx, parent, dirs, files)
	return err
}

func (d *davRequest) replace(existing *davEntry, promote func() error) error {
	if existing == nil {
		return promote()
	}
	ctx := d.request.Context()
	if err := d.rename(ctx, existing, davTemporaryName(existing.Name)); err != nil {
		return err
	}
	if err := promote(); err != nil {
		d.cleanup(func(ctx context.Context) error { return d.rename(ctx, existing, existing.Name) })
		return err
	}
	d.cleanup(func(ctx context.Context) error { return d.remove(ctx, existing) })
	return nil
}

func (d *davRequest) copyMove(w http.ResponseWriter, source *davEntry) error {
	if source == nil {
		return davStatus(http.StatusNotFound)
	}
	dest, err := d.reference(d.request.Header.Get("Destination"))
	if err != nil {
		return err
	}
	move := d.request.Method == "MOVE"
	if d.name == "/" || dest == "/" || davWithin(d.name, dest) || move && davWithin(dest, d.name) {
		return davStatus(http.StatusForbidden)
	}
	depth := d.request.Header.Get("Depth")
	if depth != "" && depth != "infinity" && (move || depth != "0") {
		return davStatus(http.StatusBadRequest)
	}
	overwrite := d.request.Header.Get("Overwrite")
	if overwrite != "" && overwrite != "T" && overwrite != "F" {
		return davStatus(http.StatusBadRequest)
	}
	parent, err := d.parent(dest)
	if err != nil {
		return err
	}
	name := path.Base(dest)
	existing, err := d.child(parent.ID, name)
	if err != nil {
		return err
	}
	if existing != nil && overwrite == "F" {
		return davStatus(http.StatusPreconditionFailed)
	}
	if err := d.requireLocks(dest, existing != nil, true); err != nil {
		return err
	}
	if move {
		if err := d.requireLocks(d.name, true, true); err != nil {
			return err
		}
	}
	keys, err := d.propertyKeys(existing)
	if err != nil {
		return err
	}
	if move {
		err = d.replace(existing, func() error { return d.moveNamed(source, parent.ID, name) })
	} else {
		stageParent := parent.ID
		if davWithin(dest, d.name) {
			// Stage outside the source tree so the upstream copy cannot copy itself.
			stageParent = source.Parent
		}
		err = d.copyResource(source, parent.ID, stageParent, name, existing, depth != "0")
	}
	if err != nil {
		return err
	}
	if move {
		d.removeLocks(d.name, false)
	}
	if existing != nil {
		d.removeLocks(dest, move)
		if move {
			for token, lock := range d.server.davLocks {
				if lock.Root == dest {
					lock.Directory = source.Directory
					d.server.davLocks[token] = lock
				}
			}
		}
		d.removeProperties(keys)
	}
	d.writeCreated(w, existing == nil, source.Directory, dest)
	return nil
}

func (d *davRequest) writeCreated(w http.ResponseWriter, created, directory bool, name string) {
	if created {
		w.Header().Set("Location", davHref(name, directory))
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (d *davRequest) moveNamed(source *davEntry, parent, name string) error {
	ctx := d.request.Context()
	switch {
	case source.Parent == parent:
		return d.rename(ctx, source, name)
	case source.Name == name:
		return d.move(ctx, source, parent)
	}
	temporary := davTemporaryName(source.Name)
	if err := d.rename(ctx, source, temporary); err != nil {
		return err
	}
	if err := d.move(ctx, source, parent); err != nil {
		d.cleanup(func(ctx context.Context) error { return d.rename(ctx, source, source.Name) })
		return err
	}
	if err := d.rename(ctx, source, name); err != nil {
		d.cleanup(func(ctx context.Context) error {
			if err := d.move(ctx, source, source.Parent); err != nil {
				return err
			}
			return d.rename(ctx, source, source.Name)
		})
		return err
	}
	return nil
}

func (d *davRequest) copyResource(source *davEntry, parent, stageParent, name string, existing *davEntry, recursive bool) error {
	ctx := d.request.Context()
	// Native COPY keeps the original name. Isolate it from collisions in the destination.
	stageName := davTemporaryName("")
	if _, err := d.client.Mkdir(ctx, stageParent, stageName); err != nil {
		return err
	}
	delete(d.lists, stageParent)
	stage, err := d.expectChild(stageParent, stageName)
	if err != nil {
		return err
	}
	defer d.cleanup(func(ctx context.Context) error { return d.remove(ctx, stage) })
	if source.Directory && !recursive {
		_, err = d.client.Mkdir(ctx, stage.ID, source.Name)
	} else {
		dirs, files := source.ids()
		_, err = d.client.Copy(ctx, stage.ID, dirs, files)
	}
	if err != nil {
		return err
	}
	copied, err := d.expectChild(stage.ID, source.Name)
	if err != nil {
		return err
	}
	if err := d.cloneProperties(source, copied, recursive); err != nil {
		return err
	}
	if err := d.replace(existing, func() error {
		if source.Name != name {
			if err := d.rename(ctx, copied, name); err != nil {
				return err
			}
		}
		return d.move(ctx, copied, parent)
	}); err != nil {
		keys, _ := d.propertyKeys(copied)
		d.removeProperties(keys)
		return err
	}
	return nil
}
