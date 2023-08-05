package demo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis/v8"
	"golang.org/x/exp/slog"
)

var ctx = context.Background()

type RedisFeatureCache struct {
	client *redis.Client
	prefix string
}

func (c *RedisFeatureCache) Initialize() {}

func (c *RedisFeatureCache) Clear() {
	err := c.client.FlushDB(ctx).Err()
	if err != nil {
		slog.Error("failed clearing cache")
	}
}

func (c *RedisFeatureCache) Get(key RepositoryKey) *CacheEntry {
	val, err := c.client.Get(ctx, c.prefix+string(key)).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		slog.Error("failed getting cache data")
	}
	var entry CacheEntry
	err = json.Unmarshal([]byte(val), &entry)
	if err != nil {
		slog.Error("failed decoding cache data")
		return nil
	}
	return &entry
}

func (c *RedisFeatureCache) Set(key RepositoryKey, entry *CacheEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		slog.Error("failed encoding cache data")
	}
	expiry := entry.StaleAt.Sub(time.Now())
	if expiry < 0 {
		c.client.Del(ctx, c.prefix+string(key)).Err()
		return
	}
	err = c.client.Set(ctx, c.prefix+string(key), string(data), expiry).Err()
	if err != nil {
		slog.Error("failed setting cache data")
	}
}

func NewRedisFeatureCache(prefix string, options *redis.Options) *RedisFeatureCache {
	client := redis.NewClient(options)
	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil
	}
	return &RedisFeatureCache{client, prefix}
}

var options *redis.Options = &redis.Options{
	Addr: "localhost:6379",
}

growthbook.ConfigureCache(NewRedisFeatureCache("gb:", options))
