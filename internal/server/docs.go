package server

// OpenAPI document and Swagger UI. The spec is generated from the /v1 route
// table (internal/openapi) once at startup and cached. Swagger UI is vendored and
// served from the binary (go:embed), so /docs works offline and in distroless.

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"

	"x-rest-api/internal/openapi"
)

//go:embed static/swagger-ui-bundle.js static/swagger-ui.css
var docsAssets embed.FS

// docsHTML loads the vendored Swagger UI against /openapi.json. No CDN.
const docsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>x-rest-api — API docs</title>
<link rel="stylesheet" href="/docs-static/swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="/docs-static/swagger-ui-bundle.js"></script>
<script>
window.onload = function () {
  window.ui = SwaggerUIBundle({ url: "/openapi.json", dom_id: "#swagger-ui" });
};
</script>
</body>
</html>`

// buildSpec generates and caches the OpenAPI document from the route table.
func (s *Server) buildSpec(routes []apiRoute) {
	meta := make([]openapi.Route, len(routes))
	for i, rt := range routes {
		meta[i] = rt.Route
	}
	doc := openapi.Build("x-rest-api", "1.0.0", "/", meta)
	b, err := json.Marshal(doc)
	if err != nil {
		log.Printf("openapi: marshal spec: %v", err)
		b = []byte(`{"openapi":"3.0.3","info":{"title":"x-rest-api","version":"1.0.0"},"paths":{}}`)
	}
	s.spec = b
}

// openapiJSON serves the cached spec (no auth).
func (s *Server) openapiJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	_, _ = w.Write(s.spec)
}

// docsUI serves the Swagger UI shell (no auth).
func (s *Server) docsUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("content-type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(docsHTML))
}

// docsStatic serves the vendored Swagger UI assets under /docs-static/.
func docsStatic() http.Handler {
	sub, err := fs.Sub(docsAssets, "static")
	if err != nil {
		log.Printf("openapi: docs static fs: %v", err)
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}
