package actiongen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// Sync escribe el YAML de a en path si difiere de lo que ya hay.
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
