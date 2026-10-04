package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type KeyFunc func(*gin.Context) string

var fixedWindowScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`)

func ByIP(c *gin.Context) string { return fmt.Sprintf("%s:%s", c.ClientIP(), c.FullPath()) }

func ByAuthenticatedUser(c *gin.Context) string {
	if value, exists := c.Get(string(ContextKeyUserID)); exists {
		return fmt.Sprintf("%v:%s", value, c.FullPath())
	}
	return ByIP(c)
}
func RateLimit(rdb *redis.Client, prefix string, limit int, window time.Duration, keyFn KeyFunc) gin.HandlerFunc {
	if keyFn == nil {
		keyFn = ByIP
	}
	return func(c *gin.Context) {
		key := fmt.Sprintf("ratelimit:%s:%s", prefix, keyFn(c))
		count, err := fixedWindowScript.Run(c.Request.Context(), rdb, []string{key}, window.Milliseconds()).Int64()
		if err != nil {
			c.Next()
			return
		}
		if int(count) > limit {
			ttl, _ := rdb.TTL(c.Request.Context(), key).Result()
			retry := int(ttl.Seconds())
			if retry < 1 {
				retry = 1
			}
			c.Header("Retry-After", fmt.Sprintf("%d", retry))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": "TOO_MANY_REQUESTS", "message": "too many requests, please try again later"})
			return
		}
		c.Next()
	}
}
