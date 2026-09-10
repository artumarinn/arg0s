package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// loadEnvFile parsea un .env simple (KEY=VALUE por línea, # comentarios).
// No exporta al proceso — solo lo usamos para validar presencia de keys.
func loadEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return values, scanner.Err()
}

// EnvValue busca key primero en el entorno del proceso, después en
// ~/.arg0s/.env (sección 6.5: las API keys solo se leen de ahí).
func EnvValue(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	home, err := Home()
	if err != nil {
		return ""
	}
	values, err := loadEnvFile(filepath.Join(home, ".env"))
	if err != nil {
		return ""
	}
	return values[key]
}
