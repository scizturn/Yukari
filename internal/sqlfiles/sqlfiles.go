package sqlfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Loader struct {
	dir string
}

func NewLoader(dir string) Loader {
	return Loader{dir: dir}
}

func (l Loader) Read(name string) (string, error) {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return "", fmt.Errorf("unsafe sql file name %q", name)
	}
	content, err := os.ReadFile(filepath.Join(l.dir, name+".sql"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}
