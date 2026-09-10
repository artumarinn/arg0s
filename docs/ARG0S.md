# ARG0S — Especificación Maestra del Proyecto

> **Versión:** v0.2 (post-review)
> **Fecha:** 10 de septiembre de 2026
> **Estado:** Diseño cerrado / listo para implementación de Fase 0
> **Autor:** proyecto personal de un AI Engineer
> **Documento anterior congelado como:** `ARG0S_SPEC_v0.1` (histórico, no implementar)

---

## 0. CÓMO USAR ESTE DOCUMENTO (leer primero)

Este archivo es la **única fuente de verdad** del proyecto. Está escrito para que un agente de código (Claude Code, Codex, etc.) pueda leerlo completo y empezar a trabajar sin contexto adicional.

### Instrucciones para el agente de código

1. **No implementes todo el documento.** Las secciones 5 a 18 describen el sistema *final*. La sección 21 dice **qué construir ahora**. Si hay conflicto entre "el sistema final" y "la fase actual", gana la fase actual.
2. **Respetá los anti-objetivos** (sección 20). Hay cosas explícitamente prohibidas en las fases tempranas. No las agregues "porque ya que estamos".
3. **Cada fase tiene un Definition of Done** verificable. No pases a la fase siguiente sin cumplirlo.
4. **Si una decisión no está en este documento, preguntá antes de inventarla.** No agregues dependencias, subsistemas ni abstracciones nuevas sin que estén acá.
5. **Escribí tests desde la Fase 0.** No al final.
6. **Los IDs de modelos cambian.** Los que aparecen en los ejemplos de config son *placeholders realistas*, no verdad absoluta. La config debe leerlos de archivo, nunca hardcodearlos.

### Ubicación recomendada del archivo

```
arg0s/
├── CLAUDE.md          → copia o symlink de este documento
├── docs/
│   └── ARG0S.md       → este documento (fuente)
│   └── decisions/     → ADRs incrementales
```

---

## 1. QUÉ ES ARG0S

**Arg0s es un motor personal de ejecución de tareas de IA**, orientado a terminal, para uso individual.

No es un chatbot. No es un wrapper de CLIs. Es un sistema que recibe una tarea, decide **cómo** ejecutarla (qué modelo, qué estrategia, qué contexto), la ejecuta, la evalúa y guarda evidencia de todo lo que pasó.

### Definición en una frase

> Un orquestador local que enruta tareas a modelos heterogéneos (cloud y locales), aplica estrategias de ejecución adaptativas (single / fusion / adversarial judgment), compila contexto mínimo desde memoria y código, y mide empíricamente su propio rendimiento en calidad, costo y latencia.

### Qué NO es

| No es | Por qué importa |
|---|---|
| Un producto comercial | Sin multi-usuario, sin billing, sin compliance, sin auth |
| Un fork de Gentle-AI | Inspiración conceptual únicamente; core propio |
| Un gateway tipo LiteLLM | Arg0s decide *qué* ejecutar, no solo *dónde* enrutar HTTP |
| Un reemplazo de Claude Code | Arg0s **orquesta** runtimes de agentes, no los reemplaza |
| Un IDE | La TUI es una vista sobre el motor, no el motor |

### El objetivo real (honesto)

Dos objetivos simultáneos, y conviene tenerlos separados:

1. **Utilidad práctica:** una herramienta que uses todos los días y que te ahorre dinero y tiempo respecto de usar un solo modelo frontier para todo.
2. **Valor de portfolio:** demostrar competencia real en orchestration, context engineering, evaluation, model economics y systems design en Go. Esto se logra con **métricas medidas**, no con features listadas.

---

## 2. PRINCIPIOS DE DISEÑO (no negociables)

Estos principios resuelven ambigüedades. Cuando dudes, aplicá el principio.

### P1 — El motor es independiente de la interfaz
La TUI es un cliente. Todo lo que hace la TUI se debe poder hacer desde CLI headless y desde scripts. **Ninguna lógica de negocio vive dentro de Bubbletea.**

### P2 — Todo es una Task
Una pregunta trivial, un refactor con Judgment Day y un análisis de repo son la misma estructura: `Task → Strategy → Execution → Result`. No hay "modos" con caminos de código separados.

### P3 — Las estrategias son intercambiables
Router, Fusion y Judgment no son features paralelas: son implementaciones de `Strategy`. Se pueden componer y anidar.

### P4 — El Executor es el único que habla con modelos
Router, Fusion, Judgment, Context Compiler: ninguno sabe qué es Gemini. Todos llaman `executor.Execute(ctx, task)`.

### P5 — El contexto se compila, no se acumula
Nunca se manda "todo lo que hay". Un componente explícito decide qué entra al prompt bajo un presupuesto de tokens.

### P6 — El grafo localiza, los archivos son la verdad
Code Graph responde "dónde está esto" y "quién lo llama". El contenido siempre se lee del archivo real.

### P7 — La memoria es retrieval, no inyección
Nunca "toda la memoria al prompt". Query → retrieval → ranking → budget → contexto.

### P8 — El consenso de jueces es señal, no verdad
Dos jueces de acuerdo aumentan la confianza. No la garantizan. Todo finding requiere evidencia (archivo:línea).

### P9 — El humano autoriza lo destructivo
Arg0s nunca hace commit, push, `rm`, ni ejecuta shell arbitrario sin confirmación explícita. El veredicto de Judgment Day es evidencia, no autorización.

### P10 — Todo se mide o no existe
Cada claim de mejora (tokens ahorrados, calidad de fusion, precisión de jueces) se mide contra un baseline propio. **No se copian cifras de papers ni de READMEs ajenos al spec.**

### P11 — Local-first cuando sea posible
Si una tarea la puede resolver Qwen local con calidad suficiente, va a Qwen local. Cloud es escalamiento, no default.

### P12 — Fallar barato
Antes de gastar 8 llamadas a modelos, verificar si 1 alcanza. Fail-fast en todas las estrategias multi-llamada.

---

## 3. DECISIONES YA TOMADAS

### 3.1 Stack (cerrado)

| Capa | Elección | Notas |
|---|---|---|
| Lenguaje | **Go 1.23+** | Concurrencia nativa, binario único, buen fit para orquestación |
| CLI | **Cobra** | Comandos, flags, subcomandos |
| TUI | **Bubbletea + Lipgloss + Bubbles** | Solo en la capa cliente |
| Persistencia | **SQLite** vía `modernc.org/sqlite` | **Driver puro Go, sin CGO.** Crítico |
| Config | **YAML** (`gopkg.in/yaml.v3`) en `~/.arg0s/` | Human-editable |
| Logging | `log/slog` (stdlib) | Structured logging, salida JSON opcional |
| HTTP | `net/http` stdlib + retry propio | Sin SDKs pesados de proveedores |
| Testing | stdlib `testing` + `testify/require` | Golden files para prompts |
| Migraciones | SQL embebido con `embed.FS` | Sin herramienta externa |

### 3.2 Identidad

| Item | Valor |
|---|---|
| Nombre | **Arg0s** |
| Tagline | *Adaptive Reasoning & Orchestration System* |
| Binario motor | `arg0sd` |
| Binario cliente | `arg0s` |
| Home | `~/.arg0s/` |

### 3.3 Tema visual (tokens de color)

```
background      #0a0a0a    fondo principal
surface         #111827    paneles
border          #1e3a5f    bordes inactivos
border_active   #3b82f6    bordes activos
accent          #3b82f6    azul primario
accent_bright   #60a5fa    azul claro / highlights
text            #e2e8f0    texto principal
text_secondary  #64748b    labels, metadata
success         #22c55e    ✓, APPROVED
warning         #f59e0b    warnings, suspect findings
critical        #ef4444    CRITICAL, ESCALATED
muted           #334155    separadores
```

Tipografía objetivo: cualquier monoespaciada con buen soporte de box-drawing (JetBrains Mono, Fira Code, IosevkaTerm).

### 3.4 Decisiones descartadas explícitamente

Estas se evaluaron y se rechazaron. **No reintroducir sin ADR nuevo.**

| Descartado | Razón |
|---|---|
| Forkear Gentle-AI | Overkill; el core propio es el valor del proyecto |
| LiteLLM como capa central | Dependencia externa para un problema que Arg0s debe entender internamente. Queda como *adapter opcional* en V6+ |
| `go-tree-sitter` para Code Graph | Requiere CGO + mantener gramáticas por lenguaje. **Reemplazado por LSP/SCIP** (ver sección 13) |
| PostgreSQL / Redis / Neo4j / Qdrant | Innecesarios para uso personal. SQLite alcanza |
| Magnitude como dependencia arquitectónica | Opcional, detrás de la interfaz `Provider`. No se diseña alrededor de él |
| Engram como dependencia obligatoria | Primero `MemoryStore` interface, después implementación |
| Claude API en el router | Claude Pro ≠ API. Claude Code entra como **Runtime**, no como Provider |
| Claim "35–120× de ahorro de tokens" | Cifra de terceros, no medida acá. Reemplazada por benchmark propio |
| "Dos jueces = verdad" | Reformulado: consenso = señal de confianza sujeta a evidencia |
| TUI como punto de partida | El core headless va primero |

---

## 4. ARQUITECTURA

### 4.1 Vista de capas

```
┌──────────────────────────────────────────────────────────┐
│  CLIENTES                                                │
│  arg0s (TUI Bubbletea)  │  arg0s CLI  │  scripts / hooks │
└────────────────────────┬─────────────────────────────────┘
                         │ IPC (Unix socket, JSON-RPC)
┌────────────────────────▼─────────────────────────────────┐
│  arg0sd — DAEMON                                         │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │ ORCHESTRATOR                                       │  │
│  │  selecciona y ejecuta una Strategy                 │  │
│  └───────────────────┬────────────────────────────────┘  │
│                      │                                   │
│  ┌───────────────────▼────────────────────────────────┐  │
│  │ STRATEGIES                                         │  │
│  │  Direct │ Router │ Fusion │ Judgment │ Pipeline    │  │
│  └───────────────────┬────────────────────────────────┘  │
│                      │                                   │
│  ┌───────────────────▼────────────────────────────────┐  │
│  │ CONTEXT COMPILER    (qué entra al prompt)          │  │
│  │   ← Memory  ← Code Graph  ← Skills  ← Artifacts    │  │
│  └───────────────────┬────────────────────────────────┘  │
│                      │                                   │
│  ┌───────────────────▼────────────────────────────────┐  │
│  │ POLICY ENGINE       (¿esto está permitido?)        │  │
│  └───────────────────┬────────────────────────────────┘  │
│                      │                                   │
│  ┌───────────────────▼────────────────────────────────┐  │
│  │ EXECUTION ENGINE                                   │  │
│  │  retry │ timeout │ cancel │ stream │ token count   │  │
│  └───────┬────────────────┬─────────────────┬─────────┘  │
│          ▼                ▼                 ▼            │
│      PROVIDERS        RUNTIMES            TOOLS          │
│    Gemini│Ollama│OR  Claude Code│Codex   fs│exec│http    │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │ EVENT BUS  →  TELEMETRY  →  SQLite                 │  │
│  └────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────┘
```

### 4.2 Distinción crítica: Model / Provider / Runtime

Esta separación evita el 80% del dolor arquitectónico del proyecto.

| Concepto | Qué es | Ejemplos | Interfaz |
|---|---|---|---|
| **Model** | Metadata: capacidades, ventana, costo | `gemini-2.5-flash`, `qwen2.5-coder:14b` | struct de datos |
| **Provider** | Endpoint que ejecuta inferencia | Gemini API, Ollama API, OpenRouter | `Provider` |
| **Runtime** | Agente completo con su propio loop, tools y filesystem | Claude Code, Codex CLI, Gemini CLI | `Runtime` |

**Claude Code no es un modelo.** Es un agente que ya tiene su propio contexto, sus tools y su ciclo. Arg0s le delega una tarea entera y recibe un resultado. Nunca se lo trata como un endpoint de chat.

### 4.3 Árbol de directorios

```
arg0s/
├── cmd/
│   ├── arg0s/                 # cliente: CLI + TUI
│   │   └── main.go
│   └── arg0sd/                # daemon: motor
│       └── main.go
│
├── internal/
│   ├── core/                  # tipos de dominio, sin dependencias
│   │   ├── task.go
│   │   ├── result.go
│   │   ├── model.go
│   │   ├── message.go
│   │   ├── artifact.go
│   │   ├── usage.go
│   │   └── errors.go
│   │
│   ├── config/                # carga, validación, defaults
│   │   ├── config.go
│   │   ├── providers.go
│   │   ├── models.go
│   │   ├── roles.go
│   │   ├── validate.go
│   │   └── defaults.go
│   │
│   ├── execution/             # Execution Engine
│   │   ├── executor.go
│   │   ├── retry.go
│   │   ├── timeout.go
│   │   ├── stream.go
│   │   └── accounting.go
│   │
│   ├── providers/
│   │   ├── provider.go        # interfaz + registry
│   │   ├── gemini/
│   │   ├── ollama/
│   │   ├── openrouter/
│   │   └── mock/              # provider determinista para tests
│   │
│   ├── runtimes/              # FASE 6 — no antes
│   │   ├── runtime.go
│   │   ├── claudecode/
│   │   └── codex/
│   │
│   ├── orchestrator/
│   │   ├── orchestrator.go
│   │   └── strategies/
│   │       ├── strategy.go
│   │       ├── direct.go
│   │       ├── router.go
│   │       ├── fusion.go
│   │       └── judgment.go
│   │
│   ├── router/
│   │   ├── router.go
│   │   ├── profile.go         # TaskProfile
│   │   ├── heuristics.go
│   │   └── scoring.go
│   │
│   ├── fusion/
│   │   ├── fusion.go
│   │   ├── divergence.go      # medición de desacuerdo
│   │   └── synthesizer.go
│   │
│   ├── judgment/
│   │   ├── judgment.go
│   │   ├── snapshot.go
│   │   ├── judge.go
│   │   ├── ledger.go
│   │   ├── merge.go
│   │   ├── fix.go
│   │   └── prompts/
│   │
│   ├── contextc/              # Context Compiler
│   │   ├── compiler.go
│   │   ├── budget.go
│   │   ├── retrieval.go
│   │   ├── ranking.go
│   │   └── source.go
│   │
│   ├── memory/                # FASE 3
│   │   ├── store.go           # MemoryStore interface
│   │   ├── sqlite/
│   │   └── engram/            # adapter opcional
│   │
│   ├── codegraph/             # FASE 3
│   │   ├── graph.go
│   │   ├── lsp/               # cliente LSP
│   │   ├── index.go
│   │   └── query.go
│   │
│   ├── skills/                # FASE 5
│   │   ├── registry.go
│   │   ├── skill.go
│   │   └── inject.go
│   │
│   ├── policy/
│   │   ├── policy.go
│   │   ├── rules.go
│   │   └── confirm.go
│   │
│   ├── session/
│   │   ├── session.go
│   │   ├── run.go
│   │   └── history.go
│   │
│   ├── telemetry/
│   │   ├── events.go
│   │   ├── bus.go
│   │   ├── metrics.go
│   │   └── tracing.go
│   │
│   ├── storage/
│   │   ├── db.go
│   │   ├── migrations/
│   │   └── queries/
│   │
│   ├── ipc/                   # protocolo daemon ↔ cliente
│   │   ├── protocol.go
│   │   ├── server.go
│   │   └── client.go
│   │
│   └── tui/                   # FASE 4 — solo presentación
│       ├── app.go
│       ├── theme.go
│       ├── keys.go
│       └── panels/
│
├── skills/                    # skills nativas embebidas
│   └── native/
│       └── judgment-day/
│
├── testdata/                  # golden files, fixtures
├── bench/                     # benchmarks propios
├── docs/
│   ├── ARG0S.md
│   └── decisions/             # ADR-001.md, ADR-002.md...
├── Makefile
├── go.mod
└── README.md
```

---

## 5. MODELO DE DOMINIO

Todo en `internal/core/`. **Este paquete no importa nada de otros paquetes internos.**

### 5.1 Task

```go
package core

type TaskID string

type Task struct {
    ID        TaskID
    SessionID SessionID
    ParentID  *TaskID          // para sub-tasks (jueces, fixes)

    Prompt    string
    Messages  []Message        // historial si aplica
    Profile   TaskProfile      // llenado por el router
    Context   *CompiledContext // llenado por el context compiler

    Strategy  StrategyName     // vacío = decide el orchestrator
    Constraints Constraints

    CreatedAt time.Time
    Metadata  map[string]any
}

type Constraints struct {
    MaxCostUSD   float64
    MaxTokens    int
    MaxDuration  time.Duration
    MaxRounds    int
    AllowCloud   bool
    AllowLocal   bool
    RequireJudgment bool
}
```

### 5.2 TaskProfile

Reemplaza el simple SIMPLE/MEDIUM/COMPLEX. En Fase 2 solo se llenan `Type` y `Complexity`; el resto queda con defaults.

```go
type TaskProfile struct {
    Type        TaskType    // qa, coding, refactor, review, architecture, research, creative
    Complexity  Level       // low, medium, high
    Reasoning   Level       // cuánta cadena de razonamiento requiere
    ContextSize Level       // cuánto contexto necesita
    Multimodal  bool
    Latency     Priority    // low = necesita respuesta rápida
    Privacy     Privacy     // public, private, secret (secret = nunca cloud)
    CostBudget  Priority
    Confidence  float64     // qué tan seguro está el clasificador (0..1)
}
```

### 5.3 Result

```go
type Result struct {
    TaskID    TaskID
    Content   string
    Artifacts []Artifact       // patches, archivos, JSON estructurado
    Usage     Usage
    Verdict   *Verdict         // solo si pasó por Judgment
    Strategy  StrategyName
    ModelRuns []ModelRun       // trazabilidad de cada llamada
    StartedAt time.Time
    EndedAt   time.Time
    Err       error
}

type ModelRun struct {
    ModelID   string
    Role      Role             // generator, judge_a, judge_b, synthesizer, fix, classifier
    Usage     Usage
    Latency   time.Duration
    Attempts  int
    Err       error
}

type Usage struct {
    InputTokens  int
    OutputTokens int
    CachedTokens int
    CostUSD      float64
}
```

### 5.4 Interfaces centrales

Mantenerlas mínimas. Si una interfaz crece más de 3 métodos, revisar el diseño.

```go
// El único que habla con modelos.
type Executor interface {
    Execute(ctx context.Context, req Request) (Response, error)
    Stream(ctx context.Context, req Request) (<-chan Chunk, error)
}

// Endpoint de inferencia.
type Provider interface {
    Name() string
    Models(ctx context.Context) ([]Model, error)
    Complete(ctx context.Context, req Request) (Response, error)
    Stream(ctx context.Context, req Request) (<-chan Chunk, error)
}

// Agente externo con su propio loop.
type Runtime interface {
    Name() string
    Available(ctx context.Context) error
    Run(ctx context.Context, req RuntimeRequest) (RuntimeResult, error)
}

// Modo de ejecución.
type Strategy interface {
    Name() StrategyName
    Run(ctx context.Context, task *Task) (*Result, error)
}

// Fuente de contexto.
type ContextSource interface {
    Name() string
    Retrieve(ctx context.Context, q Query, budget int) ([]Fragment, error)
}

// Memoria.
type MemoryStore interface {
    Store(ctx context.Context, m Memory) error
    Query(ctx context.Context, q MemoryQuery) ([]Memory, error)
    Forget(ctx context.Context, id string) error
}
```

---

## 6. CONFIGURACIÓN DE PROVEEDORES Y MODELOS

**Esta es la sección más importante para el uso diario.** Todo se define en archivos, nada se hardcodea.

### 6.1 Estructura de `~/.arg0s/`

```
~/.arg0s/
├── config.yaml           # config principal
├── models.yaml           # catálogo de modelos (editable, actualizable)
├── policies.yaml         # reglas de policy engine
├── .env                  # API keys (NUNCA en config.yaml, NUNCA en git)
├── arg0s.db              # SQLite: sesiones, runs, eventos, memoria, ledgers
├── skills/
│   ├── native/
│   └── external/
├── projects/
│   └── <hash-del-path>/  # estado por proyecto: graph, memoria local
├── cache/
├── logs/
│   └── arg0sd.log
└── daemon.sock           # unix socket
```

`~/.arg0s/` se resuelve así: si la variable de entorno `ARG0S_HOME` está
seteada, se usa ese path en vez de `~/.arg0s/`. Es el mecanismo oficial
para dos casos — no hay una segunda forma de lograrlos:

- **Tests aislados.** Cada test setea su propio `ARG0S_HOME` a un
  directorio temporal (`t.TempDir()`), así nunca toca el `~/.arg0s/`
  real de quien corre la suite.
- **Más de un perfil.** Ej: `ARG0S_HOME=~/.arg0s-work arg0s doctor` para
  separar config/memoria de trabajo de la personal, sin flags nuevos en
  cada comando.

`arg0s init --home <path>` (sección 21) hace lo mismo pero para un solo
comando, sin tocar el entorno.

### 6.2 Precedencia de configuración

De menor a mayor prioridad:

```
1. defaults compilados en el binario
2. ~/.arg0s/config.yaml    (o $ARG0S_HOME/config.yaml si está seteada)
3. ./.arg0s.yaml           (config por proyecto, si existe)
4. variables de entorno    (ARG0S_*)
5. flags de CLI
```

`ARG0S_HOME` no es una capa de precedencia más — es una reubicación del
`~/.arg0s/` que usan TODAS las capas de arriba (la 1 no aplica porque
son defaults compilados, no un archivo). Se resuelve antes que
cualquier otra cosa.

### 6.3 `config.yaml` completo

```yaml
version: 1

# ─────────────────────────────────────────────────────────────
# GENERAL
# ─────────────────────────────────────────────────────────────
general:
  default_strategy: router        # direct | router | fusion | judgment
  theme: arg0s-dark
  log_level: info                 # debug | info | warn | error
  telemetry: true                 # eventos locales a SQLite (nunca sale de la máquina)
  editor: nvim

daemon:
  socket: ~/.arg0s/daemon.sock
  autostart: true                 # el cliente levanta el daemon si no corre
  idle_shutdown: 30m              # 0 = nunca

# ─────────────────────────────────────────────────────────────
# PROVIDERS — endpoints de inferencia
# ─────────────────────────────────────────────────────────────
# Cada provider define CÓMO se accede. Los modelos van en models.yaml.
providers:

  gemini:
    enabled: true
    type: gemini
    base_url: https://generativelanguage.googleapis.com/v1beta
    api_key_env: GEMINI_API_KEY   # se lee de ~/.arg0s/.env o del entorno
    timeout: 120s
    max_retries: 3
    retry_backoff: exponential    # exponential | linear | none
    rate_limit:
      requests_per_minute: 60
      concurrent: 4

  ollama:
    enabled: true
    type: ollama
    base_url: http://localhost:11434
    api_key_env: ""               # local, sin key
    timeout: 300s                 # los locales son lentos
    max_retries: 1
    rate_limit:
      concurrent: 1               # una sola inferencia local a la vez
    options:
      keep_alive: 10m
      num_ctx: 32768

  openrouter:
    enabled: false                # activar cuando haya key
    type: openai_compatible
    base_url: https://openrouter.ai/api/v1
    api_key_env: OPENROUTER_API_KEY
    timeout: 180s
    max_retries: 2
    headers:
      HTTP-Referer: https://localhost/arg0s
      X-Title: Arg0s
    rate_limit:
      requests_per_minute: 30
      concurrent: 3

  # Plantilla para cualquier endpoint OpenAI-compatible
  # (vLLM, LM Studio, llama.cpp server, Groq, Together, etc.)
  custom_openai:
    enabled: false
    type: openai_compatible
    base_url: http://localhost:8000/v1
    api_key_env: CUSTOM_API_KEY
    timeout: 120s

# ─────────────────────────────────────────────────────────────
# RUNTIMES — agentes externos (FASE 6, dejar declarado pero disabled)
# ─────────────────────────────────────────────────────────────
runtimes:

  claude_code:
    enabled: false
    type: claude_code
    binary: claude
    args: ["-p"]                  # modo print / no interactivo
    working_dir: "."
    timeout: 600s
    env_passthrough: [PATH, HOME, ANTHROPIC_API_KEY]
    # Claude Pro NO da API key. Este runtime usa la sesión del CLI, no la API.

  codex:
    enabled: false
    type: codex
    binary: codex
    timeout: 600s

  gemini_cli:
    enabled: false
    type: gemini_cli
    binary: gemini
    timeout: 600s

# ─────────────────────────────────────────────────────────────
# ROLES — qué modelo usa cada función del sistema
# ─────────────────────────────────────────────────────────────
# Esta es la sección que más vas a tocar. Cada rol apunta a un
# model_id definido en models.yaml.
roles:

  classifier:                     # clasifica el TaskProfile (router)
    model: qwen-coder-7b
    fallback: gemini-flash
    temperature: 0.0
    max_tokens: 512

  generator:                      # respuesta principal por defecto
    model: gemini-flash
    fallback: qwen-coder-14b
    temperature: 0.7

  synthesizer:                    # fusiona/elige entre N respuestas
    model: gemini-pro
    fallback: gemini-flash
    temperature: 0.3

  judge_a:
    model: gemini-flash
    temperature: 0.0
    max_tokens: 4096
    perspective: correctness      # correctness | security | architecture | performance

  judge_b:
    model: qwen-coder-14b
    temperature: 0.0
    max_tokens: 4096
    perspective: security

  judge_c:                        # opcional, tercer juez para desempate
    enabled: false
    model: gemini-pro
    perspective: architecture

  fix_agent:                      # aplica correcciones acotadas
    model: gemini-pro
    fallback: gemini-flash
    temperature: 0.2

  summarizer:                     # comprime contexto e historial
    model: qwen-coder-7b
    temperature: 0.0

# ─────────────────────────────────────────────────────────────
# ROUTER
# ─────────────────────────────────────────────────────────────
router:
  mode: heuristic                 # heuristic | classifier | hybrid | learned
  classifier_threshold: 0.6       # bajo esto, escalar de tier

  # Mapeo tier → modelo. El router elige tier, esto resuelve el modelo.
  tiers:
    simple:
      models: [qwen-coder-7b]
      max_cost_usd: 0.0
    medium:
      models: [gemini-flash, qwen-coder-14b]
      max_cost_usd: 0.01
    complex:
      models: [gemini-pro]
      max_cost_usd: 0.10

  # Overrides por tipo de tarea
  overrides:
    - when: { privacy: secret }
      force_tier: simple
      force_local: true
    - when: { type: architecture }
      force_tier: complex
    - when: { type: review, complexity: high }
      force_strategy: judgment

# ─────────────────────────────────────────────────────────────
# FUSION
# ─────────────────────────────────────────────────────────────
fusion:
  enabled: true
  adaptive: true                  # solo escala si hay desacuerdo real
  models: [gemini-flash, qwen-coder-14b]
  max_models: 3
  parallel: true

  divergence:
    method: hybrid                # lexical | semantic | judge | hybrid
    threshold: 0.15               # sobre esto → sintetizar; bajo esto → devolver la primera
    escalate_threshold: 0.45      # sobre esto → escalar a Judgment Day

  synthesis:
    mode: select_best             # select_best | merge | critique_and_merge
    # select_best evita el "efecto promedio" de código plano/defensivo

# ─────────────────────────────────────────────────────────────
# JUDGMENT DAY
# ─────────────────────────────────────────────────────────────
judgment:
  enabled: true
  mode: fail_fast                 # fail_fast | parallel
  # fail_fast: corre judge_a primero; judge_b solo si A encuentra
  # algo CRITICAL/SEVERE o si la tarea es de alta complejidad.
  # Ahorra ~50% de llamadas en cambios triviales.

  max_rounds: 2
  blind: true                     # los jueces no ven el output del otro
  require_evidence: true          # todo finding necesita file:line o se descarta

  severities: [CRITICAL, SEVERE, MODERATE, MINOR, INFO]
  fix_severities: [CRITICAL, SEVERE]   # solo se corrige esto
  fix_requires_confirmation: [CRITICAL, SEVERE]

  consensus:
    confirmed_by: 2               # nº de jueces que deben coincidir
    on_contradiction: escalate    # escalate | trust_higher_severity | ask_human

  verdicts: [APPROVED, APPROVED_WITH_WARNINGS, ESCALATED, FAILED]

# ─────────────────────────────────────────────────────────────
# CONTEXT COMPILER
# ─────────────────────────────────────────────────────────────
context:
  default_budget_tokens: 16000
  reserve_output_tokens: 4000
  budget_split:                   # proporción del budget por fuente
    prompt: 0.10
    code: 0.45
    graph: 0.10
    memory: 0.20
    skills: 0.10
    artifacts: 0.05
  compress_when_over: true
  compression_model: summarizer

# ─────────────────────────────────────────────────────────────
# MEMORY
# ─────────────────────────────────────────────────────────────
memory:
  enabled: true
  backend: sqlite                 # sqlite | engram
  scope: project                  # project | global | session
  max_fragments_per_query: 8
  min_relevance: 0.35
  decay:
    enabled: true
    revalidate_after: 30d         # marcar como "stale", no borrar
  categories: [DECISION, FACT, BUG, PREFERENCE, ARCHITECTURE, TODO, LESSON]

# ─────────────────────────────────────────────────────────────
# CODE GRAPH
# ─────────────────────────────────────────────────────────────
codegraph:
  enabled: true
  backend: lsp                    # lsp | scip | none
  index_on_open: true
  watch: false                    # reindex incremental en cambios
  max_file_size_kb: 512
  ignore: [vendor/, node_modules/, .git/, dist/, build/, "*.min.js"]
  servers:
    go: { command: gopls, args: [serve] }
    python: { command: pyright-langserver, args: ["--stdio"] }
    typescript: { command: typescript-language-server, args: ["--stdio"] }
    rust: { command: rust-analyzer, args: [] }

# ─────────────────────────────────────────────────────────────
# COSTOS Y LÍMITES
# ─────────────────────────────────────────────────────────────
limits:
  daily_cost_usd: 2.00
  per_task_cost_usd: 0.25
  per_task_max_calls: 8           # corta cadenas fusion+judgment desbocadas
  warn_at_percent: 80
  on_exceed: block                # block | warn | downgrade_to_local

# ─────────────────────────────────────────────────────────────
# PRESETS — perfiles rápidos de costo/calidad
# ─────────────────────────────────────────────────────────────
presets:
  cheap:
    default_strategy: direct
    roles: { generator: qwen-coder-7b, judge_a: qwen-coder-7b, judge_b: qwen-coder-14b }
    limits: { per_task_cost_usd: 0.0 }
  balanced:
    default_strategy: router
  quality:
    default_strategy: fusion
    roles: { generator: gemini-pro, synthesizer: gemini-pro }
    judgment: { enabled: true, mode: parallel }

active_preset: balanced
```

### 6.4 `models.yaml` — catálogo de modelos

Separado de `config.yaml` porque cambia con otra frecuencia y se puede regenerar automáticamente.

```yaml
version: 1

# IMPORTANTE: los IDs de modelo cambian seguido. `arg0s models sync`
# consulta cada provider y actualiza este archivo. Los precios se
# ingresan a mano porque los providers no siempre los exponen.

models:

  # ── Gemini ───────────────────────────────────────────────
  gemini-flash:
    provider: gemini
    provider_model_id: gemini-2.5-flash
    context_window: 1000000
    max_output_tokens: 8192
    cost:
      input_per_1m_usd: 0.30
      output_per_1m_usd: 2.50
    capabilities: [text, code, vision, json_mode, tools, streaming]
    strengths: [general, coding, speed]
    tier: medium
    local: false

  gemini-pro:
    provider: gemini
    provider_model_id: gemini-2.5-pro
    context_window: 2000000
    max_output_tokens: 65536
    cost:
      input_per_1m_usd: 1.25
      output_per_1m_usd: 10.00
    capabilities: [text, code, vision, json_mode, tools, streaming, reasoning]
    strengths: [architecture, reasoning, long_context]
    tier: complex
    local: false

  # ── Locales vía Ollama ───────────────────────────────────
  qwen-coder-7b:
    provider: ollama
    provider_model_id: qwen2.5-coder:7b
    context_window: 32768
    max_output_tokens: 4096
    cost: { input_per_1m_usd: 0.0, output_per_1m_usd: 0.0 }
    capabilities: [text, code, json_mode, streaming]
    strengths: [speed, classification, simple_coding]
    tier: simple
    local: true
    hardware:
      min_ram_gb: 8
      recommended_vram_gb: 6

  qwen-coder-14b:
    provider: ollama
    provider_model_id: qwen2.5-coder:14b
    context_window: 32768
    max_output_tokens: 8192
    cost: { input_per_1m_usd: 0.0, output_per_1m_usd: 0.0 }
    capabilities: [text, code, json_mode, streaming]
    strengths: [coding, review, privacy]
    tier: medium
    local: true
    hardware:
      min_ram_gb: 16
      recommended_vram_gb: 12

  # ── OpenRouter (plantilla) ───────────────────────────────
  or-free-large:
    provider: openrouter
    provider_model_id: "<completar-con-models-sync>"
    context_window: 128000
    max_output_tokens: 8192
    cost: { input_per_1m_usd: 0.0, output_per_1m_usd: 0.0 }
    capabilities: [text, code, streaming]
    strengths: [fallback]
    tier: medium
    local: false
    notes: "Modelos free de OpenRouter tienen rate limits agresivos. Solo fallback."

# Aliases: nombres estables que apuntan a modelos concretos.
# Permiten cambiar de modelo sin tocar la config de roles.
aliases:
  fast: qwen-coder-7b
  smart: gemini-pro
  balanced: gemini-flash
  private: qwen-coder-14b
```

### 6.5 `.env` — secretos

```bash
# ~/.arg0s/.env  —  chmod 600, jamás en git
GEMINI_API_KEY=...
OPENROUTER_API_KEY=...
```

Reglas:
- Las API keys **solo** se leen de variables de entorno o de `~/.arg0s/.env`.
- `config.yaml` referencia el *nombre* de la variable (`api_key_env`), nunca el valor.
- `arg0s doctor` verifica permisos del archivo y avisa si son laxos.
- Ninguna key aparece en logs, eventos, ni en la TUI. Redactar siempre como `sk-...abcd`.

### 6.6 Comandos de gestión de proveedores

```bash
arg0s providers list                    # estado, latencia, disponibilidad
arg0s providers test gemini             # ping real con un prompt mínimo
arg0s providers enable openrouter
arg0s providers disable ollama

arg0s models list                       # catálogo con costo y tier
arg0s models list --provider ollama
arg0s models sync                       # consulta providers y actualiza models.yaml
arg0s models test qwen-coder-14b
arg0s models cost gemini-pro --tokens 10000

arg0s roles list                        # qué modelo cumple cada rol
arg0s roles set judge_b qwen-coder-14b
arg0s roles set generator gemini-pro

arg0s preset use cheap
arg0s preset list
```

### 6.7 Validación de configuración

`arg0s doctor` debe verificar, en orden, y reportar cada punto con ✓/✗:

1. `~/.arg0s/` existe con permisos correctos
2. `config.yaml` parsea y valida contra el esquema
3. `models.yaml` parsea; todo `model` referenciado en `roles` y `router.tiers` existe
4. Todo `provider` referenciado por un modelo existe y está `enabled`
5. Cada provider habilitado tiene su API key presente y no vacía
6. Conectividad real: un request mínimo a cada provider habilitado
7. Ollama corriendo y modelos locales descargados (`ollama list`)
8. SQLite abre y las migraciones están al día
9. Binarios de runtimes presentes en PATH (si están enabled)
10. Servidores LSP presentes (si codegraph enabled)
11. Sin roles apuntando a modelos deshabilitados
12. Límites de costo coherentes (`per_task` ≤ `daily`)

Salida esperada:

```
ARG0S DOCTOR

Config
  ✓ ~/.arg0s/config.yaml           valid
  ✓ ~/.arg0s/models.yaml           valid  (6 models, 4 aliases)
  ✓ ~/.arg0s/.env                  mode 0600

Providers
  ✓ gemini                         reachable    142ms
  ✓ ollama                         reachable     18ms   (2 models loaded)
  ⊘ openrouter                     disabled

Roles
  ✓ classifier    → qwen-coder-7b
  ✓ generator     → gemini-flash
  ✓ judge_a       → gemini-flash
  ✓ judge_b       → qwen-coder-14b
  ✗ fix_agent     → gemini-ultra   (model not found in models.yaml)

Storage
  ✓ arg0s.db                       migrations up to date (7/7)

1 error, 0 warnings
```

---

## 7. EXECUTION ENGINE

El único componente que habla con `Provider` y `Runtime`. Todo lo demás pasa por acá.

### Responsabilidades

| Responsabilidad | Detalle |
|---|---|
| Ejecución | Llamada al provider correspondiente al modelo |
| Retry | Backoff exponencial con jitter. Solo errores retryables (5xx, timeout, rate limit) |
| Timeout | Por request, por task y global. `context.Context` en todo |
| Cancelación | Cancelar una task cancela todas sus llamadas en vuelo |
| Rate limiting | Semáforo por provider según `rate_limit.concurrent` |
| Streaming | Canal de `Chunk`; el consumidor decide si lo usa |
| Token accounting | Del provider si lo reporta; estimación local si no |
| Costo | `tokens × precio` desde `models.yaml` |
| Eventos | Emitir `ModelStarted` / `ModelCompleted` / `ModelFailed` al bus |
| Errores | Clasificar en `Retryable`, `Fatal`, `Budget`, `Policy` |

### Reglas

- **Nunca** retry sobre errores de contenido o de autenticación.
- Rate limit (429) se respeta con el `Retry-After` del header si viene.
- Un fallo de provider con `fallback` configurado intenta el fallback **una sola vez** y lo registra como tal en el `ModelRun`.
- El accounting se emite aunque la llamada falle (tokens de input ya se gastaron).

### Estimación de tokens sin provider

Fase 1: heurística `len(text)/4` con corrección por lenguaje. Marcar `Usage.Estimated = true`. Fase 2+: tokenizer real si hace falta precisión.

---

## 8. ROUTER

### Flujo

```
Task
 ↓
[1] Perfilado          → TaskProfile
 ↓
[2] Aplicar overrides  → forzar tier/strategy/local si matchea
 ↓
[3] Policy check       → ¿puede ir a cloud? ¿presupuesto?
 ↓
[4] Selección de tier  → simple | medium | complex
 ↓
[5] Selección de modelo dentro del tier
 ↓
[6] Selección de strategy → direct | fusion | judgment
 ↓
RoutingDecision (registrada como evento, con razón)
```

### Modos de perfilado

| Modo | Cómo | Cuándo |
|---|---|---|
| `heuristic` | Reglas sobre el prompt: longitud, keywords, presencia de código, verbos de acción | Fase 2, default |
| `classifier` | Un modelo barato local devuelve el `TaskProfile` como JSON | Fase 2+ |
| `hybrid` | Heurística primero; si `Confidence < threshold`, se llama al classifier | Recomendado |
| `learned` | Scoring desde performance histórica | Fase 7 |

### Heurísticas iniciales (Fase 2)

```
COMPLEXITY
  low     prompt < 200 chars, sin código, pregunta directa
  medium  200-1000 chars, o menciona archivos, o pide código
  high    > 1000 chars, o menciona arquitectura/refactor/seguridad,
          o involucra > 3 archivos, o pide diseño de sistema

TYPE      keywords: refactor|refactoriza → refactor
                    revisa|review|audita → review
                    diseña|arquitectura  → architecture
                    implementa|escribe   → coding
                    qué|cómo|por qué     → qa

PRIVACY   private por defecto si hay un repo git con remote privado
          secret si el path matchea `policies.yaml: secret_paths`
```

### RoutingDecision (siempre auditable)

```json
{
  "task_id": "tsk_01H...",
  "profile": { "type": "refactor", "complexity": "high", "privacy": "private" },
  "mode": "hybrid",
  "classifier_confidence": 0.81,
  "candidates": ["gemini-pro", "gemini-flash", "qwen-coder-14b"],
  "selected_model": "gemini-pro",
  "selected_strategy": "fusion",
  "reasons": [
    "type=refactor + complexity=high → tier complex",
    "privacy=private permitido: repo no marcado como secret",
    "fusion.adaptive activo y complexity=high"
  ],
  "rejected": [
    { "model": "qwen-coder-14b", "reason": "context_window insuficiente (32k < 48k requeridos)" }
  ]
}
```

Poder responder **"¿por qué eligió ese modelo?"** es un requisito, no un extra.

---

## 9. CONTEXT COMPILER

Decide qué entra realmente al prompt bajo un presupuesto de tokens.

### Flujo

```
Task + TaskProfile
        ↓
  extraer Intent y entidades (archivos, símbolos, conceptos)
        ↓
  ┌───────────┬────────────┬───────────┬───────────┐
  ▼           ▼            ▼           ▼           ▼
Memory    Code Graph    Skills    Artifacts    Files
  │           │            │           │           │
  └───────────┴────────────┴───────────┴───────────┘
        ↓
  ranking por relevancia (score por fuente)
        ↓
  aplicar budget_split
        ↓
  comprimir lo que exceda (summarizer)
        ↓
  CompiledContext  (+ manifiesto de qué se incluyó y qué se descartó)
```

### CompiledContext

```go
type CompiledContext struct {
    Intent      string
    Fragments   []Fragment
    TotalTokens int
    Budget      int
    Manifest    Manifest   // qué entró, qué se descartó y por qué
}

type Fragment struct {
    Source    string    // "memory" | "codegraph" | "skill" | "file" | "artifact"
    Ref       string    // "auth/token.go:40-95" | "ADR-014" | "skill:secure-go"
    Content   string
    Tokens    int
    Relevance float64
}
```

El `Manifest` es obligatorio: permite responder "¿por qué el modelo no vio ese archivo?".

### Regla de oro

Nunca mandar un archivo completo si el grafo puede acotar el rango de líneas relevante. Nunca mandar el grafo en vez del código.

---

## 10. FUSION

### Fusion adaptativa (default)

```
Task
 ↓
Ejecutar modelo primario
 ↓
¿el router marcó incertidumbre alta o complexity=high?
 ├─ NO  → devolver resultado. FIN. (0 llamadas extra)
 └─ SÍ  ↓
Ejecutar modelos secundarios en paralelo
 ↓
Medir divergencia
 ├─ < 0.15  → alta confianza → devolver el primario. FIN.
 ├─ 0.15–0.45 → sintetizar (select_best)
 └─ > 0.45  → desacuerdo fuerte → escalar a Judgment Day
```

Esto es lo que evita que "1 request = 8 inference calls" por default.

### Medición de divergencia

| Método | Cómo | Costo |
|---|---|---|
| `lexical` | Similitud de tokens/AST normalizado | 0 |
| `semantic` | Embeddings locales de las respuestas | ~0 |
| `judge` | Un modelo barato responde "¿estas N respuestas dicen lo mismo?" | 1 llamada barata |
| `hybrid` | lexical primero; si es ambiguo, judge | recomendado |

Para código: comparar el AST normalizado (o el diff estructural) en vez del texto plano. Dos implementaciones idénticas con nombres de variable distintos **no** son divergencia.

### Síntesis: `select_best`, no `merge`

El synthesizer **elige la mejor respuesta completa** o extrae bloques concretos. No promedia texto. Promediar produce código plano y defensivo ("efecto promedio").

Prompt del synthesizer, en esencia:

> Recibís N soluciones candidatas al mismo problema. No las mezcles. Evaluá cada una contra los criterios: corrección, manejo de errores, adherencia a las convenciones del proyecto, simplicidad. Elegí **una** como base. Si otra candidata tiene un bloque estrictamente superior, indicá cuál y por qué. Devolvé JSON: `{"winner": n, "reasoning": "...", "borrowed_blocks": [...]}`.

---

## 11. JUDGMENT DAY

La feature diferencial. El juez **no genera: verifica**.

### 11.1 Snapshots inmutables

```
Snapshot 0  (commit abc123 / hash del contenido)
    ├── Judge A ──┐
    └── Judge B ──┤
                  ▼
              Ledger 0
                  ▼
              Patch 1  (diff explícito, no "el agente editó archivos")
                  ▼
            Snapshot 1
    ├── Judge A ──┐
    └── Judge B ──┤
                  ▼
              Ledger 1
                  ▼
          APPROVED | ESCALATED
```

**Regla:** un juez nunca evalúa "el repo actual". Evalúa un snapshot identificado y congelado. Esto hace la corrida reproducible y auditable.

### 11.2 Fail-fast

```
[1] Judge A (modelo rápido/local/flash)
     ↓
    ¿encontró CRITICAL o SEVERE?  ¿o complexity=high?  ¿o el usuario forzó?
     ├─ NO → APPROVED_WITH_WARNINGS. FIN. (1 llamada)
     └─ SÍ ↓
[2] Judge B (ciego: no ve el output de A)
     ↓
[3] Merge → Ledger
     ↓
[4] Fix acotado (solo confirmed CRITICAL/SEVERE)
     ↓
[5] Re-juicio sobre el nuevo snapshot  (máx. 2 rondas)
```

### 11.3 Ceguera de los jueces

Es esencial y fácil de romper. Reglas:

- Judge B recibe **exactamente** el mismo input que Judge A: el snapshot y la instrucción. Nada del output de A.
- No compartir `session_id` ni historial entre jueces.
- Idealmente modelos de proveedores distintos (`gemini-flash` + `qwen-local`). Dos instancias del mismo modelo **no son independientes** — registrarlo en el ledger como `independence: low`.
- Si `judge_a.model == judge_b.model`, `arg0s doctor` debe emitir un warning.

### 11.4 Perspectivas heterogéneas

Además del modelo distinto, dar a cada juez un foco distinto:

| Juez | Perspectiva | Busca |
|---|---|---|
| A | `correctness` | Bugs lógicos, edge cases, null/error handling, off-by-one |
| B | `security` | Injection, authz/authn, secretos, validación de input, path traversal |
| C (opcional) | `architecture` | Acoplamiento, violación de capas, deuda técnica, convenciones |

### 11.5 Esquema de finding (JSON estricto)

Los jueces devuelven **solo** esto. Si no parsea: 1 reintento con el error incluido, después el finding se descarta y se registra `parse_failure`.

```json
{
  "snapshot_id": "snap_abc123",
  "judge": "A",
  "model": "gemini-2.5-flash",
  "perspective": "correctness",
  "findings": [
    {
      "id": "COR-001",
      "severity": "SEVERE",
      "category": "error_handling",
      "title": "Error de refresh de token silenciado",
      "description": "El error del refresh se descarta, dejando al usuario en estado autenticado con un token inválido.",
      "evidence": [
        { "file": "auth/token.go", "line_start": 81, "line_end": 88, "excerpt_hash": "sha256:..." }
      ],
      "reasoning": "Si RefreshToken devuelve error, la función continúa y retorna el token viejo.",
      "suggested_fix": "Propagar el error y forzar re-autenticación.",
      "confidence": 0.88
    }
  ],
  "summary": { "critical": 0, "severe": 1, "moderate": 2, "minor": 1, "info": 0 },
  "overall_assessment": "El código funciona en el happy path pero tiene manejo de errores insuficiente."
}
```

**`evidence` es obligatorio.** Un finding sin `file:line` verificable se descarta automáticamente (`require_evidence: true`). Esto elimina la mayoría de las alucinaciones.

### 11.6 Merge y ledger

```
Finding de A + Finding de B
         ↓
  ¿misma evidencia (mismo archivo, rangos solapados)?
         ├─ SÍ, misma severidad     → CONFIRMED
         ├─ SÍ, distinta severidad  → CONFIRMED (se toma la mayor) + flag
         └─ NO                      → SUSPECT (solo un juez lo vio)

  ¿A dice "está bien" y B dice "está mal" sobre la misma línea?
         → CONTRADICTION → según config: escalate | ask_human
```

Entrada del ledger:

```json
{
  "ledger_id": "led_001",
  "run_id": "run_184",
  "snapshot_id": "snap_abc123",
  "finding_id": "COR-001",
  "status": "CONFIRMED",
  "severity": "SEVERE",
  "judge_a": { "found": true, "severity": "SEVERE", "confidence": 0.88 },
  "judge_b": { "found": true, "severity": "CRITICAL", "confidence": 0.91 },
  "independence": "high",
  "evidence": ["auth/token.go:81-88"],
  "action": "fix",
  "action_result": "patch_def456",
  "human_verified": null
}
```

El campo `human_verified` (`true`/`false`/`null`) es la base del benchmark de precisión de jueces (sección 17).

### 11.7 Fix agent

- Recibe **solo** los findings CONFIRMED de severidad en `fix_severities`.
- Recibe únicamente los archivos y rangos citados en la evidencia.
- Produce un **patch en formato diff**, no edita archivos directamente.
- No refactoriza nada fuera del alcance del finding. Si lo intenta, el patch se rechaza.
- El patch se aplica a una copia para crear `Snapshot N+1`. El working tree del usuario no se toca sin confirmación.

### 11.8 Veredictos

| Veredicto | Condición |
|---|---|
| `APPROVED` | Sin findings CONFIRMED de CRITICAL/SEVERE |
| `APPROVED_WITH_WARNINGS` | Solo MODERATE/MINOR, o solo SUSPECT |
| `ESCALATED` | CRITICAL/SEVERE persisten tras `max_rounds`, o hubo CONTRADICTION |
| `FAILED` | Error de ejecución, parse failures repetidos, o presupuesto excedido |

**El veredicto no autoriza nada.** No hace commit, no hace merge, no hace push. Es evidencia para que decidas vos.

---

## 12. MEMORY

### Modelo

```go
type Memory struct {
    ID           string
    Project      string
    Category     Category   // DECISION|FACT|BUG|PREFERENCE|ARCHITECTURE|TODO|LESSON
    Title        string
    Content      string
    Confidence   float64
    Source       string     // "user" | "run:184" | "judgment:led_001"
    Refs         []string   // archivos, símbolos, commits relacionados
    CreatedAt    time.Time
    LastVerified time.Time
    Stale        bool
    Embedding    []float32  // opcional
}
```

### Reglas

1. **Retrieval, no inyección.** Query → candidatos → ranking → filtro de relevancia → budget.
2. **La memoria caduca.** Pasado `revalidate_after`, se marca `stale`, no se borra. Una memoria stale se puede incluir pero etiquetada como tal en el prompt.
3. **Toda memoria tiene fuente.** Si vino de una corrida, el `run_id` queda registrado.
4. **`MemoryStore` es una interfaz.** SQLite es la implementación default; Engram es un adapter opcional que se agrega después sin tocar el resto del sistema.
5. Escritura automática de memoria solo desde: findings CONFIRMED, decisiones explícitas del usuario, y errores repetidos. **No** guardar cada respuesta.

---

## 13. CODE GRAPH

### Decisión: LSP/SCIP, no tree-sitter manual

Escribir un extractor AST→SQLite con `go-tree-sitter` implica CGO y mantener gramáticas por lenguaje. Los servidores LSP (`gopls`, `pyright`, `tsserver`, `rust-analyzer`) ya resuelven símbolos, definiciones y referencias, y están mantenidos por otros.

```
Arg0s
  ↓ LSP client (JSON-RPC sobre stdio)
gopls / pyright / tsserver
  ↓
symbols, definitions, references, call hierarchy
  ↓
normalización → SQLite graph
```

Fallback si no hay LSP para un lenguaje: indexado por regex/heurística, marcado con `confidence: low`.

### Esquema del grafo

```sql
CREATE TABLE symbols (
    id          TEXT PRIMARY KEY,
    project     TEXT NOT NULL,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL,   -- function|method|type|interface|var|const|package
    file        TEXT NOT NULL,
    line_start  INTEGER NOT NULL,
    line_end    INTEGER NOT NULL,
    signature   TEXT,
    doc         TEXT,
    language    TEXT,
    indexed_at  INTEGER NOT NULL
);

CREATE TABLE edges (
    from_symbol TEXT NOT NULL REFERENCES symbols(id),
    to_symbol   TEXT NOT NULL REFERENCES symbols(id),
    kind        TEXT NOT NULL,   -- calls|implements|embeds|references|imports
    file        TEXT,
    line        INTEGER,
    PRIMARY KEY (from_symbol, to_symbol, kind, file, line)
);

CREATE INDEX idx_symbols_name    ON symbols(project, name);
CREATE INDEX idx_symbols_file    ON symbols(project, file);
CREATE INDEX idx_edges_from      ON edges(from_symbol);
CREATE INDEX idx_edges_to        ON edges(to_symbol);
```

### Queries expuestas al Context Compiler

```go
FindSymbol(name string) []Symbol
Definition(ref string) (Symbol, error)
Callers(symbol string, depth int) []Symbol
Callees(symbol string, depth int) []Symbol
ImpactSet(symbols []string) []File     // qué archivos toca un cambio
RelatedFiles(query string, limit int) []File
```

### El grafo localiza, no reemplaza

```
Query: "refactor auth"
  ↓
Graph: AuthService, TokenManager, SessionMiddleware, 17 callers
  ↓
Candidatos: auth/service.go, auth/token.go, auth/session.go
  ↓
LEER EL CONTENIDO REAL de esos archivos (rangos acotados)
  ↓
Contexto
```

Nunca mandar solo el grafo como si fuera el código.

---

## 14. SKILLS

Alcance reducido deliberadamente. Esto es un producto dentro del producto; en V1 es apenas un registry.

### Estructura

```
~/.arg0s/skills/
├── native/                  # estándar propio de Arg0s
│   ├── judgment-day/
│   │   ├── SKILL.md
│   │   └── prompts/
│   ├── code-review/
│   └── research/
└── external/                # instaladas hacia otros ecosistemas (Fase 6)
    ├── claude-code/
    ├── codex/
    └── gemini/
```

### Formato

```markdown
---
name: secure-go
description: Convenciones de seguridad para código Go en este proyecto.
version: 1.0.0
triggers: [security, auth, crypto, input_validation]
languages: [go]
applies_to: [review, judgment, coding]
token_cost: 850
---

# Contenido de la skill
...
```

### Index-first

El registry mantiene solo `name + description + triggers + path + token_cost`. El contenido completo se carga **únicamente** cuando el Context Compiler decide que aplica. Nunca se cargan todas las skills al contexto.

### Comandos

```bash
arg0s skills list
arg0s skills show secure-go
arg0s skills install <path|url>
arg0s skills sync --target claude-code   # Fase 6
```

---

## 15. POLICY ENGINE

Puerta de control antes de cualquier ejecución. Para uso personal parece overengineering; en realidad es lo que evita que un agente interprete mal una instrucción y haga daño.

### `policies.yaml`

```yaml
version: 1

policies:
  - name: secret_paths_never_cloud
    when:
      path_matches: ["**/secrets/**", "**/.env*", "**/*_key*", "**/credentials*"]
    then:
      allow_cloud: false
      force_local: true

  - name: private_repo_no_openrouter
    when:
      repo_visibility: private
    then:
      deny_providers: [openrouter]

  - name: destructive_requires_confirmation
    when:
      tool: [shell, filesystem_write, git_push, git_commit]
    then:
      require_confirmation: true

  - name: shell_denylist
    when:
      tool: shell
    then:
      deny_patterns:
        - "rm -rf"
        - "git push --force"
        - "> /dev/"
        - "dd if="
        - "chmod -R 777"
        - "curl * | sh"
        - "sudo *"

  - name: cost_ceiling
    when:
      always: true
    then:
      daily_limit_usd: 2.00
      per_task_limit_usd: 0.25
      on_exceed: block

  - name: production_requires_judgment
    when:
      branch: [main, master, production]
    then:
      require_judgment: true
```

### Reglas duras (independientes de la config)

Estas no se pueden desactivar por configuración:

1. Arg0s **nunca** hace `git push`, `git commit`, ni `rm` sin confirmación interactiva explícita del usuario.
2. Arg0s **nunca** escribe fuera del proyecto activo ni de `~/.arg0s/`.
3. Arg0s **nunca** envía contenido de archivos que matcheen `secret_paths` a un provider cloud.
4. Las API keys nunca aparecen en prompts, logs, eventos ni artifacts.
5. Un patch generado por el fix agent **nunca** se aplica al working tree sin confirmación.

---

## 16. TELEMETRÍA, EVENTOS Y SESIONES

### 16.1 Event bus

Todo lo relevante emite un evento. Los eventos son la fuente de la observabilidad, del debugging y de la reproducibilidad.

```
SessionStarted
TaskCreated
ContextCompilationStarted / ContextFragmentSelected / ContextCompilationCompleted
RoutingStarted / RoutingDecided
PolicyEvaluated / PolicyDenied / ConfirmationRequested / ConfirmationGranted
StrategyStarted / StrategyCompleted
ModelStarted / ModelChunk / ModelCompleted / ModelFailed / ModelRetried / ModelFellBack
FusionStarted / DivergenceMeasured / SynthesisCompleted
SnapshotCreated
JudgeStarted / JudgeCompleted / FindingCreated / LedgerMerged
FixStarted / PatchCreated / PatchApplied / PatchRejected
VerdictIssued
MemoryRetrieved / MemoryStored
GraphQueried / GraphIndexed
BudgetWarning / BudgetExceeded
TaskCompleted / TaskFailed / TaskCancelled
SessionEnded
```

Evento base:

```go
type Event struct {
    ID        string
    Type      EventType
    SessionID SessionID
    RunID     RunID
    TaskID    TaskID
    Timestamp time.Time
    Payload   map[string]any
    Duration  *time.Duration
}
```

### 16.2 Sesiones y runs (no solo mensajes)

```
Session
  └── Run
        ├── Task
        ├── Strategy
        ├── RoutingDecision
        ├── CompiledContext (+ manifest)
        ├── ModelRuns[]
        ├── Artifacts[]
        ├── Ledger (si hubo judgment)
        ├── Verdict
        ├── Usage (tokens, costo)
        └── Events[]
```

Vista objetivo:

```
Session #42  ·  arg0s repo  ·  2026-09-10

Run #1   router → gemini-flash              1.2k tok   $0.001   3.4s   OK
Run #2   fusion → qwen-14b + gemini-flash   4.8k tok   $0.004  18.2s   div 8.7%
Run #3   judgment                           12.4k tok  $0.016  37.1s   APPROVED
                                            ─────────  ───────  ─────
                                            18.4k tok  $0.021  58.7s
```

### 16.3 Comandos

```bash
arg0s session list
arg0s session show 42
arg0s run show 184
arg0s run replay 184          # reconstruye la ejecución desde eventos
arg0s run export 184 --json
arg0s cost --today
arg0s cost --by-model --last 7d
```

---

## 17. MÉTRICAS Y BENCHMARK PROPIO

Sin esto, Arg0s es "un wrapper de modelos". Con esto, es "un sistema de orquestación adaptativa evaluado".

### 17.1 Métricas

| Métrica | Definición | Cómo se mide |
|---|---|---|
| **Routing accuracy** | ¿El modelo elegido fue suficiente? | Feedback del usuario (👍/👎) + re-ejecución con frontier en muestra |
| **Cost efficiency** | Costo real vs. costo de usar siempre el modelo top | Simulación sobre el histórico de runs |
| **Quality delta** | Pérdida de calidad vs. frontier | Comparación ciega en una muestra de N runs |
| **Ensemble gain** | ¿Fusion mejoró de verdad? | Comparar respuesta fusionada vs. primaria, evaluación ciega |
| **Judge precision** | findings confirmados que eran reales / total confirmados | Campo `human_verified` del ledger |
| **Judge recall** | problemas reales encontrados / problemas reales existentes | Bugs inyectados a propósito en un corpus de test |
| **Context compression** | Tokens evitados vs. mandar los archivos completos | Comparación directa por run |
| **Graph utility** | Tool calls / lecturas de archivo evitadas | Baseline: exploración sin grafo |
| **Model utility** | `quality / costo`, `quality / latencia` | Agregado sobre el histórico |

### 17.2 Baselines obligatorios

Toda métrica se compara contra un baseline explícito. Sin baseline, el número no se reporta.

| Métrica | Baseline |
|---|---|
| Cost efficiency | Todas las tasks a `gemini-pro` |
| Context compression | Mandar todos los archivos mencionados, completos |
| Graph utility | Búsqueda por grep/glob sin grafo |
| Ensemble gain | Solo el modelo primario |
| Judge precision | Un solo juez, sin consenso |

### 17.3 Comandos

```bash
arg0s bench routing --last 100
arg0s bench judges --corpus testdata/injected-bugs/
arg0s bench context --compare-baseline
arg0s bench report            # tabla completa
```

### 17.4 Corpus de test para jueces

Crear `testdata/injected-bugs/`: archivos con bugs conocidos e inyectados a propósito (null deref, race condition, SQL injection, off-by-one, error silenciado, secreto hardcodeado, authz faltante). Ground truth en JSON. Esto permite medir recall real de Judgment Day, no impresiones.

---

## 18. TUI Y ARQUITECTURA DAEMON

### 18.1 Por qué daemon

Ejecutar orquestación asíncrona (HTTP paralelo, procesos CLI, indexado LSP) dentro del event loop de Bubbletea produce UI congelada y `tea.Msg` anidados imposibles de razonar.

```
arg0s (TUI/CLI)  ←──JSON-RPC sobre Unix socket──→  arg0sd (motor)
```

Ventajas:
- La UI nunca se bloquea.
- Podés cerrar y reabrir la TUI sin perder una ejecución larga.
- El motor queda usable desde scripts bash y git hooks.
- Los eventos se streamean al cliente; la TUI solo renderiza.

### 18.2 Protocolo IPC

```
→ { "method": "task.submit",  "params": { "prompt": "...", "strategy": "auto" } }
← { "result": { "task_id": "tsk_..." } }

→ { "method": "events.subscribe", "params": { "task_id": "tsk_..." } }
← stream de Event (newline-delimited JSON)

Métodos:
  task.submit / task.cancel / task.status
  events.subscribe
  session.list / session.show
  config.get / config.set / config.reload
  models.list / models.test
  providers.status
  confirm.respond          ← respuesta a ConfirmationRequested
  daemon.ping / daemon.shutdown
```

### 18.3 Layout de la TUI

```
┌─ ARG0S ────────────── Model: ... │ Session: ... │ Elapsed: ... ─┐
├────────────┬──────────────────────────────────┬─────────────────┤
│ > Router   │ > PROMPT                         │ [ KEYBINDINGS ] │
│   Fusion   │ ┌──────────────────────────────┐ │                 │
│   Judgment │ │ refactor auth and run jd_    │ │  r   router     │
│   Memory   │ └──────────────────────────────┘ │  f   fusion     │
│ Code Graph │                                  │  j   judgment   │
│   Skills   │ [ ROUTER ]   Decision Engine     │  m   memory     │
│   Config   │   task type   : refactor         │  g   graph      │
│            │   complexity  : high             │  s   skills     │
│            │   privacy     : private          │  c   config     │
│            │   strategy    : fusion→judgment  │  q   quit       │
│            │                                  │  ?   help       │
│            │ [ MODELS ]   Parallel Execution  │                 │
│            │   ✓ qwen-coder-14b   (local) 8.1s│  ─────────────  │
│            │   ✓ gemini-flash     (cloud) 4.2s│  ctrl+c cancel  │
│            │                                  │  ctrl+r rerun   │
│            │ [ FUSION ]                       │  tab    panel   │
│            │   divergence: 8.7% (thr 15.0%)   │  /      search  │
│            │   → below threshold, no synthesis│                 │
│            │                                  │                 │
│            │ [ JUDGMENT DAY ]                 │                 │
│            │   Judge A (gemini-flash) ✓       │                 │
│            │   Judge B (qwen-14b)     ✓       │                 │
│            │   Findings 4 · Confirmed 2       │                 │
│            │   ┌──────────┐                   │                 │
│            │   │ APPROVED │                   │                 │
│            │   └──────────┘                   │                 │
├────────────┴──────────────────────────────────┴─────────────────┤
│ Tokens 18.4k/128k │ Cost $0.021 │ Latency 58.7s │ Budget 1% used│
└─────────────────────────────────────────────────────────────────┘
```

### 18.4 Reglas de renderizado

- **Los paneles aparecen progresivamente**, no pre-cargados. Con fusion adaptativa, muchas corridas no tendrán bloque FUSION en absoluto. Renderizar solo las fases que efectivamente ocurrieron.
- Cada bloque tiene estado: `pending` (gris) · `running` (azul, spinner) · `done` (verde ✓) · `failed` (rojo ✗) · `skipped` (gris tachado).
- El log es scrollable y se puede expandir cada bloque para ver el detalle (`enter` sobre un bloque).
- `ctrl+c` cancela la task en curso, no mata la TUI.
- Las confirmaciones de policy aparecen como modal bloqueante con opciones explícitas.

### 18.5 Keybindings

```
GLOBAL
  q / ctrl+q   quit          ?          help
  tab          next panel    shift+tab  prev panel
  ctrl+c       cancel task   ctrl+r     rerun last
  :            command mode  /          search

MODOS
  r router · f fusion · j judgment · m memory · g graph · s skills · c config

LOG
  ↑↓/kj scroll · enter expand · e export run · y yank output

CONFIG
  ↑↓ navegar · enter editar · space toggle · ctrl+s guardar · ctrl+t test provider
```

---

## 19. ESQUEMA DE BASE DE DATOS

Migraciones en `internal/storage/migrations/`, numeradas y embebidas con `embed.FS`.

```sql
-- 001_init.sql
CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL);

CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    name        TEXT,
    project     TEXT,
    created_at  INTEGER NOT NULL,
    ended_at    INTEGER,
    metadata    TEXT
);

CREATE TABLE runs (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL REFERENCES sessions(id),
    prompt        TEXT NOT NULL,
    strategy      TEXT NOT NULL,
    profile       TEXT,          -- JSON TaskProfile
    routing       TEXT,          -- JSON RoutingDecision
    context       TEXT,          -- JSON Manifest
    status        TEXT NOT NULL, -- running|completed|failed|cancelled
    verdict       TEXT,
    input_tokens  INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    cost_usd      REAL DEFAULT 0,
    started_at    INTEGER NOT NULL,
    ended_at      INTEGER,
    error         TEXT
);

CREATE TABLE model_runs (
    id           TEXT PRIMARY KEY,
    run_id       TEXT NOT NULL REFERENCES runs(id),
    model_id     TEXT NOT NULL,
    provider     TEXT NOT NULL,
    role         TEXT NOT NULL,
    input_tokens  INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    cost_usd      REAL DEFAULT 0,
    latency_ms    INTEGER,
    attempts      INTEGER DEFAULT 1,
    fell_back_from TEXT,
    status       TEXT NOT NULL,
    error        TEXT,
    started_at   INTEGER NOT NULL
);

CREATE TABLE events (
    id         TEXT PRIMARY KEY,
    session_id TEXT,
    run_id     TEXT,
    task_id    TEXT,
    type       TEXT NOT NULL,
    payload    TEXT,
    duration_ms INTEGER,
    timestamp  INTEGER NOT NULL
);

CREATE TABLE artifacts (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES runs(id),
    kind       TEXT NOT NULL,   -- patch|file|json|report
    path       TEXT,
    content    TEXT,
    hash       TEXT,
    created_at INTEGER NOT NULL
);

CREATE TABLE snapshots (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES runs(id),
    round      INTEGER NOT NULL,
    git_ref    TEXT,
    content_hash TEXT NOT NULL,
    files      TEXT,            -- JSON: lista de archivos incluidos
    created_at INTEGER NOT NULL
);

CREATE TABLE findings (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES runs(id),
    snapshot_id TEXT NOT NULL REFERENCES snapshots(id),
    judge       TEXT NOT NULL,
    model_id    TEXT NOT NULL,
    perspective TEXT,
    severity    TEXT NOT NULL,
    category    TEXT,
    title       TEXT NOT NULL,
    description TEXT,
    evidence    TEXT NOT NULL,  -- JSON
    reasoning   TEXT,
    suggested_fix TEXT,
    confidence  REAL,
    created_at  INTEGER NOT NULL
);

CREATE TABLE ledger (
    id           TEXT PRIMARY KEY,
    run_id       TEXT NOT NULL REFERENCES runs(id),
    snapshot_id  TEXT NOT NULL REFERENCES snapshots(id),
    finding_id   TEXT NOT NULL,
    status       TEXT NOT NULL,  -- CONFIRMED|SUSPECT|CONTRADICTION|DISMISSED
    severity     TEXT NOT NULL,
    judge_a      TEXT,           -- JSON
    judge_b      TEXT,           -- JSON
    independence TEXT,           -- high|medium|low
    action       TEXT,
    action_result TEXT,
    human_verified INTEGER,      -- NULL | 0 | 1  → base del benchmark
    created_at   INTEGER NOT NULL
);

CREATE TABLE memories (
    id            TEXT PRIMARY KEY,
    project       TEXT NOT NULL,
    category      TEXT NOT NULL,
    title         TEXT NOT NULL,
    content       TEXT NOT NULL,
    confidence    REAL DEFAULT 1.0,
    source        TEXT,
    refs          TEXT,
    embedding     BLOB,
    created_at    INTEGER NOT NULL,
    last_verified INTEGER,
    stale         INTEGER DEFAULT 0
);

CREATE TABLE costs_daily (
    date       TEXT PRIMARY KEY,
    cost_usd   REAL NOT NULL DEFAULT 0,
    tokens_in  INTEGER NOT NULL DEFAULT 0,
    tokens_out INTEGER NOT NULL DEFAULT 0,
    runs       INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_runs_session   ON runs(session_id, started_at DESC);
CREATE INDEX idx_events_run     ON events(run_id, timestamp);
CREATE INDEX idx_events_type    ON events(type, timestamp DESC);
CREATE INDEX idx_model_runs_run ON model_runs(run_id);
CREATE INDEX idx_findings_run   ON findings(run_id, severity);
CREATE INDEX idx_ledger_run     ON ledger(run_id, status);
CREATE INDEX idx_memories_proj  ON memories(project, category, stale);
```

---

## 20. ANTI-OBJETIVOS Y SCOPE GUARD

**Prohibido implementar antes de la fase indicada.** Si el agente de código quiere agregar algo de esta lista antes de tiempo, debe parar y preguntar.

| Prohibido | Antes de |
|---|---|
| TUI | Fase 4 |
| Fusion | Fase 4 |
| Judgment Day | Fase 5 |
| Memory / Engram | Fase 3 |
| Code Graph / LSP | Fase 3 |
| Skills | Fase 5 |
| Runtimes (Claude Code, Codex, Gemini CLI) | Fase 6 |
| LiteLLM adapter | Fase 7 (si acaso) |
| Routing aprendido | Fase 7 |
| Embeddings / búsqueda semántica | Fase 3 |
| Streaming en la UI | Fase 4 |
| Cualquier base de datos que no sea SQLite | nunca |
| CGO | nunca (rompe el binario único) |
| Multi-usuario, auth, billing, servidor HTTP público | nunca |
| Plugin system genérico | nunca (YAGNI) |
| Métricas sin baseline | nunca |
| Cifras de rendimiento copiadas de terceros | nunca |

**Regla de oro del scope:** si una feature no se puede usar end-to-end desde `arg0s run "..."`, no está terminada, y no se empieza otra.

---

## 21. ROADMAP — QUÉ CONSTRUIR Y EN QUÉ ORDEN

Cada fase tiene un Definition of Done verificable. **No avanzar sin cumplirlo.**

### FASE 0 — Fundación (sin IA)

**Objetivo:** que exista un binario que arranque, lea config, escriba en SQLite y emita eventos.

- [ ] `go mod init` + estructura de directorios
- [ ] Cobra: `arg0s version`, `arg0s doctor`, `arg0s config show|set`, `arg0s init [--force] [--home <path>]`
- [ ] Loader de config con precedencia completa (defaults → yaml → env → flags), resolviendo `ARG0S_HOME` (sección 6.1/6.2)
- [ ] Parser y validador de `config.yaml` y `models.yaml`
- [ ] Carga de `.env` con verificación de permisos
- [ ] SQLite: conexión, migraciones embebidas, migración 001
- [ ] Logging estructurado con `slog`
- [ ] Event bus en memoria + persistencia a `events`
- [ ] Tipos de `internal/core/` (sin lógica)
- [ ] Makefile: `build`, `test`, `lint`, `run`
- [ ] Tests: config loading, precedencia, validación, migraciones, `init`→`doctor` en verde (integración, sin red)

`arg0s init` materializa `~/.arg0s/` completo desde los defaults/templates
embebidos: `config.yaml` (sección 6.3, tal cual), `models.yaml` (catálogo
de la sección 6.4, IDs marcados como placeholders a verificar con
`arg0s models sync` cuando exista), `.env` (plantilla con keys vacías,
`chmod 0600`), `arg0s.db` (migraciones aplicadas), y los directorios
`skills/`, `projects/`, `cache/`, `logs/`. Nunca sobrescribe sin
`--force`; si `~/.arg0s/` (o `--home <path>`) ya existe, error listando
qué hay adentro. Al terminar imprime qué falta para llegar a verde en
`doctor` (típicamente: cargar `GEMINI_API_KEY`, tener ollama corriendo).

**DoD:** `arg0s doctor` corre, valida toda la config, reporta ✓/✗/⊘ por
cada punto de la sección 6.7, y no hay ninguna llamada a un modelo en el
código. Además: en una máquina limpia, `arg0s init && arg0s doctor` da
exit 0 — los checks de API key y conectividad pueden quedar en ⊘ (o ✗ si
corresponde), pero sin errores de config faltante ni roles rotos.

---

### FASE 1 — Ejecución de modelos

**Objetivo:** `arg0s run "hola"` responde usando un modelo real, con costo y tokens registrados.

- [ ] Interfaz `Provider` + registry
- [ ] Provider `mock` (determinista, para tests) — **construir este primero**
- [ ] Provider `ollama`
- [ ] Provider `gemini`
- [ ] Provider `openai_compatible` (cubre OpenRouter y cualquier endpoint compatible)
- [ ] Execution Engine: retry, timeout, cancelación, rate limit por provider
- [ ] Token accounting + cálculo de costo desde `models.yaml`
- [ ] Estrategia `direct` (un modelo, sin router)
- [ ] Persistencia de `runs` y `model_runs`
- [ ] Comandos: `arg0s run`, `arg0s providers list|test`, `arg0s models list|test|sync`, `arg0s cost`
- [ ] Tests con provider mock: retry, timeout, fallback, accounting

**DoD:**
```bash
arg0s run "explica qué es un mutex" --model qwen-coder-7b
arg0s run "explica qué es un mutex" --model gemini-flash
arg0s cost --today
arg0s run show <id>     # muestra tokens, costo, latencia, modelo
```
Todo funciona, y matar el proceso con ctrl+c cancela limpiamente la llamada en vuelo.

---

### FASE 2 — Router

**Objetivo:** Arg0s elige el modelo solo, y puede explicar por qué.

- [ ] `TaskProfile` + perfilado heurístico
- [ ] Perfilado por classifier (modelo local con salida JSON estricta)
- [ ] Modo `hybrid` con umbral de confianza
- [ ] Selección de tier y de modelo dentro del tier
- [ ] Sistema de `overrides` de la config
- [ ] `RoutingDecision` persistida con razones y rechazados
- [ ] Policy Engine básico: límites de costo y `secret_paths`
- [ ] Comandos: `arg0s run` sin `--model`, `arg0s route explain "<prompt>"`
- [ ] Tests: cada heurística, cada override, casos límite

**DoD:**
```bash
arg0s run "cuánto es 2+2"                    # → qwen local, $0
arg0s run "diseñá la arquitectura de X"      # → gemini-pro
arg0s route explain "refactor auth"          # → decisión + razones + rechazados
```

---

### FASE 3 — Motor de contexto

**Objetivo:** el modelo recibe contexto mínimo relevante, no archivos completos.

- [ ] Interfaz `MemoryStore` + implementación SQLite
- [ ] Cliente LSP genérico (JSON-RPC sobre stdio) + `gopls`
- [ ] Indexador → tablas `symbols` / `edges`
- [ ] Queries del grafo (`FindSymbol`, `Callers`, `ImpactSet`...)
- [ ] Context Compiler: retrieval, ranking, budget, manifest
- [ ] Compresión con `summarizer` cuando se excede el budget
- [ ] Comandos: `arg0s graph index|query`, `arg0s memory add|list|search`, `arg0s context preview "<prompt>"`
- [ ] **Benchmark de compresión de contexto vs. baseline**

**DoD:** `arg0s context preview "refactor auth"` muestra exactamente qué entra al prompt, de qué fuente, cuántos tokens y qué se descartó. Y hay un número medido de reducción vs. mandar los archivos completos.

---

### FASE 4 — Fusion + TUI

**Objetivo:** ensemble adaptativo, y por fin una interfaz.

- [ ] Ejecución paralela con `errgroup` + cancelación
- [ ] Medición de divergencia (lexical → hybrid)
- [ ] Umbrales adaptativos: skip / synthesize / escalate
- [ ] Synthesizer en modo `select_best`
- [ ] Daemon `arg0sd` + protocolo IPC + cliente
- [ ] TUI Bubbletea: paneles, tema, keybindings, render progresivo
- [ ] Streaming de eventos daemon → TUI
- [ ] **Benchmark de ensemble gain**

**DoD:** la TUI muestra una corrida completa en vivo sin congelarse; cerrar la TUI y reabrirla reengancha con la ejecución en curso; y hay un número que dice si fusion mejora o no.

---

### FASE 5 — Judgment Day + Skills

**Objetivo:** la feature diferencial, con tests fuertes.

- [ ] Snapshots inmutables con hash de contenido
- [ ] Judge runner con ceguera garantizada
- [ ] Esquema JSON estricto + reintento en parse failure
- [ ] Validación de evidencia (descartar findings sin `file:line` verificable)
- [ ] Merge → ledger (CONFIRMED / SUSPECT / CONTRADICTION)
- [ ] Modo `fail_fast`
- [ ] Fix agent acotado → patch en formato diff
- [ ] Re-juicio con `max_rounds`
- [ ] Veredictos + confirmación humana para aplicar patches
- [ ] Skill registry index-first + skill nativa `judgment-day`
- [ ] Corpus `testdata/injected-bugs/` con ground truth
- [ ] **Benchmark de precision y recall de jueces**

**DoD:** `arg0s judge ./auth --run` produce un ledger auditable, y el benchmark contra el corpus de bugs inyectados da números reales de precision/recall.

---

### FASE 6 — Runtimes de agentes

- [ ] Interfaz `Runtime` con `Available()` y `Run()`
- [ ] Adapter Claude Code (proceso, no PTY interactivo si se puede evitar)
- [ ] Adapter Codex / Gemini CLI
- [ ] Detección de versión y degradación elegante si el formato cambia
- [ ] Timeouts agresivos y cancelación de procesos hijos
- [ ] `arg0s skills sync --target claude-code`

**DoD:** un runtime caído o con formato cambiado **no rompe Arg0s**; se reporta como no disponible y el router lo excluye.

---

### FASE 7 — Inteligencia adaptativa

- [ ] Base de performance histórica por (modelo, tipo de task)
- [ ] Scoring: `quality × capability × historical_success / (cost × latency)`
- [ ] Router en modo `learned` con fallback a heurística
- [ ] Feedback loop (👍/👎 por run)
- [ ] Suite completa de benchmarks + `arg0s bench report`
- [ ] Adapter LiteLLM opcional

**DoD:** `arg0s bench report` produce la tabla completa de métricas, todas con baseline explícito.

---

### POR DÓNDE EMPEZAR HOY — primer sprint concreto

Esta es la secuencia exacta de las primeras tareas. En orden.

1. `go mod init github.com/<usuario>/arg0s` + árbol de directorios vacío + Makefile
2. `internal/core/`: `Task`, `Result`, `Model`, `Usage`, `Message`, errores. Sin lógica, solo tipos.
3. `internal/config/`: structs de `config.yaml` y `models.yaml` + loader con precedencia + validador
4. `internal/storage/`: `db.go` + migración `001_init.sql` embebida + runner de migraciones
5. `internal/telemetry/`: `Event`, bus en memoria, sink a SQLite
6. `cmd/arg0s/main.go`: Cobra con `version`, `doctor`, `config show`, `init`
7. Implementar `arg0s doctor` con los 12 checks de la sección 6.7 (los que apliquen sin providers) y `arg0s init` para materializar `~/.arg0s/` (sección 6.1)
8. Tests: config precedence, validación, migraciones idempotentes, `init`→`doctor` en verde
9. **Recién ahora:** `internal/providers/mock/` — provider determinista
10. `internal/execution/executor.go` con el mock, incluyendo retry y timeout, con tests
11. `internal/providers/ollama/` — el primero real, porque es local y gratis
12. Estrategia `direct` + `arg0s run "..." --model qwen-coder-7b`
13. `internal/providers/gemini/`
14. Accounting de tokens y costo + `arg0s cost`

En el punto 12 ya tenés algo usable. Ese es el primer hito real.

---

## 22. CONVENCIONES DE CÓDIGO

### Estructura
- `internal/core` no importa ningún otro paquete interno. Nunca.
- Las dependencias van hacia adentro: `tui → ipc → orchestrator → execution → providers → core`.
- Un paquete por concepto. Si un paquete tiene más de ~8 archivos, revisar.

### Errores
```go
// Errores tipados para clasificación en el executor
var (
    ErrRetryable    = errors.New("retryable")
    ErrRateLimited  = errors.New("rate limited")
    ErrBudget       = errors.New("budget exceeded")
    ErrPolicy       = errors.New("policy denied")
    ErrUnavailable  = errors.New("provider unavailable")
)

// Siempre wrap con contexto
return fmt.Errorf("gemini complete: %w", err)
```
- Nunca `panic` fuera de `main`.
- Todo error que cruza una capa se enriquece con contexto.

### Contexto y concurrencia
- `context.Context` como primer parámetro en toda función que hace I/O.
- Nada de goroutines sin cancelación. `errgroup` para paralelismo.
- Los canales de streaming siempre se cierran, incluso en error.

### Testing
- Provider `mock` para todo test que involucre modelos. **Cero llamadas de red en `go test ./...`**.
- Golden files en `testdata/` para prompts y salidas esperadas.
- Tests de integración con red detrás de build tag: `//go:build integration`.
- Cobertura objetivo: >70% en `core`, `config`, `execution`, `judgment`.

### Prompts
- Los prompts viven en archivos, no en strings inline. `internal/judgment/prompts/*.md`, embebidos con `embed.FS`.
- Versionar los prompts: un cambio de prompt invalida los benchmarks previos.
- Todo prompt que espera JSON incluye el esquema explícito y un ejemplo.

### Naming
- `snake_case` en YAML y SQL. `camelCase`/`PascalCase` en Go. `kebab-case` en comandos CLI.
- IDs con prefijo: `tsk_`, `run_`, `snap_`, `led_`, `mem_`.

---

## 23. GLOSARIO

| Término | Significado |
|---|---|
| **Task** | Unidad de trabajo. Prompt + perfil + contexto + restricciones |
| **Run** | Una ejecución concreta de una Task, con su estrategia y resultados |
| **Strategy** | Cómo se ejecuta una Task: direct, router, fusion, judgment |
| **Provider** | Endpoint de inferencia (Gemini API, Ollama, OpenRouter) |
| **Runtime** | Agente externo completo con su propio loop (Claude Code, Codex) |
| **Role** | Función dentro del sistema: generator, judge_a, synthesizer, fix_agent... |
| **Tier** | Nivel de capacidad/costo: simple, medium, complex |
| **Divergence** | Medida de desacuerdo entre respuestas de modelos distintos |
| **Snapshot** | Estado congelado e identificado del código bajo evaluación |
| **Finding** | Problema reportado por un juez, con evidencia obligatoria |
| **Ledger** | Registro auditable del merge de findings de todos los jueces |
| **Confirmed** | Finding reportado por ≥2 jueces sobre la misma evidencia |
| **Suspect** | Finding reportado por un solo juez |
| **Contradiction** | Jueces en desacuerdo explícito sobre la misma línea |
| **Verdict** | Resultado de Judgment Day. Evidencia, no autorización |
| **Fragment** | Unidad de contexto con fuente, referencia y relevancia |
| **Manifest** | Registro de qué entró al contexto y qué se descartó, y por qué |

---

## 24. RIESGOS CONOCIDOS

| Riesgo | Impacto | Mitigación |
|---|---|---|
| Scope creep | Alto | Sección 20 es vinculante. Una fase a la vez |
| TUI acoplada al motor | Alto | Arquitectura daemon desde Fase 4 |
| Adapters CLI frágiles | Medio | Fase 6, detrás de interfaz, con degradación elegante |
| Latencia de fusion+judgment | Medio | Adaptativo + fail-fast + límite `per_task_max_calls` |
| Costo descontrolado | Medio | Policy engine con `on_exceed: block` desde Fase 2 |
| Jueces que alucinan findings | Alto | `require_evidence` + validación de `file:line` + corpus de test |
| Jueces correlacionados | Medio | Modelos de providers distintos + flag `independence` + warning en doctor |
| Modelos locales lentos | Bajo | Timeouts largos, `concurrent: 1`, tier simple únicamente |
| IDs de modelo obsoletos | Bajo | `models.yaml` externo + `arg0s models sync` |
| Sobre-ingeniería temprana | Alto | DoD por fase. Nada se construye "para después" |

---

## 25. CRITERIOS DE ÉXITO

El proyecto es exitoso cuando:

1. Lo usás todos los días en vez de abrir un chat web.
2. `arg0s cost --last 30d` muestra un gasto menor al de usar un frontier para todo, con calidad aceptable.
3. Judgment Day encuentra al menos un bug real que un solo modelo aprobó en falso, y está registrado en el ledger con evidencia.
4. Cambiar "Judge A = Gemini, Judge B = Qwen local" es editar dos líneas de YAML.
5. Podés responder "¿por qué eligió ese modelo?" con datos, no con intuición.
6. `arg0s bench report` produce una tabla de métricas con baselines reales.
7. Arg0s se usa para desarrollar Arg0s: se enruta, se juzga y recuerda decisiones sobre su propio código.
8. Se siente tuyo: negro y azul, en terminal, sin parecerse a un fork de nada.

---

## 26. NOTA FINAL PARA EL AGENTE DE CÓDIGO

Si estás leyendo esto para empezar a trabajar:

1. Andá a la sección **21 → "POR DÓNDE EMPEZAR HOY"**.
2. Hacé los puntos 1 a 8 (Fase 0 completa). Nada más.
3. Cuando `arg0s doctor` funcione con sus checks, pará y mostrá el resultado.
4. No implementes providers, router, fusion ni judgment hasta que la Fase 0 esté cerrada y verificada.
5. Si algo del documento es ambiguo, preguntá. No inventes.

El error más caro de este proyecto sería construir diez subsistemas a medias. La secuencia correcta es: **un flujo end-to-end funcionando, después el siguiente**.