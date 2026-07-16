package v1

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

const aggregateRequestTimeout = 15 * time.Second

// aggregateRequestContext bounds ordinary aggregate responses; SSE endpoints stream independently.
func aggregateRequestContext(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), aggregateRequestTimeout)
}
