package goflare

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferMode(t *testing.T) {
	tests := []struct {
		name      string
		mainGo    string
		publicDir string
		want      Mode
		wantErr   string
	}{
		{
			name: "PagesFunctions",
			mainGo: `package main
import _ "github.com/tinywasm/goflare/edge"
func main() {}`,
			want: ModePagesFunctions,
		},
		{
			name: "Workers",
			mainGo: `package main
import _ "github.com/tinywasm/goflare/workers"
func main() {}`,
			want: ModeWorkers,
		},
		{
			name: "PagesStatic",
			publicDir: "public",
			want: ModePagesStatic,
		},
		{
			name: "NoKnownImport",
			mainGo: `package main
import "fmt"
func main() { fmt.Println("hello") }`,
			wantErr: ErrNoKnownImport,
		},
		{
			name: "Ambiguous",
			mainGo: `package main
import (
	_ "github.com/tinywasm/goflare/edge"
	_ "github.com/tinywasm/goflare/workers"
)
func main() {}`,
			wantErr: ErrAmbiguous,
		},
		{
			name: "CommentedImport",
			mainGo: `package main
// import "github.com/tinywasm/goflare/edge"
func main() {}`,
			wantErr: ErrNoKnownImport,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			entry := filepath.Join(tmp, "edge")
			public := ""

			if tt.mainGo != "" {
				os.MkdirAll(entry, 0755)
				os.WriteFile(filepath.Join(entry, "main.go"), []byte(tt.mainGo), 0644)
			}
			if tt.publicDir != "" {
				public = filepath.Join(tmp, tt.publicDir)
				os.MkdirAll(public, 0755)
			}

			got, err := inferMode(entry, public)
			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("expected error %q, got nil", tt.wantErr)
				} else if err.Error() != tt.wantErr {
					t.Errorf("expected error %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
