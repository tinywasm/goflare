//go:build !wasm

package goflare

import (
	"fmt"
	"strings"

	"github.com/tinywasm/git"
	"github.com/tinywasm/goflare/actiongen"
)

const (
	ActionFilePath     = "action.yml"
	ReleaseAssetURLFmt = "https://github.com/tinywasm/goflare/releases/download/%s/%s"
	TinyGoCacheKeyFmt  = "tinygo-${{ runner.os }}-${{ runner.arch }}-%s"
)

// LatestReleaseTag devuelve el tag semver mas alto del repositorio.
func LatestReleaseTag() (string, error) {
	g, err := git.NewGit()
	if err != nil {
		return "", fmt.Errorf("git init: %w", err)
	}
	tag, err := g.GetLatestTag()
	if err != nil {
		return "", fmt.Errorf("get latest tag: %w", err)
	}
	return strings.TrimSpace(tag), nil
}

// GoflareAction construye la descripcion de la action de goflare.
func GoflareAction(tinyGoVersion, goflareVersion string) actiongen.Action {
	if goflareVersion == "" {
		goflareVersion = "v0.5.22"
	}

	downloadScript := fmt.Sprintf(`set -euo pipefail

version="${{ inputs.version }}"
if [ -z "$version" ]; then
  ref="${{ github.action_ref }}"
  case "$ref" in
    v[0-9]*.[0-9]*.[0-9]*) version="$ref" ;;
    *) version="%s" ;;
  esac
fi

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64)  asset=goflare-linux-amd64 ;;
  Linux-aarch64) asset=goflare-linux-arm64 ;;
  Darwin-arm64)  asset=goflare-darwin-arm64 ;;
  Darwin-x86_64) asset=goflare-darwin-amd64 ;;
  *) echo "goflare: plataforma no soportada: $(uname -s)-$(uname -m)" >&2; exit 1 ;;
esac

url="https://github.com/tinywasm/goflare/releases/download/${version}/${asset}"
dest="${RUNNER_TEMP}/goflare"

if ! curl -fsSL "$url" -o "$dest"; then
  echo "goflare: no existe el binario ${asset} en el release ${version}." >&2
  echo "  URL: ${url}" >&2
  echo "  Lo mas probable es que gorelease no haya corrido para ese tag." >&2
  echo "  Revisa https://github.com/tinywasm/goflare/releases y fija una version que si tenga binarios con el input 'version'." >&2
  exit 1
fi

chmod +x "$dest"
echo "$(dirname "$dest")" >> "$GITHUB_PATH"
echo "version=${version}" >> "$GITHUB_OUTPUT"`, goflareVersion)

	installTinyGoScript := `set -euo pipefail
out="$(goflare tinygo)"
bindir="${out#*TINYGO_BINDIR=}"
bindir="${bindir%%$'\n'*}"
version="$(printf '%s' "$out" | sed -n 's/^TINYGO_VERSION=//p')"

if [ -z "$bindir" ]; then
  echo "goflare tinygo no devolvio un directorio bin" >&2
  exit 1
fi

echo "$bindir" >> "$GITHUB_PATH"
echo "version=${version}" >> "$GITHUB_OUTPUT"`

	envMapping := []actiongen.KeyValue{
		{Key: "PROJECT_NAME", Value: "${{ inputs.project || inputs.worker }}"},
		{Key: "WORKER_NAME", Value: "${{ inputs.worker }}"},
		{Key: "DOMAIN", Value: "${{ inputs.domain }}"},
		{Key: "D1_DATABASE_NAME", Value: "${{ inputs.d1-binding }}"},
		{Key: "R2_BUCKET_NAME", Value: "${{ inputs.r2-binding }}"},
		{Key: "COMPATIBILITY_DATE", Value: "${{ inputs.compatibility-date }}"},
		{Key: "NOT_FOUND_HANDLING", Value: "${{ inputs.not-found-handling }}"},
	}

	return actiongen.Action{
		Name:        "Deploy with goflare",
		Description: "Compila un proyecto Go a WASM y lo despliega como Cloudflare Worker",
		Author:      "tinywasm",
		Branding: actiongen.Branding{
			Icon:  "upload-cloud",
			Color: "orange",
		},
		Inputs: []actiongen.Input{
			{Name: "version", Description: "qué release de goflare descargar; vacío = resolución automática", Default: "", Required: false},
			{Name: "project", Description: "PROJECT_NAME; vacío = el valor de worker", Default: "", Required: false},
			{Name: "worker", Description: "WORKER_NAME", Required: true},
			{Name: "domain", Description: "DOMAIN", Default: "", Required: false},
			{Name: "d1-binding", Description: "D1_DATABASE_NAME", Default: "", Required: false},
			{Name: "r2-binding", Description: "R2_BUCKET_NAME", Default: "", Required: false},
			{Name: "compatibility-date", Description: "COMPATIBILITY_DATE", Default: "", Required: false},
			{Name: "not-found-handling", Description: "NOT_FOUND_HANDLING", Default: "", Required: false},
			{Name: "setup-go", Description: "correr actions/setup-go con el go.mod del proyecto", Default: "true", Required: false},
			{Name: "vet", Description: "correr go vet ./...", Default: "true", Required: false},
			{Name: "test", Description: "patrón para go test; vacío = no correr tests", Default: "./tests/...", Required: false},
			{Name: "pre-deploy", Description: "comando a correr entre build y deploy", Default: "", Required: false},
			{Name: "deploy", Description: "poner 'false' en pull requests para compilar sin desplegar", Default: "true", Required: false},
			{Name: "cache", Description: "cachear el árbol de TinyGo", Default: "true", Required: false},
		},
		Outputs: []actiongen.Output{
			{Name: "goflare-version", Description: "la versión que se resolvió y descargó", Value: "${{ steps.goflare.outputs.version }}"},
			{Name: "tinygo-version", Description: "lo que reporta tinygo version", Value: "${{ steps.tinygo.outputs.version }}"},
		},
		Steps: []actiongen.Step{
			{
				Name:    "Setup Go",
				If:      "inputs.setup-go == 'true'",
				Uses:    "actions/setup-go@v5",
				With:    []actiongen.KeyValue{{Key: "go-version-file", Value: "go.mod"}},
				Comment: "La versión sale del go.mod del proyecto, así que la elige el llamador, no nosotros. setup-go además cachea ~/go/pkg/mod y ~/.cache/go-build por su cuenta, con key derivada de go.sum.",
			},
			{
				Name:  "Resolver y descargar el binario de goflare",
				ID:    "goflare",
				Shell: "bash",
				Run:   downloadScript,
			},
			{
				Name: "Restaurar la caché de TinyGo",
				If:   "inputs.cache == 'true'",
				Uses: "actions/cache@v4",
				With: []actiongen.KeyValue{
					{Key: "path", Value: "/usr/local/tinygo\n~/.local/tinygo"},
					{Key: "key", Value: fmt.Sprintf(TinyGoCacheKeyFmt, tinyGoVersion)},
				},
				Comment: "Se cachea el árbol instalado, no el tarball: un acierto no cuesta ni descarga ni extracción. La versión va dentro de la key a propósito. Las keys de actions/cache son inmutables —un acierto nunca vuelve a guardar—, así que una key sin versión se queda clavada en un árbol viejo cuando la versión de TinyGo sube, y a partir de ahí cada corrida paga el restore y además la descarga completa. El generador mantiene esta cifra al día.",
			},
			{
				Name:    "Instalar TinyGo",
				ID:      "tinygo",
				Shell:   "bash",
				Run:     installTinyGoScript,
				Comment: "TinyGo lo instala el propio binario de goflare, vía tinywasm/tinygo. A propósito no se usa uses: tinywasm/tinygo@v1: esa action instala la versión del ref con que se la invoca, mientras sitec la vuelve a resolver durante el build. Serían dos fuentes, y el día que dejen de coincidir, sitec desinstala lo que puso la action y descarga lo suyo. Con el binario como único instalador hay una sola fuente. Este paso va antes de los tests porque algunos proyectos invocan el binario tinygo pelado desde el PATH durante go test.",
			},
			{
				Name:  "go vet",
				If:    "inputs.vet == 'true'",
				Shell: "bash",
				Run:   "go vet ./...",
			},
			{
				Name:  "go test",
				If:    "inputs.test != ''",
				Shell: "bash",
				Run:   "go test ${{ inputs.test }}",
			},
			{
				Name:  "goflare build",
				Shell: "bash",
				Run:   "goflare build",
				Env:   envMapping,
			},
			{
				Name:    "Comando pre-deploy",
				If:      "inputs.pre-deploy != '' && inputs.deploy == 'true'",
				Shell:   "bash",
				Run:     "${{ inputs.pre-deploy }}",
				Comment: "Aquí es donde va la migración del esquema, que corre una vez por despliegue y no una vez por arranque de isolate.",
			},
			{
				Name:  "goflare deploy",
				If:    "inputs.deploy == 'true'",
				Shell: "bash",
				Run:   "goflare deploy",
				Env:   envMapping,
			},
		},
	}
}
