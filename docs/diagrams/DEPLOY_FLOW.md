```mermaid
sequenceDiagram
    participant G as GoFlare
    participant ENV as Environment (CI/Local)
    participant CF as Cloudflare API

    Note over G: After Build

    G->>ENV: os.Getenv("CLOUDFLARE_API_TOKEN")
    ENV-->>G: Token string

    alt Target: Worker standalone (cfg.Entry set AND no .wasm/.mjs in FunctionsDir)
        G->>CF: PUT /accounts/:id/workers/scripts/:name (Multipart: edge.js + edge.wasm)
        CF-->>G: 200 OK
        G->>CF: GET /accounts/:id/workers/subdomain
        CF-->>G: subdomain string
        Note over G: URL: https://WORKER_NAME.subdomain.workers.dev
    end

    alt Target: Pages (cfg.PublicDir set)
        G->>CF: GET /accounts/:id/pages/projects/:name
        alt Project Missing
            G->>CF: POST /accounts/:id/pages/projects
        end
        G->>CF: POST /accounts/:id/pages/projects/:name/uploadToken
        CF-->>G: JWT
        loop Each Batch (max 50) — PublicDir files
            G->>CF: POST /pages/assets/upload (Auth: JWT)
        end
        alt FunctionsDir has .wasm/.mjs (Pages Functions project)
            loop Each Batch (max 50) — FunctionsDir files → /functions/ prefix
                G->>CF: POST /pages/assets/upload (Auth: JWT)
            end
            Note over G: edge.wasm + [[path]].mjs deployed as Pages Function
        end
        G->>CF: POST /accounts/:id/pages/projects/:name/deployments (Manifest)
        CF-->>G: 200 OK
        Note over G: URL: https://PROJECT_NAME.pages.dev
    end

    G->>User: Deployment Summary
```

## Regla: Worker vs Pages Functions

| Condición | Comportamiento |
|---|---|
| `cfg.Entry != ""` + NO hay `.wasm`/`.mjs` en `FunctionsDir` | `DeployWorker` (Worker standalone) |
| `cfg.Entry != ""` + SÍ hay `.wasm`/`.mjs` en `FunctionsDir` | Solo `DeployPages` (edge como Pages Function) |
| Solo `cfg.PublicDir` | Solo `DeployPages` sin Functions |
