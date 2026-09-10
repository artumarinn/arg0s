package storage

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/artumarinn/arg0s/internal/core"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "arg0s.db")
	db, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpen_RunsMigrations(t *testing.T) {
	db := openTestDB(t)

	applied, total, err := db.MigrationStatus()
	require.NoError(t, err)
	require.Equal(t, total, applied)
	require.Positive(t, total)

	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'runs'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestOpen_MigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "arg0s.db")

	db1, err := Open(path)
	require.NoError(t, err)
	db1.Close()

	// Reabrir sobre la misma base no debe reintentar crear tablas ya
	// existentes ni fallar.
	db2, err := Open(path)
	require.NoError(t, err)
	defer db2.Close()

	applied, total, err := db2.MigrationStatus()
	require.NoError(t, err)
	require.Equal(t, total, applied)
}

func TestHealth_ReportsUpToDate(t *testing.T) {
	db := openTestDB(t)
	checks := db.Health("arg0s.db")
	require.Len(t, checks, 1)
	require.Equal(t, core.CheckPass, checks[0].Status)
}
