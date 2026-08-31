package infra

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
)

type Clients struct {
	DB        *sql.DB
	Redis     *redis.Client
	Messaging *messaging.ConnectionManager
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
	if cfg.AsyncMessagingEnabled && cfg.RabbitMQURL != "" {
		cm, err := messaging.NewConnectionManager(messaging.RabbitMQConfig{
			URL:       cfg.RabbitMQURL,
			Heartbeat: 10 * time.Second,
		})
		if err != nil {
			if cfg.IsProduction() {
				return nil, fmt.Errorf("failed to create messaging connection manager: %w", err)
			}
			log.Printf("messaging connection manager creation failed, will retry later: %v", err)
		} else {
			clients.Messaging = cm
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
	if c.Messaging != nil {
		_ = c.Messaging.Close()
	}
	if c.DB != nil {
		return c.DB.Close()
	}
	return nil
}
