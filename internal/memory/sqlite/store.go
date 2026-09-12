// Package sqlite es la implementación default de memory.MemoryStore
// (sección 12.4: "SQLite es la implementación default; Engram es un
// adapter opcional que se agrega después sin tocar el resto del
// sistema"). Vive sobre la tabla `memories` que ya crea
// internal/storage -- este paquete no abre la base ni corre
// migraciones, solo lee/escribe esa tabla; recibe un *sql.DB ya
// migrado (mismo patrón que telemetry.NewBus).
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/artumarinn/arg0s/internal/memory"
)

type Store struct {
	db *sql.DB

	// revalidateAfter es memory.decay.revalidate_after -- Query() lo usa
	// para CALCULAR (nunca escribir) si una memoria ya venció; escribir
	// el flag stale es responsabilidad de Revalidate(), no de una
	// lectura (ver comentario en Query).
	revalidateAfter time.Duration
}

func New(db *sql.DB, revalidateAfter time.Duration) *Store {
	return &Store{db: db, revalidateAfter: revalidateAfter}
}

func NewID() string { return "mem_" + uuid.NewString() }

func (s *Store) Store(ctx context.Context, m memory.Memory) error {
	if m.ID == "" {
		m.ID = NewID()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.LastVerified.IsZero() {
		m.LastVerified = m.CreatedAt
	}

	refs, err := json.Marshal(m.Refs)
	if err != nil {
		return fmt.Errorf("memory: marshal refs: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO memories (id, project, category, title, content, confidence, source, refs, created_at, last_verified, stale)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Project, string(m.Category), m.Title, m.Content, m.Confidence,
		nullIfEmpty(m.Source), string(refs), m.CreatedAt.Unix(), m.LastVerified.Unix(), boolToInt(m.Stale),
	)
	if err != nil {
		return fmt.Errorf("memory: insert %s: %w", m.ID, err)
	}
	return nil
}

func (s *Store) Forget(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("memory: forget %s: %w", id, err)
	}
	return nil
}

// Query implementa sección 12 regla 1 (query → candidatos → ranking →
// filtro de relevancia → budget): trae los candidatos del proyecto (y
// categorías, si se pidieron), calcula Relevance lexical contra
// q.Text, descarta lo que no llega a MinRelevance, ordena por
// relevancia descendente y corta en Limit.
//
// Query es de LECTURA PURA -- no escribe nada, ni siquiera el flag
// `stale`. La regla dura #11 (comandos de consulta son solo-lectura)
// aplica a todo método que el CLI expone como consulta, no solo a la
// lista original de comandos; un Context Compiler (Fase 3 parte B) va
// a llamar Query() en cada task, y una escritura como side-effect de
// leer sería sorprendente ahí. La staleness se CALCULA al vuelo
// (effectiveStale) e se informa en el campo Stale del resultado;
// escribirla de verdad es responsabilidad explícita de Revalidate().
func (s *Store) Query(ctx context.Context, q memory.MemoryQuery) ([]memory.Memory, error) {
	candidates, err := s.candidates(ctx, q)
	if err != nil {
		return nil, err
	}

	type scored struct {
		m memory.Memory
		r float64
	}
	var ranked []scored
	for _, m := range candidates {
		m.Stale = s.effectiveStale(m)
		if m.Stale && !q.IncludeStale {
			continue
		}
		r := Relevance(q.Text, m.Title, m.Content)
		if r < q.MinRelevance {
			continue
		}
		ranked = append(ranked, scored{m, r})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].r > ranked[j].r })

	if q.Limit > 0 && len(ranked) > q.Limit {
		ranked = ranked[:q.Limit]
	}

	out := make([]memory.Memory, len(ranked))
	for i, sc := range ranked {
		out[i] = sc.m
	}
	return out, nil
}

// Revalidate es la ÚNICA operación que persiste `stale` -- explícita
// (comando `arg0s memory revalidate`), nunca disparada por una
// lectura. Recorre las memorias de project (todas las categorías) y
// escribe stale=1 en las que acaban de cruzar revalidate_after.
// Devuelve cuántas marcó.
func (s *Store) Revalidate(ctx context.Context, project string) (int, error) {
	candidates, err := s.candidates(ctx, memory.MemoryQuery{Project: project, IncludeStale: true})
	if err != nil {
		return 0, err
	}

	marked := 0
	for _, m := range candidates {
		if m.Stale || !s.effectiveStale(m) {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE memories SET stale = 1 WHERE id = ?`, m.ID); err != nil {
			return marked, fmt.Errorf("memory: revalidate %s: %w", m.ID, err)
		}
		marked++
	}
	return marked, nil
}

// effectiveStale es la staleness "de verdad" en este instante: lo que
// ya está persistido, O lo que acaba de vencer según revalidate_after
// aunque la fila todavía diga stale=0. revalidateAfter<=0 desactiva el
// decaimiento (equivalente a decay.enabled: false) -- ahí solo cuenta
// lo persistido.
func (s *Store) effectiveStale(m memory.Memory) bool {
	if m.Stale {
		return true
	}
	if s.revalidateAfter <= 0 {
		return false
	}
	return time.Since(m.LastVerified) > s.revalidateAfter
}

func (s *Store) candidates(ctx context.Context, q memory.MemoryQuery) ([]memory.Memory, error) {
	query := `SELECT id, project, category, title, content, confidence, COALESCE(source, ''), refs, created_at, last_verified, stale FROM memories WHERE project = ?`
	args := []any{q.Project}

	if len(q.Categories) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(q.Categories)), ",")
		query += fmt.Sprintf(" AND category IN (%s)", placeholders)
		for _, c := range q.Categories {
			args = append(args, string(c))
		}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: query: %w", err)
	}
	defer rows.Close()

	var out []memory.Memory
	for rows.Next() {
		var m memory.Memory
		var category, refsJSON string
		var created, lastVerified int64
		var stale int
		if err := rows.Scan(&m.ID, &m.Project, &category, &m.Title, &m.Content, &m.Confidence,
			&m.Source, &refsJSON, &created, &lastVerified, &stale); err != nil {
			return nil, fmt.Errorf("memory: scan: %w", err)
		}
		m.Category = memory.Category(category)
		m.CreatedAt = time.Unix(created, 0)
		m.LastVerified = time.Unix(lastVerified, 0)
		m.Stale = stale != 0
		_ = json.Unmarshal([]byte(refsJSON), &m.Refs)
		out = append(out, m)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
