package goflare_test

import (
	"path/filepath"
	"testing"

	"github.com/tinywasm/goflare"
	"github.com/tinywasm/goflare/actiongen"
)

func TestActionYmlIsInSync(t *testing.T) {
	tag, err := goflare.LatestReleaseTag()
	if err != nil || tag == "" {
		t.Skipf("skipping TestActionYmlIsInSync: git tags not available or empty (err=%v, tag=%q)", err, tag)
	}

	actionPath := filepath.Join("..", goflare.ActionFilePath)
	a := goflare.GoflareAction(goflare.TinyGoVersion, tag)

	changed, err := actiongen.Sync(actionPath, a)
	if err != nil {
		t.Fatal(err)
	}

	if changed {
		t.Fatalf("%s estaba desincronizado y se acaba de regenerar. "+
			"Revisa el diff y commitealo; despues este test queda verde.",
			goflare.ActionFilePath)
	}
}
