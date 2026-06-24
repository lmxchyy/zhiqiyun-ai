package infra

import (
	"context"
	"database/sql"

	"github.com/redis/go-redis/v9"
	_ "github.com/jackc/pgx/v5/stdlib"

	"xianzhi-ai/backend-go/internal/config"
)

type Clients struct {
	DB    *sql.DB
	Redis *redis.Client
}

func Open(ctx context.Context, cfg config.Config) (*Clients, error) {
	clients := &Clients{}
	if cfg.DatabaseURL != "" {
		db, err := sql.Open("pgx", cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
		clients.DB = db
	}
	if cfg.RedisURL != "" {
		options, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return nil, err
		}
		clients.Redis = redis.NewClient(options)
		_ = clients.Redis.Ping(ctx).Err()
	}
	return clients, nil
}

func (c *Clients) Close() error {
	if c == nil {
		return nil
	}
	if c.Redis != nil {
		_ = c.Redis.Close()
	}
	if c.DB != nil {
		return c.DB.Close()
	}
	return nil
}
