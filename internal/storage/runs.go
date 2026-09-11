package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Session es el mínimo necesario para satisfacer el FK de runs.session_id
// (sessions.id). La gestión completa de sesiones (list/show/history)
// es internal/session/, fuera de alcance de Fase 1 — acá solo se crea
// una sesión implícita por invocación de `arg0s run`.
func NewSessionID() string  { return "sess_" + uuid.NewString() }
func NewRunID() string      { return "run_" + uuid.NewString() }
func NewModelRunID() string { return "mr_" + uuid.NewString() }

func (db *DB) InsertSession(ctx context.Context, id, project string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO sessions (id, project, created_at) VALUES (?, ?, ?)`,
		id, project, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("insert session %s: %w", id, err)
	}
	return nil
}

// Run es una fila de la tabla `runs` (sección 19).
type Run struct {
	ID           string
	SessionID    string
	Prompt       string
	Strategy     string
	Status       string // completed | failed | cancelled
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	StartedAt    time.Time
	EndedAt      time.Time
	Error        string
}

// InsertRun persiste un run ya terminado. Fase 1 no tiene progreso en
// vivo que mostrar (eso es la TUI de Fase 4) — se inserta una sola vez,
// con el resultado final.
func (db *DB) InsertRun(ctx context.Context, r Run) error {
	var errVal any
	if r.Error != "" {
		errVal = r.Error
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO runs (id, session_id, prompt, strategy, status, input_tokens, output_tokens, cost_usd, started_at, ended_at, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.SessionID, r.Prompt, r.Strategy, r.Status,
		r.InputTokens, r.OutputTokens, r.CostUSD,
		r.StartedAt.Unix(), r.EndedAt.Unix(), errVal,
	)
	if err != nil {
		return fmt.Errorf("insert run %s: %w", r.ID, err)
	}
	return nil
}

// ModelRunRow es una fila de `model_runs`.
type ModelRunRow struct {
	ID           string
	RunID        string
	ModelID      string
	Provider     string
	Role         string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	LatencyMS    int64
	Attempts     int
	FellBackFrom string
	Status       string // ok | failed
	Error        string
	StartedAt    time.Time
}

func (db *DB) InsertModelRun(ctx context.Context, mr ModelRunRow) error {
	var fellBack, errVal any
	if mr.FellBackFrom != "" {
		fellBack = mr.FellBackFrom
	}
	if mr.Error != "" {
		errVal = mr.Error
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO model_runs (id, run_id, model_id, provider, role, input_tokens, output_tokens, cost_usd, latency_ms, attempts, fell_back_from, status, error, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		mr.ID, mr.RunID, mr.ModelID, mr.Provider, mr.Role,
		mr.InputTokens, mr.OutputTokens, mr.CostUSD, mr.LatencyMS, mr.Attempts,
		fellBack, mr.Status, errVal, mr.StartedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert model_run %s: %w", mr.ID, err)
	}
	return nil
}

var ErrRunNotFound = errors.New("run not found")

// GetRun trae un run y sus model_runs asociados — para `arg0s run show`.
func (db *DB) GetRun(ctx context.Context, id string) (*Run, []ModelRunRow, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, session_id, prompt, strategy, status, input_tokens, output_tokens, cost_usd, started_at, ended_at, COALESCE(error, '')
		FROM runs WHERE id = ?`, id)

	var r Run
	var started, ended int64
	if err := row.Scan(&r.ID, &r.SessionID, &r.Prompt, &r.Strategy, &r.Status,
		&r.InputTokens, &r.OutputTokens, &r.CostUSD, &started, &ended, &r.Error); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrRunNotFound
		}
		return nil, nil, fmt.Errorf("get run %s: %w", id, err)
	}
	r.StartedAt = time.Unix(started, 0)
	r.EndedAt = time.Unix(ended, 0)

	rows, err := db.QueryContext(ctx, `
		SELECT id, run_id, model_id, provider, role, input_tokens, output_tokens, cost_usd, latency_ms, attempts, COALESCE(fell_back_from, ''), status, COALESCE(error, ''), started_at
		FROM model_runs WHERE run_id = ? ORDER BY started_at`, id)
	if err != nil {
		return nil, nil, fmt.Errorf("list model_runs for %s: %w", id, err)
	}
	defer rows.Close()

	var modelRuns []ModelRunRow
	for rows.Next() {
		var mr ModelRunRow
		var started int64
		if err := rows.Scan(&mr.ID, &mr.RunID, &mr.ModelID, &mr.Provider, &mr.Role,
			&mr.InputTokens, &mr.OutputTokens, &mr.CostUSD, &mr.LatencyMS, &mr.Attempts,
			&mr.FellBackFrom, &mr.Status, &mr.Error, &started); err != nil {
			return nil, nil, fmt.Errorf("scan model_run: %w", err)
		}
		mr.StartedAt = time.Unix(started, 0)
		modelRuns = append(modelRuns, mr)
	}
	return &r, modelRuns, rows.Err()
}

// CostSummary suma costo/tokens/runs desde `since` — `arg0s cost --today`.
type CostSummary struct {
	CostUSD      float64
	InputTokens  int
	OutputTokens int
	Runs         int
}

func (db *DB) CostSummary(ctx context.Context, since time.Time) (CostSummary, error) {
	row := db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COUNT(*)
		FROM runs WHERE started_at >= ?`, since.Unix())

	var s CostSummary
	if err := row.Scan(&s.CostUSD, &s.InputTokens, &s.OutputTokens, &s.Runs); err != nil {
		return CostSummary{}, fmt.Errorf("cost summary: %w", err)
	}
	return s, nil
}

// ModelCost es una fila de `arg0s cost --by-model`.
type ModelCost struct {
	ModelID string
	CostUSD float64
	Runs    int
}

func (db *DB) CostByModel(ctx context.Context, since time.Time) ([]ModelCost, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT model_id, COALESCE(SUM(cost_usd), 0), COUNT(*)
		FROM model_runs WHERE started_at >= ?
		GROUP BY model_id ORDER BY SUM(cost_usd) DESC`, since.Unix())
	if err != nil {
		return nil, fmt.Errorf("cost by model: %w", err)
	}
	defer rows.Close()

	var out []ModelCost
	for rows.Next() {
		var m ModelCost
		if err := rows.Scan(&m.ModelID, &m.CostUSD, &m.Runs); err != nil {
			return nil, fmt.Errorf("scan model cost: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
