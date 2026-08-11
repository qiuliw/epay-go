// internal/middleware/logger.go
package middleware

import (
	"bytes"
	"io"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxLoggedBodyBytes = 4096

// Logger 请求日志中间件
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		bodyLog := ""
		if shouldLogRequestBody(path) && c.Request.Body != nil {
			body, err := io.ReadAll(c.Request.Body)
			_ = c.Request.Body.Close()
			if err == nil {
				c.Request.Body = io.NopCloser(bytes.NewReader(body))
				if len(body) > maxLoggedBodyBytes {
					bodyLog = string(body[:maxLoggedBodyBytes]) + "...(truncated)"
				} else {
					bodyLog = string(body)
				}
			} else {
				c.Request.Body = io.NopCloser(bytes.NewReader(nil))
			}
		}

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		if query != "" {
			path = path + "?" + query
		}

		if bodyLog != "" {
			log.Printf("[GIN] %3d | %13v | %15s | %-7s %s | body=%s",
				status, latency, clientIP, method, path, bodyLog)
			return
		}

		log.Printf("[GIN] %3d | %13v | %15s | %-7s %s",
			status, latency, clientIP, method, path)
	}
}

func shouldLogRequestBody(path string) bool {
	switch path {
	case "/submit.php", "/mapi.php", "/api.php":
		return true
	default:
		return strings.HasPrefix(path, "/api/pay/")
	}
}
