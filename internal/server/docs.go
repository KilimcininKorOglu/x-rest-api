package server

// OpenAPI document and Swagger UI. The spec is generated from the /v1 route
// table (internal/openapi) once at startup and cached. Swagger UI is vendored and
// served from the binary (go:embed), so /docs works offline and in distroless.

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"x-rest-api/internal/httpcache"
	"x-rest-api/internal/openapi"
	"x-rest-api/internal/version"
)

//go:embed static/swagger-ui-bundle.js static/swagger-ui.css
var docsAssets embed.FS

// jsonContentType is the content type of every JSON document served here.
const jsonContentType = "application/json; charset=utf-8"

// docsHTMLFormat loads the vendored Swagger UI against /openapi.json. No CDN.
// The two %s are the content-versioned asset URLs.
const docsHTMLFormat = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>x-rest-api — API docs</title>
<link rel="stylesheet" href="%s">
</head>
<body>
<div id="swagger-ui"></div>
<script src="%s"></script>
<script>
window.onload = function () {
  window.ui = SwaggerUIBundle({ url: "/openapi.json", dom_id: "#swagger-ui" });
};
</script>
</body>
</html>`

// docsShell renders the Swagger UI page with content-versioned asset URLs, so a
// browser fetches a rebuilt bundle instead of replaying its cached copy.
func docsShell() []byte {
	return fmt.Appendf(nil, docsHTMLFormat,
		docsAssetURL("swagger-ui.css"), docsAssetURL("swagger-ui-bundle.js"))
}

// docsAssetURL returns one vendored asset path carrying a content hash.
func docsAssetURL(name string) string {
	return httpcache.AssetURL(docsAssets, "static/"+name, "/docs-static/"+name)
}

// buildSpec generates and caches the OpenAPI document from the route table.
func (s *Server) buildSpec(routes []apiRoute) {
	meta := make([]openapi.Route, len(routes))
	for i, rt := range routes {
		meta[i] = rt.Route
	}
	doc := openapi.Build("x-rest-api", version.Version, "/", meta)
	b, err := json.Marshal(doc)
	if err != nil {
		log.Printf("openapi: marshal spec: %v", err)
		b = fmt.Appendf(nil, `{"openapi":"3.0.3","info":{"title":"x-rest-api","version":%q},"paths":{}}`, version.Version)
	}
	s.spec = b
}

// docsStatic serves the vendored Swagger UI assets under /docs-static/ with a
// content ETag, because an embedded file carries no usable ModTime.
func docsStatic() http.Handler {
	sub, err := fs.Sub(docsAssets, "static")
	if err != nil {
		log.Printf("openapi: docs static fs: %v", err)
		return http.NotFoundHandler()
	}
	return httpcache.FS(sub, httpcache.Asset)
}
