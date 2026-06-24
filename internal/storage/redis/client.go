package redis

import (
	"context"
	"fmt"

	"github.com/h2o/gps-platform/internal/config"
	goredis "github.com/redis/go-redis/v9"
)

// NewClient creates a Redis universal client (supports single, sentinel, cluster)
func NewClient(cfg *config.RedisConfig) (goredis.UniversalClient, error) {
	var client goredis.UniversalClient

	if len(cfg.Addrs) == 1 {
		// Single node Redis
		opt, err := goredis.ParseURL(fmt.Sprintf("redis://:%s@%s/%d",
			cfg.Password, cfg.Addrs[0], cfg.DB))
		if err != nil {
			return nil, fmt.Errorf("parse redis URL: %w", err)
		}
		opt.PoolSize = cfg.PoolSize
		client = goredis.NewClient(opt)
	} else {
		// Redis Cluster
		client = goredis.NewClusterClient(&goredis.ClusterOptions{
			Addrs:    cfg.Addrs,
			Password: cfg.Password,
			PoolSize: cfg.PoolSize,
		})
	}

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
