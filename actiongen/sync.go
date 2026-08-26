package actiongen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// Sync writes a's YAML to path when it differs from what is already there.
func Sync(path string, a Action) (changed bool, err error) {
	rendered := a.Render()

	existing, readErr := os.ReadFile(path)
	if readErr == nil && bytes.Equal(existing, rendered) {
		return false, nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return true, fmt.Errorf("actiongen sync mkdir: %w", err)
	}

	if err := os.WriteFile(path, rendered, 0644); err != nil {
		return true, fmt.Errorf("actiongen sync write file: %w", err)
	}

	return true, nil
}
