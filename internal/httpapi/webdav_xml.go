package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"unicode/utf16"

	"golang.org/x/net/webdav"
)

func davName(local string) xml.Name { return xml.Name{Space: "DAV:", Local: local} }

type davProtocolError struct {
	status             int
	condition, href    string
	member, dependency string
}

func (e *davProtocolError) Error() string { return http.StatusText(e.status) }

func (e *davProtocolError) write(w http.ResponseWriter) {
	value := ""
	if e.href != "" {
		value = "<D:href>" + davEscape(e.href) + "</D:href>"
	}
	condition := "<D:" + e.condition + ">" + value + "</D:" + e.condition + ">"
	if e.member == "" {
		davXML(w, e.status, `<D:error xmlns:D="DAV:">`+condition+"</D:error>")
		return
	}
	body := fmt.Sprintf(`<D:multistatus xmlns:D="DAV:"><D:response><D:href>%s</D:href><D:status>HTTP/1.1 %d %s</D:status><D:error>%s</D:error></D:response>`,
		davEscape(e.member), e.status, http.StatusText(e.status), condition)
	if e.dependency != "" {
		body += "<D:response><D:href>" + davEscape(e.dependency) + "</D:href><D:status>HTTP/1.1 424 Failed Dependency</D:status></D:response>"
	}
	davXML(w, http.StatusMultiStatus, body+"</D:multistatus>")
}

func davXML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, xml.Header+body)
}

func davEscape(text string) string {
	var buf strings.Builder
	_ = xml.EscapeText(&buf, []byte(text))
	return buf.String()
}

// davXMLNode retains the attributes and mixed content of dead properties.
// Copying inner XML alone would lose namespace declarations on ancestor elements.
type davXMLNode struct {
	name    xml.Name
	attrs   []xml.Attr
	content []any
	parent  *davXMLNode
}

func readDAVXML(w http.ResponseWriter, r *http.Request) (*davXMLNode, error) {
	const maxBody = 1 << 20
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return nil, davStatus(http.StatusRequestEntityTooLarge)
		}
		return nil, davStatus(http.StatusBadRequest)
	}
	data, utf16Input, err := davDecodeXML(data)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	if utf16Input {
		dec.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
			switch strings.ToLower(label) {
			case "utf-16", "utf-16le", "utf-16be":
				return input, nil // already converted above
			default:
				return nil, davStatus(http.StatusBadRequest)
			}
		}
	}
	var root *davXMLNode
	var stack []*davXMLNode
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, davStatus(http.StatusBadRequest)
		}
		switch t := token.(type) {
		case xml.StartElement:
			if len(stack) >= 64 {
				return nil, davStatus(http.StatusBadRequest)
			}
			node := &davXMLNode{name: t.Name, attrs: t.Attr}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				node.parent = parent
				parent.content = append(parent.content, node)
			} else if root == nil {
				root = node
			} else {
				return nil, davStatus(http.StatusBadRequest)
			}
			if node.qualified(node.name, false).Space != "" {
				return nil, davStatus(http.StatusBadRequest)
			}
			seen := make(map[xml.Name]bool)
			for _, attr := range node.attrs {
				if seen[attr.Name] || !davNamespaceDecl(attr.Name) && node.qualified(attr.Name, true).Space != "" {
					return nil, davStatus(http.StatusBadRequest)
				}
				seen[attr.Name] = true
			}
			stack = append(stack, node)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				node := stack[len(stack)-1]
				node.content = append(node.content, t.Copy())
			} else if len(bytes.TrimSpace(t)) != 0 {
				return nil, davStatus(http.StatusBadRequest)
			}
		case xml.Directive:
			return nil, davStatus(http.StatusBadRequest)
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, davStatus(http.StatusBadRequest)
	}
	return root, nil
}

func davDecodeXML(data []byte) ([]byte, bool, error) {
	var order binary.ByteOrder
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		order, data = binary.LittleEndian, data[2:]
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		order, data = binary.BigEndian, data[2:]
	case bytes.HasPrefix(data, []byte{'<', 0}):
		order = binary.LittleEndian
	case bytes.HasPrefix(data, []byte{0, '<'}):
		order = binary.BigEndian
	default:
		return bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}), false, nil
	}
	if len(data)%2 != 0 {
		return nil, true, davStatus(http.StatusBadRequest)
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = order.Uint16(data[2*i:])
	}
	return []byte(string(utf16.Decode(units))), true, nil
}

func (n *davXMLNode) children() []*davXMLNode {
	var nodes []*davXMLNode
	for _, item := range n.content {
		if node, ok := item.(*davXMLNode); ok {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func (n *davXMLNode) hasOnly(name xml.Name) bool {
	children := n.children()
	return len(children) == 1 && children[0].name == name
}

func (n *davXMLNode) serialize() string {
	var buf strings.Builder
	enc := xml.NewEncoder(&buf)
	attrs := slices.Clone(n.attrs)
	// A detached property can refer to prefix bindings and xml:lang on its ancestors.
	inherited := make(map[xml.Name]bool)
	for _, attr := range attrs {
		inherited[attr.Name] = true
	}
	lang := xml.Name{Space: "http://www.w3.org/XML/1998/namespace", Local: "lang"}
	for parent := n.parent; parent != nil; parent = parent.parent {
		for _, attr := range parent.attrs {
			if (davNamespaceDecl(attr.Name) || attr.Name == lang) && !inherited[attr.Name] {
				attrs = append(attrs, attr)
				inherited[attr.Name] = true
			}
		}
	}
	n.encode(enc, attrs)
	_ = enc.Flush()
	return buf.String()
}

func (n *davXMLNode) encode(enc *xml.Encoder, attrs []xml.Attr) {
	start := xml.StartElement{Name: n.qualified(n.name, false)}
	for _, attr := range attrs {
		if attr.Name.Space == "xmlns" {
			attr.Name = xml.Name{Local: "xmlns:" + attr.Name.Local}
		} else {
			attr.Name = n.qualified(attr.Name, true)
		}
		start.Attr = append(start.Attr, attr)
	}
	_ = enc.EncodeToken(start)
	for _, item := range n.content {
		switch v := item.(type) {
		case *davXMLNode:
			v.encode(enc, v.attrs)
		case xml.CharData:
			_ = enc.EncodeToken(v)
		}
	}
	_ = enc.EncodeToken(start.End())
}

func davNamespaceDecl(name xml.Name) bool {
	return name.Space == "xmlns" || name == (xml.Name{Local: "xmlns"})
}

func (n *davXMLNode) qualified(name xml.Name, attribute bool) xml.Name {
	if name.Space == "" {
		return name
	}
	if name.Space == "http://www.w3.org/XML/1998/namespace" {
		return xml.Name{Local: "xml:" + name.Local}
	}
	seen := make(map[xml.Name]bool)
	for node := n; node != nil; node = node.parent {
		for _, attr := range node.attrs {
			if !davNamespaceDecl(attr.Name) || seen[attr.Name] {
				continue
			}
			seen[attr.Name] = true
			if attr.Value != name.Space {
				continue
			}
			if attr.Name.Space == "xmlns" {
				return xml.Name{Local: attr.Name.Local + ":" + name.Local}
			}
			if !attribute {
				return xml.Name{Local: name.Local}
			}
		}
	}
	return name
}

type davProperties map[string]string

func davPropertyName(n xml.Name) string { return n.Space + "\x00" + n.Local }

func davPropertyXMLName(key string) xml.Name {
	ns, local, _ := strings.Cut(key, "\x00")
	return xml.Name{Space: ns, Local: local}
}

func davEmptyProperty(key string) string {
	return (&davXMLNode{name: davPropertyXMLName(key)}).serialize()
}

func (d *davRequest) propertyKey(entry *davEntry) string {
	kind := "file"
	if entry.Directory {
		kind = "collection"
	}
	cfg := d.server.cfg
	return strings.Join([]string{cfg.WopanSpaceType, cfg.WopanFamilyID, kind, entry.ID}, "\x00")
}

func (d *davRequest) properties(entry *davEntry) (davProperties, error) {
	var raw []byte
	err := d.server.store.DB().QueryRowContext(d.request.Context(),
		"SELECT properties FROM dav_properties WHERE resource = ?", d.propertyKey(entry)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return make(davProperties), nil
	}
	if err != nil {
		return nil, err
	}
	var props davProperties
	err = json.Unmarshal(raw, &props)
	return props, err
}

func (d *davRequest) propertyResources() (map[string]bool, error) {
	rows, err := d.server.store.DB().QueryContext(d.request.Context(), "SELECT resource FROM dav_properties")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make(map[string]bool)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys[key] = true
	}
	return keys, rows.Err()
}

func (d *davRequest) propertyKeys(entry *davEntry) ([]string, error) {
	if entry == nil {
		return nil, nil
	}
	if !entry.Directory {
		return []string{d.propertyKey(entry)}, nil
	}
	resources, err := d.propertyResources()
	if err != nil || len(resources) == 0 {
		return nil, err
	}
	var keys []string
	err = d.walkProperties(entry, nil, resources, func(source, _ string) {
		keys = append(keys, source)
	})
	return keys, err
}

func (d *davRequest) walkProperties(source, destination *davEntry, resources map[string]bool, found func(string, string)) error {
	key := d.propertyKey(source)
	if resources[key] {
		target := ""
		if destination != nil {
			target = d.propertyKey(destination)
		}
		found(key, target)
	}
	if !source.Directory {
		return nil
	}
	children, err := d.list(source.ID)
	if err != nil {
		return err
	}
	for i := range children {
		child := &children[i]
		var target *davEntry
		if destination != nil {
			target, err = d.expectChild(destination.ID, child.Name)
			if err != nil {
				return err
			}
		}
		if err := d.walkProperties(child, target, resources, found); err != nil {
			return err
		}
	}
	return nil
}

func (d *davRequest) cloneProperties(source, destination *davEntry, recursive bool) error {
	pairs := map[string]string{d.propertyKey(source): d.propertyKey(destination)}
	if recursive && source.Directory {
		resources, err := d.propertyResources()
		if err != nil || len(resources) == 0 {
			return err
		}
		if err := d.walkProperties(source, destination, resources, func(from, to string) {
			pairs[from] = to
		}); err != nil {
			return err
		}
	}
	ctx := d.request.Context()
	tx, err := d.server.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for from, to := range pairs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO dav_properties(resource, properties)
			SELECT ?, properties FROM dav_properties WHERE resource = ?
			ON CONFLICT(resource) DO UPDATE SET properties = excluded.properties`, to, from); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *davRequest) removeProperties(keys []string) {
	if len(keys) == 0 {
		return
	}
	d.cleanup(func(ctx context.Context) error {
		tx, err := d.server.store.DB().BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, key := range keys {
			if _, err := tx.ExecContext(ctx, "DELETE FROM dav_properties WHERE resource = ?", key); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

func (d *davRequest) propfind(w http.ResponseWriter, entry *davEntry) error {
	if entry == nil {
		return davStatus(http.StatusNotFound)
	}
	switch d.request.Header.Get("Depth") {
	case "", "infinity":
		return &davProtocolError{status: http.StatusForbidden, condition: "propfind-finite-depth"}
	case "0", "1":
	default:
		return davStatus(http.StatusBadRequest)
	}
	root, err := readDAVXML(w, d.request)
	if err != nil {
		return err
	}
	cloud := &davCloudFS{dav: d, resources: make(map[string]davResourceProperties)}
	body := ""
	if root != nil {
		required := make(map[xml.Name]bool)
		for _, child := range root.children() {
			switch child.name {
			case davName("propname"):
				cloud.namesOnly = true
			case davName("prop"), davName("include"):
				for _, property := range child.children() {
					if !required[property.name] {
						cloud.required = append(cloud.required, property.name)
						required[property.name] = true
					}
				}
			}
		}
		body = root.serialize()
	}
	return serveDAVLibrary(w, davRequestWithXML(d.request, body), cloud)
}

type davPropertyEdit struct {
	name    xml.Name
	value   string
	invalid bool
}

func (d *davRequest) proppatch(w http.ResponseWriter, entry *davEntry) error {
	if entry == nil {
		return davStatus(http.StatusNotFound)
	}
	if err := d.requireLocks(d.name, false, false); err != nil {
		return err
	}
	root, err := readDAVXML(w, d.request)
	if err != nil {
		return err
	}
	if root == nil || root.name != davName("propertyupdate") {
		return davStatus(http.StatusBadRequest)
	}
	cloud := &davCloudFS{dav: d}
	var body strings.Builder
	body.WriteString(`<D:propertyupdate xmlns:D="DAV:">`)
	for _, action := range root.children() {
		if action.name != davName("set") && action.name != davName("remove") || !action.hasOnly(davName("prop")) {
			return davStatus(http.StatusBadRequest)
		}
		fmt.Fprintf(&body, "<D:%s><D:prop>", action.name.Local)
		for _, property := range action.children()[0].children() {
			name := property.name
			edit := davPropertyEdit{name: name}
			if action.name == davName("set") {
				edit.value = property.serialize()
				edit.invalid = name == davName("displayname") && len(property.children()) != 0
			}
			cloud.edits = append(cloud.edits, edit)
			// iPan treats these two metadata fields as writable. x/net protects
			// them, so pass private names to its protection checks.
			if name == davName("displayname") || name == davName("getcontentlanguage") {
				if cloud.aliases == nil {
					cloud.aliases = make(map[xml.Name]xml.Name)
				}
				name.Space = "urn:ipan:dav:edit:" + randomBase64URL(12)
				cloud.aliases[name] = property.name
			}
			// x/net supplies the instructions and property statuses. Keep full
			// values separately because webdav.Property omits root attributes.
			body.WriteString(davEmptyProperty(davPropertyName(name)))
		}
		fmt.Fprintf(&body, "</D:prop></D:%s>", action.name.Local)
	}
	if len(cloud.edits) == 0 {
		return davStatus(http.StatusBadRequest)
	}
	body.WriteString("</D:propertyupdate>")
	return serveDAVLibrary(w, davRequestWithXML(d.request, body.String()), cloud)
}

func davRequestWithXML(r *http.Request, body string) *http.Request {
	req := r.Clone(r.Context())
	req.Body = io.NopCloser(strings.NewReader(body))
	req.ContentLength = int64(len(body))
	return req
}

func (f *davCloudFile) Patch(patches []webdav.Proppatch) ([]webdav.Propstat, error) {
	props, err := f.fs.dav.properties(f.entry)
	if err != nil {
		return nil, f.fs.record(err)
	}
	var valid, invalid []webdav.Property
	index := 0
	for _, patch := range patches {
		for _, p := range patch.Props {
			if index == len(f.fs.edits) {
				return nil, f.fs.record(davStatus(http.StatusBadRequest))
			}
			edit := f.fs.edits[index]
			index++
			name := p.XMLName
			if original, ok := f.fs.aliases[name]; ok {
				name = original
			}
			if name != edit.name {
				return nil, f.fs.record(davStatus(http.StatusBadRequest))
			}
			property := webdav.Property{XMLName: name}
			if edit.invalid {
				invalid = append(invalid, property)
				continue
			}
			valid = append(valid, property)
			key := davPropertyName(name)
			if patch.Remove {
				delete(props, key)
			} else {
				props[key] = edit.value
			}
		}
	}
	if index != len(f.fs.edits) {
		return nil, f.fs.record(davStatus(http.StatusBadRequest))
	}
	if len(invalid) > 0 {
		stats := []webdav.Propstat{{Status: http.StatusConflict, Props: invalid}}
		if len(valid) > 0 {
			stats = append(stats, webdav.Propstat{Status: http.StatusFailedDependency, Props: valid})
		}
		return stats, nil
	}
	if err := f.fs.dav.saveProperties(f.entry, props); err != nil {
		return nil, f.fs.record(err)
	}
	return []webdav.Propstat{{Status: http.StatusOK, Props: valid}}, nil
}

func (d *davRequest) saveProperties(entry *davEntry, props davProperties) error {
	ctx := d.request.Context()
	key := d.propertyKey(entry)
	db := d.server.store.DB()
	if len(props) == 0 {
		_, err := db.ExecContext(ctx, "DELETE FROM dav_properties WHERE resource = ?", key)
		return err
	}
	raw, err := json.Marshal(props)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO dav_properties(resource, properties) VALUES (?, ?)
		ON CONFLICT(resource) DO UPDATE SET properties = excluded.properties`, key, raw)
	return err
}
