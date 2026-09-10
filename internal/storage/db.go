// Package storage abre la base SQLite de Arg0s (modernc.org/sqlite, sin
// CGO) y corre las migraciones embebidas. Ver docs/ARG0S.md sección 19.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

// Open abre (o crea) la base en path y corre las migraciones pendientes.
// Crea el directorio padre si hace falta (~/.arg0s/ en un primer run).
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite %s: %w", path, err)
	}

	db := &DB{DB: sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return db, nil
}
