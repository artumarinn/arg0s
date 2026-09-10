package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/artumarinn/arg0s/internal/core"
)

// Validate corre los checks de la sección 6.7 que aplican sin providers
// implementados (checks 1-5, 11, 12). Los checks 6, 7, 9 y 10 requieren
// providers/runtimes/LSP reales (Fase 1+) y se reportan como skip desde
// cmd/arg0s/doctor.go, no acá.
func Validate(cfg *Config, models *ModelsFile) []core.Check {
	var checks []core.Check
	checks = append(checks, checkHomeDir()...)
	checks = append(checks, checkConfigFile()...)
	checks = append(checks, checkModelsFile(models)...)
	checks = append(checks, checkEnvFile(cfg)...)
	checks = append(checks, checkProviders(cfg)...)
	checks = append(checks, checkRoles(cfg, models)...)
	checks = append(checks, checkLimits(cfg)...)
	return checks
}

func checkHomeDir() []core.Check {
	home, err := Home()
	if err != nil {
		return []core.Check{{Group: "Config", Name: home, Status: core.CheckFail, Detail: err.Error()}}
	}
	info, err := os.Stat(home)
	if err != nil {
		if os.IsNotExist(err) {
			return []core.Check{{Group: "Config", Name: home, Status: core.CheckFail, Detail: "no existe"}}
		}
		return []core.Check{{Group: "Config", Name: home, Status: core.CheckFail, Detail: err.Error()}}
	}
	return []core.Check{{
		Group: "Config", Name: home, Status: core.CheckPass,
		Detail: fmt.Sprintf("mode %04o", info.Mode().Perm()),
	}}
}

func checkConfigFile() []core.Check {
	home, err := Home()
	if err != nil {
		return nil
	}
	path := filepath.Join(home, "config.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []core.Check{{Group: "Config", Name: path, Status: core.CheckPass, Detail: "valid (usando defaults, no existe archivo)"}}
	}
	// Si llegamos acá con un *Config ya cargado, es porque Load() lo
	// parseó sin error — si hubiera fallado, doctor no habría llegado
	// a llamar Validate. Confirmamos existencia y legibilidad.
	return []core.Check{{Group: "Config", Name: path, Status: core.CheckPass, Detail: "valid"}}
}

func checkModelsFile(models *ModelsFile) []core.Check {
	home, err := Home()
	if err != nil {
		return nil
	}
	path := filepath.Join(home, "models.yaml")
	detail := fmt.Sprintf("valid  (%d models, %d aliases)", len(models.Models), len(models.Aliases))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		detail = fmt.Sprintf("valid  (%d models, %d aliases, usando defaults, no existe archivo)", len(models.Models), len(models.Aliases))
	}
	return []core.Check{{Group: "Config", Name: path, Status: core.CheckPass, Detail: detail}}
}

func checkEnvFile(cfg *Config) []core.Check {
	home, err := Home()
	if err != nil {
		return nil
	}
	path := filepath.Join(home, ".env")
	info, err := os.Stat(path)
	if err != nil {
		// Solo importa si algún provider habilitado necesita una key.
		for _, p := range cfg.Providers {
			if p.Enabled && p.APIKeyEnv != "" {
				return []core.Check{{Group: "Config", Name: path, Status: core.CheckFail, Detail: "no existe, pero hay providers habilitados que requieren api_key_env"}}
			}
		}
		return []core.Check{{Group: "Config", Name: path, Status: core.CheckSkip, Detail: "no existe (ningún provider habilitado lo requiere)"}}
	}
	perm := info.Mode().Perm()
	if perm&0o077 != 0 {
		return []core.Check{{Group: "Config", Name: path, Status: core.CheckFail, Detail: fmt.Sprintf("mode %04o demasiado permisivo, usar chmod 600", perm)}}
	}
	return []core.Check{{Group: "Config", Name: path, Status: core.CheckPass, Detail: fmt.Sprintf("mode %04o", perm)}}
}

// checkProviders reporta declaración/keys de cada provider (checks 4 y 5).
// La conectividad real (check 6) es Fase 1 — no se hace acá.
func checkProviders(cfg *Config) []core.Check {
	var checks []core.Check
	for _, name := range sortedKeys(cfg.Providers) {
		p := cfg.Providers[name]
		if !p.Enabled {
			checks = append(checks, core.Check{Group: "Providers", Name: name, Status: core.CheckSkip, Detail: "disabled"})
			continue
		}
		if p.APIKeyEnv == "" {
			checks = append(checks, core.Check{Group: "Providers", Name: name, Status: core.CheckPass, Detail: "sin api_key_env (local)"})
			continue
		}
		if EnvValue(p.APIKeyEnv) == "" {
			// Vacía no es un error de configuración en sí — es un
			// to-do pendiente (llenar .env). La ausencia total de
			// .env cuando hace falta sí es error: la marca checkEnvFile.
			checks = append(checks, core.Check{Group: "Providers", Name: name, Status: core.CheckSkip, Detail: fmt.Sprintf("%s no seteada (ver .env)", p.APIKeyEnv)})
			continue
		}
		checks = append(checks, core.Check{Group: "Providers", Name: name, Status: core.CheckPass, Detail: fmt.Sprintf("%s presente", p.APIKeyEnv)})
	}
	return checks
}

// checkRoles cubre los checks 3 (modelos referenciados existen), 4
// (provider del modelo existe y está enabled) y 11 (nada apunta a un
// modelo deshabilitado).
func checkRoles(cfg *Config, models *ModelsFile) []core.Check {
	var checks []core.Check
	for name, role := range cfg.Roles.All() {
		modelID := models.ResolveModel(role.Model)
		m, ok := models.Models[modelID]
		if !ok {
			checks = append(checks, core.Check{Group: "Roles", Name: name, Status: core.CheckFail,
				Detail: fmt.Sprintf("→ %s (model not found in models.yaml)", role.Model)})
			continue
		}
		p, ok := cfg.Providers[m.Provider]
		if !ok {
			checks = append(checks, core.Check{Group: "Roles", Name: name, Status: core.CheckFail,
				Detail: fmt.Sprintf("→ %s (provider %q no declarado)", role.Model, m.Provider)})
			continue
		}
		if !p.Enabled {
			checks = append(checks, core.Check{Group: "Roles", Name: name, Status: core.CheckFail,
				Detail: fmt.Sprintf("→ %s (provider %q disabled)", role.Model, m.Provider)})
			continue
		}
		checks = append(checks, core.Check{Group: "Roles", Name: name, Status: core.CheckPass, Detail: "→ " + role.Model})
	}
	sort.Slice(checks, func(i, j int) bool { return checks[i].Name < checks[j].Name })
	return checks
}

func checkLimits(cfg *Config) []core.Check {
	if cfg.Limits.PerTaskCostUSD > cfg.Limits.DailyCostUSD {
		return []core.Check{{Group: "Config", Name: "limits", Status: core.CheckFail,
			Detail: fmt.Sprintf("per_task_cost_usd (%.2f) > daily_cost_usd (%.2f)", cfg.Limits.PerTaskCostUSD, cfg.Limits.DailyCostUSD)}}
	}
	return []core.Check{{Group: "Config", Name: "limits", Status: core.CheckPass,
		Detail: fmt.Sprintf("per_task %.2f ≤ daily %.2f", cfg.Limits.PerTaskCostUSD, cfg.Limits.DailyCostUSD)}}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
