package redisc

import (
	"context"
	"ditto/models"
	"ditto/shared/common"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	sctx "github.com/phathdt/service-context"
	"github.com/redis/go-redis/v9"
)

type RedisComp interface {
	Publish(topic string, event models.Event) error
}

type redisComp struct {
	client *redis.Client
	url    string
}

func New(key string, dsn string) sctx.Component {
	return &redisComp{
		url: "redis://localhost:6379",
	}
}

func (r *redisComp) ID() string {
	return common.KeyCompRedis
}

func (r *redisComp) InitFlags() {
	flag.StringVar(
		&r.url,
		"redis-url",
		"redis://localhost:6379",
		"Redis URL (e.g. redis://redis-db:6379)",
	)
}

func (r *redisComp) Activate(sc sctx.ServiceContext) error {
	opts, err := redis.ParseURL(r.url)
	if err != nil {
		return fmt.Errorf("parse redis url failed: %w", err)
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis connection failed: %w", err)
	}

	r.client = client
	return nil
}

func (r *redisComp) Stop() error {
	return r.client.Close()
}

func (r *redisComp) Publish(topic string, event models.Event) error {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event failed: %w", err)
	}

	cmd := r.client.LPush(context.Background(), topic, eventJSON)
	return cmd.Err()
}
