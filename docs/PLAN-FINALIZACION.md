# Plan de Finalización — Arg0s

> Fecha: 2026-09-18
> Base: `docs/ARG0S.md` §21 (roadmap), estado real del repo, memoria Engram.
> Cada bloque termina con su DoD verificable del spec.

## Paso 0 — Cerrar Fase 4 (bloqueante)

El código está completo y commiteado. Falta la verificación manual en TTY real
(Engram #627) que no se puede hacer en sandbox sin `/dev/tty`.

- [ ] `arg0sd` levantado + `arg0s tui` muestra corrida en vivo sin congelarse
- [ ] Cerrar la TUI y reabrirla → reengancha con la ejecución en curso
- [ ] `arg0s bench fusion` reporta ensemble gain vs baseline
- [ ] Cerrar Engram #627 + `session_summary` de Fase 4 (gap desde el 12/09)
- [ ] Actualizar README (dice "Fase 0", está desactualizado)
- [ ] Decidir `AGENTS.md` y `.codegraph/` en git
- [ ] Commit de cierre

**DoD (spec §21 Fase 4):** la TUI muestra una corrida completa en vivo sin
congelarse; cerrar la TUI y reabrirla reengancha con la ejecución en curso;
hay un número que dice si fusion mejora o no.

## Fase 5 — Judgment Day + Skills

La feature diferencial. Un commit por ítem.

1. **Snapshots inmutables** — `internal/judgment/snapshot.go`: hash de
   contenido, copia aislada del scope. Un juez nunca evalúa "el repo actual"
   (spec §11.1).
2. **Judge runner con ceguera garantizada** — A y B reciben input idéntico
   (snapshot + instrucción), sin session compartido. Perspectivas
   heterogéneas (A=correctness, B=security). `independence` registrado en el
   ledger. Warning en `doctor` si `judge_a.model == judge_b.model` (§11.3).
3. **Esquema JSON estricto** — prompt en `internal/judgment/prompts/`
   (dir existe, vacío). 1 retry en parse failure, luego `parse_failure` y
   descarte. Findings sin `file:line` verificable se descartan
   (`require_evidence`, §11.5).
4. **Merge → ledger** — CONFIRMED / SUSPECT / CONTRADICTION con la regla de
   solapamiento de rangos de §11.6.
5. **`fail_fast`** — si Judge A no ve CRITICAL/SEVERE →
   `APPROVED_WITH_WARNINGS` en 1 llamada (§11.2).
6. **Fix agent acotado** — solo findings CONFIRMED en `fix_severities`, solo
   archivos de la evidencia, produce diff (no edita), se aplica a copia →
   Snapshot N+1. El working tree no se toca sin confirmación (§11.7).
7. **Re-juicio + veredictos** — `max_rounds`, APPROVED /
   APPROVED_WITH_WARNINGS / ESCALATED / FAILED, confirmación humana antes de
   aplicar (§11.8).
8. **CLI** — `arg0s judge <path> --run` + `judge ledger <id>` (auditable).
9. **Skills** — registry index-first (`name + description + triggers + path
   + token_cost`), `arg0s skills list/show/install`, skill nativa
   `judgment-day` (`skills/native/judgment-day/` existe, vacío) (§14).
10. **Corpus + benchmark** — `testdata/injected-bugs/` con ground truth
    (está vacío: diseñar ~10-15 bugs inyectados con severidades conocidas),
    `arg0s bench judges` → precision/recall reales.

**DoD:** `arg0s judge ./auth --run` produce un ledger auditable, y el
benchmark contra el corpus da números reales de precision/recall.

## Fase 6 — Runtimes de agentes

1. Interfaz `Runtime` (`Available()`, `Run()`) en `internal/runtimes/`.
2. Adapters `claudecode` y `codex` (dirs scaffolded, vacíos) + gemini-cli —
   proceso, no PTY interactivo si se puede evitar.
3. Detección de versión, timeouts agresivos, kill de procesos hijos,
   degradación elegante.
4. `arg0s skills sync --target claude-code`.

**DoD:** un runtime caído o con formato cambiado no rompe Arg0s; se reporta
como no disponible y el router lo excluye.

## Fase 7 — Inteligencia adaptativa

1. Tabla de performance histórica por (modelo, task type) en `arg0s.db`
   (migración nueva).
2. Scoring `quality × capability × historical_success / (cost × latency)`.
3. Router modo `learned` con fallback a heurística.
4. Feedback 👍/👎 por run (persiste en `runs`).
5. `arg0s bench report` — tabla completa de métricas, todas con baseline
   explícito.
6. Adapter LiteLLM — opcional, solo si acaso (Engram #600: adapter V6+, no
   capa central).

**DoD:** `arg0s bench report` produce la tabla completa de métricas, todas
con baseline explícito.

## Cierre final

- Sweep: `go vet`, tests, `arg0s doctor` verde end-to-end.
- Docs: README con estado real, `docs/decisions/` con ADRs nuevos si
  surgieron (ej. formato del ledger).
- Portfolio: el `bench report` final es la pieza que demuestra la
  orquestación medida (P10: todo se mide o no existe).

## Dependencias externas

- Fase 5 necesita **dos providers reales y distintos** configurados
  (gemini + ollama, p.ej.) — con uno solo todo cae en `independence: low` y
  el benchmark pierde sentido.
- Fase 6 necesita los CLIs instalados (claude, codex) — con degradación
  elegante igual funciona sin ellos.
