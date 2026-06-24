package gateway

import (
	"context"
	"fmt"

	"github.com/h2o/gps-platform/internal/storage/redis"
	"go.uber.org/zap"
)

// CachedDeviceResolver resolves IMEI → tenantID/deviceID using Redis cache.
type CachedDeviceResolver struct {
	cache *redis.Cache
	log   *zap.Logger
}

func NewCachedDeviceResolver(cache *redis.Cache, log *zap.Logger) *CachedDeviceResolver {
	return &CachedDeviceResolver{cache: cache, log: log}
}

func (r *CachedDeviceResolver) ResolveByIMEI(ctx context.Context, imei string) (string, string, error) {
	info, err := r.cache.GetDeviceInfo(ctx, imei)
	if err != nil {
		r.log.Warn("cache lookup error", zap.Error(err), zap.String("imei", imei))
	}

	if info != nil {
		if !info.IsActive {
			return "", "", fmt.Errorf("device %s is inactive", imei)
		}
		return info.TenantID, info.DeviceID, nil
	}

	// Cache miss: in production, fall back to PostgreSQL
	return "", "", fmt.Errorf("device not found: %s", imei)
}
