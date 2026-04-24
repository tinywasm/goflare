```mermaid
flowchart TD
    classDef cmd fill:#0f172a,stroke:#38bdf8,color:#e0f2fe
    classDef detect fill:#1e293b,stroke:#94a3b8,color:#cbd5e1
    classDef compile fill:#1e3a5f,stroke:#38bdf8,color:#e0f2fe
    classDef front fill:#713f12,stroke:#facc15,color:#fef9c3
    classDef output fill:#064e3b,stroke:#34d399,color:#d1fae5
    classDef auth fill:#4c1d95,stroke:#a78bfa,color:#ede9fe
    classDef error fill:#7f1d1d,stroke:#f87171,color:#fecaca
    classDef cf fill:#f97316,stroke:#fdba74,color:#1e293b
    classDef cfpages fill:#0284c7,stroke:#7dd3fc,color:#e0f2fe
    classDef key fill:#365314,stroke:#84cc16,color:#ecfccb
    classDef sdk fill:#1e3a5f,stroke:#38bdf8,color:#bae6fd
    classDef env fill:#78350f,stroke:#f59e0b,color:#fef3c7

    USER(["$ goflare comando"]):::cmd
    USER --> CMD_SWITCH{"Comando"}:::detect
    CMD_SWITCH --> CMD_INIT["init"]:::cmd
    CMD_SWITCH --> CMD_BUILD["build"]:::cmd
    CMD_SWITCH --> CMD_DEPLOY["deploy"]:::cmd
    CMD_SWITCH --> CMD_AUTH["auth"]:::cmd

    %% GOFLARE INIT
    subgraph INIT ["goflare init"]
        direction TB
        CMD_INIT --> ASK_NAME["Prompt: Project name?"]:::detect
        ASK_NAME --> ASK_ACC["Prompt: Account ID?\ndash.cloudflare.com → right sidebar"]:::detect
        ASK_ACC --> ASK_DOMAIN["Prompt: Domain? vacío = *.pages.dev"]:::detect
        ASK_DOMAIN --> AUTO_ENTRY{"edge/main.go existe?"}:::detect
        AUTO_ENTRY -->|Sí| SET_ENTRY["Entry = 'edge' (auto-detectado)"]:::output
        AUTO_ENTRY -->|No| ASK_ENTRY["Prompt: Entry dir? vacío = solo Pages"]:::detect
        SET_ENTRY --> ASK_PUBLIC
        ASK_ENTRY --> ASK_PUBLIC["Prompt: Public dir? vacío = solo Worker"]:::detect
        ASK_PUBLIC --> VALIDATE_INIT{"ENTRY vacío Y PUBLIC_DIR vacío?"}:::detect
        VALIDATE_INIT -->|Sí| ERR_INIT["Error: se requiere al menos uno"]:::error
        VALIDATE_INIT -->|No| WRITE_ENV["Write .env"]:::env
        WRITE_ENV --> GI_CHECK{"¿.env en .gitignore?"}:::detect
        GI_CHECK -->|No| GI_ADD["Añadir .env y .build/ a .gitignore"]:::output
        GI_CHECK -->|Sí| INIT_DONE["Init complete"]:::output
        GI_ADD --> INIT_DONE
    end

    %% GOFLARE BUILD
    subgraph BUILD ["goflare build"]
        direction TB
        CMD_BUILD --> READ_ENV_B["Read .env"]:::env
        READ_ENV_B -->|Missing| ERR_ENV["Error: .env not found — run goflare init"]:::error
        READ_ENV_B -->|OK| SCAN["Check ENTRY y PUBLIC_DIR"]:::detect

        SCAN --> BOTH_EMPTY{"ENTRY vacío Y PUBLIC_DIR vacío?"}:::detect
        BOTH_EMPTY -->|Sí| ERR_EMPTY["Error: nothing to build"]:::error
        BOTH_EMPTY -->|No| HAS_BACK{"ENTRY set?"}:::detect

        HAS_BACK -->|Sí| TINYGO["tinygo build → .build/edge.wasm"]:::compile
        HAS_BACK -->|No| SKIP_W["Worker: skip"]:::detect

        TINYGO -->|Error| ERR_BUILD["Error: tinygo build failed"]:::error
        TINYGO -->|OK| GEN_JS["Generate bundled + minified JS: .build/edge.js"]:::compile

        SCAN --> HAS_FRONT{"PUBLIC_DIR set?"}:::detect
        HAS_FRONT -->|Sí| WASM_COMPILE["Compile web/client.go → PublicDir/client.wasm"]:::front
        HAS_FRONT -->|No| SKIP_P["Pages: skip"]:::detect
        WASM_COMPILE --> ASSETS_GEN["Generate script.js + style.css → PublicDir"]:::front

        GEN_JS --> BUILD_DONE["Build complete — .build/ ready"]:::output
        ASSETS_GEN --> BUILD_DONE
        SKIP_W --> BUILD_DONE
        SKIP_P --> BUILD_DONE
    end

    %% GOFLARE AUTH
    subgraph AUTH_SUB ["goflare auth"]
        direction TB
        CMD_AUTH --> AUTH_FLAGS{"Flag?"}:::detect
        AUTH_FLAGS -->|"--reset"| AUTH_DELETE["Delete keyring goflare/PROJECT_NAME"]:::key
        AUTH_FLAGS -->|"--check"| AUTH_CHECK["Get token + validate"]:::key
        AUTH_FLAGS -->|ninguno| AUTH_FLOW["Auth() — keyring → env var → prompt"]:::auth
        AUTH_DELETE --> AUTH_DONE["Done"]:::output
        AUTH_CHECK -->|OK| AUTH_OK["Token OK"]:::output
        AUTH_CHECK -->|Fail| AUTH_FAIL["Error + actionable steps"]:::error
        AUTH_FLOW --> AUTH_DONE
    end

    %% GOFLARE DEPLOY
    subgraph DEPLOY_FLOW ["goflare deploy"]
        direction TB
        CMD_DEPLOY --> D_ENV["Read .env"]:::env
        D_ENV --> D_AUTH["Auth() — keyring → CLOUDFLARE_API_TOKEN → prompt"]:::auth
        D_AUTH -->|Error| D_AUTH_ERR["Error: token inválido"]:::error
        D_AUTH -->|OK| D_TARGETS["Detect deploy targets (config-driven)"]:::detect

        D_TARGETS --> D_HAS_ENTRY{"cfg.Entry set?"}:::detect
        D_TARGETS --> D_HAS_PUBLIC{"cfg.PublicDir set?"}:::detect

        D_HAS_ENTRY -->|Sí| W_UPLOAD["PUT /workers/scripts/WORKER_NAME\nmultipart: edge.js + edge.wasm"]:::cf
        D_HAS_ENTRY -->|No| W_SKIP["Worker: skip"]:::detect

        W_UPLOAD -->|Error| W_ERR["Worker deploy failed — record error"]:::error
        W_UPLOAD -->|OK| W_SUB["GET /workers/subdomain → URL real"]:::cf
        W_SUB --> W_DONE["Worker live"]:::cf

        D_HAS_PUBLIC -->|Sí| P_CHECK{"Pages project exists?"}:::detect
        D_HAS_PUBLIC -->|No| P_SKIP["Pages: skip"]:::detect
        P_CHECK -->|No| P_CREATE["POST /pages/projects name=PROJECT_NAME"]:::cfpages
        P_CHECK -->|Sí| P_UPLOAD["Upload PublicDir → Pages deployment"]:::cfpages
        P_CREATE --> P_UPLOAD
        P_UPLOAD -->|Error| P_ERR["Pages deploy failed — record error"]:::error
        P_UPLOAD -->|OK| P_DOMAIN{"cfg.Domain set?"}:::cfpages
        P_DOMAIN -->|Sí| P_DOMAINSET["POST /pages/projects/PROJECT_NAME/domains"]:::cfpages
        P_DOMAIN -->|No| P_DONE["Pages live on pages.dev"]:::cfpages
        P_DOMAINSET --> P_DONE

        W_DONE --> RESULT["Deploy summary: successes + errors"]:::output
        W_SKIP --> RESULT
        W_ERR --> RESULT
        P_DONE --> RESULT
        P_SKIP --> RESULT
        P_ERR --> RESULT
    end

    subgraph CONFIG ["Config vs Secrets"]
        direction LR
        C_ENV[".env config: PROJECT_NAME, ACCOUNT_ID, DOMAIN, WORKER_NAME, ENTRY, PUBLIC_DIR"]:::env
        C_KEY["Keyring secrets: goflare/PROJECT_NAME = CLOUDFLARE_API_TOKEN"]:::key
        C_CI["CI/CD: CLOUDFLARE_API_TOKEN env var (no se guarda en keyring)"]:::auth
    end

    subgraph DEPS ["Go-only Dependencies"]
        direction LR
        D1["cfClient direct HTTP — baseURL injectable for tests"]:::sdk
        D2["go-keyring OS secrets"]:::key
        D3["bufio.Scanner stdlib .env parser"]:::env
        D4["tinywasm/client WASM build + JS templates"]:::compile
        D5["httptest.Server mock in tests"]:::output
    end
```
