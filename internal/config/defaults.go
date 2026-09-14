package config

// DefaultConfig son los defaults compilados en el binario — la capa de
// menor precedencia (sección 6.2). Reproduce el config.yaml de ejemplo
// de la sección 6.3: gemini y ollama enabled:true, openrouter disabled.
func DefaultConfig() *Config {
	return &Config{
		Version: 1,
		General: GeneralConfig{
			DefaultStrategy: "router",
			Theme:           "arg0s-dark",
			LogLevel:        "info",
			Telemetry:       true,
			Editor:          "nvim",
		},
		Daemon: DaemonConfig{
			Socket:       "~/.arg0s/daemon.sock",
			Autostart:    true,
			IdleShutdown: "30m",
		},
		Providers: map[string]ProviderConfig{
			"gemini": {
				Enabled:      true,
				Type:         "gemini",
				BaseURL:      "https://generativelanguage.googleapis.com/v1beta",
				APIKeyEnv:    "GEMINI_API_KEY",
				Timeout:      "120s",
				MaxRetries:   3,
				RetryBackoff: "exponential",
				RateLimit:    RateLimitConfig{RequestsPerMinute: 60, Concurrent: 4},
			},
			"ollama": {
				Enabled:      true,
				Type:         "ollama",
				BaseURL:      "http://localhost:11434",
				Timeout:      "300s",
				MaxRetries:   1,
				RetryBackoff: "exponential",
				RateLimit:    RateLimitConfig{Concurrent: 1},
				Options:      map[string]any{"keep_alive": "10m", "num_ctx": 32768},
			},
			"openrouter": {
				Enabled:      false,
				Type:         "openai_compatible",
				BaseURL:      "https://openrouter.ai/api/v1",
				APIKeyEnv:    "OPENROUTER_API_KEY",
				Timeout:      "180s",
				MaxRetries:   2,
				RetryBackoff: "exponential",
				RateLimit:    RateLimitConfig{RequestsPerMinute: 30, Concurrent: 3},
				Headers: map[string]string{
					"HTTP-Referer": "https://localhost/arg0s",
					"X-Title":      "Arg0s",
				},
			},
		},
		Runtimes: map[string]RuntimeConfig{
			"claude_code": {
				Enabled:        false,
				Type:           "claude_code",
				Binary:         "claude",
				Args:           []string{"-p"},
				WorkingDir:     ".",
				Timeout:        "600s",
				EnvPassthrough: []string{"PATH", "HOME", "ANTHROPIC_API_KEY"},
			},
			"codex": {
				Enabled: false,
				Type:    "codex",
				Binary:  "codex",
				Timeout: "600s",
			},
			"gemini_cli": {
				Enabled: false,
				Type:    "gemini_cli",
				Binary:  "gemini",
				Timeout: "600s",
			},
		},
		Roles: RolesConfig{
			Classifier:  RoleConfig{Model: "qwen-coder-7b", Fallback: "gemini-flash", Temperature: 0.0, MaxTokens: 512},
			Generator:   RoleConfig{Model: "gemini-flash", Fallback: "qwen-coder-14b", Temperature: 0.7},
			Synthesizer: RoleConfig{Model: "gemini-pro", Fallback: "gemini-flash", Temperature: 0.3},
			JudgeA:      RoleConfig{Model: "gemini-flash", Temperature: 0.0, MaxTokens: 4096, Perspective: "correctness"},
			JudgeB:      RoleConfig{Model: "qwen-coder-14b", Temperature: 0.0, MaxTokens: 4096, Perspective: "security"},
			JudgeC:      RoleConfig{Enabled: boolPtr(false), Model: "gemini-pro", Perspective: "architecture"},
			FixAgent:    RoleConfig{Model: "gemini-pro", Fallback: "gemini-flash", Temperature: 0.2},
			Summarizer:  RoleConfig{Model: "qwen-coder-7b", Temperature: 0.0},
		},
		Router: RouterConfig{
			Mode:                "heuristic",
			ClassifierThreshold: 0.6,
			Tiers: map[string]TierConfig{
				"simple":  {Models: []string{"qwen-coder-7b"}, MaxCostUSD: 0.0},
				"medium":  {Models: []string{"gemini-flash", "qwen-coder-14b"}, MaxCostUSD: 0.01},
				"complex": {Models: []string{"gemini-pro"}, MaxCostUSD: 0.10},
			},
			Overrides: []OverrideConfig{
				{When: OverrideWhen{Privacy: "secret"}, ForceTier: "simple", ForceLocal: true},
				{When: OverrideWhen{Type: "architecture"}, ForceTier: "complex"},
				{When: OverrideWhen{Type: "review", Complexity: "high"}, ForceStrategy: "judgment"},
			},
		},
		Limits: LimitsConfig{
			DailyCostUSD:    2.00,
			PerTaskCostUSD:  0.25,
			PerTaskMaxCalls: 8,
			WarnAtPercent:   80,
			OnExceed:        "block",
		},
		Memory: MemoryConfig{
			Enabled:              true,
			Backend:              "sqlite",
			Scope:                "project",
			MaxFragmentsPerQuery: 8,
			MinRelevance:         0.35,
			Decay:                DecayConfig{Enabled: true, RevalidateAfter: "30d"},
			Categories:           []string{"DECISION", "FACT", "BUG", "PREFERENCE", "ARCHITECTURE", "TODO", "LESSON"},
		},
		CodeGraph: CodeGraphConfig{
			Enabled:       true,
			Backend:       "lsp",
			IndexOnOpen:   true,
			Watch:         false,
			MaxFileSizeKB: 512,
			Ignore:        []string{"vendor/", "node_modules/", ".git/", "dist/", "build/", "*.min.js"},
			Servers: map[string]LSPServerConfig{
				"go":         {Command: "gopls", Args: []string{"serve"}},
				"python":     {Command: "pyright-langserver", Args: []string{"--stdio"}},
				"typescript": {Command: "typescript-language-server", Args: []string{"--stdio"}},
				"rust":       {Command: "rust-analyzer"},
			},
		},
		Fusion: FusionConfig{
			Enabled: true, Adaptive: true, MaxModels: 3, Parallel: true,
			Models: []string{"gemini-flash", "qwen-coder-14b"},
			// method:lexical acá, NO "hybrid" como en el config.yaml de
			// ejemplo (sección 6.3, template embebido) -- Fase 4 solo
			// implementa lexical de verdad; "hybrid" caería a lexical
			// igual (no hay judge todavía, Fase 5) así que el default
			// compilado dice lo que realmente hace, sin prometer más.
			Divergence: DivergenceConfig{Method: "lexical", Threshold: 0.15, EscalateThreshold: 0.45},
			Synthesis:  SynthesisConfig{Mode: "select_best"},
		},
		Context: ContextConfig{
			DefaultBudgetTokens: 16000,
			ReserveOutputTokens: 4000,
			BudgetSplit: map[string]float64{
				"prompt": 0.10, "code": 0.45, "graph": 0.10,
				"memory": 0.20, "skills": 0.10, "artifacts": 0.05,
			},
			CompressWhenOver: true,
			CompressionModel: "summarizer",
		},
	}
}

func boolPtr(b bool) *bool { return &b }
