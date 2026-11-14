package middlewares

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MaxBodySize limits request bodies to n bytes.
func MaxBodySize(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}
