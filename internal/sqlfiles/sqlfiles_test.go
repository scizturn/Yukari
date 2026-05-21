package sqlfiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoaderReadsNamedSQLFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "birthday_users.sql"), []byte("SELECT 1;"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := NewLoader(dir).Read("birthday_users")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "SELECT 1;" {
		t.Fatalf("expected SQL text, got %q", got)
	}
}

func TestLoaderRejectsUnsafeNames(t *testing.T) {
	_, err := NewLoader(t.TempDir()).Read("../secret")
	if err == nil {
		t.Fatal("expected unsafe name error")
	}
}
