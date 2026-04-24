# goflare — Plan de mejoras pendientes

---

## Problema 3 — Subcomando `goflare auth` (para el agente)

### `run.go`: añadir `RunAuth`

```go
func RunAuth(envPath string, in io.Reader, out io.Writer, reset bool, check bool) error {
    cfg, err := LoadConfigFromEnv(envPath)
    if err != nil {
        return err
    }
    g := New(cfg)
    store := NewKeyringStore()
    key := "goflare/" + cfg.ProjectName

    if reset {
        store.Delete(key)
        fmt.Fprintln(out, "Token reset. Run goflare auth to set a new one.")
        return nil
    }

    if check {
        token, err := store.Get(key)
        if err != nil || token == "" {
            fmt.Fprintln(out, "No token saved.")
            return fmt.Errorf("not authenticated")
        }
        if err := g.validateToken(token); err != nil {
            fmt.Fprintln(out, "Token invalid:", err)
            return err
        }
        fmt.Fprintln(out, "Token OK.")
        return nil
    }

    return g.Auth(store, in)
}
```

### `cmd/goflare/main.go`: añadir `case "auth"` y actualizar `Usage()`

```go
case "auth":
    reset := fs.Bool("reset", false, "delete saved token")
    check := fs.Bool("check", false, "verify saved token")
    fs.Parse(os.Args[2:])
    err = goflare.RunAuth(*env, os.Stdin, os.Stdout, *reset, *check)
```

---

## Problema 6 — Worker URL real en deploy summary (para el agente)

### `cloudflare.go`: añadir `getWorkerSubdomain`

```go
func (g *Goflare) getWorkerSubdomain(client *cfClient) string {
    path := fmt.Sprintf("/accounts/%s/workers/subdomain", g.Config.AccountID)
    data, err := client.get(path)
    if err != nil {
        return "<your-subdomain>"
    }
    var result struct {
        Result struct {
            Subdomain string `json:"subdomain"`
        } `json:"result"`
    }
    json.Unmarshal(data, &result)
    return result.Result.Subdomain
}
```

Llamar desde `RunDeploy` después de `DeployWorker` exitoso para construir la URL real
en vez del placeholder `<your-subdomain>`.

---

## Checklist

- [x] `goflare/goflare.go:59` — `edgeCompiler.Change()` → `edgeCompiler.SetMode()` (Bug 1)
- [x] `auth.go` — prompt con link + permisos + keyring note
- [x] `auth.go` — env var `CLOUDFLARE_API_TOKEN` sin guardar en keyring
- [x] `auth.go` — error de validación accionable
- [x] `store.go` — `Delete(key)` en interfaz + `KeyringStore` + `MemoryStore`
- [x] `store.go` — campo `ProjectName` unused eliminado de `KeyringStore`
- [x] `init.go` — contexto visual para Account ID
- [x] `docs/diagrams/AUTH_FLOW.md` — env var CI/CD + `goflare auth --reset/--check`
- [x] `docs/diagrams/DEPLOY_FLOW.md` — `dist/` → `PublicDir`; URL real con subdomain
- [x] `docs/diagrams/goflare-generic.md` — `ASK_ENTRY` auto-detect; `COPY_DIST`/`D_HAS_DIST` eliminados; rama `CMD_AUTH` añadida
- [ ] `run.go` — `RunAuth()` (Problema 3)
- [ ] `cmd/goflare/main.go` — `case "auth"` + `Usage()` (Problema 3)
- [ ] `cloudflare.go` — `getWorkerSubdomain()` + URL real en summary (Problema 6)
