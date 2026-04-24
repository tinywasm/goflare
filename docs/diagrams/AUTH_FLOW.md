# Authentication Flow

```mermaid
sequenceDiagram
    participant U as User
    participant G as GoFlare
    participant E as Env (CI/CD)
    participant K as Keyring
    participant CF as Cloudflare API

    U->>G: goflare deploy / goflare auth

    G->>K: Get("goflare/PROJECT_NAME")
    alt Token in keyring
        K-->>G: Token
        G->>CF: Proceed
    else No keyring token
        G->>E: os.Getenv("CLOUDFLARE_API_TOKEN")
        alt Env var set (CI/CD)
            E-->>G: Token
            G->>CF: GET /user/tokens/verify
            alt Invalid
                CF-->>G: Error + actionable steps
            else Valid
                CF-->>G: OK
                Note over G: No keyring write — CI orchestrator manages secrets
                G->>CF: Proceed
            end
        else Interactive
            G->>U: Show link: dash.cloudflare.com/profile/api-tokens
            G->>U: Show required permissions
            G->>U: Prompt: "Paste token:"
            U-->>G: Token
            G->>CF: GET /user/tokens/verify
            alt Invalid
                CF-->>G: Error + actionable steps (goflare auth --reset)
            else Valid
                CF-->>G: OK
                G->>K: Save token (goflare/PROJECT_NAME)
                G->>CF: Proceed
            end
        end
    end
```

## goflare auth subcommands

```
goflare auth           # guarda/verifica token (interactivo si no hay token)
goflare auth --reset   # borra token del keyring, pide uno nuevo en el próximo deploy
goflare auth --check   # verifica token guardado sin pedir nada, exit 0/1
```
