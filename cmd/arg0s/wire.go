package main

import (
	"net/http"
	"time"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/execution"
	"github.com/artumarinn/arg0s/internal/providers"
	"github.com/artumarinn/arg0s/internal/providers/gemini"
	"github.com/artumarinn/arg0s/internal/providers/ollama"
	openaicompatible "github.com/artumarinn/arg0s/internal/providers/openai_compatible"
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

// modelCatalog adapta *config.ModelsFile a execution.ModelCatalog.
type modelCatalog struct {
	mf *config.ModelsFile
}

func (c modelCatalog) Resolve(id string) (core.Model, bool) {
	real := c.mf.ResolveModel(id)
	mc, ok := c.mf.Models[real]
	if !ok {
		return core.Model{}, false
	}
	return core.Model{
		ID:              real,
		Provider:        mc.Provider,
		ProviderModelID: mc.ProviderModelID,
		ContextWindow:   mc.ContextWindow,
		MaxOutputTokens: mc.MaxOutputTokens,
		CostInputPer1M:  mc.Cost.InputPer1MUSD,
		CostOutputPer1M: mc.Cost.OutputPer1MUSD,
		Capabilities:    mc.Capabilities,
		Strengths:       mc.Strengths,
		Tier:            mc.Tier,
		Local:           mc.Local,
	}, true
}

func buildExecutor(cfg *config.Config, models *config.ModelsFile, bus *telemetry.Bus) *execution.Executor {
	return execution.New(buildRegistry(cfg), modelCatalog{mf: models}, buildPolicies(cfg), bus)
}
