package infra

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"

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
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, err
		}
		clients.DB = db
	}
	if cfg.RedisURL != "" {
		options, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			_ = clients.Close()
			return nil, err
		}
		clients.Redis = redis.NewClient(options)
		if err := clients.Redis.Ping(ctx).Err(); err != nil {
			_ = clients.Close()
			return nil, err
		}
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
