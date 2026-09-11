package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openRunsTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "arg0s.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInsertRun_RoundTripsViaGetRun(t *testing.T) {
	db := openRunsTestDB(t)
	ctx := context.Background()

	sessionID := NewSessionID()
	require.NoError(t, db.InsertSession(ctx, sessionID, "/tmp/proj"))

	runID := NewRunID()
	started := time.Now().Add(-2 * time.Second)
	ended := time.Now()
	require.NoError(t, db.InsertRun(ctx, Run{
		ID: runID, SessionID: sessionID, Prompt: "hola", Strategy: "direct",
		Status: "completed", InputTokens: 10, OutputTokens: 20, CostUSD: 0.001,
		StartedAt: started, EndedAt: ended,
	}))

	mrID := NewModelRunID()
	require.NoError(t, db.InsertModelRun(ctx, ModelRunRow{
		ID: mrID, RunID: runID, ModelID: "gemini-flash", Provider: "gemini", Role: "generator",
		InputTokens: 10, OutputTokens: 20, CostUSD: 0.001, LatencyMS: 342, Attempts: 1,
		Status: "ok", StartedAt: started,
	}))

	r, modelRuns, err := db.GetRun(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, "hola", r.Prompt)
	require.Equal(t, "completed", r.Status)
	require.Equal(t, 0.001, r.CostUSD)
	require.Len(t, modelRuns, 1)
	require.Equal(t, "gemini-flash", modelRuns[0].ModelID)
	require.Equal(t, int64(342), modelRuns[0].LatencyMS)
}

func TestGetRun_NotFound(t *testing.T) {
	db := openRunsTestDB(t)
	_, _, err := db.GetRun(context.Background(), "run_no-existe")
	require.ErrorIs(t, err, ErrRunNotFound)
}

func TestCostSummary_SumsOnlyRunsSinceGivenTime(t *testing.T) {
	db := openRunsTestDB(t)
	ctx := context.Background()

	sessionID := NewSessionID()
	require.NoError(t, db.InsertSession(ctx, sessionID, "/tmp/proj"))

	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, db.InsertRun(ctx, Run{
		ID: NewRunID(), SessionID: sessionID, Prompt: "viejo", Strategy: "direct", Status: "completed",
		CostUSD: 5.0, StartedAt: old, EndedAt: old,
	}))

	recent := time.Now()
	require.NoError(t, db.InsertRun(ctx, Run{
		ID: NewRunID(), SessionID: sessionID, Prompt: "reciente", Strategy: "direct", Status: "completed",
		InputTokens: 3, OutputTokens: 4, CostUSD: 0.01, StartedAt: recent, EndedAt: recent,
	}))

	since := time.Now().Add(-24 * time.Hour)
	summary, err := db.CostSummary(ctx, since)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Runs)
	require.InDelta(t, 0.01, summary.CostUSD, 0.0001)
}

func TestCostByModel_GroupsAndSums(t *testing.T) {
	db := openRunsTestDB(t)
	ctx := context.Background()

	sessionID := NewSessionID()
	require.NoError(t, db.InsertSession(ctx, sessionID, "/tmp/proj"))
	runID := NewRunID()
	now := time.Now()
	require.NoError(t, db.InsertRun(ctx, Run{ID: runID, SessionID: sessionID, Prompt: "x", Strategy: "direct", Status: "completed", StartedAt: now, EndedAt: now}))

	require.NoError(t, db.InsertModelRun(ctx, ModelRunRow{ID: NewModelRunID(), RunID: runID, ModelID: "gemini-flash", Provider: "gemini", Role: "generator", CostUSD: 0.01, StartedAt: now}))
	require.NoError(t, db.InsertModelRun(ctx, ModelRunRow{ID: NewModelRunID(), RunID: runID, ModelID: "gemini-flash", Provider: "gemini", Role: "generator", CostUSD: 0.02, StartedAt: now}))
	require.NoError(t, db.InsertModelRun(ctx, ModelRunRow{ID: NewModelRunID(), RunID: runID, ModelID: "qwen-coder-7b", Provider: "ollama", Role: "generator", CostUSD: 0.0, StartedAt: now}))

	costs, err := db.CostByModel(ctx, now.Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, costs, 2)
	require.Equal(t, "gemini-flash", costs[0].ModelID) // orden desc por costo
	require.InDelta(t, 0.03, costs[0].CostUSD, 0.0001)
	require.Equal(t, 2, costs[0].Runs)
}
