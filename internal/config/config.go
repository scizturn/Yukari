package config

import (
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
)

type Config struct {
	Mode              string
	Timezone          string
	SQLDir            string
	DatabaseDSN       string
	VoucherConfigPath string
	VoucherCodeSecret string
	ActionURL         string
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	QueueName                  string
	AnniversaryEnabled         bool
	AnniversaryQueueName       string
	AnniversaryVoucherConfigPath string
}

func Load() Config {
	databaseDSN := oldDatabaseDSN()
	return Config{
		Mode:                       env("YUKARI_MODE", "all"),
		Timezone:                   env("YUKARI_TIMEZONE", "Asia/Jakarta"),
		SQLDir:                     env("YUKARI_SQL_DIR", "data/sql"),
		DatabaseDSN:                databaseDSN,
		VoucherConfigPath:          "data/vouchers/birthday.json",
		AnniversaryVoucherConfigPath: env("YUKARI_ANNIVERSARY_VOUCHER_CONFIG", "data/vouchers/anniversary.json"),
		VoucherCodeSecret:          os.Getenv("VOUCHER_CODE_SECRET"),
		ActionURL:                  env("YUKARI_ACTION_URL", "https://kyou.id/user/my-voucher"),
		RedisAddr:                  env("REDIS_ADDR", "redis:6379"),
		RedisPassword:              os.Getenv("REDIS_PASSWORD"),
		RedisDB:                    envInt("REDIS_DB", 0),
		QueueName:                  env("YUKARI_QUEUE_NAME", "birthday_email_jobs"),
		AnniversaryEnabled:         envBool("YUKARI_ANNIVERSARY_ENABLED", false),
		AnniversaryQueueName:       env("YUKARI_ANNIVERSARY_QUEUE_NAME", "anniversary_email_jobs"),
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

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}
