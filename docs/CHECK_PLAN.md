# PLAN: Migrate goflare to the new client + assetmin storage APIs

> Status: Ready for execution. Mechanical migration. No new features, no
> design decisions.

## Why

`tinywasm/client` published `v0.6.6` removing `SetBuildOnDisk(bool, bool)`,
replaced by `UseDiskStorage()` / `UseMemoryStorage()`
(see [client/docs/CHECK_PLAN.md](../../client/docs/CHECK_PLAN.md)).

`tinywasm/assetmin` will publish an equivalent removal of its own
`SetBuildOnDisk(bool)` in favor of `FlushToDisk()` + the new SSR-mode APIs
(see [assetmin/docs/PLAN.md](../../assetmin/docs/PLAN.md)).

`goflare` currently fails `go vet` because it calls both removed APIs. The
downstream packages that import `goflare` (e.g. `tinywasm/deploy`) cannot
compile until `goflare` migrates and publishes a new tag.

## Affected call sites in this repo

| File                                                         | Line | Current call                            | Replacement                              |
|--------------------------------------------------------------|------|-----------------------------------------|------------------------------------------|
| [goflare.go](../goflare.go#L58)                              | 58   | `edgeCompiler.SetBuildOnDisk(true, false)`    | `edgeCompiler.UseDiskStorage()`    |
| [goflare.go](../goflare.go#L82)                              | 82   | `browserCompiler.SetBuildOnDisk(true, false)` | `browserCompiler.UseDiskStorage()` |
| [build.go](../build.go#L82)                                  | 82   | `g.assetMin.SetBuildOnDisk(true)`             | `g.assetMin.FlushToDisk()`         |

## Migration

### 1. Client calls (goflare.go)

```go
// BEFORE:
edgeCompiler.SetBuildOnDisk(true, false)
// AFTER:
edgeCompiler.UseDiskStorage()
```

Both existing call sites in `goflare.go` follow the same pattern: the
boolean pair `(true, false)` meant "switch to disk, do NOT auto-compile".
`compileNow=false` matches the new API's pure-setter semantics; no
behavior change.

Subsequent explicit `Compile()` calls (already present elsewhere in the
file) remain unchanged.

### 2. AssetMin call (build.go:82)

```go
// BEFORE:
g.assetMin.SetBuildOnDisk(true)
// AFTER:
if err := g.assetMin.FlushToDisk(); err != nil {
    return fmt.Errorf("assetmin flush failed: %w", err)
}
```

Semantic note: the old `SetBuildOnDisk(true)` was a deprecated alias that
both toggled the `buildOnDisk` flag and triggered a partial flush of the 5
main handlers. The new `FlushToDisk()` is the explicit, complete-flush
replacement. It returns an error which **must** be propagated — the caller
context (`build.go`) is a deploy build pipeline where a silent failure to
write assets is the same class of bug the parent refactor fixed.

Add the same import-time guard for `fmt` if not already present.

### 3. Dependency bump

Update `go.mod`:

```
require (
    github.com/tinywasm/client v0.6.6  // or later
    github.com/tinywasm/assetmin v<next>  // after assetmin PLAN publishes
)
```

If `assetmin` has not yet published the new API at execution time, this
PLAN blocks on it. Order of execution:

1. Wait for [assetmin/docs/PLAN.md](../../assetmin/docs/PLAN.md) to merge
   and publish a tag.
2. Then run this PLAN.

`tinywasm/client v0.6.6` is already published.

## Tests

`goflare` has no test that directly exercises the removed methods today.
After migration:

1. `go vet ./...` must be clean.
2. `go build ./...` must succeed.
3. Existing tests (if any) must still pass.

No new tests are added — this is a mechanical rename, not a feature
change. The semantic correctness of `FlushToDisk` is owned by the
`tinywasm/assetmin` test suite, not by `goflare`.

## Out of scope

- Any behavior change in goflare's build pipeline.
- Refactoring the Goflare struct, configuration, or edge-compiler logic.
- Adding tests for goflare beyond keeping `go test ./...` green.

## Acceptance criteria

1. No call site in this repo references `SetBuildOnDisk` (on `client` or
   `assetmin`).
2. `go vet ./...` is clean.
3. `go build ./...` succeeds.
4. `go test ./...` passes.
5. `go.mod` pins `tinywasm/client >= v0.6.6` and the appropriate
   `tinywasm/assetmin` version.
