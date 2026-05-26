package config

import (
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestLoadBuildsDatabaseDSNFromOldDatabaseEnv(t *testing.T) {
	t.Setenv("DATABASE_DSN", "ignored:ignored@tcp(127.0.0.1:10110)/ignored")
	t.Setenv("OLD_DATABASE_HOST", "kyou-prod-central-db.example.ap-southeast-1.rds.amazonaws.com")
	t.Setenv("OLD_DATABASE_PORT", "3306")
	t.Setenv("OLD_DATABASE_NAME", "hanayo_prod")
	t.Setenv("OLD_DATABASE_USERNAME", "readonly_user")
	t.Setenv("OLD_DATABASE_PASSWORD", "secret")

	cfg := Load()
	dsn, err := mysql.ParseDSN(cfg.DatabaseDSN)
	if err != nil {
		t.Fatalf("expected valid dsn, got %v", err)
	}

	if dsn.User != "readonly_user" {
		t.Fatalf("expected username from OLD_DATABASE_USERNAME, got %q", dsn.User)
	}
	if dsn.DBName != "hanayo_prod" {
		t.Fatalf("expected database from OLD_DATABASE_NAME, got %q", dsn.DBName)
	}
	if dsn.Addr != "kyou-prod-central-db.example.ap-southeast-1.rds.amazonaws.com:3306" {
		t.Fatalf("expected host and port from OLD_DATABASE env, got %q", dsn.Addr)
	}
	if !dsn.ParseTime {
		t.Fatal("expected parseTime=true")
	}
	if !dsn.AllowNativePasswords {
		t.Fatal("expected native password auth to be enabled")
	}
	if dsn.Collation != "utf8mb4_unicode_ci" {
		t.Fatalf("expected utf8mb4 collation, got %q", dsn.Collation)
	}
	if !strings.Contains(cfg.DatabaseDSN, "charset=utf8mb4") {
		t.Fatalf("expected charset=utf8mb4 in dsn, got %q", cfg.DatabaseDSN)
	}
}
