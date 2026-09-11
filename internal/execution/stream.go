package execution

import (
	"context"
	"fmt"

	"github.com/artumarinn/arg0s/internal/core"
)

// Stream resuelve modelo+provider igual que Execute y delega el
// streaming — sin retry (una vez arrancado, un stream que falla a
// mitad de camino no se reintenta transparentemente). El timeout de la
// policy aplica a todo el stream, no por-chunk.
func (e *Executor) Stream(ctx context.Context, req core.Request) (<-chan core.Chunk, error) {
	model, ok := e.models.Resolve(req.ModelID)
	if !ok {
		return nil, fmt.Errorf("stream: model %q no encontrado en el catálogo", req.ModelID)
	}
	provider, ok := e.registry.Get(model.Provider)
	if !ok {
		return nil, fmt.Errorf("stream: provider %q no registrado", model.Provider)
	}

	release, err := e.acquire(ctx, model.Provider)
	if err != nil {
		return nil, err
	}

	policy := e.policies[model.Provider]
	streamCtx, cancel := withTimeout(ctx, policy.Timeout)

	src, err := provider.Stream(streamCtx, req)
	if err != nil {
		cancel()
		release()
		return nil, redactErr(err)
	}

	out := make(chan core.Chunk)
	go func() {
		defer close(out)
		defer cancel()
		defer release()
		for c := range src {
			out <- c
		}
	}()
	return out, nil
}
