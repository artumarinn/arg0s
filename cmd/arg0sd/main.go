// Command arg0sd es el daemon de Arg0s (sección 18.1): corre las
// tasks, el cliente (arg0s CLI/TUI) solo manda comandos y recibe
// eventos por el socket. Nace en Fase 4 -- hasta acá todo corría
// embebido en el binario arg0s (ver cmd/arg0s/main.go).
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/execution"
	"github.com/artumarinn/arg0s/internal/ipc"
	"github.com/artumarinn/arg0s/internal/providers"
	"github.com/artumarinn/arg0s/internal/providers/gemini"
	"github.com/artumarinn/arg0s/internal/providers/ollama"
	openaicompatible "github.com/artumarinn/arg0s/internal/providers/openai_compatible"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	models, err := config.LoadModels()
	if err != nil {
		return err
	}
	socketPath, err := cfg.SocketPath()
	if err != nil {
		return err
	}

	exec := buildExecutor(cfg, models)
	srv := ipc.NewServer(exec, cfg.Roles.Generator.Fallback)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	log.Printf("arg0sd escuchando en %s", socketPath)
	err = srv.ListenAndServe(ctx, socketPath)
	if err != nil && ctx.Err() != nil {
		log.Println("arg0sd: apagado")
		return nil
	}
	return err
}

// buildExecutor duplica (a propósito, por ahora) el wiring de
// cmd/arg0s/wire.go -- son dos binarios distintos, sección 18.1. Si
// esta duplicación crece, se extrae a un paquete interno compartido;
// hoy son ~15 líneas, no amerita la indirección todavía.
func buildExecutor(cfg *config.Config, models *config.ModelsFile) *execution.Executor {
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

	policies := map[string]execution.ProviderPolicy{}
	for name, p := range cfg.Providers {
		policies[name] = execution.ProviderPolicy{
			MaxRetries: p.MaxRetries, RetryBackoff: p.RetryBackoff,
			Timeout: parseDuration(p.Timeout, 120*time.Second), Concurrent: p.RateLimit.Concurrent,
		}
	}

	return execution.New(reg, models, policies, nil)
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
