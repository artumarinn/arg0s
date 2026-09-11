package execution

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/telemetry"
)

// Stream resuelve modelo+provider igual que Execute y delega el
// streaming — sin retry (una vez arrancado, un stream que falla a
// mitad de camino no se reintenta transparentemente). El timeout de la
// policy aplica a todo el stream, no por-chunk.
//
// El *core.ModelRun devuelto se completa cuando el canal de chunks se
// cierra — leerlo antes de que el consumidor termine de vaciar el canal
// es una carrera del caller, no del executor: el close() del canal
// pasa a ser el punto de sincronización (todo lo que el executor
// escribe en run pasa a ser visible una vez que el receive del canal
// devuelve ok=false).
func (e *Executor) Stream(ctx context.Context, req core.Request) (<-chan core.Chunk, *core.ModelRun, error) {
	e.emit(ctx, telemetry.EventModelStarted, req.ModelID, req.Role, nil)

	model, ok := e.models.Resolve(req.ModelID)
	if !ok {
		err := fmt.Errorf("stream: model %q no encontrado en el catálogo", req.ModelID)
		return nil, &core.ModelRun{ModelID: req.ModelID, Role: req.Role, Err: err}, err
	}
	provider, ok := e.registry.Get(model.Provider)
	if !ok {
		err := fmt.Errorf("stream: provider %q no registrado", model.Provider)
		return nil, &core.ModelRun{ModelID: req.ModelID, Role: req.Role, Err: err}, err
	}

	release, err := e.acquire(ctx, model.Provider)
	if err != nil {
		return nil, &core.ModelRun{ModelID: req.ModelID, Role: req.Role, Err: err}, err
	}

	policy := e.policies[model.Provider]
	streamCtx, cancel := withTimeout(ctx, policy.Timeout)

	src, err := provider.Stream(streamCtx, req)
	if err != nil {
		cancel()
		release()
		err = redactErr(err)
		run := &core.ModelRun{ModelID: req.ModelID, Role: req.Role, Attempts: 1, Err: err, Usage: estimateFailureUsage(req)}
		e.emit(ctx, telemetry.EventModelFailed, req.ModelID, req.Role, err)
		return nil, run, err
	}

	run := &core.ModelRun{ModelID: req.ModelID, Role: req.Role, Attempts: 1}
	out := make(chan core.Chunk)

	go func() {
		defer close(out)
		defer cancel()
		defer release()

		start := time.Now()
		var content strings.Builder
		var finalUsage *core.Usage
		var streamErr error

		for c := range src {
			if c.Err != nil {
				streamErr = c.Err
			}
			if c.Usage != nil {
				finalUsage = c.Usage
			}
			content.WriteString(c.Delta)

			select {
			case out <- c:
			case <-ctx.Done():
				streamErr = ctx.Err()
			}
			if streamErr != nil {
				break
			}
		}
		// El for-range también termina en silencio si src se cerró
		// porque EL PROVIDER vio ctx.Done() primero (nunca llegó a
		// pasar por nuestro select de arriba) — sin este chequeo, un
		// stream cancelado que el provider corta por su cuenta se
		// reportaría como éxito.
		if streamErr == nil && ctx.Err() != nil {
			streamErr = ctx.Err()
		}

		run.Latency = time.Since(start)
		if streamErr != nil {
			run.Err = redactErr(streamErr)
			run.Usage = partialStreamUsage(req, content.String(), finalUsage)
			e.emit(ctx, telemetry.EventModelFailed, req.ModelID, req.Role, streamErr)
			return
		}

		run.Usage = accountStreamUsage(finalUsage, model, req, content.String())
		e.emit(ctx, telemetry.EventModelCompleted, req.ModelID, req.Role, nil)
	}()

	return out, run, nil
}
