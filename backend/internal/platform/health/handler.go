package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type Check func(context.Context) error

// Readiness returns success only while every required dependency is reachable.
// Error details are logged but never exposed to unauthenticated callers.
func Readiness(log zerolog.Logger, timeout time.Duration, checks map[string]Check) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		for name, check := range checks {
			if err := check(ctx); err != nil {
				log.Warn().Err(err).Str("dependency", name).Msg("readiness check failed")
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}
