package handler

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed frontend/dist
var frontendFS embed.FS

func serveFrontend(router *gin.Engine) {
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		return
	}

	if _, err := fs.Stat(distFS, "index.html"); err != nil {
		return
	}
	indexHTML, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		return
	}

	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/api" || strings.HasPrefix(path, "/api/") ||
			path == "/ws" || strings.HasPrefix(path, "/ws/") {
			c.Status(http.StatusNotFound)
			return
		}

		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}

		filePath := strings.TrimPrefix(path, "/")
		if filePath == "" {
			filePath = "index.html"
		}

		if filePath == "index.html" {
			c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
			return
		}

		if _, err := fs.Stat(distFS, filePath); err == nil {
			c.FileFromFS(filePath, http.FS(distFS))
			return
		}

		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})
}
