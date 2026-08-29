//go:build !wasm

package goflare_test

import (
	"path/filepath"
	"testing"

	"github.com/tinywasm/ghaction"
	"github.com/tinywasm/goflare"
)

func TestWorkflowsAreInSync(t *testing.T) {
	relPath := filepath.Join("..", ".github", "workflows", "release.yml")
	rel := goflare.ReleaseWorkflow()
	if changed, err := ghaction.SyncWorkflow(relPath, rel); err != nil {
		t.Fatalf("sync release workflow: %v", err)
	} else if changed {
		t.Fatalf("%s was out of sync and has just been regenerated. Review the diff and commit it.", relPath)
	}
}
