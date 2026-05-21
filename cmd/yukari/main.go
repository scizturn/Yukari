package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/reader"
	"github.com/kyou-id/yukari/internal/repository"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatalf("load timezone: %v", err)
	}
	now := time.Now().In(location)

	store, err := buildStore(cfg, now)
	if err != nil {
		log.Fatalf("build store: %v", err)
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()

	count, err := reader.New(store, redisQueue).Run(ctx, now)
	if err != nil {
		log.Fatalf("reader failed: %v", err)
	}
	log.Printf("yukari enqueued %d birthday email job(s)", count)
}

func buildStore(cfg config.Config, now time.Time) (reader.Store, error) {
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Print("DATABASE_DSN is empty; using stub repository")
		return repository.NewStubStore(now), nil
	}
	return repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
}
