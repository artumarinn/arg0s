package main

import (
	"net/http"
	"time"

	"github.com/artumarinn/arg0s/internal/codegraph"
	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/contextc"
	"github.com/artumarinn/arg0s/internal/execution"
	"github.com/artumarinn/arg0s/internal/fusion"
	"github.com/artumarinn/arg0s/internal/providers"
	"github.com/artumarinn/arg0s/internal/providers/gemini"
	"github.com/artumarinn/arg0s/internal/providers/ollama"
	openaicompatible "github.com/artumarinn/arg0s/internal/providers/openai_compatible"
	"github.com/artumarinn/arg0s/internal/router"
	"github.com/artumarinn/arg0s/internal/storage"
	"github.com/artumarinn/arg0s/internal/telemetry"
)

// buildRegistry construye un provider real por cada entrada enabled de
// cfg.Providers. Es el único lugar que conoce el mapeo type->paquete —
// ni execution ni providers saben de config (ver ADR de desacople).
func buildRegistry(cfg *config.Config) *providers.Registry {
	reg := providers.NewRegistry()
	client := &http.Client{}

	for name, p := range cfg.Providers {
		if !p.Enabled {
			continue
		}
		switch p.Type {
		case "ollama":
			reg.Register(ollama.New(ollama.Config{BaseURL: p.BaseURL, HTTP: client}))
		case "gemini":
			reg.Register(gemini.New(gemini.Config{BaseURL: p.BaseURL, APIKey: config.EnvValue(p.APIKeyEnv), HTTP: client}))
		case "openai_compatible":
			reg.Register(openaicompatible.New(openaicompatible.Config{
				Name: name, BaseURL: p.BaseURL, APIKey: config.EnvValue(p.APIKeyEnv), Headers: p.Headers, HTTP: client,
			}))
		}
	}
	return reg
}

// buildPolicies traduce cfg.Providers a execution.ProviderPolicy — sin
// que execution importe config.
func buildPolicies(cfg *config.Config) map[string]execution.ProviderPolicy {
	policies := map[string]execution.ProviderPolicy{}
	for name, p := range cfg.Providers {
		policies[name] = execution.ProviderPolicy{
			MaxRetries:   p.MaxRetries,
			RetryBackoff: p.RetryBackoff,
			Timeout:      parseDuration(p.Timeout, 120*time.Second),
			Concurrent:   p.RateLimit.Concurrent,
		}
	}
	return policies
}

func parseDuration(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

func buildExecutor(cfg *config.Config, models *config.ModelsFile, bus *telemetry.Bus) *execution.Executor {
	return execution.New(buildRegistry(cfg), models, buildPolicies(cfg), bus)
}

// buildRouter arma el Router de Fase 2. classifierExec es el mismo
// Executor de la corrida -- el router nunca habla con un provider
// directo (sección 8/P4) -- solo se usa si router.mode es classifier
// o hybrid.
func buildRouter(cfg *config.Config, models *config.ModelsFile, costs router.CostSource, exec *execution.Executor) *router.Router {
	return router.New(cfg.Router, cfg.Limits, cfg.Fusion, models, costs, exec, cfg.Roles.Classifier)
}

// buildContextCompiler arma el Context Compiler de Fase 3 parte B.
// exec/summarizerRole son nil-safe -- si no hay executor (ej `arg0s
// context preview` corriendo sin providers configurados), simplemente
// no hay compresión disponible y applyBudget cae al fallback de
// exclusión (comportamiento correcto, no un error).
func buildContextCompiler(cfg *config.Config, db *storage.DB, exec *execution.Executor) *contextc.Compiler {
	memStore := buildMemoryStore(cfg, db)
	graph := codegraph.NewGraph(db.DB)

	var summarizer contextc.Summarizer
	if exec != nil && cfg.Context.CompressWhenOver && cfg.Roles.Summarizer.Model != "" {
		summarizer = &contextc.ExecutorSummarizer{Exec: exec, Role: cfg.Roles.Summarizer}
	}

	return contextc.New(cfg.Context, memStore, cfg.Memory.MinRelevance, graph, summarizer)
}

// buildFusionEngine arma el Engine de Fase 4 parte A. El synthesizer
// es nil-safe en fusion.Engine (se degrada al primario si divergencia
// media y no hay con qué sintetizar) -- pero acá siempre hay executor
// real, así que siempre se arma.
func buildFusionEngine(cfg *config.Config, exec *execution.Executor) *fusion.Engine {
	synth := &fusion.ExecutorSynthesizer{Exec: exec, Role: cfg.Roles.Synthesizer}
	return fusion.New(exec, cfg.Fusion, synth)
}
