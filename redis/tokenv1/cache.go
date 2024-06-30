package tokenv1

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/braumsmilk/go-auth/pg/userv1"
	"github.com/braumsmilk/go-log"
	"github.com/braumsmilk/go-token/redis"
	"github.com/braumsmilk/go-token/token"
	"github.com/go-redis/cache/v8"
	"go.uber.org/zap"
)

var _ TokenCache = &RedisTokenCache{}

var ErrNoTokenFound error = errors.New("could not find token for userid")
var l *zap.Logger = log.NewLogger("redis")

type TokenCache interface {
	GetNewAuthToken(ctx context.Context, userid userv1.Id) (string, error)
	IsAuthenticated(ctx context.Context, id userv1.Id, token string) (bool, error)
}

func NewRedisTokenCache(cfg TokenConfig) *RedisTokenCache {
	r := redis.NewRedis()
	return &RedisTokenCache{
		cfg: cfg,
		r: cache.New(&cache.Options{
			Redis: r,
		}),
	}

}

type RedisTokenCache struct {
	cfg TokenConfig
	r   *cache.Cache
}

func (c *RedisTokenCache) GetNewAuthToken(ctx context.Context, id userv1.Id) (string, error) {
	var token string

	key := fmt.Sprintf("%d", id)

	err := c.r.Get(ctx, key, &token)
	if err != nil {
		if err == cache.ErrCacheMiss {
			token, err = c.createToken(id)
		} else {
			return "", fmt.Errorf("failed to get token from redis: %v", err)
		}
	}

	return token, err
}

func (c *RedisTokenCache) createToken(id userv1.Id) (string, error) {
	tkn, err := token.NewJwtToken(fmt.Sprintf("%d", id), c.cfg.Audience, c.cfg.GenerateId(), c.cfg.Issuer)
	if err != nil {
		return "", fmt.Errorf("failed to create new token: %v", err)
	}

	ttl := time.Hour * time.Duration(c.cfg.ValidityHours)
	err = c.r.Set(&cache.Item{
		Key:   fmt.Sprintf("%d", id),
		Value: tkn,
		TTL:   ttl,
	})
	if err != nil {
		return "", fmt.Errorf("failed to set userid => token mapping: %v", err)
	}

	return tkn, nil
}

func (c *RedisTokenCache) IsAuthenticated(ctx context.Context, id userv1.Id, token string) (bool, error) {
	authedToken, err := c.GetCurrentToken(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNoTokenFound) {
			return false, nil
		}

		return false, fmt.Errorf("failed to get auth token: %v", err)
	}

	return authedToken == token, nil
}

func (c *RedisTokenCache) GetCurrentToken(ctx context.Context, id userv1.Id) (string, error) {
	var token string
	err := c.r.Get(ctx, fmt.Sprintf("%d", id), &token)
	if err != nil {
		if errors.Is(err, cache.ErrCacheMiss) {
			return "", ErrNoTokenFound
		}

		return "", fmt.Errorf("failed to query for user token: %v", err)
	}

	return token, nil
}
