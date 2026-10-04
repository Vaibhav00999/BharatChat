package cache

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/redis/go-redis/v9"
)

func NewClient(ctx context.Context, addr, password string, useTLS bool, serverName string) (*redis.Client, error) {
	options := &redis.Options{Addr: addr, Password: password, DB: 0}
	if useTLS {
		options.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: serverName,
		}
	}
	c := redis.NewClient(options)
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("cache: redis ping failed: %w", err)
	}
	return c, nil
}
