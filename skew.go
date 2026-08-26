//go:build !wasm

package goflare

import (
	"fmt"
	"runtime/debug"

	"github.com/tinywasm/modfind"
)

const (
	// CloudflareModulePath es el modulo del runtime del edge. goflare embebe sus
	// assets JS; el proyecto compila su codigo Go. Las dos mitades tienen que venir
	// de la misma version.
	CloudflareModulePath = "github.com/tinywasm/cloudflare"

	skewErrFmt = "desajuste de versiones de tinywasm/cloudflare: tu go.mod resuelve %s y este binario de goflare lleva embebidos los assets JS de %s. El pegamento JavaScript y el runtime Go del Worker comparten una ABI; si divergen, el Worker aborta al inicializar paquetes sin dejar mensaje. Corrige con: go get github.com/tinywasm/cloudflare@%s"
)

// EmbeddedCloudflareVersion devuelve la version de github.com/tinywasm/cloudflare
// con la que se compilo ESTE binario.
func EmbeddedCloudflareVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range info.Deps {
		if dep != nil && dep.Path == CloudflareModulePath {
			return dep.Version
		}
	}
	return ""
}

// ProjectCloudflareVersion devuelve la version de github.com/tinywasm/cloudflare
// que resuelve el go.mod del proyecto en moduleRoot.
func ProjectCloudflareVersion(moduleRoot string) (string, error) {
	f := modfind.New()
	mods, err := f.Discover(moduleRoot)
	if err != nil {
		return "", fmt.Errorf("discover project modules: %w", err)
	}
	for _, mod := range mods {
		if mod.Path == CloudflareModulePath {
			return mod.Version, nil
		}
	}
	return "", nil
}

// CompareVersions implementa la tabla de decision de desajuste de versiones.
func CompareVersions(project, embedded string) error {
	if project == "" || embedded == "" {
		return nil
	}
	if project != embedded {
		return fmt.Errorf(skewErrFmt, project, embedded, embedded)
	}
	return nil
}

// CheckVersionSkew falla si el proyecto y este binario resuelven versiones
// distintas de tinywasm/cloudflare.
func CheckVersionSkew(moduleRoot string) error {
	projVer, err := ProjectCloudflareVersion(moduleRoot)
	if err != nil {
		return err
	}
	embVer := EmbeddedCloudflareVersion()
	return CompareVersions(projVer, embVer)
}
