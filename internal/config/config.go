package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Timezone      string
	SQLDir        string
	DatabaseDSN   string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	QueueName     string
}

func Load() Config {
	return Config{
		Timezone:      env("YUKARI_TIMEZONE", "Asia/Jakarta"),
		SQLDir:        env("YUKARI_SQL_DIR", "data/sql"),
		DatabaseDSN:   os.Getenv("DATABASE_DSN"),
		RedisAddr:     env("REDIS_ADDR", "redis:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       envInt("REDIS_DB", 0),
		QueueName:     env("YUKARI_QUEUE_NAME", "birthday_email_jobs"),
	}
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}
