package storage

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationFiles devuelve los nombres de archivo *.sql embebidos,
// ordenados por su prefijo numérico (001_init.sql, 002_xxx.sql, ...).
func migrationFiles() ([]string, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func migrationVersion(filename string) (int, error) {
	prefix, _, ok := strings.Cut(filename, "_")
	if !ok {
		return 0, fmt.Errorf("migration filename %q sin prefijo numérico", filename)
	}
	return strconv.Atoi(prefix)
}

// migrate aplica, en orden y de forma idempotente, las migraciones que
// todavía no estén en schema_migrations. La propia tabla
// schema_migrations la crea 001_init.sql — no se bootstrapea acá, para
// no duplicar su definición.
func (db *DB) migrate() error {
	tableExists, err := db.tableExists("schema_migrations")
	if err != nil {
		return fmt.Errorf("check schema_migrations: %w", err)
	}

	files, err := migrationFiles()
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}

	for _, name := range files {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}

		if tableExists {
			var applied int
			row := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version)
			if err := row.Scan(&applied); err != nil {
				return fmt.Errorf("check migration %d: %w", version, err)
			}
			if applied > 0 {
				continue
			}
		}

		sqlBytes, err := migrationsFS.ReadFile(path.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			version, time.Now().Unix()); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}
		tableExists = true
	}
	return nil
}

func (db *DB) tableExists(name string) (bool, error) {
	var found string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// MigrationStatus devuelve (aplicadas, totales) para reportar en doctor.
func (db *DB) MigrationStatus() (applied, total int, err error) {
	files, err := migrationFiles()
	if err != nil {
		return 0, 0, err
	}
	total = len(files)

	row := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`)
	if err := row.Scan(&applied); err != nil {
		return 0, total, err
	}
	return applied, total, nil
}
