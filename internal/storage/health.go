package storage

import (
	"fmt"

	"github.com/artumarinn/arg0s/internal/core"
)

// Health reporta el check 8 de la sección 6.7 (SQLite abre, migraciones
// al día).
func (db *DB) Health(name string) []core.Check {
	applied, total, err := db.MigrationStatus()
	if err != nil {
		return []core.Check{{Group: "Storage", Name: name, Status: core.CheckFail, Detail: err.Error()}}
	}
	if applied != total {
		return []core.Check{{Group: "Storage", Name: name, Status: core.CheckFail,
			Detail: fmt.Sprintf("migrations desactualizadas (%d/%d)", applied, total)}}
	}
	return []core.Check{{Group: "Storage", Name: name, Status: core.CheckPass,
		Detail: fmt.Sprintf("migrations up to date (%d/%d)", applied, total)}}
}
