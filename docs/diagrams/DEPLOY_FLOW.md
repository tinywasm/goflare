```mermaid
sequenceDiagram
    participant G as GoFlare
    participant ENV as Environment (CI/Local)
    participant CF as Cloudflare API

    Note over G: After Build

    G->>ENV: os.Getenv("CLOUDFLARE_API_TOKEN")
    ENV-->>G: Token string

    alt Has Public Assets (cfg.PublicDir set)
        G->>CF: POST /accounts/:id/workers/scripts/:name/assets-upload-session (Manifest)
        CF-->>G: Session JWT + Buckets
        loop Each Bucket
            G->>CF: POST /workers/assets/upload?base64=true (Auth: Session JWT)
            CF-->>G: Completion JWT
        end
    end

    G->>CF: PUT /accounts/:id/workers/scripts/:name (Multipart: metadata + edge.js + edge.wasm)
    CF-->>G: 200 OK

    opt Custom Domain Set
        G->>CF: GET /zones
        G->>CF: PUT /accounts/:id/workers/domains
    end

    opt Edge Script Deployed
        G->>CF: GET /api/__goflare_probe
        Note over G: Verify x-goflare identity header
    end

    G->>User: Deployment Summary
```

## Regla: Despliegue único

| Condición | Comportamiento |
|---|---|
| Solo `cfg.PublicDir` | Assets subidos + Worker sin `main_module` |
| Solo `cfg.Entry` | Script Worker (`edge.js` + `edge.wasm`) sin key `assets` |
| Ambos (`cfg.Entry` + `cfg.PublicDir`) | Assets subidos + Script Worker con `run_worker_first` |
