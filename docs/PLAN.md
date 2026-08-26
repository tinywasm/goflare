---
PLAN: "feat!: goflare deploys from one binary — GitHub Action, size gate, automated releases"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# PLAN — `tinywasm/goflare`: un solo binario, una sola línea de YAML

> Si te dijeron "ejecuta el plan descrito en docs/PLAN.md", ejecuta **TODAS las
> etapas de abajo, en orden (de arriba a abajo)**. Cada etapa es autocontenida:
> termina una (sus criterios de aceptación en verde) antes de empezar la
> siguiente. Nunca mezcles cambios de una etapa en otra.

## Por qué existe este plan

Hoy desplegar con goflare cuesta ~40 líneas de YAML repetidas en tres repos, y
cada una compila goflare desde fuente en el runner. Tres problemas medidos:

1. **Los releases de goflare se detuvieron en `v0.5.13` mientras los tags van en
   `v0.5.22`.** Nadie corre `gorelease`. Consecuencia real y actual:
   `tinywasm/goflare-demo` descarga
   `…/releases/download/v0.5.22/goflare-linux-amd64` y recibe **HTTP 404**. Su
   deploy está roto ahora mismo.

2. **`go run github.com/tinywasm/goflare/cmd/goflare` compila 15 MB de binario
   en el runner.** Medido en una máquina de desarrollo rápida con la caché de
   módulos caliente: **18 s en frío, 1 s en caliente**. En un runner de GitHub
   (2-4 vCPU) el frío se va a 40-70 s. Y la caché de `actions/setup-go` está
   clavada a `go.sum`, que en este ecosistema rota en el **43 %** de los commits
   (16 de 37 en `veltylabs/iam`), así que casi la mitad de los deploys pagan el
   precio completo. Descargar el binario publicado (10,2 MB) cuesta **1-2 s**.

3. **El chequeo de tamaño del wasm es incorrecto y silencioso.** `build.go`
   declara `maxWasmSize = 1 MiB` con el mensaje *"exceeds Cloudflare Free
   limit"*. Ese dato es falso: el límite Free de Cloudflare es **3 MB
   comprimido** (10 MB en Paid) y 64 MB sin comprimir. Además mide el tamaño
   **crudo** cuando el límite es sobre el **comprimido**, y **nunca imprime el
   tamaño** — no hay `Logger` en esa función, solo un `error` que jamás se
   dispara. Por eso los logs de despliegue nunca han dicho cuánto pesa lo que
   suben.

Al final de este plan, un consumidor despliega así:

```yaml
- uses: actions/checkout@v4
- uses: tinywasm/goflare@v1
  with:
    worker: iam
    domain: iam.velty.cl
    d1-binding: DB
    test: './tests/...'
    pre-deploy: 'go run ./cmd/migrate'
  env:
    CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
    CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
    D1_DATABASE_ID: ${{ secrets.D1_DATABASE_ID }}
```

## Decisiones ya tomadas — no las re-abras

Estas se resolvieron en la conversación de planificación. El agente ejecutor las
implementa tal cual; no son opciones.

| Decisión | Resolución |
|---|---|
| Qué instala TinyGo | **El binario de goflare**, vía `tinywasm/tinygo`. La action NO usa `uses: tinywasm/tinygo@v1` ni un paso propio de instalación. Un solo binario que descargar, y una sola fuente de la versión. |
| Alcance de la action | **Todo**: Go, TinyGo, vet, tests, build, gate de tamaño, comando pre-deploy opcional, deploy. |
| Resolución de versión del binario | `inputs.version` → si no, `github.action_ref` cuando es un tag semver completo → si no, el literal horneado por el generador. |
| Umbrales de tamaño | Aviso en **256 KiB crudo**, aborto en **900 KiB crudo**. Ambos sobre el tamaño **crudo**, porque el crudo es lo que el isolate compila e instancia. Se reportan siempre crudo y gzip. |
| Los 900 KiB | Son **presupuesto propio de goflare**, no un límite de Cloudflare. La documentación y los mensajes deben decirlo así. |
| Caché de TinyGo | Sí, con la **versión exacta** en la key (no el literal `'default'`), y un input `cache` para apagarlo. |
| `action.yml` | **Generado por código**, nunca escrito a mano. Un test lo escribe y falla si difería. |
| Pages / wrangler | **Fuera de alcance.** Cloudflare está migrando todo a Workers y queremos una sola forma de desplegar. `goflare-demo` migra en un plan aparte. |

## Anti-footguns

> ⚠️ **Este repo es tooling de backend y usa la biblioteca estándar
> legítimamente.** `net/http`, `os`, `os/exec`, `strings`, `compress/gzip`,
> `encoding/json` son correctos y esperados en todo archivo con `//go:build
> !wasm`. La regla del ecosistema "nada de stdlib" aplica **solo** al código que
> se compila a WASM, que desde el split ya **no vive en este repo** (está en
> `github.com/tinywasm/cloudflare`). **No "arregles" imports de stdlib aquí.**

> ⚠️ **No muevas código de tooling a `tinywasm/cloudflare`.** Esa librería es el
> runtime del edge; goflare depende de ella, nunca al revés. Cualquier cliente
> HTTP, script de CI u orquestación de despliegue va **aquí**, sin importar el
> build tag.

> ⚠️ **No toques `tinywasm/sitec` ni `tinywasm/tinygo`.** Todo lo de este plan se
> implementa dentro de `tinywasm/goflare`. Si crees necesitar un cambio en otro
> repo, detente y repórtalo en el PR en vez de hacerlo.

## Reglas de calidad — obligatorias en cada etapa

**Nada de strings mágicos.** Toda cadena repetida (clave de entorno, ruta,
prefijo, nombre de flag, URL) es una constante nombrada en el paquete de
librería. Los literales están prohibidos en la lógica. Sigue el patrón que ya
existe en [config.go](../config.go): `const EnvKeyProjectName = "PROJECT_NAME"`.

**`cmd/` delgado.** `cmd/goflare/main.go` contiene SOLO: parseo de argumentos,
inyección de dependencias, e imprimir/salir. Toda condicional, validación o
lectura de entorno es una función exportada de la librería. El patrón vigente es
`goflare.RunBuild(envPath, out)` — sigue ese.

**Contrato de CLI consumible por IA.**
- Sin argumentos → imprime la ayuda en stdout y sale con **0**.
- **stdout = datos consumibles; stderr = todo diagnóstico.**
- Sale **0** en éxito/ayuda; distinto de 0 ante flags inválidos o fallo.

**Sin duplicación librería↔cmd.** Si la librería ya calcula un valor, `cmd/` usa
la constante exportada; nunca lo re-deriva.

**Tests.** Van en `tests/` como `package goflare_test`, caja negra, a través de
la API pública. Única excepción: un test que necesite un símbolo **no exportado**
va junto al código como `*_internal_test.go` (ver
[build_internal_test.go](../build_internal_test.go)). Solo aserciones de stdlib.

Corre `gotest ./...` al terminar cada etapa; nunca `go test` a secas.

## Etapas

| Orden | Etapa | Asunto |
|---|---|---|
| 1 | [PLAN_STAGE_1_SIZE_GATE.md](PLAN_STAGE_1_SIZE_GATE.md) | Reportar crudo+gzip siempre; avisar en 256 KiB, abortar en 900 KiB |
| 2 | [PLAN_STAGE_2_SIZE_DIAGNOSTIC.md](PLAN_STAGE_2_SIZE_DIAGNOSTIC.md) | `goflare size`: desglose por paquete y guarda de imports prohibidos |
| 3 | [PLAN_STAGE_3_TINYGO_SUBCOMMAND.md](PLAN_STAGE_3_TINYGO_SUBCOMMAND.md) | `goflare tinygo`: instala TinyGo e imprime su bindir |
| 4 | [PLAN_STAGE_4_VERSION_SKEW.md](PLAN_STAGE_4_VERSION_SKEW.md) | Fallar si el `tinywasm/cloudflare` del proyecto no coincide con el del binario |
| 5 | [PLAN_STAGE_5_ACTION_GENERATOR.md](PLAN_STAGE_5_ACTION_GENERATOR.md) | Generador portable de `action.yml` + test de drift |
| 6 | [PLAN_STAGE_6_ACTION_AND_RELEASE.md](PLAN_STAGE_6_ACTION_AND_RELEASE.md) | `action.yml` commiteado, workflow que la consume, `gorelease` automático |
| 7 | [PLAN_STAGE_7_DOCS.md](PLAN_STAGE_7_DOCS.md) | README, ARCHITECTURE, CI_GITHUB_ACTIONS, diagrama |

Al terminar todas las etapas corre `gotest ./...` una última vez: todo en verde.
