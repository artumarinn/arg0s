package config

// ProviderConfig es la representación parseada de una entrada bajo
// `providers:` en config.yaml. internal/providers (Fase 1) consume este
// struct sin importar el paquete config — ver ADR sobre desacoplamiento
// providers/config.
type ProviderConfig struct {
	Enabled      bool              `yaml:"enabled"`
	Type         string            `yaml:"type"`
	BaseURL      string            `yaml:"base_url"`
	APIKeyEnv    string            `yaml:"api_key_env"`
	Timeout      string            `yaml:"timeout"`
	MaxRetries   int               `yaml:"max_retries"`
	RetryBackoff string            `yaml:"retry_backoff"`
	RateLimit    RateLimitConfig   `yaml:"rate_limit"`
	Options      map[string]any    `yaml:"options,omitempty"`
	Headers      map[string]string `yaml:"headers,omitempty"`
}

type RateLimitConfig struct {
	RequestsPerMinute int `yaml:"requests_per_minute"`
	Concurrent        int `yaml:"concurrent"`
}

// RuntimeConfig es la entrada bajo `runtimes:` (Fase 6, declarado y
// deshabilitado desde Fase 0).
type RuntimeConfig struct {
	Enabled        bool     `yaml:"enabled"`
	Type           string   `yaml:"type"`
	Binary         string   `yaml:"binary"`
	Args           []string `yaml:"args,omitempty"`
	WorkingDir     string   `yaml:"working_dir,omitempty"`
	Timeout        string   `yaml:"timeout"`
	EnvPassthrough []string `yaml:"env_passthrough,omitempty"`
}
