package authz

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// CachedLoader wraps a Loader with a Valkey cache keyed by tenant and user, per
// docs/architecture/code-structure.md: "AuthZ ... อ่านสิทธิ์จาก cache ใน Valkey (60 วินาที) และเมื่อ
// cache miss ให้โหลดผ่าน db.WithTenantTx แบบอ่านอย่างเดียวของตัวเอง".
type CachedLoader struct {
	rdb  *redis.Client
	load Loader
	ttl  time.Duration
}

func NewCachedLoader(rdb *redis.Client, load Loader) *CachedLoader {
	return &CachedLoader{rdb: rdb, load: load, ttl: 60 * time.Second}
}

func (c *CachedLoader) key(tenantID, userID string) string {
	return fmt.Sprintf("authz:grants:%s:%s", tenantID, userID)
}

// Load returns the cached Grants for tenantID/userID, loading and caching them on a miss.
func (c *CachedLoader) Load(ctx context.Context, tenantID, userID string) (Grants, error) {
	key := c.key(tenantID, userID)

	if raw, err := c.rdb.Get(ctx, key).Bytes(); err == nil {
		var g Grants
		if json.Unmarshal(raw, &g) == nil {
			return g, nil
		}
	}

	g, err := c.load(ctx, tenantID, userID)
	if err != nil {
		return Grants{}, err
	}

	if raw, err := json.Marshal(g); err == nil {
		_ = c.rdb.Set(ctx, key, raw, c.ttl).Err()
	}
	return g, nil
}

// Invalidate drops the cached grants for tenantID/userID — call after a role or role-assignment
// change so the new grants take effect on the next request instead of waiting out the TTL.
func (c *CachedLoader) Invalidate(ctx context.Context, tenantID, userID string) error {
	return c.rdb.Del(ctx, c.key(tenantID, userID)).Err()
}
