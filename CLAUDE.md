# Arg0s — Guía para el agente de código

Fuente de verdad completa: [docs/ARG0S.md](docs/ARG0S.md). Este archivo
es un resumen operativo, no un reemplazo — ante cualquier duda, gana
docs/ARG0S.md.

**Fase actual: FASE 3** (motor de contexto). Fase 0 (fundación), Fase 1
(ejecución de modelos) y Fase 2 (router) cerradas y commiteadas.

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
11. Los comandos de diagnóstico y consulta son **solo-lectura**: `doctor`,
    `providers list`, `models list`, `graph query`, `session show`,
    `cost`. Ninguno crea, migra ni muta estado — si falta algo, lo
    reportan y dicen qué comando lo crea. Solo `init`, `run` y los
    mutadores explícitos escriben.
    Origen: `doctor` creaba `arg0s.db` como side effect al chequear
    storage, y eso rompía la detección de conflicto de `arg0s init` en
    la corrida siguiente (un `arg0s.db` huérfano bloqueaba el `init`
    posterior sin `--force`). Ver Engram `arg0s/preference/read-only-diagnostics`.

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
