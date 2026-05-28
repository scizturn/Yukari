package config

import (
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
)

type Config struct {
	Timezone          string
	SQLDir            string
	DatabaseDSN       string
	VoucherConfigPath string
	VoucherCodeSecret string
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	QueueName         string
}

func Load() Config {
	databaseDSN := oldDatabaseDSN()
	return Config{
		Timezone:          env("YUKARI_TIMEZONE", "Asia/Jakarta"),
		SQLDir:            env("YUKARI_SQL_DIR", "data/sql"),
		DatabaseDSN:       databaseDSN,
		VoucherConfigPath: "data/vouchers/birthday.json",
		VoucherCodeSecret: os.Getenv("VOUCHER_CODE_SECRET"),
		RedisAddr:         env("REDIS_ADDR", "redis:6379"),
		RedisPassword:     os.Getenv("REDIS_PASSWORD"),
		RedisDB:           envInt("REDIS_DB", 0),
		QueueName:         env("YUKARI_QUEUE_NAME", "birthday_email_jobs"),
	}
}

func oldDatabaseDSN() string {
	host := env("OLD_DATABASE_HOST", "")
	name := env("OLD_DATABASE_NAME", "")
	username := env("OLD_DATABASE_USERNAME", "")
	password := os.Getenv("OLD_DATABASE_PASSWORD")
	if host == "" || name == "" || username == "" {
		return ""
	}

	cfg := mysql.Config{
		User:                 username,
		Passwd:               password,
		Net:                  "tcp",
		Addr:                 net.JoinHostPort(host, env("OLD_DATABASE_PORT", "3306")),
		DBName:               name,
		ParseTime:            true,
		AllowNativePasswords: true,
		Params: map[string]string{
			"charset":   "utf8mb4",
			"collation": "utf8mb4_unicode_ci",
		},
	}
	return cfg.FormatDSN()
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
