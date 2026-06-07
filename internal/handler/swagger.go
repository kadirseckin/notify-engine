package handler

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func RegisterSwaggerRoutes(r *gin.Engine) {
	// Serve swagger YAML spec
	r.GET("/swagger/spec", func(c *gin.Context) {
		// Try multiple paths (Docker vs local)
		paths := []string{"docs/swagger.yaml", "/app/docs/swagger.yaml"}
		for _, path := range paths {
			if data, err := os.ReadFile(path); err == nil {
				c.Data(http.StatusOK, "application/x-yaml", data)
				return
			}
		}
		c.String(http.StatusNotFound, "swagger.yaml not found")
	})

	// Serve Swagger UI (CDN-based, no extra dependencies)
	r.GET("/swagger", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerHTML))
	})
}

const swaggerHTML = `<!DOCTYPE html>
<html>
<head>
    <title>Notify Engine API — Swagger</title>
    <meta charset="utf-8"/>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
    <style>
        body { margin: 0; }
        .topbar { display: none; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
        SwaggerUIBundle({
            url: '/swagger/spec',
            dom_id: '#swagger-ui',
            presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
            layout: "BaseLayout"
        });
    </script>
</body>
</html>`
