> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan: tinywasm/goflare — Chequeo de tamaño WASM post-build

## Context

`goflare build` compila `edge/main.go` con TinyGo y mueve el resultado a
`functions/edge.wasm`. Cloudflare Workers/Pages Free tiene un límite de **1 MiB**
para el binario WASM. Hoy el build no verifica el tamaño antes del deploy — el error
llega solo cuando Cloudflare rechaza el upload.

El objetivo es que `buildPagesFunctions()` y `buildWorker()` verifiquen el tamaño de
`edge.wasm` inmediatamente después de generarlo y fallen con un mensaje accionable
antes de intentar el deploy.

## Límites

| Plan | WASM limit | JS bundle limit |
|---|---|---|
| Free | 1 MiB | 1 MiB |
| Paid | 10 MiB | 10 MiB |

El chequeo implementa el límite Free (1 MiB). No hay forma automática de detectar
el plan — se usa el límite más conservador como default seguro.

## Constante

```go
// maxWasmSize es el límite Free de Cloudflare Workers/Pages para el binario WASM.
// https://developers.cloudflare.com/workers/platform/limits/#worker-size
const maxWasmSize = 1 * 1024 * 1024 // 1 MiB
```

## Función helper

Nueva función en `build.go`:

```go
func checkWasmSize(path string) error {
    info, err := os.Stat(path)
    if err != nil {
        return fmt.Errorf("wasm size check: %w", err)
    }
    size := info.Size()
    if size > maxWasmSize {
        return fmt.Errorf(
            "edge.wasm exceeds Cloudflare Free limit: %d bytes (%.1f KiB) > 1 MiB — "+
                "reduce binary size or upgrade to a paid plan",
            size, float64(size)/1024,
        )
    }
    return nil
}
```

## Stages

### Stage 1 — `build.go` (editar): agregar constante + `checkWasmSize`

Agregar `maxWasmSize` y `checkWasmSize` en `build.go`.

### Stage 2 — `buildPagesFunctions()` (editar): llamar checkWasmSize después de mover el wasm

Después de `moveFile(srcWasm, dstWasm)` y antes de `generatePagesFunctionFile()`:

```go
if err := checkWasmSize(dstWasm); err != nil {
    return err
}
```

### Stage 3 — `buildWorker()` (editar): llamar checkWasmSize después de generateWasmFile

Después de `generateWasmFile()` y antes de `generateWorkerFile()`:

```go
wasmPath := filepath.Join(g.Config.OutputDir, "edge.wasm")
if err := checkWasmSize(wasmPath); err != nil {
    return err
}
```

---

## Stage 4 — Staging temporal con `os.MkdirTemp` (eliminar `.build/` del repo)

### Problema

`tinywasm/client` recibe `OutputDir: func() string { return cfg.OutputDir }` y llama
`UseDiskStorage()` en `New()`, que cachea el path en ese momento. El default es
`.build/` — dentro del repo. Después del build el directorio persiste, ensuciando el
árbol de trabajo aunque el dev no lo pidió.

### Diseño

Agregar campo `stagingDir string` a `Goflare`. En `New()`:

```go
staging, err := os.MkdirTemp("", "goflare-*")
if err != nil {
    // fallback al OutputDir configurado si MkdirTemp falla
    staging = cfg.OutputDir
}
g.stagingDir = staging
```

Pasar `staging` como `OutputDir` al `edgeCompiler` en lugar de `cfg.OutputDir`:

```go
edgeCompiler := client.New(&client.Config{
    // ...
    OutputDir: func() string { return g.stagingDir },
})
edgeCompiler.UseDiskStorage()
```

`Build()` registra el cleanup **solo si se creó un dir temporal** (distinguible porque
`g.stagingDir != cfg.OutputDir`):

```go
func (g *Goflare) Build() error {
    if g.stagingDir != g.Config.OutputDir {
        defer os.RemoveAll(g.stagingDir)
    }
    // ... resto del build
}
```

### Tratamiento por modo

| Modo | Artifact final | Acción después de compile |
|---|---|---|
| pages-functions | `functions/edge.wasm` | `moveFile(stagingDir/edge.wasm, functions/edge.wasm)` — ya existe |
| workers | `g.Config.OutputDir/edge.{js,wasm}` | mover desde `stagingDir/` a `Config.OutputDir/` |
| pages-static | sin edge WASM | sin cambio |

Para **workers**, `buildWorker()` mueve los archivos del staging al `Config.OutputDir`
configurado (default `.build/`, ahora es el destino real del usuario, no el staging):

```go
func (g *Goflare) buildWorker() error {
    // ...compile a g.stagingDir...
    for _, name := range []string{"edge.wasm", "edge.js"} {
        src := filepath.Join(g.stagingDir, name)
        dst := filepath.Join(g.Config.OutputDir, name)
        if err := moveFile(src, dst); err != nil && !os.IsNotExist(err) {
            return err
        }
    }
    return nil
}
```

Para **pages-functions**, el `moveFile` existente ya usa `g.Config.OutputDir` como
`srcWasm` — cambiar a `g.stagingDir`:

```go
srcWasm := filepath.Join(g.stagingDir, "edge.wasm")   // era g.Config.OutputDir
```

### Tests

Archivo: `goflare_test.go` (o `build_test.go`), `package goflare_test`.

- **`TestBuild_NoDotBuildInRepo`**: crea un `Goflare` con un entry file fake, verifica
  que después de `Build()` no existe `.build/` en el working dir.
- El test de size check existente (Stage 2) sigue pasando porque usa `dstWasm` (en
  `functions/`) que ya es el path post-move.

---

## Stages Summary

| # | Archivo | Acción |
|---|---|---|
| 1 | `build.go` | Agregar `const maxWasmSize` + func `checkWasmSize(path string) error` |
| 2 | `buildPagesFunctions()` | Llamar `checkWasmSize(dstWasm)` después de mover el wasm |
| 3 | `buildWorker()` | Llamar `checkWasmSize(wasmPath)` después de mover desde staging |
| 4 | `goflare.go` + `build.go` | Agregar `stagingDir`, `os.MkdirTemp` en `New()`, `defer RemoveAll` en `Build()`, actualizar paths en `buildPagesFunctions` y `buildWorker` |

## Verification

```bash
gotest
```

- El test existente de build debe seguir pasando.
- `TestBuild_NoDotBuildInRepo`: verifica que `.build/` no existe en el repo después del build.
- Test sintético de 1.1 MiB verifica que `checkWasmSize` retorna error con el tamaño en el mensaje.
