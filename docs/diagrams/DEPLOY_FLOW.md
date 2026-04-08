# Deployment Flow

```mermaid
sequenceDiagram
    participant G as GoFlare
    participant CF as Cloudflare API

    Note over G: After Build & Auth

    alt Target: Worker
        G->>CF: PUT /accounts/:id/workers/scripts/:name (Multipart)
        CF-->>G: 200 OK
    end

    alt Target: Pages
        G->>CF: GET /accounts/:id/pages/projects/:name
        alt Project Missing
            G->>CF: POST /accounts/:id/pages/projects
        end
        G->>CF: POST /accounts/:id/pages/projects/:name/uploadToken
        CF-->>G: JWT
        Note over G: Batch Files (max 50)
        loop Each Batch
            G->>CF: POST /pages/assets/upload (Auth: JWT)
        end
        G->>CF: POST /accounts/:id/pages/projects/:name/deployments (Manifest)
        CF-->>G: 200 OK
        alt Domain Set
            G->>CF: POST /accounts/:id/pages/projects/:name/domains
        end
    end

    G->>User: Deployment Summary (URLs)
```
