// Package execution es el único componente que habla con Provider (P4).
// Router, Fusion, Judgment y Context Compiler llaman executor.Execute,
// nunca a un provider directamente.
package execution

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/providers"
	"github.com/artumarinn/arg0s/internal/telemetry"
)

// ModelCatalog resuelve un model ID a su metadata (costo, provider,
// tier...). Definido acá, no en internal/config — el executor no
// importa config, igual que providers (ver ADR de desacople).
type ModelCatalog interface {
	Resolve(modelID string) (core.Model, bool)
}

// ProviderPolicy son los parámetros de ejecución de un provider —
// equivalentes a lo que trae config.yaml, pero como struct propio del
// paquete execution para no importar internal/config.
type ProviderPolicy struct {
	MaxRetries   int
	RetryBackoff string // exponential | linear | none
	Timeout      time.Duration
	Concurrent   int // 0 = sin límite
}

type Executor struct {
	registry *providers.Registry
	models   ModelCatalog
	policies map[string]ProviderPolicy
	bus      *telemetry.Bus // nil-safe: sin bus, no emite eventos

	semMu sync.Mutex
	sems  map[string]chan struct{}
}

func New(registry *providers.Registry, models ModelCatalog, policies map[string]ProviderPolicy, bus *telemetry.Bus) *Executor {
	return &Executor{
		registry: registry,
		models:   models,
		policies: policies,
		bus:      bus,
		sems:     map[string]chan struct{}{},
	}
}

// Execute corre req contra req.ModelID con retry/timeout/rate-limit, y
// si falla y fallbackModelID no está vacío, lo intenta UNA sola vez
// (sección 7). Devuelve siempre un ModelRun, incluso en error — el
// accounting se emite aunque la llamada falle.
func (e *Executor) Execute(ctx context.Context, req core.Request, role core.Role, fallbackModelID string) (core.Response, core.ModelRun, error) {
	e.emit(ctx, telemetry.EventModelStarted, req.ModelID, role, nil)
	start := time.Now()

	resp, attempts, model, err := e.tryModel(ctx, req, req.ModelID, true)
	finalModelID := req.ModelID
	fellBackFrom := ""

	if err != nil && fallbackModelID != "" && ctx.Err() == nil {
		fbResp, fbAttempts, fbModel, fbErr := e.tryModel(ctx, req, fallbackModelID, false)
		attempts += fbAttempts
		if fbErr == nil {
			resp, model, finalModelID, fellBackFrom = fbResp, fbModel, fallbackModelID, req.ModelID
			err = nil
			e.emit(ctx, telemetry.EventModelFellBack, fallbackModelID, role, nil)
		} else {
			err = fbErr
		}
	}

	latency := time.Since(start)
	err = redactErr(err)

	run := core.ModelRun{
		ModelID:      finalModelID,
		Role:         role,
		Latency:      latency,
		Attempts:     attempts,
		Err:          err,
		FellBackFrom: fellBackFrom,
	}

	if err != nil {
		run.Usage = estimateFailureUsage(req)
		e.emit(ctx, telemetry.EventModelFailed, finalModelID, role, err)
		return core.Response{}, run, err
	}

	run.Usage = accountUsage(resp.Usage, model, req, resp)
	e.emit(ctx, telemetry.EventModelCompleted, finalModelID, role, nil)
	return resp, run, nil
}

// tryModel resuelve modelo+provider, adquiere el semáforo del provider
// y ejecuta — con retry loop si allowRetry, o una sola llamada si no
// (así se usa para el intento único de fallback).
func (e *Executor) tryModel(ctx context.Context, req core.Request, modelID string, allowRetry bool) (core.Response, int, core.Model, error) {
	model, ok := e.models.Resolve(modelID)
	if !ok {
		return core.Response{}, 0, core.Model{}, fmt.Errorf("execute: model %q no encontrado en el catálogo", modelID)
	}
	provider, ok := e.registry.Get(model.Provider)
	if !ok {
		return core.Response{}, 0, model, fmt.Errorf("execute: provider %q no registrado", model.Provider)
	}

	release, err := e.acquire(ctx, model.Provider)
	if err != nil {
		return core.Response{}, 0, model, err
	}
	defer release()

	policy := e.policies[model.Provider]
	req.ModelID = modelID

	if !allowRetry {
		callCtx, cancel := withTimeout(ctx, policy.Timeout)
		resp, err := provider.Complete(callCtx, req)
		cancel()
		return resp, 1, model, err
	}

	resp, attempts, err := e.executeWithRetry(ctx, provider, req, policy)
	return resp, attempts, model, err
}

func (e *Executor) acquire(ctx context.Context, provider string) (release func(), err error) {
	policy := e.policies[provider]
	if policy.Concurrent <= 0 {
		return func() {}, nil
	}

	e.semMu.Lock()
	sem, ok := e.sems[provider]
	if !ok {
		sem = make(chan struct{}, policy.Concurrent)
		e.sems[provider] = sem
	}
	e.semMu.Unlock()

	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (e *Executor) emit(ctx context.Context, t telemetry.EventType, modelID string, role core.Role, err error) {
	if e.bus == nil {
		return
	}
	payload := map[string]any{"model_id": modelID, "role": string(role)}
	if err != nil {
		payload["error"] = core.RedactSecrets(err.Error())
	}
	_ = e.bus.Publish(ctx, telemetry.Event{Type: t, Payload: payload})
}
