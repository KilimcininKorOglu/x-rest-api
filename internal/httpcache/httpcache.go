// Package httpcache holds the conditional-request helpers the API and the admin
// panel share: a content ETag, an If-None-Match comparison, and handlers that
// answer 304 Not Modified. Embedded files report a zero ModTime, so
// http.FileServer cannot build a validator for them; these handlers hash the
// content once at startup and validate on the ETag instead.
package httpcache

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// Cache-Control values this project uses. Pick NoStore for anything behind auth
// or anything computed per request.
const (
	// NoStore forbids any cache from keeping the response.
	NoStore = "no-store"
	// Asset suits an embedded stylesheet, script or vendored bundle.
	Asset = "public, max-age=3600"
	// ShortLived suits a document that is stable per build, such as an OpenAPI
	// spec or a static HTML shell.
	ShortLived = "public, max-age=300"
)

// ETag returns the quoted SHA-256 of data. RFC 7232 requires the quotes.
func ETag(data []byte) string {
	return fmt.Sprintf(`"%x"`, sha256.Sum256(data))
}

// Match reports whether an If-None-Match header selects etag. It reads the
// header as a comma-separated list, accepts `*`, and compares weakly by
// ignoring a `W/` prefix, per RFC 7232 section 2.3.2.
func Match(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" {
		return false
	}
	if strings.TrimSpace(ifNoneMatch) == "*" {
		return true
	}
	want := strings.TrimPrefix(etag, "W/")
	for tag := range strings.SplitSeq(ifNoneMatch, ",") {
		if strings.TrimPrefix(strings.TrimSpace(tag), "W/") == want {
			return true
		}
	}
	return false
}

// Version returns a short content hash for one file in fsys. Use it in an asset
// URL, because a browser keeps a cached asset until its URL changes, so a
// response header alone never delivers a rebuilt stylesheet or script.
func Version(fsys fs.FS, name string) string {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))[:8]
}

// AssetURL returns urlPath with the content hash of fsPath as a `v` query, so a
// browser fetches the rebuilt file instead of replaying its cached copy. It
// returns urlPath unchanged when the file cannot be read.
func AssetURL(fsys fs.FS, fsPath, urlPath string) string {
	v := Version(fsys, fsPath)
	if v == "" {
		return urlPath
	}
	return urlPath + "?v=" + v
}

// NoStoreMiddleware marks every response of next uncacheable.
func NoStoreMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", NoStore)
		next.ServeHTTP(w, r)
	})
}

// Bytes returns a handler that serves a fixed body with an ETag, and answers a
// matching If-None-Match with 304. It hashes the body once, at build time.
func Bytes(data []byte, contentType, cacheControl string) http.HandlerFunc {
	etag := ETag(data)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", cacheControl)
		w.Header().Set("ETag", etag)
		if Match(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(data)
	}
}

// FS serves the files of fsys with a content ETag. It hashes every file once at
// startup, so a request costs no hashing, and it keeps only the hashes in
// memory rather than a second copy of the bytes.
func FS(fsys fs.FS, cacheControl string) http.Handler {
	h := &fsHandler{fsys: fsys, cacheControl: cacheControl, etags: map[string]string{}}
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // a walk error skips one file, it never fails startup
		}
		if data, readErr := fs.ReadFile(fsys, p); readErr == nil {
			h.etags[p] = ETag(data)
		}
		return nil
	})
	return h
}

// fsHandler serves one embedded file system with precomputed ETags.
type fsHandler struct {
	fsys         fs.FS
	cacheControl string
	etags        map[string]string
}

func (h *fsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	etag, ok := h.etags[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	content, err := h.readSeeker(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if c, isCloser := content.(io.Closer); isCloser {
		defer func() { _ = c.Close() }()
	}
	w.Header().Set("Cache-Control", h.cacheControl)
	w.Header().Set("ETag", etag)
	// A zero modTime makes ServeContent omit Last-Modified and validate on the
	// ETag alone. An embedded file has no meaningful mtime, and time.Now() would
	// claim the content changed on every restart.
	http.ServeContent(w, r, name, time.Time{}, content)
}

// readSeeker opens name for ServeContent. An embedded file is already seekable;
// anything else is read into memory so ServeContent still gets a ReadSeeker.
func (h *fsHandler) readSeeker(name string) (io.ReadSeeker, error) {
	f, err := h.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	if rs, ok := f.(io.ReadSeeker); ok {
		return rs, nil
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}
