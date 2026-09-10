package core

// CheckStatus es el resultado de un check individual de `arg0s doctor`.
type CheckStatus string

const (
	CheckPass CheckStatus = "pass" // ✓
	CheckFail CheckStatus = "fail" // ✗
	CheckSkip CheckStatus = "skip" // ⊘ — disabled o no implementado en esta fase
)

// Check es un punto verificado por doctor (sección 6.7). Cada subsistema
// expone los suyos; cmd/arg0s/doctor.go solo agrega y renderiza.
type Check struct {
	Group  string // "Config" | "Providers" | "Roles" | "Storage" ...
	Name   string
	Status CheckStatus
	Detail string
}
