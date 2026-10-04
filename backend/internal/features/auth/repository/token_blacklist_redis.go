package repository

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"time"
)

type TokenBlacklistRedis struct{ rdb *redis.Client }

func NewTokenBlacklistRedis(r *redis.Client) *TokenBlacklistRedis {
	return &TokenBlacklistRedis{rdb: r}
}
func (b *TokenBlacklistRedis) Revoke(ctx context.Context, jti string, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	return b.rdb.Set(ctx, "blacklist:jti:"+jti, "1", ttl).Err()
}
func (b *TokenBlacklistRedis) IsRevoked(ctx context.Context, jti string) (bool, error) {
	if jti == "" {
		return true, fmt.Errorf("token blacklist: empty jti")
	}
	n, err := b.rdb.Exists(ctx, "blacklist:jti:"+jti).Result()
	return n > 0, err
}
