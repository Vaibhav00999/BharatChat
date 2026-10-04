package repository

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"time"
)

type OTPRateLimiterRedis struct {
	rdb    *redis.Client
	limit  int
	window time.Duration
}

var otpFixedWindowScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`)

func NewOTPRateLimiterRedis(r *redis.Client, l int, w time.Duration) *OTPRateLimiterRedis {
	return &OTPRateLimiterRedis{rdb: r, limit: l, window: w}
}
func (l *OTPRateLimiterRedis) Allow(ctx context.Context, phone string) (bool, error) {
	key := "otp_rate:" + phone
	count, err := otpFixedWindowScript.Run(ctx, l.rdb, []string{key}, l.window.Milliseconds()).Int64()
	if err != nil {
		return true, fmt.Errorf("otp limiter: %w", err)
	}
	return int(count) <= l.limit, nil
}
