# Arg0s — Guía para el agente de código

Fuente de verdad completa: [docs/ARG0S.md](docs/ARG0S.md). Este archivo
es un resumen operativo, no un reemplazo — ante cualquier duda, gana
docs/ARG0S.md.

**Fase actual: FASE 4** (fusion + TUI, parte A -- fusion -- cerrada;
parte B -- daemon + TUI -- en curso). Fase 0 (fundación), Fase 1
(ejecución de modelos), Fase 2 (router) y Fase 3 (motor de contexto)
cerradas y commiteadas.

## La carrera de streaming, versión IPC (resuelta en el micro-checkpoint de Parte B)

La misma clase de bug de `internal/execution/stream.go` (Engram
`arg0s/lesson/stream-ctx-race`) tuvo TRES apariciones en este proyecto,
cada una más difícil que la anterior:

1. **Fase 1** -- `Executor.Stream` consumido por un `for-range` directo
   en el mismo proceso. Resuelto (comentario inline en stream.go).
2. **Fase 4 parte A** -- `fusion.Engine.runSecondaries` consumido por
   goroutines paralelas EN EL MISMO PROCESO. Resuelto y testeado
   (`TestRunSecondaries_OneFailsFast_...`, internal/fusion/fusion_test.go).
3. **Fase 4 parte B** -- el daemon streamea eventos a un cliente
   **cruzando un proceso** vía IPC (Unix socket, sección 18.2). Era la
   versión más difícil: no hay happens-before de un `close()` de canal
   Go cruzando un socket. **Resuelto así** (internal/ipc/):
   - `Event.Type` es siempre uno de `chunk|done|failed|cancelled` --
     `task.run` (internal/ipc/server.go) SIEMPRE manda exactamente un
     evento terminal antes de dejar de escribir, nunca infiere éxito
     del silencio ni del cierre del socket.
   - `task.Subscribe` (internal/ipc/task.go) guarda TODO el historial
     de eventos y hace replay desde `from` -- un cliente que se
     reconecta (DoD: "cerrar la TUI a mitad de una task y reabrirla →
     reengancha") pide `events.subscribe` de nuevo y recibe el
     historial completo más lo que falte, nunca un hueco.
   - Probado con un socket Unix REAL, no un mock de la capa IPC:
     `TestSubscribe_Reconnect_ReplaysFromZero` cierra un cliente a
     mitad de un stream y abre uno nuevo, que recibe el contenido
     completo. `TestCancel_MidStream_ProducesExplicitCancelledEvent`
     prueba que `task.cancel` corta una task viva y el evento
     `cancelled` llega explícito, no un socket que se cierra sin más.

## Reglas duras

No negociables. No se desactivan por config, no se saltan "porque ya
que estamos".

1. Arg0s nunca hace `git push`, `git commit` ni `rm` sin confirmación
   interactiva explícita del usuario. (§15)
2. Arg0s nunca escribe fuera del proyecto activo ni de `~/.arg0s/`. (§15)
3. Arg0s nunca envía contenido de archivos que matcheen `secret_paths`
   a un provider cloud. (§15)
4. Las API keys nunca aparecen en prompts, logs, eventos ni artifacts.
   Redactar siempre como `sk-...abcd`. (§15)
5. Un patch generado por el fix agent nunca se aplica al working tree
   sin confirmación. (§15)
6. Cero CGO, siempre — rompe el binario único. (§20)
7. Cero llamadas a modelos fuera de las fases que las requieren — Fase 0
   no llamó ningún modelo; Fase 1 en adelante sí, pero solo a través del
   Execution Engine (P4). (§21 DoD Fase 0)
8. `go test ./...` nunca toca la red. Tests contra providers reales van
   detrás de `//go:build integration`. (§22)
9. `.env` y `*.db` nunca se commitean — en `.gitignore` desde el primer
   commit. (§21 DoD Fase 0)
10. `internal/core` no importa ningún otro paquete interno. Nunca. (§22)
11. **Todo método que el CLI expone como consulta es solo-lectura** —
    no una lista cerrada de comandos, sino un principio: si el usuario
    lo invoca para LEER (un `list`, `search`, `query`, `show`, `get`,
    o el método de un store que un comando de solo-lectura llama por
    debajo), no puede crear `arg0s.db`, no puede migrar, y no puede
    mutar una fila como side effect de haber leído. Ejemplos ya
    cubiertos: `doctor`, `providers list`, `models list`, `graph
    query`/`graph callers`, `session show`, `cost`, `memory
    list`/`memory search`, `context preview`, `bench context`. Si
    falta algo (la db no existe, no hay datos), lo reportan y dicen qué
    comando lo crea. Solo `init`, `run`, y los mutadores explícitos
    (`memory add/forget/revalidate`, `graph index`, etc) escriben.
    Origen 1: `doctor` creaba `arg0s.db` como side effect al chequear
    storage, y eso rompía la detección de conflicto de `arg0s init` en
    la corrida siguiente (un `arg0s.db` huérfano bloqueaba el `init`
    posterior sin `--force`).
    Origen 2: `memory.MemoryStore.Query()` marcaba `stale` en la fila
    como side effect de leer -- mutaba estado desde un método que el
    CLI expone como consulta (`memory list`/`memory search`), y que el
    Context Compiler de Fase 3 iba a llamar en cada task. Se corrigió:
    Query() calcula y devuelve `Stale` pero nunca escribe; la escritura
    real quedó en `Store.Revalidate()`, invocado solo por el comando
    explícito `arg0s memory revalidate`.
    Ver Engram `arg0s/preference/read-only-diagnostics`.

## Estado conocido del Router (Fase 2, cerrada)

- `router.mode` default es `heuristic`. Los modos `classifier` y
  `hybrid` están implementados pero **solo probados contra el provider
  mock** (`internal/router/classifier_test.go`) — nunca contra un
  modelo real. No asumir que están validados en producción; no
  cambiarlos a default sin correrlos antes contra un modelo real. Ver
  Engram `arg0s/discovery/classifier-mode-untested-e2e`.
- Orden real: perfilar → overrides → **filtrar candidatos por
  presupuesto → recién ahí elegir modelo** (`Router.selectAffordable`,
  internal/router/router.go). Si el tier "ideal" no entra en
  `per_task_cost_usd`/`daily_cost_usd`, degrada al tier más capaz que
  SÍ entre, según `limits.on_exceed` (`block` busca el más barato que
  alcance y sirve; `warn` ejecuta el ideal igual; `downgrade_to_local`
  fuerza local en esa búsqueda). Bloquea con `BudgetExceededError` solo
  si ningún tier entra.
- El chequeo+reserva de presupuesto diario es atómico bajo un mutex por
  `*Router` (`checkAndReserveBudget`, internal/router/policy.go) — sirve
  para no pisarse entre goroutines de UN MISMO proceso. **No** protege
  contra dos procesos `arg0s run` separados corriendo a la vez; eso
  necesita el daemon (Fase 4).

## Estado conocido del Context Compiler (Fase 3 parte B)

- Tokens: `contextc.EstimateTokens` es `len(s)/4` -- no hay tokenizer
  real integrado. `arg0s bench context` compara compilado vs. baseline
  con la MISMA función en los dos lados, así que la comparación es
  válida aunque el número absoluto sea una aproximación.
- `budget_split.graph` queda reservado sin gastar: el contenido real
  que localiza el grafo (rangos de código leídos del disco) se
  contabiliza en `code`, no en `graph` -- sección 13, "el grafo
  localiza, no reemplaza". Si se agrega un fragmento de puro resumen
  estructural (ej "callers de X: A, B, C" como texto plano), ESE
  consumiría `graph`.
- `arg0s bench context` solo mide la categoría `code` (codegraph +
  file) contra mandar los archivos completos -- memory/skills quedan
  afuera de la comparación a propósito porque son idénticos en ambos
  escenarios (ver comentario de `codeVsBaselineTokens` en
  cmd/arg0s/bench.go).
- Compresión: `applyBudget` intenta comprimir el fragmento MÁS GRANDE
  de la categoría (vía el rol `summarizer`) antes de excluir nada,
  acotado a `maxCompressionAttempts=3`; si el summarizer no está
  disponible (sin executor, o `context.compress_when_over: false`) cae
  directo al fallback de exclusión, sin error.

## Estado conocido de Fusion (Fase 4 parte A, cerrada)

- **Aplicación de la lección de streaming** (Engram
  `arg0s/lesson/stream-ctx-race`): `Engine.runSecondaries`
  (internal/fusion/fusion.go) corre los secundarios en paralelo con un
  canal bufferizado de tamaño `len(models)` -- cada goroutine SIEMPRE
  manda su resultado (éxito, error de modelo, o `ctx.Err()`) antes de
  terminar, así el canal recibe exactamente N mensajes sin importar qué
  pasó y el for-range del colector nunca se cuelga esperando algo que
  no va a llegar. Después de ese for-range, `Run` hace el MISMO chequeo
  post-loop que stream.go: `if ctx.Err() != nil { return ... }` -- no
  asume que "el for-range terminó sin panic" significa "todo corrió
  bien". Test que lo prueba con tiempos reales (no solo con mocks
  instantáneos): `TestRunSecondaries_OneFailsFast_DoesNotBlockSlowerOnesRunningInParallel`
  (internal/fusion/fusion_test.go) -- un secundario falla ya mientras
  otro sigue corriendo, se mide que el tiempo total es ~el de la
  goroutine lenta (paralelismo real, no serializado) y que el colector
  distingue cuál cerró por error y cuál por éxito.
- `router.Router` ahora puede elegir `SelectedStrategy: "fusion"`
  (`selectStrategy`, internal/router/router.go) -- **solo cuando
  corresponde**: `fusion.enabled && fusion.adaptive` y
  (`complexity=high` o `Confidence` baja). El motor de fusion (
  `Engine.shouldEscalate`) vuelve a chequear lo mismo puertas adentro,
  independiente de quién lo invocó (router o `--strategy fusion` a
  mano) -- es una protección deliberada, no redundancia: un
  `--strategy fusion` manual sobre una tarea trivial sigue sin escalar.
- `arg0s run --strategy fusion` no soporta `--stream` todavía (error
  explícito, no un fallback silencioso a direct).
- `arg0s bench fusion`: sin Judgment Day (Fase 5) no hay forma de medir
  calidad independiente. Reporta DOS números por separado, sin que uno
  se disfrace del otro: `substitution_rate` (medido -- si el
  synthesizer prefirió un secundario sobre el primario; mide ACTIVIDAD
  del synthesizer, no calidad) y `ensemble_gain` (explícitamente
  "pendiente" -- requiere correr con un judge real o revisión humana,
  sección 17.2). No renombrar substitution_rate como "ensemble gain":
  son cosas distintas y ya se confundieron una vez en esta fase.
- `fusion.divergence.method` soporta de verdad solo `lexical`
  (Jaccard de tokens + canonización de identificadores para código, así
  variables renombradas no cuentan como divergencia). `semantic`/
  `judge`/`hybrid` degradan a lexical -- no hay embeddings locales ni
  Judgment Day todavía. El default COMPILADO (`DefaultConfig`) es
  `lexical`; el `config.yaml` de ejemplo (sección 6.3, template
  embebido) todavía dice `hybrid` porque así está en la spec -- no se
  tocó ese archivo, pero corre igual como lexical hasta que exista un
  método real detrás.
- No se pudo demostrar `arg0s run --strategy fusion` ni `arg0s bench
  fusion` en vivo en la sesión donde se implementó esto -- sin API
  keys ni Ollama corriendo en ese entorno. La evidencia del checkpoint
  fueron los tests (`go test ./internal/fusion/... -race -v`, 17/17
  verdes, incluido el test de paralelismo con fallo) contra el
  provider mock. Antes de confiar en el comportamiento contra un
  modelo real, correrlo una vez con providers configurados.

## Estado conocido de IPC/daemon (Fase 4 parte B, punto 1 -- daemon + cliente tonto)

- `arg0sd` (cmd/arg0sd/) es un binario NUEVO y separado de `arg0s`
  (sección 18.1) -- duplica ~15 líneas de wiring de providers/executor
  de cmd/arg0s/wire.go a propósito (dos binarios, cada uno arma el
  suyo); si esa duplicación crece se extrae a un paquete compartido,
  hoy no amerita la indirección.
- `arg0s daemon submit/cancel/ping` es el "cliente tonto" del
  checkpoint -- imprime eventos crudos (`[chunk]`/`[done]`/`[failed]`/
  `[cancelled]`), no interpreta nada. La TUI (punto 2 de la Parte B)
  reemplaza esto, pero el cliente tonto queda para debug de arg0sd sin
  levantar Bubbletea.
- `daemon.socket` default `~/.arg0s/daemon.sock` se resuelve con
  `Config.SocketPath()` (internal/config/config.go) -- respeta
  ARG0S_HOME igual que el resto del sistema.
- Diseño de `internal/ipc/task.go`: un solo `task` en memoria por
  `task_id`, con TODO el historial de eventos guardado (no hay límite
  ni rotación todavía) y un broadcast simple (`notify chan struct{}`
  que se cierra y recrea en cada evento nuevo) para que múltiples
  `Subscribe` concurrentes o secuenciales lean desde cualquier punto.
  Sin persistencia en disco: si `arg0sd` muere, se pierde el historial
  de tasks en vuelo -- aceptable para Fase 4 (reengancharse es
  reconectar al MISMO proceso de arg0sd, no sobrevivir su reinicio).
- `ListenAndServe` borra un socket huérfano existente antes de
  escuchar (`os.Remove` + `IsNotExist` check) -- un `arg0sd` anterior
  que murió mal (sin limpiar su socket) no debe impedir que el
  siguiente arranque.
